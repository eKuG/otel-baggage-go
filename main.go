package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// CustomSpanProcessor implements the SpanProcessor interface
// to automatically annotate spans with baggage data
type CustomSpanProcessor struct {
	sdktrace.SpanProcessor
}

func (csp *CustomSpanProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	bag := baggage.FromContext(parent)

	for _, member := range bag.Members() {
		key := fmt.Sprintf("baggage.%s", member.Key())
		s.SetAttributes(attribute.String(key, member.Value()))
	}

	if csp.SpanProcessor != nil {
		csp.SpanProcessor.OnStart(parent, s)
	}
}

// OnEnd is called when a span is ended
func (csp *CustomSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	if csp.SpanProcessor != nil {
		csp.SpanProcessor.OnEnd(s)
	}
}

// Shutdown shuts down the processor
func (csp *CustomSpanProcessor) Shutdown(ctx context.Context) error {
	if csp.SpanProcessor != nil {
		return csp.SpanProcessor.Shutdown(ctx)
	}
	return nil
}

// ForceFlush forces the processor to flush
func (csp *CustomSpanProcessor) ForceFlush(ctx context.Context) error {
	if csp.SpanProcessor != nil {
		return csp.SpanProcessor.ForceFlush(ctx)
	}
	return nil
}

// initTracer initializes the OpenTelemetry tracer with our custom span processor
func initTracer() func() {
	// Create OTLP HTTP exporter for SigNoz
	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint("ingest.us.staging.signoz.cloud:443"),
		otlptracehttp.WithURLPath("/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"signoz-access-token": "<TOKEN>",
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create a basic span processor that exports to stdout
	batchProcessor := sdktrace.NewBatchSpanProcessor(exporter)

	// Wrap it with our custom processor
	customProcessor := &CustomSpanProcessor{
		SpanProcessor: batchProcessor,
	}

	// Create tracer provider with our custom processor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(customProcessor),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("baggage-annotation-demo"),
			semconv.ServiceVersion("v1.0.0"),
		)),
	)

	otel.SetTracerProvider(tp)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}
}

// Middleware to extract request info and add to baggage
func baggageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Create baggage members from request context
		members := []baggage.Member{}

		// Add user ID from header (simulate authentication)
		if userID := r.Header.Get("X-User-ID"); userID != "" {
			member, _ := baggage.NewMember("user_id", userID)
			members = append(members, member)
		}

		// Add tenant ID from header
		if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
			member, _ := baggage.NewMember("tenant_id", tenantID)
			members = append(members, member)
		}

		// Add request ID (generate or extract from header)
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
		}
		member, _ := baggage.NewMember("request_id", requestID)
		members = append(members, member)

		// Add client IP
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = forwarded
		}
		member, _ = baggage.NewMember("client_ip", clientIP)
		members = append(members, member)

		// Add user agent
		if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
			member, _ := baggage.NewMember("user_agent", userAgent)
			members = append(members, member)
		}

		// Create new baggage and add to context
		bag, err := baggage.New(members...)
		if err != nil {
			log.Printf("Error creating baggage: %v", err)
			next.ServeHTTP(w, r)
			return
		}

		// Add baggage to context
		ctx = baggage.ContextWithBaggage(ctx, bag)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// Business logic functions that create spans
func processOrder(ctx context.Context, orderID string) error {
	tracer := otel.Tracer("order-service")
	ctx, span := tracer.Start(ctx, "process_order")
	defer span.End()

	span.SetAttributes(attribute.String("order.id", orderID))

	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	if err := validatePayment(ctx, orderID); err != nil {
		return err
	}

	if err := updateInventory(ctx, orderID); err != nil {
		return err
	}

	return nil
}

func validatePayment(ctx context.Context, orderID string) error {
	tracer := otel.Tracer("payment-service")
	ctx, span := tracer.Start(ctx, "validate_payment")
	defer span.End()

	span.SetAttributes(attribute.String("payment.order_id", orderID))

	// Simulate payment validation
	time.Sleep(50 * time.Millisecond)

	return nil
}

func updateInventory(ctx context.Context, orderID string) error {
	tracer := otel.Tracer("inventory-service")
	ctx, span := tracer.Start(ctx, "update_inventory")
	defer span.End()

	span.SetAttributes(attribute.String("inventory.order_id", orderID))

	// Simulate inventory update
	time.Sleep(30 * time.Millisecond)

	return nil
}

// HTTP handlers
func orderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Start a span for the HTTP request
	tracer := otel.Tracer("http-server")
	ctx, span := tracer.Start(ctx, "POST /orders")
	defer span.End()

	orderID := r.URL.Query().Get("order_id")
	if orderID == "" {
		orderID = "order_123"
	}

	span.SetAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.route", "/orders"),
		attribute.String("order.id", orderID),
	)

	if err := processOrder(ctx, orderID); err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Order %s processed successfully", orderID)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tracer := otel.Tracer("http-server")
	_, span := tracer.Start(ctx, "GET /health")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.route", "/health"),
	)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func main() {
	// Initialize tracing
	shutdown := initTracer()
	defer shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", orderHandler)
	mux.HandleFunc("/health", healthHandler)

	handler := baggageMiddleware(mux)

	fmt.Println("Server starting on :8080")
	fmt.Println("Try these commands to test:")
	fmt.Println("curl -H 'X-User-ID: user123' -H 'X-Tenant-ID: tenant456' 'http://localhost:8080/orders?order_id=order789'")
	fmt.Println("curl -H 'X-User-ID: alice' -H 'X-Tenant-ID: acme-corp' 'http://localhost:8080/health'")

	log.Fatal(http.ListenAndServe(":8080", handler))
}
