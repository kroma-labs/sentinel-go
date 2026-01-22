// Package fiber provides middleware adapters for Fiber framework.
//
// This package wraps httpserver middleware for seamless integration with Fiber.
//
// # Performance Note
//
// Fiber uses fasthttp, not net/http. This adapter uses gofiber/adaptor to bridge
// the gap. While there may be minor performance overhead, it allows using all
// sentinel-go middleware consistently across frameworks.
//
// # Quick Start
//
//	app := fiber.New()
//
//	// Use sentinel-go middleware
//	app.Use(fibersentinel.RequestID())
//	app.Use(fibersentinel.Recovery(logger))
//	app.Use(fibersentinel.Tracing(httpserver.DefaultTracingConfig()))
//
//	// Rate limiting
//	app.Use(fibersentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 100,
//	    Burst: 200,
//	}))
//
//	// Register health endpoints
//	fibersentinel.RegisterHealth(app, healthHandler)
//
// # Available Middleware
//
//   - RequestID: Generates/forwards X-Request-ID header
//   - Recovery: Panic recovery with structured logging
//   - Logger: Structured request/response logging
//   - Tracing: OpenTelemetry distributed tracing
//   - Metrics: OpenTelemetry request metrics
//   - CORS: Cross-Origin Resource Sharing
//   - Timeout: Per-request timeout
//   - RateLimit: Token bucket rate limiting (in-memory or Redis)
//   - ServiceAuth: Service-to-service authentication
//
// # Service Endpoints
//
//   - RegisterHealth: /ping, /livez, /readyz
//   - RegisterPprof: /debug/pprof/*
//   - RegisterPrometheus: /metrics
package fiber

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// WrapMiddleware adapts httpserver middleware to Fiber middleware.
//
// Use this to wrap any httpserver.Middleware for use with Fiber:
//
//	app.Use(fibersentinel.WrapMiddleware(myCustomMiddleware))
func WrapMiddleware(m httpserver.Middleware) fiber.Handler {
	return adaptor.HTTPMiddleware(func(next http.Handler) http.Handler {
		return m(next)
	})
}

// Recovery returns Fiber middleware that recovers from panics.
//
// On panic, logs the stack trace and returns 500 Internal Server Error.
//
//	app.Use(fibersentinel.Recovery(logger))
func Recovery(logger zerolog.Logger) fiber.Handler {
	return WrapMiddleware(httpserver.Recovery(logger))
}

// RequestID returns Fiber middleware that generates/forwards X-Request-ID.
//
// If X-Request-ID header exists, it's forwarded. Otherwise, a new UUID is generated.
//
//	app.Use(fibersentinel.RequestID())
func RequestID() fiber.Handler {
	return WrapMiddleware(httpserver.RequestID())
}

// Logger returns Fiber middleware for structured request logging.
//
// Logs method, path, status, duration, and more.
//
//	app.Use(fibersentinel.Logger(httpserver.LoggerConfig{
//	    Logger:    logger,
//	    SkipPaths: []string{"/healthz", "/metrics"},
//	}))
func Logger(cfg httpserver.LoggerConfig) fiber.Handler {
	return WrapMiddleware(httpserver.Logger(cfg))
}

// Tracing returns Fiber middleware for OpenTelemetry tracing.
//
// Creates spans for each request with attributes like method, path, status.
//
//	app.Use(fibersentinel.Tracing(httpserver.DefaultTracingConfig()))
func Tracing(cfg httpserver.TracingConfig) fiber.Handler {
	return WrapMiddleware(httpserver.Tracing(cfg))
}

// CORS returns Fiber middleware for CORS handling.
//
//	app.Use(fibersentinel.CORS(httpserver.DefaultCORSConfig()))
func CORS(cfg httpserver.CORSConfig) fiber.Handler {
	return WrapMiddleware(httpserver.CORS(cfg))
}

