package httpserver

import (
	"net/http"

	"github.com/rs/zerolog"
)

// Option configures the server.
type Option func(*Config)

// WithConfig applies all settings from a Config struct.
//
// This is the recommended way to configure the server. Use one of the preset
// configurations (DefaultConfig, ProductionConfig, DevelopmentConfig) as a
// starting point, then override specific fields as needed.
//
// Example:
//
//	cfg := httpserver.ProductionConfig()
//	cfg.Addr = ":9090"
//	cfg.ShutdownTimeout = 30 * time.Second
//
//	server := httpserver.New(
//	    httpserver.WithConfig(cfg),
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHandler(mux),
//	)
func WithConfig(cfg Config) Option {
	return func(c *Config) {
		*c = cfg
	}
}

// WithServiceName sets the service name for the entire server.
//
// This is the SINGLE source of truth for service identity. The server
// automatically passes this value to all components that need it:
//   - Tracing spans (service.name attribute)
//   - Metrics (service.name label)
//   - Request logs (service field)
//   - Health check responses (service field)
//
// Example:
//
//	server := httpserver.New(
//	    httpserver.WithServiceName("payment-api"),
//	    httpserver.WithHandler(mux),
//	)
func WithServiceName(name string) Option {
	return func(c *Config) {
		c.ServiceName = name
	}
}

// WithHandler sets the HTTP handler for the server.
//
// This is required. The handler will be wrapped with any configured middleware
// in the order they are specified.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/", homeHandler)
//
//	server := httpserver.New(
//	    httpserver.WithHandler(mux),
//	)
func WithHandler(h http.Handler) Option {
	return func(c *Config) {
		c.Handler = h
	}
}

// WithLogger sets the server logger for lifecycle events only.
//
// This logger is used for:
//   - Server startup messages
//   - Graceful shutdown logging
//   - Internal errors
//
// For REQUEST logging (each HTTP request/response), use WithLogging instead.
// You can use both: WithLogger for server events, WithLogging for requests.
//
// Example:
//
//	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
//	server := httpserver.New(
//	    httpserver.WithLogger(logger),  // Server events
//	    httpserver.WithLogging(httpserver.LoggerConfig{Logger: logger}),  // Requests
//	    httpserver.WithHandler(mux),
//	)
func WithLogger(l zerolog.Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithMiddleware adds middleware to wrap the handler.
//
// Middleware is applied in order (first middleware wraps outermost).
// Use this for custom middleware or when you need fine-grained control.
//
// For built-in middleware (tracing, metrics, logging), prefer the dedicated
// options (WithTracing, WithMetrics, WithLogging) which automatically
// inject the server's ServiceName.
//
// Example:
//
//	server := httpserver.New(
//	    httpserver.WithHandler(mux),
//	    httpserver.WithMiddleware(
//	        httpserver.Recovery(logger),
//	        httpserver.RequestID(),
//	        httpserver.CORS(corsConfig),
//	    ),
//	)
func WithMiddleware(ms ...Middleware) Option {
	return func(c *Config) {
		c.Middleware = append(c.Middleware, ms...)
	}
}

// WithTracing enables OpenTelemetry tracing middleware.
//
// The server's ServiceName is automatically applied to all spans.
// You don't need to set ServiceName in TracingConfig.
//
// Example:
//
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithTracing(httpserver.TracingConfig{
//	        SkipPaths: []string{"/livez", "/readyz", "/ping"},
//	    }),
//	    httpserver.WithHandler(mux),
//	)
func WithTracing(cfg TracingConfig) Option {
	return func(c *Config) {
		c.TracingConfig = &cfg
	}
}

// WithMetrics enables OpenTelemetry metrics middleware.
//
// The server's ServiceName is automatically applied to all metrics.
// You don't need to set ServiceName in MetricsConfig.
//
// Example:
//
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithMetrics(httpserver.MetricsConfig{
//	        SkipPaths: []string{"/livez", "/readyz", "/ping"},
//	    }),
//	    httpserver.WithHandler(mux),
//	)
func WithMetrics(cfg MetricsConfig) Option {
	return func(c *Config) {
		c.MetricsConfig = &cfg
	}
}

// WithLogging enables request logging middleware.
//
// The server's ServiceName is automatically included in all log entries.
// You don't need to set ServiceName in LoggerConfig.
//
// Example:
//
//	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithLogging(httpserver.LoggerConfig{
//	        Logger:    logger,
//	        SkipPaths: []string{"/livez", "/readyz", "/ping"},
//	    }),
//	    httpserver.WithHandler(mux),
//	)
func WithLogging(cfg LoggerConfig) Option {
	return func(c *Config) {
		c.LoggerConfig = &cfg
	}
}

// WithHealth enables health check endpoints with auto-configured ServiceName.
//
// This creates a HealthHandler with the server's ServiceName and version,
// and returns it for adding checks and registering routes.
//
// Example:
//
//	var health *httpserver.HealthHandler
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHealth(&health, "1.0.0"),
//	    httpserver.WithHandler(mux),
//	)
//
//	health.AddReadinessCheck("database", dbPingCheck)
//	mux.Handle("/ping", health.PingHandler())
//	mux.Handle("/livez", health.LiveHandler())
//	mux.Handle("/readyz", health.ReadyHandler())
func WithHealth(handler **HealthHandler, version string) Option {
	return func(c *Config) {
		c.HealthVersion = version
		c.HealthHandler = handler
	}
}

// WithRateLimit enables global rate limiting for all requests.
//
// For per-endpoint rate limiting, use the RateLimit middleware directly
// on specific routes instead.
//
// Example (global rate limit):
//
//	server := httpserver.New(
//	    httpserver.WithRateLimit(httpserver.RateLimitConfig{
//	        Limit: 100,  // 100 requests per second
//	        Burst: 200,  // Allow bursts up to 200
//	    }),
//	    httpserver.WithHandler(mux),
//	)
//
// Example (per-endpoint rate limit - add middleware to specific routes):
//
//	// Apply stricter limit to sensitive endpoint
//	mux.Handle("/api/login", httpserver.RateLimitByIP(10, 20)(loginHandler))
//
//	// Apply different limit to API endpoints
//	mux.Handle("/api/", httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 50,
//	    Burst: 100,
//	})(apiHandler))
func WithRateLimit(cfg RateLimitConfig) Option {
	return func(c *Config) {
		c.RateLimitConfig = &cfg
	}
}
