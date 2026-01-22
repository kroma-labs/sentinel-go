// Package gin provides middleware adapters for Gin framework.
//
// This package wraps httpserver middleware for seamless integration with Gin.
//
// # Quick Start
//
//	r := gin.New()
//
//	// Use sentinel-go middleware
//	r.Use(ginsentinel.RequestID())
//	r.Use(ginsentinel.Recovery(logger))
//	r.Use(ginsentinel.Tracing(httpserver.DefaultTracingConfig()))
//
//	// Rate limiting
//	r.Use(ginsentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 100,
//	    Burst: 200,
//	}))
//
//	// Register health endpoints
//	ginsentinel.RegisterHealth(r, healthHandler)
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
package gin

import (
	"net/http"
	"time"

	ginlib "github.com/gin-gonic/gin"
	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// WrapMiddleware adapts httpserver middleware to Gin middleware.
//
// Use this to wrap any httpserver.Middleware for use with Gin:
//
//	r.Use(ginsentinel.WrapMiddleware(myCustomMiddleware))
func WrapMiddleware(m httpserver.Middleware) ginlib.HandlerFunc {
	return func(c *ginlib.Context) {
		var aborted bool
		handler := m(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			c.Request = r
			c.Next()
			aborted = c.IsAborted()
		}))
		handler.ServeHTTP(c.Writer, c.Request)
		if aborted {
			c.Abort()
		}
	}
}

// Recovery returns Gin middleware that recovers from panics.
//
// On panic, logs the stack trace and returns 500 Internal Server Error.
//
//	r.Use(ginsentinel.Recovery(logger))
func Recovery(logger zerolog.Logger) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.Recovery(logger))
}

// RequestID returns Gin middleware that generates/forwards X-Request-ID.
//
// If X-Request-ID header exists, it's forwarded. Otherwise, a new UUID is generated.
//
//	r.Use(ginsentinel.RequestID())
func RequestID() ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.RequestID())
}

// Logger returns Gin middleware for structured request logging.
//
// Logs method, path, status, duration, and more.
//
//	r.Use(ginsentinel.Logger(httpserver.LoggerConfig{
//	    Logger:    logger,
//	    SkipPaths: []string{"/healthz", "/metrics"},
//	}))
func Logger(cfg httpserver.LoggerConfig) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.Logger(cfg))
}

// Tracing returns Gin middleware for OpenTelemetry tracing.
//
// Creates spans for each request with attributes like method, path, status.
//
//	r.Use(ginsentinel.Tracing(httpserver.DefaultTracingConfig()))
func Tracing(cfg httpserver.TracingConfig) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.Tracing(cfg))
}

// CORS returns Gin middleware for CORS handling.
//
//	r.Use(ginsentinel.CORS(httpserver.DefaultCORSConfig()))
func CORS(cfg httpserver.CORSConfig) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.CORS(cfg))
}

// Timeout returns Gin middleware for per-handler timeout.
//
// Requests exceeding the timeout return 504 Gateway Timeout.
//
//	r.Use(ginsentinel.Timeout(30 * time.Second))
func Timeout(timeout time.Duration) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.Timeout(timeout))
}

// Metrics returns Gin middleware for OTel metrics.
//
// Records request duration, size, response size, status codes.
//
//	metrics, _ := httpserver.NewMetrics(httpserver.DefaultMetricsConfig())
//	r.Use(ginsentinel.Metrics(metrics))
func Metrics(m *httpserver.Metrics) ginlib.HandlerFunc {
	return WrapMiddleware(m.Middleware())
}

// RateLimit returns Gin middleware for token bucket rate limiting.
//
// Global rate limit:
//
//	r.Use(ginsentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 100,  // 100 requests per second
//	    Burst: 200,  // allow bursts up to 200
//	}))
//
// Per-IP rate limit:
//
//	r.Use(ginsentinel.RateLimitByIP(100, 200))
//
// With Redis for distributed rate limiting:
//
//	r.Use(ginsentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    Redis:   redisClient,
//	    KeyFunc: httpserver.KeyFuncByIP(),
//	}))
func RateLimit(cfg httpserver.RateLimitConfig) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.RateLimit(cfg))
}

// RateLimitByIP returns Gin middleware that rate limits per client IP.
//
//	r.Use(ginsentinel.RateLimitByIP(100, 200))  // 100/sec, burst 200
func RateLimitByIP(limit rate.Limit, burst int) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.RateLimitByIP(limit, burst))
}

// ServiceAuth returns Gin middleware for service-to-service auth.
//
// Validates Client-ID and Pass-Key headers against provided validator.
//
//	validator := httpserver.NewMemoryCredentialValidator(map[string]string{
//	    os.Getenv("CLIENT_ID"): os.Getenv("PASS_KEY"),
//	})
//	r.Use(ginsentinel.ServiceAuth(httpserver.ServiceAuthConfig{
//	    Validator: validator,
//	}))
func ServiceAuth(cfg httpserver.ServiceAuthConfig) ginlib.HandlerFunc {
	return WrapMiddleware(httpserver.ServiceAuth(cfg))
}

// WrapHandler wraps an http.Handler as a Gin handler.
//
//	r.GET("/custom", ginsentinel.WrapHandler(myHandler))
func WrapHandler(h http.Handler) ginlib.HandlerFunc {
	return func(c *ginlib.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// RegisterHealth registers health endpoints on a Gin router.
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
//     ginsentinel.RegisterHealth(r, health)
func RegisterHealth(r *ginlib.Engine, h *httpserver.HealthHandler) {
	r.GET("/ping", WrapHandler(h.PingHandler()))
	r.GET("/livez", WrapHandler(h.LiveHandler()))
	r.GET("/readyz", WrapHandler(h.ReadyHandler()))
}

// RegisterPprof registers pprof endpoints on a Gin router.
//
// Registers /debug/pprof/* endpoints for profiling.
//
//	ginsentinel.RegisterPprof(r, httpserver.PprofConfig{})
func RegisterPprof(r *ginlib.Engine, cfg httpserver.PprofConfig) {
	handler := httpserver.PprofHandler(cfg)
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}
	r.Any(cfg.Prefix+"/*action", WrapHandler(handler))
}

// RegisterPrometheus registers the Prometheus metrics endpoint.
//
//	ginsentinel.RegisterPrometheus(r, "/metrics")
func RegisterPrometheus(r *ginlib.Engine, path string) {
	if path == "" {
		path = "/metrics"
	}
	r.GET(path, WrapHandler(httpserver.PrometheusHandler()))
}
