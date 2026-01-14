package httpclient

import (
	"net/http"
)

// Client is a high-level HTTP client with fluent request building,
// OpenTelemetry instrumentation, and retry support.
//
// Create a Client using New():
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithServiceName("payment-service"),
//	)
//
//	resp, err := client.Request("CreatePayment").
//	    Path("/payments").
//	    Body(payment).
//	    Post(ctx)
type Client struct {
	// httpClient is the underlying HTTP client with transport chain.
	httpClient *http.Client

	// config holds all client configuration.
	config *internalConfig

	// baseURL is the base URL for all requests.
	baseURL string

	// defaultHeaders are applied to all requests.
	defaultHeaders http.Header

	// debug enables request/response logging.
	debug bool

	// generateCurl enables cURL command generation.
	generateCurl bool

	// enableTrace enables timing trace info collection.
	enableTrace bool
}

// HTTP returns the underlying *http.Client for advanced use cases.
//
// Use this when you need to:
//   - Pass the client to third-party libraries expecting *http.Client
//   - Access transport-level settings
//   - Make requests without the fluent builder
//
// Example:
//
//	rawClient := client.HTTP()
//	resp, err := rawClient.Do(req)
func (c *Client) HTTP() *http.Client {
	return c.httpClient
}

// Request creates a new RequestBuilder for the given operation name.
//
// The operation name is used for:
//   - OpenTelemetry span naming (e.g., "HTTP POST CreatePayment")
//   - Debug logging identification
//   - Metrics labeling
//
// Example:
//
//	resp, err := client.Request("CreateUser").
//	    Path("/users").
//	    Body(user).
//	    Post(ctx)
func (c *Client) Request(operationName string) *RequestBuilder {
	return &RequestBuilder{
		client:        c,
		operationName: operationName,
		headers:       make(http.Header),
		pathParams:    make(map[string]string),
	}
}

// New creates a Client with production-ready defaults and OpenTelemetry instrumentation.
//
// The client includes:
//   - Connection pooling and timeouts
//   - OpenTelemetry tracing and metrics
//   - Retry with exponential backoff
//   - Fluent request builder via Request()
//
// Example - Basic usage:
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithServiceName("my-service"),
//	)
//
//	resp, err := client.Request("GetUsers").Get(ctx, "/users")
//
// Example - With retry configuration:
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithRetryConfig(httpclient.AggressiveRetryConfig()),
//	)
func New(opts ...Option) *Client {
	cfg := newConfig(opts...)
	transport := cfg.buildTransport()

	withRetry := newRetryTransport(transport, cfg)
	withBreaker := newCircuitBreakerTransport(withRetry, cfg)
	instrumented := newOtelTransport(withBreaker, cfg)

	httpClient := &http.Client{
		Transport: instrumented,
		Timeout:   cfg.httpConfig.Timeout,
	}

	return &Client{
		httpClient:     httpClient,
		config:         cfg,
		baseURL:        cfg.BaseURL,
		defaultHeaders: cfg.DefaultHeaders,
		debug:          cfg.Debug,
		generateCurl:   cfg.GenerateCurl,
		enableTrace:    cfg.EnableTrace,
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
//	transport := httpclient.NewTransport(http.DefaultTransport,
//	    httpclient.WithServiceName("my-service"),
//	)
//	client := &http.Client{
//	    Transport: transport,
//	    Timeout:   30 * time.Second,
//	}
func NewTransport(base http.RoundTripper, opts ...Option) http.RoundTripper {
	cfg := newConfig(opts...)
	return newOtelTransport(base, cfg)
}

// NewWithTransport creates a Client using a custom base transport
// with OpenTelemetry instrumentation wrapped around it.
//
// The provided transport will be wrapped with tracing and metrics.
// Use this when you need precise control over the underlying transport
// but want to add observability.
//
// Example:
//
//	transport := &http.Transport{
//	    MaxIdleConnsPerHost: 50,
//	    DisableCompression:  true,
//	}
//	client := httpclient.NewWithTransport(transport,
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithServiceName("my-service"),
//	)
func NewWithTransport(base http.RoundTripper, opts ...Option) *Client {
	cfg := newConfig(opts...)

	httpClient := &http.Client{
		Transport: newOtelTransport(base, cfg),
		Timeout:   cfg.httpConfig.Timeout,
	}

	return &Client{
		httpClient:     httpClient,
		config:         cfg,
		baseURL:        cfg.BaseURL,
		defaultHeaders: cfg.DefaultHeaders,
		debug:          cfg.Debug,
		generateCurl:   cfg.GenerateCurl,
		enableTrace:    cfg.EnableTrace,
	}
}

// WrapClient wraps an existing http.Client's transport with OpenTelemetry instrumentation.
//
// This modifies the client in-place and returns a new Client wrapper.
// If the client has no transport, http.DefaultTransport is used.
//
// Example:
//
//	httpClient := &http.Client{Timeout: 30 * time.Second}
//	client := httpclient.WrapClient(httpClient,
//	    httpclient.WithServiceName("my-service"),
//	)
func WrapClient(httpClient *http.Client, opts ...Option) *Client {
	cfg := newConfig(opts...)

	base := httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	httpClient.Transport = newOtelTransport(base, cfg)

	return &Client{
		httpClient:     httpClient,
		config:         cfg,
		baseURL:        cfg.BaseURL,
		defaultHeaders: cfg.DefaultHeaders,
		debug:          cfg.Debug,
		generateCurl:   cfg.GenerateCurl,
		enableTrace:    cfg.EnableTrace,
	}
}
