package httpclient

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	// scope is the instrumentation scope name for OpenTelemetry.
	scope = "github.com/kroma-labs/sentinel-go/httpclient"
)

// =============================================================================
// Config - HTTP Transport Configuration
// =============================================================================

// Config holds the HTTP transport configuration parameters.
// Use DefaultConfig() to get a properly initialized configuration,
// then modify specific fields as needed.
//
// Example:
//
//	cfg := sentinelhttpclient.DefaultConfig()
//	cfg.Timeout = 5 * time.Second
//	cfg.MaxIdleConnsPerHost = 25
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(cfg),
//	    sentinelhttpclient.WithServiceName("payment-service"),
//	)
type Config struct {
	// =======================================================================
	// Request Timeout
	// =======================================================================

	// Timeout specifies a time limit for the entire request lifecycle,
	// including connection establishment, TLS handshake, sending the request,
	// and reading the response body.
	//
	// A Timeout of zero means no timeout. Be cautious with zero timeout
	// in production as it can lead to hanging requests.
	//
	// Example: 15*time.Second for most API calls, 60*time.Second for uploads
	//
	// Default: 15s
	Timeout time.Duration

	// =======================================================================
	// Connection Pool Settings (Transport)
	// =======================================================================

	// MaxIdleConns controls the maximum number of idle (keep-alive)
	// connections across ALL hosts combined.
	//
	// Rule of thumb: Set to 2-3x your expected peak concurrent requests.
	// If you typically have 30 concurrent requests, set this to 60-100.
	//
	// Too low: Frequent connection establishment (slow, resource-intensive)
	// Too high: Wasted memory holding unused connections
	//
	// Example:
	//   - Low traffic service: 20-50
	//   - Medium traffic API: 100 (default)
	//   - High traffic gateway: 500-1000
	//
	// Default: 100
	MaxIdleConns int

	// MaxIdleConnsPerHost controls the maximum idle connections to keep
	// for each host (downstream service).
	//
	// This is often the most important setting for performance. If you
	// primarily call one service (common in microservices), set this close
	// to MaxIdleConns.
	//
	// Too low: Connection churn, increased latency from repeated handshakes
	// Too high: Resource waste if you call many different hosts
	//
	// Example:
	//   - Single downstream service: 50-100
	//   - Multiple downstream services: 10-20 each
	//   - Gateway calling many services: 5-10 each
	//
	// Default: 20
	MaxIdleConnsPerHost int

	// MaxConnsPerHost limits the TOTAL number of connections (idle + active)
	// per host. This prevents overwhelming a single downstream service.
	//
	// Set this higher than MaxIdleConnsPerHost to allow bursts.
	// A value of 0 means unlimited (not recommended for production).
	//
	// Example:
	//   - Conservative: 50
	//   - Normal: 100 (default)
	//   - High throughput: 0 (unlimited) or 500+
	//
	// Default: 100
	MaxConnsPerHost int

	// IdleConnTimeout is how long an idle connection remains in the pool
	// before being closed. Should match or slightly exceed your downstream
	// service's idle timeout to avoid "connection reset" errors.
	//
	// Example:
	//   - Most services: 90s (default)
	//   - AWS ALB default: 60s (set to 55s to close before ALB does)
	//   - Long-lived connections: 300s
	//
	// Default: 90s
	IdleConnTimeout time.Duration

	// TLSHandshakeTimeout is the maximum time to wait for a TLS handshake.
	// Increase for services with slow TLS (e.g., mutual TLS with HSM).
	//
	// Example:
	//   - Internal services: 5s
	//   - External APIs: 10s (default)
	//   - Mutual TLS with HSM: 30s
	//
	// Default: 10s
	TLSHandshakeTimeout time.Duration

	// ExpectContinueTimeout is how long to wait for a server's
	// "100 Continue" response when using the "Expect: 100-continue" header.
	// This header is sent for large request bodies to check if the server
	// will accept the request before sending the body.
	//
	// Default: 1s
	ExpectContinueTimeout time.Duration

	// ResponseHeaderTimeout is the time to wait for response headers
	// after the request is fully written. Zero means no timeout (uses Timeout).
	//
	// Use this to fail fast on slow backends while still allowing
	// large response body downloads.
	//
	// Example:
	//   - Fast APIs: 5s header timeout + 30s overall timeout
	//   - File downloads: 10s header timeout + 5min overall timeout
	//
	// Default: 0 (disabled, uses overall Timeout)
	ResponseHeaderTimeout time.Duration

	// =======================================================================
	// TCP Dial Settings
	// =======================================================================

	// DialTimeout is the maximum time to wait for a TCP connection
	// to be established (before TLS handshake).
	//
	// Should be less than the overall Timeout. Set lower for fast
	// failover to alternative endpoints.
	//
	// Example:
	//   - Internal services: 2-5s
	//   - External APIs: 5-10s
	//   - Cross-region: 10-15s
	//
	// Default: 5s
	DialTimeout time.Duration

	// KeepAlive specifies the TCP keep-alive probe interval.
	// This detects dead connections that weren't properly closed.
	//
	// Lower values detect dead connections faster but use more bandwidth.
	// Higher values are more efficient but slower to detect failures.
	//
	// Example:
	//   - Typical: 30s (default)
	//   - Cloud environments with aggressive NAT: 15s
	//   - Stable internal networks: 60s
	//
	// Default: 30s
	KeepAlive time.Duration

	// FallbackDelay is the RFC 6555 "Happy Eyeballs" delay for dual-stack
	// (IPv4/IPv6) connections. After this delay, a connection attempt to
	// the secondary address family starts in parallel.
	//
	// 300ms is the RFC recommendation. Set to negative to disable Happy Eyeballs.
	//
	// Default: 300ms
	FallbackDelay time.Duration

	// =======================================================================
	// Buffer Settings
	// =======================================================================

	// WriteBufferSize is the size of the write buffer for the connection.
	// Larger buffers improve throughput for large request bodies but use
	// more memory per connection.
	//
	// Example:
	//   - Small requests (JSON API): 4KB (4*1024)
	//   - Medium requests: 32KB (32*1024)
	//   - Large uploads: 64KB (64*1024) or 128KB
	//
	// Default: 64KB
	WriteBufferSize int

	// ReadBufferSize is the size of the read buffer for the connection.
	// Larger buffers improve throughput for large response bodies.
	//
	// Default: 64KB
	ReadBufferSize int

	// MaxResponseHeaderBytes limits the size of response headers.
	// Protects against malicious servers sending huge headers.
	//
	// Default: 0 (uses http.DefaultMaxHeaderBytes, ~1MB)
	MaxResponseHeaderBytes int64

	// =======================================================================
	// Protocol Settings
	// =======================================================================

	// DisableKeepAlives disables HTTP keep-alives, forcing a new connection
	// for each request. Almost never what you want in production.
	//
	// Use only for debugging or when connecting to servers that don't
	// properly support keep-alives.
	//
	// Default: false (keep-alives enabled)
	DisableKeepAlives bool

	// DisableCompression disables the "Accept-Encoding: gzip" header,
	// preventing automatic decompression of responses.
	//
	// This is disabled by default because not all downstream services support
	// compression. Enable compression explicitly when you know the downstream
	// supports it and responses are large enough to benefit.
	//
	// Default: true (compression disabled)
	DisableCompression bool

	// ForceHTTP2 forces HTTP/2 protocol (requires HTTPS).
	// HTTP/2 multiplexes requests over a single connection, reducing latency.
	//
	// Default: false (protocol negotiated automatically via ALPN)
	ForceHTTP2 bool
}