// Timeout returns Fiber middleware for per-handler timeout.
//
// Requests exceeding the timeout return 504 Gateway Timeout.
//
//	app.Use(fibersentinel.Timeout(30 * time.Second))
func Timeout(timeout time.Duration) fiber.Handler {
	return WrapMiddleware(httpserver.Timeout(timeout))
}

// Metrics returns Fiber middleware for OTel metrics.
//
// Records request duration, size, response size, status codes.
//
//	metrics, _ := httpserver.NewMetrics(httpserver.DefaultMetricsConfig())
//	app.Use(fibersentinel.Metrics(metrics))
func Metrics(m *httpserver.Metrics) fiber.Handler {
	return WrapMiddleware(m.Middleware())
}

// RateLimit returns Fiber middleware for token bucket rate limiting.
//
// Global rate limit:
//
//	app.Use(fibersentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 100,  // 100 requests per second
//	    Burst: 200,  // allow bursts up to 200
//	}))
//
// Per-IP rate limit:
//
//	app.Use(fibersentinel.RateLimitByIP(100, 200))
//
// With Redis for distributed rate limiting:
//
//	app.Use(fibersentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    Redis:   redisClient,
//	    KeyFunc: httpserver.KeyFuncByIP(),
//	}))
func RateLimit(cfg httpserver.RateLimitConfig) fiber.Handler {
	return WrapMiddleware(httpserver.RateLimit(cfg))
}

// RateLimitByIP returns Fiber middleware that rate limits per client IP.
//
//	app.Use(fibersentinel.RateLimitByIP(100, 200))  // 100/sec, burst 200
func RateLimitByIP(limit rate.Limit, burst int) fiber.Handler {
	return WrapMiddleware(httpserver.RateLimitByIP(limit, burst))
}

// ServiceAuth returns Fiber middleware for service-to-service auth.
//
// Validates Client-ID and Pass-Key headers against provided validator.
//
//	validator := httpserver.NewMemoryCredentialValidator(map[string]string{
//	    os.Getenv("CLIENT_ID"): os.Getenv("PASS_KEY"),
//	})
//	app.Use(fibersentinel.ServiceAuth(httpserver.ServiceAuthConfig{
//	    Validator: validator,
//	}))
func ServiceAuth(cfg httpserver.ServiceAuthConfig) fiber.Handler {
	return WrapMiddleware(httpserver.ServiceAuth(cfg))
}

// RegisterHealth registers health endpoints on a Fiber app.
//
// Registers:
//
//   - GET /ping - Simple ping/pong
//
//   - GET /livez - Kubernetes liveness probe
//
//   - GET /readyz - Kubernetes readiness probe
//
//     health := httpserver.NewHealthHandler(httpserver.WithVersion("1.0.0"))
//     fibersentinel.RegisterHealth(app, health)
func RegisterHealth(app *fiber.App, h *httpserver.HealthHandler) {
	app.Get("/ping", adaptor.HTTPHandler(h.PingHandler()))
	app.Get("/livez", adaptor.HTTPHandler(h.LiveHandler()))
	app.Get("/readyz", adaptor.HTTPHandler(h.ReadyHandler()))
}

// RegisterPprof registers pprof endpoints on a Fiber app.
//
// Registers /debug/pprof/* endpoints for profiling.
//
//	fibersentinel.RegisterPprof(app, httpserver.PprofConfig{})
func RegisterPprof(app *fiber.App, cfg httpserver.PprofConfig) {
	handler := httpserver.PprofHandler(cfg)
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}
	app.All(cfg.Prefix+"/*", adaptor.HTTPHandler(handler))
}

// RegisterPrometheus registers the Prometheus metrics endpoint.
//
//	fibersentinel.RegisterPrometheus(app, "/metrics")
func RegisterPrometheus(app *fiber.App, path string) {
	if path == "" {
		path = "/metrics"
	}
	app.Get(path, adaptor.HTTPHandler(httpserver.PrometheusHandler()))
}
