Rquest :
```
curl -H 'X-User-ID: user123' \
     -H 'X-Tenant-ID: tenant456' \
     -H 'X-Request-ID: req-abc-123' \
     'http://localhost:8080/orders?order_id=order700'
```

Add your auth token or pass by environment varibale :
```
	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint("ingest.us.staging.signoz.cloud:443"),
		otlptracehttp.WithURLPath("/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"signoz-access-token": "<TOKEN>",
		}),
	)
```

Change your endpoint for signoz :
`ingest.us.staging.signoz.cloud:443`

Disclaimer : 

##` You can use it with the `baggage` keyword. It is just for example