// DefaultConfig returns a balanced configuration suitable for most use cases.
//
// This configuration balances connection reuse, reasonable timeouts,
// and resource efficiency. It's designed for typical microservice
// communication patterns.
//
// Example:
//
//	// Use defaults as-is
//	client := sentinelhttpclient.New(sentinelhttpclient.WithConfig(sentinelhttpclient.DefaultConfig()))
//
//	// Or customize specific fields
//	cfg := sentinelhttpclient.DefaultConfig()
//	cfg.Timeout = 10 * time.Second
//	client := sentinelhttpclient.New(sentinelhttpclient.WithConfig(cfg))
func DefaultConfig() Config {
	return Config{
		// Overall timeout for a single logical request
		Timeout: 15 * time.Second,

		// Connection pool tuning (balanced)
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,

		// TLS and protocol timeouts
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 0, // Uses overall Timeout

		// TCP dial settings
		DialTimeout:   5 * time.Second,
		KeepAlive:     30 * time.Second,
		FallbackDelay: 300 * time.Millisecond,

		// Buffers (64KB for good throughput)
		WriteBufferSize: 64 * 1024,
		ReadBufferSize:  64 * 1024,

		// Protocol (defaults)
		DisableKeepAlives:  false,
		DisableCompression: true,
		ForceHTTP2:         false,
	}
}

