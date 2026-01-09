package httpclient

import (
	"net/http"
)

// New creates an HTTP client with production-ready defaults and OpenTelemetry instrumentation.
//
// The client uses DefaultConfig() for HTTP transport settings, which provides:
//   - Connection pooling: 100 max idle, 20 per host
//   - Timeouts: 15s overall, 5s dial, 10s TLS handshake
//   - Buffers: 64KB read/write for better throughput
//   - Keep-alives: enabled for connection reuse
//   - Compression: enabled (Accept-Encoding: gzip)
//   - Network tracing: enabled for DNS/TLS/connect visibility
//
// Example - Basic usage:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
//	resp, err := client.Do(req)
//
// Example - Custom configuration:
//
//	cfg := sentinelhttpclient.HighThroughputConfig()
//	cfg.Timeout = 10 * time.Second
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(cfg),
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
//
// Example - With retry configuration:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryConfig(sentinelhttpclient.AggressiveRetryConfig()),
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
func New(opts ...Option) *http.Client {
	cfg := newConfig(opts...)
	transport := cfg.buildTransport()

	// Build transport chain: base -> otel tracing -> retry
	instrumented := newOtelTransport(transport, cfg)
	withRetry := newRetryTransport(instrumented, cfg)

	return &http.Client{
		Transport: withRetry,
		Timeout:   cfg.httpConfig.Timeout,
	}
}

// NewTransport creates an instrumented http.RoundTripper that can be used
// with a custom http.Client.
//
// This is useful when you need more control over the http.Client configuration
// but still want OpenTelemetry instrumentation.
//
// Example:
//
//	transport := sentinelhttpclient.NewTransport(http.DefaultTransport,
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
//	client := &http.Client{
//	    Transport: transport,
//	    Timeout:   30 * time.Second,
//	}
func NewTransport(base http.RoundTripper, opts ...Option) http.RoundTripper {
	cfg := newConfig(opts...)
	return newOtelTransport(base, cfg)
}

// NewWithTransport creates an HTTP client using a custom base transport
// with OpenTelemetry instrumentation wrapped around it.
//
// The provided transport will be wrapped with tracing and metrics.
// Use this when you need precise control over the underlying transport
// but want to add observability.
//
// Example:
//
//	// Custom transport with specific settings
//	transport := &http.Transport{
//	    MaxIdleConnsPerHost: 50,
//	    DisableCompression:  true,
//	}
//	client := sentinelhttpclient.NewWithTransport(transport,
//	    sentinelhttpclient.WithServiceName("my-service"),
//	    sentinelhttpclient.WithTimeout(10 * time.Second),
//	)
func NewWithTransport(base http.RoundTripper, opts ...Option) *http.Client {
	cfg := newConfig(opts...)
	return &http.Client{
		Transport: newOtelTransport(base, cfg),
		Timeout:   cfg.httpConfig.Timeout,
	}
}

// WrapClient wraps an existing http.Client's transport with OpenTelemetry instrumentation.
//
// This modifies the client in-place and returns it for chaining.
// If the client has no transport, http.DefaultTransport is used.
//
// Example:
//
//	client := &http.Client{Timeout: 30 * time.Second}
//	sentinelhttpclient.WrapClient(client,
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
//	// client is now instrumented
func WrapClient(client *http.Client, opts ...Option) *http.Client {
	cfg := newConfig(opts...)

	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	client.Transport = newOtelTransport(base, cfg)
	return client
}
