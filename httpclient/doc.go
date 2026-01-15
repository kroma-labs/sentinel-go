// Package httpclient provides a production-ready HTTP client with built-in
// resilience, observability, and OpenTelemetry instrumentation.
//
// # Features
//
//   - OpenTelemetry tracing with detailed span attributes
//   - Prometheus-compatible metrics for request latency, errors, retries
//   - Automatic retries with exponential backoff and jitter
//   - Semantic retry classification (429, 502-504 → retry; 4xx → stop)
//   - Circuit Breaker pattern (Local and Distributed Redis-backed)
//   - Hedged Requests for tail latency optimization
//   - Chaos Injection for resilience testing
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
// # Circuit Breaker Configuration
//
// The client supports both local (in-memory) and distributed (Redis-backed) circuit breakers.
// The breaker is scoped to the client instance and named using the ServiceName.
//
// Local Circuit Breaker (Default):
//
//	client := httpclient.New(
//	    httpclient.WithServiceName("payment-service"),
//	    httpclient.WithBreakerConfig(httpclient.DefaultBreakerConfig()),
//	)
//
// Distributed Circuit Breaker (Redis):
//
//	// Initialize Redis client
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	store := httpclient.NewRedisStore(rdb)
//
//	// Create config using the store
//	client := httpclient.New(
//	    httpclient.WithServiceName("payment-service"),
//	    httpclient.WithBreakerConfig(httpclient.DistributedBreakerConfig(store)),
//	)
//
// Note: If distributed breaker initialization fails (e.g., due to configuration issues),
// the client automatically falls back to a Local Circuit Breaker to ensure the service remains protected (Graceful Degradation).
//
// Custom Configuration:
//
//	cfg := httpclient.DefaultBreakerConfig()
//	cfg.FailureThreshold = 5
//	cfg.Timeout = 60 * time.Second
//
//	client := httpclient.New(
//	    httpclient.WithServiceName("critical-service"),
//	    httpclient.WithBreakerConfig(cfg),
//	)
//
// # Chaos Injection (Testing)
//
// Simulate failures to test resilience patterns:
//
//	// Add latency to test timeout handling
//	client := httpclient.New(
//	    httpclient.WithChaos(httpclient.ChaosConfig{
//	        LatencyMs:       200,  // 200ms delay
//	        LatencyJitterMs: 100,  // 0-100ms additional jitter
//	    }),
//	)
//
//	// Inject errors to test circuit breaker
//	client := httpclient.New(
//	    httpclient.WithChaos(httpclient.ChaosConfig{
//	        ErrorRate: 0.5, // 50% of requests fail
//	    }),
//	    httpclient.WithBreakerConfig(httpclient.DefaultBreakerConfig()),
//	)
//
// WARNING: Do not use in production.
//
// # Hedged Requests (Tail Latency)
//
// Reduce tail latency by sending duplicate requests on slow responses.
// Hedging is enabled per-request via the fluent builder:
//
//	// Simple: hedge after 50ms
//	resp, err := client.Request("GetUser").
//	    Hedge(50 * time.Millisecond).
//	    Get(ctx, "/users/123")
//
//	// Advanced: custom config
//	resp, err := client.Request("GetUser").
//	    HedgeConfig(httpclient.HedgeConfig{
//	        Delay:     50 * time.Millisecond,
//	        MaxHedges: 2,
//	    }).
//	    Get(ctx, "/users/123")
//
// Based on Google's "The Tail at Scale" paper. First response wins,
// remaining requests are cancelled.
//
// IMPORTANT: Only use for idempotent operations (GET, HEAD, etc.).
//
// # Adaptive Hedging
//
// Don't know your P95 latency? Use adaptive hedging to automatically
// calculate the hedge delay based on historical endpoint latency:
//
//	// Adaptive: delay calculated from P95 of prior requests
//	resp, err := client.Request("GetUser").
//	    AdaptiveHedge(httpclient.DefaultAdaptiveHedgeConfig()).
//	    Get(ctx, "/users/123")
//
//	// Custom adaptive config
//	resp, err := client.Request("GetUser").
//	    AdaptiveHedge(httpclient.AdaptiveHedgeConfig{
//	        TargetPercentile: 0.99,     // P99 instead of P95
//	        MinSamples:       20,       // Wait for 20 samples
//	        FallbackDelay:    100 * time.Millisecond,
//	        MaxHedges:        2,
//	    }).
//	    Get(ctx, "/users/123")
//
// The tracker records latencies per-endpoint (using operation name).
// Until MinSamples is reached, FallbackDelay is used.
//
// # Request Coalescing
//
// Deduplicate simultaneous identical requests using singleflight:
//
//	// Multiple goroutines → one network call
//	resp, err := client.Request("GetUser").
//	    Coalesce().
//	    Get(ctx, "/users/123")
//
// When multiple goroutines make the same request simultaneously:
//   - Only one request actually executes
//   - Others wait and receive the same response
//   - No stale data: sequential requests make fresh calls
//
// Key generation: SHA256(method + URL + sorted query params + body hash)
//
// Use for idempotent read operations to reduce downstream load during
// cache stampedes or high concurrency.
//
// # Per-Request Timeout
//
// Override the client's default timeout for specific endpoints:
//
//	// Fast endpoint - use shorter timeout
//	resp, err := client.Request("HealthCheck").
//	    Timeout(1 * time.Second).
//	    Get(ctx, "/health")
//
//	// Slow endpoint - use longer timeout
//	resp, err := client.Request("BulkExport").
//	    Timeout(5 * time.Minute).
//	    Get(ctx, "/exports/large")
//
// IMPORTANT: The effective timeout is the MINIMUM of:
//   - Context deadline
//   - Client timeout
//   - Per-request timeout
//
// This means Timeout() can only REDUCE the timeout, never extend it.
//
// # Rate Limiting
//
// Proactively respect API rate limits to prevent 429 errors.
//
// Client-level rate limiting (applies to all requests):
//
//	client := httpclient.New(
//	    httpclient.WithRateLimit(httpclient.RateLimitConfig{
//	        RequestsPerSecond: 100,
//	        Burst:             10,
//	        WaitOnLimit:       true, // Wait for token
//	    }),
//	)
//
// Per-request rate limiting (different limits per endpoint):
//
//	// Bulk export endpoint limited to 10 req/s
//	resp, err := client.Request("BulkExport").
//	    RateLimit(10).
//	    Get(ctx, "/exports")
//
//	// Regular endpoint can handle more
//	resp, err := client.Request("GetUser").
//	    RateLimit(500).
//	    Get(ctx, "/users/123")
//
// Behavior options:
//   - WaitOnLimit=true: Wait for token (default, respects context deadline)
//   - WaitOnLimit=false: Return ErrRateLimited immediately
//
// Client-level and request-level limits are both enforced (must pass both).
//
// # Request/Response Interceptors
//
// Add middleware-style hooks for cross-cutting concerns:
//
// Client-level interceptors (apply to all requests):
//
//	client := httpclient.New(
//	    httpclient.WithRequestInterceptor(httpclient.AuthBearerInterceptor("my-token")),
//	    httpclient.WithResponseInterceptor(func(resp *http.Response, req *http.Request) error {
//	        log.Printf("%s %s -> %d", req.Method, req.URL, resp.StatusCode)
//	        return nil
//	    }),
//	)
//
// Per-request interceptors (run after client interceptors):
//
//	resp, err := client.Request("AdminAction").
//	    Intercept(func(req *http.Request) error {
//	        req.Header.Set("X-Admin-Token", getAdminToken())
//	        return nil
//	    }).
//	    Post(ctx, "/admin/action")
//
// Built-in request interceptors:
//   - AuthBearerInterceptor(token) - Static bearer token
//   - AuthBearerFuncInterceptor(fn) - Dynamic/refreshable token
//   - APIKeyInterceptor(header, key) - API key header
//   - CorrelationIDInterceptor(header, fn) - Request correlation
//   - UserAgentInterceptor(ua) - Custom User-Agent
//
// Execution order: Client interceptors → Per-request interceptors → Send
//
// # Mock Transport (Testing)
//
// Test HTTP clients without network calls using MockTransport:
//
//	mock := httpclient.NewMockTransport().
//	    StubPath("/users", http.StatusOK, `[{"id":1}]`).
//	    StubPath("/posts", http.StatusNotFound, `{"error":"not found"}`)
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithMockTransport(mock),
//	)
//
//	resp, _ := client.Request("GetUsers").Get(ctx, "/users")
//
// Stubbing methods:
//   - StubResponse(status, body) - Default for all requests
//   - StubPath(path, status, body) - Exact path match
//   - StubPathRegex(pattern, status, body) - Regex path match
//   - StubMethod(method, status, body) - HTTP method match
//   - StubFunc(matcher, status, body) - Custom matcher function
//   - StubError(err) - Simulate network errors
//
// Request tracking:
//
//	_ = mock.Requests()      // All captured requests
//	_ = mock.RequestCount()  // Number of requests
//	_ = mock.LastRequest()   // Most recent request
//
// # Observability
//
// The client automatically emits:
//
// Metrics:
//   - http.client.request.duration (histogram)
//   - http.client.retry.attempts (counter)
//   - http.client.retry.exhausted (counter)
//   - http.client.circuit_breaker.state (gauge, 0=Closed, 1=HalfOpen, 2=Open)
//   - http.client.circuit_breaker.requests (counter, result=success/failure/rejected)
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