// HighThroughputConfig returns a configuration optimized for high-concurrency
// scenarios with many concurrent requests to the same downstream services.
//
// Key differences from DefaultConfig:
//   - Higher connection pool limits for more concurrent connections
//   - Larger buffers for better I/O throughput
//   - Unlimited MaxConnsPerHost for burst handling
//
// Best for:
//   - API gateways
//   - Data processing pipelines
//   - High-traffic services
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(sentinelhttpclient.HighThroughputConfig()),
//	    sentinelhttpclient.WithServiceName("api-gateway"),
//	)
func HighThroughputConfig() Config {
	return Config{
		Timeout: 30 * time.Second,

		// Aggressive connection pooling
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     0, // Unlimited for bursts
		IdleConnTimeout:     120 * time.Second,

		// Standard timeouts
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// TCP settings
		DialTimeout:   5 * time.Second,
		KeepAlive:     30 * time.Second,
		FallbackDelay: 300 * time.Millisecond,

		// Larger buffers for throughput
		WriteBufferSize: 128 * 1024,
		ReadBufferSize:  128 * 1024,

		DisableKeepAlives:  false,
		DisableCompression: true,
		ForceHTTP2:         false,
	}
}

// LowLatencyConfig returns a configuration optimized for latency-sensitive
// applications where fast response times are critical.
//
// Key differences from DefaultConfig:
//   - Shorter timeouts to fail fast
//   - Quick dial timeout for fast failover
//   - Lower connection pool to reduce connection management overhead
//
// Best for:
//   - Real-time APIs
//   - User-facing services requiring fast responses
//   - Health checks and monitoring
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(sentinelhttpclient.LowLatencyConfig()),
//	    sentinelhttpclient.WithServiceName("realtime-api"),
//	)
func LowLatencyConfig() Config {
	return Config{
		Timeout: 5 * time.Second,

		// Reasonable pool for low latency
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 25,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     60 * time.Second,

		// Fast timeouts
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 500 * time.Millisecond,
		ResponseHeaderTimeout: 3 * time.Second,

		// Quick dial
		DialTimeout:   2 * time.Second,
		KeepAlive:     15 * time.Second,
		FallbackDelay: 150 * time.Millisecond,

		// Standard buffers
		WriteBufferSize: 32 * 1024,
		ReadBufferSize:  32 * 1024,

		DisableKeepAlives:  false,
		DisableCompression: true,
		ForceHTTP2:         true, // HTTP/2 reduces latency
	}
}

// ConservativeConfig returns a resource-conscious configuration suitable
// for environments with limited resources or many HTTP clients.
//
// Key differences from DefaultConfig:
//   - Lower connection pool limits to conserve memory
//   - Smaller buffers to reduce per-connection memory
//   - Shorter idle timeout to release resources faster
//
// Best for:
//   - Serverless functions (Lambda, Cloud Run)
//   - Sidecar containers with memory limits
//   - Applications with many HTTP client instances
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(sentinelhttpclient.ConservativeConfig()),
//	    sentinelhttpclient.WithServiceName("lambda-handler"),
//	)
func ConservativeConfig() Config {
	return Config{
		Timeout: 10 * time.Second,

		// Minimal connection pool
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     30 * time.Second,

		// Standard timeouts
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// TCP settings
		DialTimeout:   5 * time.Second,
		KeepAlive:     30 * time.Second,
		FallbackDelay: 300 * time.Millisecond,

		// Smaller buffers to save memory
		WriteBufferSize: 4 * 1024,
		ReadBufferSize:  4 * 1024,

		DisableKeepAlives:  false,
		DisableCompression: true,
		ForceHTTP2:         false,
	}
}

// =============================================================================
// Internal Configuration
// =============================================================================

