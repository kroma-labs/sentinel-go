// Package httpclient provides a production-ready HTTP client with built-in
// resilience, observability, and OpenTelemetry instrumentation.
//
// # Features
//
//   - OpenTelemetry tracing with detailed span attributes
//   - Prometheus-compatible metrics for request latency, errors, retries
//   - Automatic retries with exponential backoff and jitter
//   - Semantic retry classification (429, 502-504 → retry; 4xx → stop)
//   - Connection pooling with configurable limits
//   - Network tracing (DNS, TLS, connect timing)
//   - Request filtering for selective tracing
//
// # Quick Start
//
// Basic usage with the fluent request builder:
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithServiceName("my-service"),
//	)
//
//	// Simple GET request
//	resp, err := client.Request("GetUsers").Get(ctx, "/users")
//
//	// POST with JSON body and response decoding
//	var user User
//	resp, err := client.Request("CreateUser").
//	    Body(newUser).
//	    Decode(&user).
//	    Post(ctx, "/users")
//
// For raw http.Client access (advanced usage):
//
//	httpClient := client.HTTP()
//	resp, err := httpClient.Do(req)
//
// # Configuration Presets
//
// The package provides pre-tuned configurations for common scenarios:
//
//	// High-throughput: 200 idle conns, 50 per host, 30s timeout
//	client := httpclient.New(
//	    httpclient.WithConfig(httpclient.HighThroughputConfig()),
//	)
//
//	// Low-latency: 5s timeout, 2s dial, minimal buffers
//	client := httpclient.New(
//	    httpclient.WithConfig(httpclient.LowLatencyConfig()),
//	)
//
//	// Conservative: 50 idle conns, 10 per host, 30s timeout
//	client := httpclient.New(
//	    httpclient.WithConfig(httpclient.ConservativeConfig()),
//	)
//
// # Retry Configuration
//
// Automatic retries with exponential backoff:
//
//	// Default: 3 retries, 500ms initial, 2x multiplier, 50% jitter
//	client := httpclient.New(
//	    httpclient.WithRetryConfig(httpclient.DefaultRetryConfig()),
//	)
//
//	// Aggressive: 5 retries, 200ms initial, for critical operations
//	client := httpclient.New(
//	    httpclient.WithRetryConfig(httpclient.AggressiveRetryConfig()),
//	)
//
//	// Custom classifier: retry on specific conditions
//	client := httpclient.New(
//	    httpclient.WithRetryClassifier(func(resp *http.Response, err error) bool {
//	        return resp != nil && resp.StatusCode >= 500
//	    }),
//	)
//
// # Custom Backoff Strategies
//
// Beyond exponential backoff, the package provides:
//
//	// Linear backoff: 500ms → 1s → 1.5s → 2s
//	client := httpclient.New(
//	    httpclient.WithRetryBackOff(httpclient.NewLinearBackOff()),
//	)
//
//	// AWS-style decorrelated jitter for high-contention scenarios
//	client := httpclient.New(
//	    httpclient.WithRetryBackOff(httpclient.NewDecorrelatedJitterBackOff()),
//	)
//
//	// Tiered retry: Fixed delays then exponential backoff
//	tiers := []httpclient.RetryTier{
//	    {MaxRetries: 5, Delay: 1 * time.Minute},   // Tier 1: 5 retries at 1 min
//	    {MaxRetries: 5, Delay: 2 * time.Minute},   // Tier 2: 5 retries at 2 min
//	}
//	client := httpclient.New(
//	    httpclient.WithTieredRetry(tiers, 10*time.Minute),
//	)
//
// # Observability
//
// The client automatically emits:
//
// Metrics:
//   - http.client.request.duration (histogram)
//   - http.client.retry.attempts (counter)
//   - http.client.retry.exhausted (counter)
//   - http.client.dns.duration (histogram)
//   - http.client.tls.duration (histogram)
//
// Traces:
//   - Spans for each request with method, URL, status code
//   - Retry events with attempt number and delay
//   - Network timing events (DNS, TLS, connect)
//
// # Transport Wrapping
//
// Wrap an existing transport with instrumentation:
//
//	transport := httpclient.NewTransport(http.DefaultTransport,
//	    httpclient.WithServiceName("my-service"),
//	)
//	client := &http.Client{Transport: transport}
//
// Or wrap an existing client:
//
//	client := &http.Client{Timeout: 30 * time.Second}
//	httpclient.WrapClient(client,
//	    httpclient.WithServiceName("my-service"),
//	)
//
// # Fluent Request Builder
//
// The package provides a fluent API for building requests:
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithServiceName("payment-service"),
//	)
//
//	var users []User
//	resp, err := client.Request("GetUsers").
//	    Query("page", "1").
//	    Header("Authorization", "Bearer "+token).
//	    Decode(&users).
//	    Get(ctx, "/users")
//
//	if resp.IsSuccess() {
//	    fmt.Printf("Got %d users\n", len(users))
//	}
//
// File uploads are also supported:
//
//	resp, err := client.Request("Upload").
//	    File("document", "/path/to/file.pdf").
//	    FormField("title", "My Document").
//	    Post(ctx, "/upload")
//
// # Debug Utilities
//
// Enable debug logging and cURL command generation:
//
//	client := httpclient.New(
//	    httpclient.WithDebug(true),       // Logs requests/responses with zerolog
//	    httpclient.WithGenerateCurl(true), // Generates cURL commands
//	)
//
//	resp, err := client.Request("Test").
//	    EnableTrace().  // Capture timing info
//	    Get(ctx, "/api")
//
//	fmt.Println(resp.TraceInfo())   // DNS, connect, TLS, server timing
//	fmt.Println(resp.CurlCommand()) // Equivalent cURL command
package httpclient
