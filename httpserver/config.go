package httpserver

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Config holds the HTTP server configuration parameters.
//
// Use DefaultConfig(), ProductionConfig(), or DevelopmentConfig() to get
// a properly initialized configuration, then modify specific fields as needed.
//
// Example:
//
//	cfg := httpserver.DefaultConfig()
//	cfg.Addr = ":9090"
//	cfg.ShutdownTimeout = 15 * time.Second
//
//	server := httpserver.New(
//	    httpserver.WithConfig(cfg),
//	    httpserver.WithHandler(mux),
//	)
type Config struct {
	// Addr is the TCP address to listen on (default: ":8080").
	Addr string

	// ServiceName is the name of the service (e.g. "my-service").
	// This is used for metrics, tracing, and health checks.
	// Default: "http-server"
	ServiceName string

	// ReadTimeout is the maximum duration for reading the entire request,
	// including the body. A zero or negative value means no timeout.
	//
	// Setting this helps protect against slow-loris attacks where a client
	// sends data very slowly to hold connections open.
	//
	// Default: 15s
	ReadTimeout time.Duration

	// ReadHeaderTimeout is the maximum duration for reading request headers.
	// If zero, ReadTimeout is used. If both are zero, there is no timeout.
	//
	// Default: 10s
	ReadHeaderTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	// A zero or negative value means no timeout.
	//
	// Default: 15s
	WriteTimeout time.Duration

	// IdleTimeout is the maximum duration to wait for the next request when
	// keep-alives are enabled. If zero, ReadTimeout is used.
	//
	// Default: 60s
	IdleTimeout time.Duration

	// MaxHeaderBytes controls the maximum number of bytes the server will
	// read parsing the request header's keys and values.
	//
	// Default: 1MB (1 << 20)
	MaxHeaderBytes int

	// TLSConfig optionally provides TLS configuration for HTTPS.
	// If nil, the server runs in HTTP mode.
	TLSConfig *tls.Config

	// Logger configures the server logger.
	Logger zerolog.Logger

	// Middleware is a list of middleware to apply to the server.
	Middleware []Middleware

	// Handler is the HTTP handler to serve requests.
	// This is required and must be set via WithHandler().
	Handler http.Handler

	// ShutdownTimeout is the maximum duration to wait for active connections
	// to complete during graceful shutdown.
	//
	// When shutdown is triggered:
	// 1. Server stops accepting new connections
	// 2. Waits up to ShutdownTimeout for in-flight requests to complete
	// 3. Forcibly closes remaining connections
	//
	// Default: 10s
	ShutdownTimeout time.Duration

	// DrainInterval is the interval between checks during shutdown to see if
	// all connections have been drained.
	//
	// Default: 500ms
	DrainInterval time.Duration

	// TracingConfig enables tracing. If set, ServiceName is automatically applied.
	TracingConfig *TracingConfig

	// MetricsConfig enables metrics. If set, ServiceName is automatically applied.
	MetricsConfig *MetricsConfig

	// LoggerConfig enables request logging. If set, ServiceName is automatically applied.
	LoggerConfig *LoggerConfig

	// HealthHandler is populated by WithHealth with the server's ServiceName.
	HealthHandler **HealthHandler

	// HealthVersion is the version string for health responses.
	HealthVersion string

	// RateLimitConfig enables global rate limiting.
	RateLimitConfig *RateLimitConfig
}

// DefaultConfig returns a balanced configuration suitable for most use cases.
//
// This configuration provides reasonable timeouts that protect against common
// attacks while being lenient enough for typical web applications.
//
// Timeout values:
//   - ReadTimeout: 15s
//   - WriteTimeout: 15s
//   - IdleTimeout: 60s
//   - ShutdownTimeout: 10s
func DefaultConfig() Config {
	return Config{
		Addr:              ":8080",
		ServiceName:       "http-server",
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		ShutdownTimeout:   10 * time.Second,
		DrainInterval:     500 * time.Millisecond,
	}
}

// ProductionConfig returns a hardened configuration optimized for production.
//
// Designed for Kubernetes environments where the default terminationGracePeriodSeconds is 30s.
//
// Rationale:
//   - ReadTimeout (10s): Most API requests complete in <1s; 10s catches slow clients
//     while failing fast for genuinely stalled connections.
//   - WriteTimeout (10s): Matches read; responses should complete quickly.
//   - IdleTimeout (30s): Shorter than default to free resources faster; most
//     HTTP/2 clients handle reconnection gracefully.
//   - ShutdownTimeout (25s): K8s sends SIGTERM, then waits terminationGracePeriodSeconds (30s)
//     before SIGKILL. We use 25s to complete gracefully with 5s buffer.
//     This allows in-flight requests to finish while ensuring we exit before
//     K8s forcibly kills the pod.
//
// Timeout values:
//   - ReadTimeout: 10s
//   - WriteTimeout: 10s
//   - IdleTimeout: 30s
//   - ShutdownTimeout: 25s (5s buffer before K8s SIGKILL at 30s)
func ProductionConfig() Config {
	return Config{
		Addr:              ":8080",
		ServiceName:       "http-server",
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		ShutdownTimeout:   25 * time.Second,
		DrainInterval:     500 * time.Millisecond,
	}
}

// DevelopmentConfig returns a lenient configuration for local development.
//
// Key differences from DefaultConfig:
//   - No read/write timeouts (allows debugging with breakpoints)
//   - Longer idle timeout (less connection churn during development)
//   - Very short shutdown timeout (fast restart during iteration)
//
// Warning: Do not use this in production!
//
// Timeout values:
//   - ReadTimeout: 0 (unlimited)
//   - WriteTimeout: 0 (unlimited)
//   - IdleTimeout: 120s
//   - ShutdownTimeout: 3s
func DevelopmentConfig() Config {
	return Config{
		Addr:              ":8080",
		ServiceName:       "http-server",
		ReadTimeout:       0, // Unlimited for debugging
		ReadHeaderTimeout: 0,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		ShutdownTimeout:   3 * time.Second,
		DrainInterval:     100 * time.Millisecond,
	}
}