// internalConfig holds all configuration including HTTP transport and OTel settings.
type internalConfig struct {
	// HTTP transport configuration
	httpConfig Config

	// === OpenTelemetry Configuration ===

	// TracerProvider is the tracer provider to use.
	// If not set, uses the global provider via otel.GetTracerProvider().
	TracerProvider trace.TracerProvider

	// MeterProvider is the meter provider to use.
	// If not set, uses the global provider via otel.GetMeterProvider().
	MeterProvider metric.MeterProvider

	// Tracer is the tracer instance created from TracerProvider.
	Tracer trace.Tracer

	// Meter is the meter instance created from MeterProvider.
	Meter metric.Meter

	// Metrics holds the metric instruments.
	Metrics *metrics

	// === Service Identification ===

	// ServiceName identifies the HTTP client for tracing purposes.
	// Added as "http.client.name" attribute on spans.
	ServiceName string

	// === Network Tracing ===

	// EnableNetworkTrace enables httptrace integration for detailed
	// network timing (DNS, TLS, Connect). Default: true
	EnableNetworkTrace bool

	// === Advanced Settings ===

	// TLSConfig specifies the TLS configuration.
	// If nil, the default configuration is used.
	TLSConfig *tls.Config

	// ProxyURL specifies a proxy URL for requests.
	// If nil and ProxyFromEnvironment is true, uses environment variables.
	ProxyURL *url.URL

	// ProxyFromEnvironment uses HTTP_PROXY, HTTPS_PROXY and NO_PROXY
	// environment variables. Default: true
	ProxyFromEnvironment bool

	// === Request Filtering ===

	// Filters determine which requests should be traced.
	// If any filter returns false, the request is not traced.
	// If no filters are set, all requests are traced.
	Filters []Filter

	// === Span Customization ===

	// SpanNameFormatter formats span names from request.
	// Default: "HTTP {method}"
	SpanNameFormatter SpanNameFormatter

	// SpanStartOptions are additional options applied when starting spans.
	SpanStartOptions []trace.SpanStartOption

	// MetricAttributesFn adds dynamic attributes to metrics based on request.
	MetricAttributesFn func(*http.Request) []attribute.KeyValue

	// === Context Propagation ===

	// Propagators configures the context propagators.
	// Default: TraceContext + Baggage (W3C standard)
	Propagators propagation.TextMapPropagator

	// ClientTrace provides a custom httptrace.ClientTrace factory.
	// This can be used to completely customize or replace network tracing.
	// If nil, the default network tracing is used when EnableNetworkTrace is true.
	ClientTrace func(context.Context) *httptrace.ClientTrace

	// === Retry Configuration ===

	// RetryConfig holds retry behavior configuration.
	// Default: DefaultRetryConfig() (3 retries, exponential backoff)
	RetryConfig RetryConfig

	// RetryClassifier determines which errors/responses should trigger retries.
	// Default: DefaultClassifier
	RetryClassifier RetryClassifier

	// RetryBackOff allows providing a custom backoff strategy.
	// If nil, uses ExponentialBackOff based on RetryConfig.
	RetryBackOff backoff.BackOff

	// === Circuit Breaker Configuration ===

	// BreakerConfig holds the circuit breaker configuration.
	// If nil, the circuit breaker is disabled.
	BreakerConfig *BreakerConfig

	// === Chaos Injection Configuration ===

	// ChaosConfig holds the chaos injection configuration for testing.
	// If nil, chaos injection is disabled.
	ChaosConfig *ChaosConfig

	// === Request Builder Configuration ===

	// BaseURL is the base URL for all requests made via the Request builder.
	// Example: "https://api.example.com"
	BaseURL string

	// DefaultHeaders are applied to all requests.
	DefaultHeaders http.Header

	// Debug enables request/response logging to zerolog.
	Debug bool

	// GenerateCurl enables cURL command generation for debugging.
	GenerateCurl bool

	// EnableTrace enables timing trace info collection.
	EnableTrace bool
}

// newConfig creates a new internal config with defaults and applies options.
func newConfig(opts ...Option) *internalConfig {
	cfg := &internalConfig{
		httpConfig:     DefaultConfig(),
		TracerProvider: otel.GetTracerProvider(),
		MeterProvider:  otel.GetMeterProvider(),

		// Defaults
		EnableNetworkTrace:   true,
		ProxyFromEnvironment: true,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// Initialize tracer and meter after options are applied
	cfg.Tracer = cfg.TracerProvider.Tracer(scope)
	cfg.Meter = cfg.MeterProvider.Meter(scope)

	// Initialize metrics (ignore errors, will just be nil if fails)
	cfg.Metrics, _ = newMetrics(cfg.Meter)

	// Initialize retry defaults if not explicitly configured.
	// We check if RetryConfig is still the zero value (not configured).
	// Note: NoRetryConfig explicitly sets MaxRetries=0, which we detect
	// by checking if Multiplier is also 0 (NoRetryConfig sets it to 0,
	// while a real config would have a positive multiplier).
	zeroConfig := RetryConfig{}
	if cfg.RetryConfig == zeroConfig {
		cfg.RetryConfig = DefaultRetryConfig()
	}
	if cfg.RetryClassifier == nil {
		cfg.RetryClassifier = DefaultClassifier
	}

	return cfg
}

// buildTransport creates an http.Transport from the configuration.
func (cfg *internalConfig) buildTransport() *http.Transport {
	hc := cfg.httpConfig

	dialer := &net.Dialer{
		Timeout:       hc.DialTimeout,
		KeepAlive:     hc.KeepAlive,
		FallbackDelay: hc.FallbackDelay,
	}

	transport := &http.Transport{
		DialContext:            dialer.DialContext,
		MaxIdleConns:           hc.MaxIdleConns,
		MaxIdleConnsPerHost:    hc.MaxIdleConnsPerHost,
		MaxConnsPerHost:        hc.MaxConnsPerHost,
		IdleConnTimeout:        hc.IdleConnTimeout,
		TLSHandshakeTimeout:    hc.TLSHandshakeTimeout,
		ResponseHeaderTimeout:  hc.ResponseHeaderTimeout,
		ExpectContinueTimeout:  hc.ExpectContinueTimeout,
		DisableKeepAlives:      hc.DisableKeepAlives,
		DisableCompression:     hc.DisableCompression,
		WriteBufferSize:        hc.WriteBufferSize,
		ReadBufferSize:         hc.ReadBufferSize,
		MaxResponseHeaderBytes: hc.MaxResponseHeaderBytes,
		TLSClientConfig:        cfg.TLSConfig,
		ForceAttemptHTTP2:      hc.ForceHTTP2,
	}

	// Configure proxy
	if cfg.ProxyURL != nil {
		transport.Proxy = http.ProxyURL(cfg.ProxyURL)
	} else if cfg.ProxyFromEnvironment {
		transport.Proxy = http.ProxyFromEnvironment
	}

	return transport
}

// baseAttributes returns common attributes for all spans and metrics.
func (cfg *internalConfig) baseAttributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 1)
	if cfg.ServiceName != "" {
		attrs = append(attrs, attribute.String("http.client.name", cfg.ServiceName))
	}
	return attrs
}

// =============================================================================
// Options - Functional Options for Client Configuration
// =============================================================================

// Filter determines whether a request should be traced.
// Return true to trace the request, false to skip tracing.
// All filters must return true for a request to be traced.
//
// Common use cases:
//   - Skip health check endpoints: return !strings.HasPrefix(r.URL.Path, "/health")
//   - Skip static assets: return !strings.HasPrefix(r.URL.Path, "/static/")
//   - Skip internal endpoints: return r.URL.Host != "localhost"
type Filter func(r *http.Request) bool

// SpanNameFormatter formats span names based on the HTTP request.
// The method parameter is the HTTP method (GET, POST, etc.).
// Return the desired span name.
//
// Default behavior produces: "HTTP {method}" (e.g., "HTTP GET")
//
// Example custom formatter:
//
//	func(method string, r *http.Request) string {
//	    return method + " " + r.URL.Path
//	}
type SpanNameFormatter func(method string, r *http.Request) string

// Option configures the HTTP client.
type Option func(*internalConfig)

// WithConfig sets the HTTP transport configuration.
// Use DefaultConfig(), HighThroughputConfig(), LowLatencyConfig(), or
// ConservativeConfig() as a starting point, then customize as needed.
//
// Example - Using a pre-defined config:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(sentinelhttpclient.HighThroughputConfig()),
//	)
//
// Example - Customizing the default config:
//
//	cfg := sentinelhttpclient.DefaultConfig()
//	cfg.Timeout = 10 * time.Second
//	cfg.MaxIdleConnsPerHost = 50
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithConfig(cfg),
//	)
func WithConfig(c Config) Option {
	return func(cfg *internalConfig) {
		cfg.httpConfig = c
	}
}

// WithServiceName sets an identifier for this HTTP client in traces.
// This value is added as the "http.client.name" attribute on all spans,
// making it easy to identify requests from this specific client in your
// observability tools.
//
// Best practices:
//   - Use a descriptive name that identifies the client's purpose
//   - Keep it short but meaningful
//   - Use lowercase with hyphens (e.g., "payment-client", "user-api")
//
// Example:
//
//	// Basic usage
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithServiceName("order-service"),
//	)
//
//	// In your traces, you'll see:
//	//   Span: HTTP GET
//	//   └── http.client.name: order-service
func WithServiceName(name string) Option {
	return func(cfg *internalConfig) {
		cfg.ServiceName = name
	}
}

// WithTracerProvider sets a custom OpenTelemetry TracerProvider.
// If not called, the global provider from otel.GetTracerProvider() is used.
//
// Use this when you need to:
//   - Use a different TracerProvider than the global one
//   - Configure custom span processors or exporters for HTTP spans
//   - Isolate HTTP tracing from other instrumentation
//
// Example:
//
//	// Create a custom tracer provider
//	tp := sdktrace.NewTracerProvider(
//	    sdktrace.WithBatcher(exporter),
//	    sdktrace.WithSampler(sdktrace.AlwaysSample()),
//	)
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithTracerProvider(tp),
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(cfg *internalConfig) {
		cfg.TracerProvider = tp
	}
}

// WithMeterProvider sets a custom OpenTelemetry MeterProvider.
// If not called, the global provider from otel.GetMeterProvider() is used.
//
// Use this when you need to:
//   - Use a different MeterProvider than the global one
//   - Configure custom metric readers or exporters for HTTP metrics
//   - Isolate HTTP metrics from other instrumentation
//
// Example:
//
//	// Create a custom meter provider
//	mp := sdkmetric.NewMeterProvider(
//	    sdkmetric.WithReader(periodicReader),
//	)
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithMeterProvider(mp),
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(cfg *internalConfig) {
		cfg.MeterProvider = mp
	}
}

// WithTLSConfig sets a custom TLS configuration.
// Use this for custom certificate verification, client certificates (mTLS),
// or specific TLS version requirements.
//
// Example - Skip certificate verification (NOT for production):
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithTLSConfig(&tls.Config{
//	        InsecureSkipVerify: true,
//	    }),
//	)
//
// Example - Mutual TLS with client certificate:
//
//	cert, _ := tls.LoadX509KeyPair("client.crt", "client.key")
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithTLSConfig(&tls.Config{
//	        Certificates: []tls.Certificate{cert},
//	    }),
//	)
func WithTLSConfig(tlsCfg *tls.Config) Option {
	return func(cfg *internalConfig) {
		cfg.TLSConfig = tlsCfg
	}
}

// WithProxyURL sets a specific proxy URL for all requests.
// When set, this takes precedence over environment variables.
//
// Example:
//
//	proxyURL, _ := url.Parse("http://proxy.internal:8080")
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithProxyURL(proxyURL),
//	)
func WithProxyURL(proxyURL *url.URL) Option {
	return func(cfg *internalConfig) {
		cfg.ProxyURL = proxyURL
		cfg.ProxyFromEnvironment = false
	}
}

// WithProxyFromEnvironment enables or disables reading proxy settings
// from environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY).
//
// Default: true (environment variables are used)
//
// Example - Disable environment proxy:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithProxyFromEnvironment(false),
//	)
func WithProxyFromEnvironment(enabled bool) Option {
	return func(cfg *internalConfig) {
		cfg.ProxyFromEnvironment = enabled
	}
}

// WithDisableNetworkTrace disables the httptrace integration that provides
// detailed network-level timing (DNS lookup, TLS handshake, connection time).
//
// You might disable this to:
//   - Reduce tracing overhead in extremely high-throughput scenarios
//   - Simplify trace output when network timing is not needed
//
// Default: Network tracing is enabled
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithDisableNetworkTrace(),
//	)
func WithDisableNetworkTrace() Option {
	return func(cfg *internalConfig) {
		cfg.EnableNetworkTrace = false
	}
}

// WithFilter adds a filter to determine which requests should be traced.
// Filters are called for each request before tracing starts.
// If any filter returns false, the request is not traced.
// Multiple filters can be added by calling WithFilter multiple times.
//
// Use filters to skip tracing for:
//   - Health check endpoints
//   - Metrics endpoints
//   - Static assets
//   - Internal/debug endpoints
//
// Example - Skip health checks:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithFilter(func(r *http.Request) bool {
//	        return !strings.HasPrefix(r.URL.Path, "/health")
//	    }),
//	    sentinelhttpclient.WithServiceName("my-service"),
//	)
//
// Example - Skip multiple endpoints:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithFilter(func(r *http.Request) bool {
//	        return !strings.HasPrefix(r.URL.Path, "/health")
//	    }),
//	    sentinelhttpclient.WithFilter(func(r *http.Request) bool {
//	        return !strings.HasPrefix(r.URL.Path, "/metrics")
//	    }),
//	)
func WithFilter(f Filter) Option {
	return func(cfg *internalConfig) {
		cfg.Filters = append(cfg.Filters, f)
	}
}

// WithSpanNameFormatter sets a custom function to generate span names.
// The default formatter produces "HTTP {method}" (e.g., "HTTP GET").
//
// Common patterns:
//   - Include path: func(m string, r *http.Request) string { return m + " " + r.URL.Path }
//   - Fixed operation: func(m string, r *http.Request) string { return "api-call" }
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithSpanNameFormatter(func(method string, r *http.Request) string {
//	        return method + " " + r.URL.Path
//	    }),
//	)
func WithSpanNameFormatter(f SpanNameFormatter) Option {
	return func(cfg *internalConfig) {
		cfg.SpanNameFormatter = f
	}
}

// WithSpanOptions adds trace.SpanStartOption to each new span.
// Use this to set custom attributes, links, or span kinds.
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithSpanOptions(
//	        trace.WithAttributes(attribute.String("team", "platform")),
//	    ),
//	)
func WithSpanOptions(opts ...trace.SpanStartOption) Option {
	return func(cfg *internalConfig) {
		cfg.SpanStartOptions = append(cfg.SpanStartOptions, opts...)
	}
}

// WithMetricAttributesFn sets a function to add dynamic attributes to metrics.
// The function is called for each request and the returned attributes
// are added to all metrics recorded for that request.
//
// Use this to add custom dimensions for filtering/grouping metrics.
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithMetricAttributesFn(func(r *http.Request) []attribute.KeyValue {
//	        return []attribute.KeyValue{
//	            attribute.String("tenant", r.Header.Get("X-Tenant-ID")),
//	        }
//	    }),
//	)
func WithMetricAttributesFn(f func(*http.Request) []attribute.KeyValue) Option {
	return func(cfg *internalConfig) {
		cfg.MetricAttributesFn = f
	}
}

// WithPropagators sets custom context propagators for trace context injection.
// By default, W3C TraceContext and Baggage propagators are used.
//
// Use this when you need to:
//   - Use different propagation formats (e.g., B3, Jaeger)
//   - Customize which headers are used for trace context
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithPropagators(b3.New()),
//	)
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(cfg *internalConfig) {
		cfg.Propagators = p
	}
}

// WithClientTrace sets a custom httptrace.ClientTrace factory.
// This completely replaces the built-in network tracing when provided.
//
// Use this when you need full control over HTTP client event tracing,
// or to integrate with custom tracing systems.
//
// Note: When set, EnableNetworkTrace is effectively ignored for the
// custom trace - you control all tracing behavior.
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
//	        return &httptrace.ClientTrace{
//	            DNSStart: func(info httptrace.DNSStartInfo) {
//	                log.Printf("DNS lookup: %s", info.Host)
//	            },
//	        }
//	    }),
//	)
func WithClientTrace(f func(context.Context) *httptrace.ClientTrace) Option {
	return func(cfg *internalConfig) {
		cfg.ClientTrace = f
	}
}

// =============================================================================
// Retry Options
// =============================================================================

// WithRetryConfig sets the retry behavior configuration.
// Use DefaultRetryConfig(), AggressiveRetryConfig(), or ConservativeRetryConfig()
// as a starting point, then customize as needed.
//
// Example - Using a preset:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryConfig(sentinelhttpclient.AggressiveRetryConfig()),
//	)
//
// Example - Custom configuration:
//
//	cfg := sentinelhttpclient.DefaultRetryConfig()
//	cfg.MaxRetries = 5
//	cfg.InitialInterval = 200 * time.Millisecond
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryConfig(cfg),
//	)
func WithRetryConfig(c RetryConfig) Option {
	return func(cfg *internalConfig) {
		cfg.RetryConfig = c
	}
}

// WithRetryDisabled disables automatic retries.
// Use this when:
//   - The operation is not idempotent
//   - You want to handle retries at a higher level
//   - Testing without retry interference
//
// Example:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryDisabled(),
//	)
func WithRetryDisabled() Option {
	return func(cfg *internalConfig) {
		cfg.RetryConfig = NoRetryConfig()
	}
}

// WithRetryClassifier sets a custom function to determine if a request should be retried.
// The default classifier (DefaultClassifier) retries on:
//   - Network errors (timeout, connection refused, DNS errors)
//   - 429 Too Many Requests
//   - 502, 503, 504 Gateway errors
//
// Example - Custom classifier that retries all 5xx errors:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryClassifier(func(resp *http.Response, err error) bool {
//	        if resp != nil && resp.StatusCode >= 500 {
//	            return true
//	        }
//	        return sentinelhttpclient.DefaultClassifier(resp, err)
//	    }),
//	)
func WithRetryClassifier(c RetryClassifier) Option {
	return func(cfg *internalConfig) {
		cfg.RetryClassifier = c
	}
}

// WithRetryBackOff sets a custom backoff strategy.
// Use this for non-exponential backoff patterns like linear or constant.
//
// Available strategies:
//   - NewLinearBackOff() - Linear growth with jitter
//   - NewDecorrelatedJitterBackOff() - AWS-style decorrelated jitter
//   - NewConstantBackOffWithJitter() - Fixed interval with jitter
//
// Example - Linear backoff:
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryBackOff(sentinelhttpclient.NewLinearBackOff()),
//	)
//
// Example - AWS-style decorrelated jitter:
//
//	backoff := sentinelhttpclient.NewDecorrelatedJitterBackOff()
//	backoff.Base = 200 * time.Millisecond
//	backoff.Cap = 10 * time.Second
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithRetryBackOff(backoff),
//	)
func WithRetryBackOff(b backoff.BackOff) Option {
	return func(cfg *internalConfig) {
		cfg.RetryBackOff = b
	}
}

// WithTieredRetry configures tiered retry with fixed-delay tiers followed by
// exponential backoff. This is useful for long-running retry scenarios.
//
// The tiered approach provides:
//   - Predictable delays in early tiers for quick recovery
//   - Gradually increasing delays for persistent failures
//   - Exponential backoff as a final fallback
//
// Example - Default tiers (5×1min + 5×2min + exponential up to 10min):
//
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithTieredRetry(nil, 10*time.Minute),
//	)
//
// Example - Custom tiers:
//
//	tiers := []sentinelhttpclient.RetryTier{
//	    {MaxRetries: 3, Delay: 30 * time.Second},
//	    {MaxRetries: 5, Delay: 1 * time.Minute},
//	    {MaxRetries: 5, Delay: 5 * time.Minute},
//	}
//	client := sentinelhttpclient.New(
//	    sentinelhttpclient.WithTieredRetry(tiers, 15*time.Minute),
//	)
func WithTieredRetry(tiers []RetryTier, maxDelay time.Duration) Option {
	return func(cfg *internalConfig) {
		if len(tiers) == 0 {
			// Use default tiers
			cfg.RetryBackOff = DefaultTieredRetryBackOff()
		} else {
			cfg.RetryBackOff = NewTieredRetryBackOff(tiers, maxDelay, DefaultJitterFactor)
		}
	}
}

// =============================================================================
// Circuit Breaker Options
// =============================================================================

// WithBreakerConfig sets the circuit breaker configuration.
// If not provided, the circuit breaker is disabled.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithBreakerConfig(httpclient.DefaultBreakerConfig()),
//	)
func WithBreakerConfig(c BreakerConfig) Option {
	return func(cfg *internalConfig) {
		cfg.BreakerConfig = &c
	}
}

// WithChaos enables chaos injection for testing resilience patterns.
//
// Chaos injection allows you to simulate failures in development/testing
// environments to verify that retry logic, circuit breakers, and fallbacks
// work correctly.
//
// WARNING: Do not use in production. This will intentionally degrade
// service performance and reliability.
//
// Example - Simulate slow responses:
//
//	client := httpclient.New(
//	    httpclient.WithChaos(httpclient.ChaosConfig{
//	        LatencyMs:       200,  // Add 200ms delay
//	        LatencyJitterMs: 100,  // Add 0-100ms jitter
//	    }),
//	)
//
// Example - Simulate failures to test circuit breaker:
//
//	client := httpclient.New(
//	    httpclient.WithChaos(httpclient.ChaosConfig{
//	        ErrorRate: 0.5, // 50% of requests fail
//	    }),
//	    httpclient.WithBreakerConfig(httpclient.DefaultBreakerConfig()),
//	)
func WithChaos(c ChaosConfig) Option {
	return func(cfg *internalConfig) {
		cfg.ChaosConfig = &c
	}
}

// =============================================================================
// Request Builder Options
// =============================================================================

// WithBaseURL sets the base URL for all requests made via the Request builder.
//
// The base URL is prepended to all request paths, allowing you to use
// relative paths in your requests.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	)
//	// Requests to "/users" will go to "https://api.example.com/users"
func WithBaseURL(url string) Option {
	return func(cfg *internalConfig) {
		cfg.BaseURL = url
	}
}

// WithDefaultHeaders sets headers that are applied to all requests.
//
// These headers are merged with per-request headers, with per-request
// headers taking precedence.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithDefaultHeaders(http.Header{
//	        "Accept": []string{"application/json"},
//	        "X-API-Key": []string{apiKey},
//	    }),
//	)
func WithDefaultHeaders(headers http.Header) Option {
	return func(cfg *internalConfig) {
		cfg.DefaultHeaders = headers
	}
}

// WithDefaultHeader adds a single default header applied to all requests.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithDefaultHeader("Accept", "application/json"),
//	    httpclient.WithDefaultHeader("X-API-Key", apiKey),
//	)
func WithDefaultHeader(key, value string) Option {
	return func(cfg *internalConfig) {
		if cfg.DefaultHeaders == nil {
			cfg.DefaultHeaders = make(http.Header)
		}
		cfg.DefaultHeaders.Set(key, value)
	}
}

// WithDebug enables request/response logging using zerolog.
//
// When enabled, the client logs:
//   - Request method and URL
//   - Response status and duration
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithDebug(true),
//	)
func WithDebug(enabled bool) Option {
	return func(cfg *internalConfig) {
		cfg.Debug = enabled
	}
}

// WithGenerateCurl enables cURL command generation for debugging.
//
// When enabled, each response will have a CurlCommand() method that
// returns the equivalent cURL command for reproducing the request.
//
// Note: This has a small performance overhead, so consider disabling
// in production.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithGenerateCurl(true),
//	)
//	resp, err := client.Request("Test").Get(ctx, "/users")
//	fmt.Println(resp.CurlCommand())
func WithGenerateCurl(enabled bool) Option {
	return func(cfg *internalConfig) {
		cfg.GenerateCurl = enabled
	}
}
