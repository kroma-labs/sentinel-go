// Package echo provides middleware adapters for Echo framework.
//
// This package wraps httpserver middleware for seamless integration with Echo.
//
// # Quick Start
//
//	e := echo.New()
//
//	// Use sentinel-go middleware
//	e.Use(echosentinel.RequestID())
//	e.Use(echosentinel.Recovery(logger))
//	e.Use(echosentinel.Tracing(httpserver.DefaultTracingConfig()))
//
//	// Rate limiting
//	e.Use(echosentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 100,
//	    Burst: 200,
//	}))
//
//	// Register health endpoints
//	echosentinel.RegisterHealth(e, healthHandler)
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
package echo

import (
	"net/http"
	"time"

	"github.com/kroma-labs/sentinel-go/httpserver"
	echolib "github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// WrapMiddleware adapts httpserver middleware to Echo middleware.
//
// Use this to wrap any httpserver.Middleware for use with Echo:
//
//	e.Use(echo.WrapMiddleware(myCustomMiddleware))
func WrapMiddleware(m httpserver.Middleware) echolib.MiddlewareFunc {
	return func(next echolib.HandlerFunc) echolib.HandlerFunc {
		return func(c echolib.Context) error {
			var err error
			handler := m(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				c.SetRequest(r)
				err = next(c)
			}))
			handler.ServeHTTP(c.Response(), c.Request())
			return err
		}
	}
}

// Recovery returns Echo middleware that recovers from panics.
//
// On panic, logs the stack trace and returns 500 Internal Server Error.
//
//	e.Use(echosentinel.Recovery(logger))
func Recovery(logger zerolog.Logger) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.Recovery(logger))
}

// RequestID returns Echo middleware that generates/forwards X-Request-ID.
//
// If X-Request-ID header exists, it's forwarded. Otherwise, a new UUID is generated.
//
//	e.Use(echosentinel.RequestID())
func RequestID() echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.RequestID())
}

// Logger returns Echo middleware for structured request logging.
//
// Logs method, path, status, duration, and more.
//
//	e.Use(echosentinel.Logger(httpserver.LoggerConfig{
//	    Logger:    logger,
//	    SkipPaths: []string{"/healthz", "/metrics"},
//	}))
func Logger(cfg httpserver.LoggerConfig) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.Logger(cfg))
}

// Tracing returns Echo middleware for OpenTelemetry tracing.
//
// Creates spans for each request with attributes like method, path, status.
//
//	e.Use(echosentinel.Tracing(httpserver.DefaultTracingConfig()))
func Tracing(cfg httpserver.TracingConfig) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.Tracing(cfg))
}

// CORS returns Echo middleware for CORS handling.
//
//	e.Use(echosentinel.CORS(httpserver.DefaultCORSConfig()))
func CORS(cfg httpserver.CORSConfig) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.CORS(cfg))
}

// Timeout returns Echo middleware for per-handler timeout.
//
// Requests exceeding the timeout return 504 Gateway Timeout.
//
//	e.Use(echosentinel.Timeout(30 * time.Second))
func Timeout(timeout time.Duration) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.Timeout(timeout))
}

// Metrics returns Echo middleware for OTel metrics.
//
// Records request duration, size, response size, status codes.
//
//	metrics, _ := httpserver.NewMetrics(httpserver.DefaultMetricsConfig())
//	e.Use(echosentinel.Metrics(metrics))
func Metrics(m *httpserver.Metrics) echolib.MiddlewareFunc {
	return WrapMiddleware(m.Middleware())
}

// RateLimit returns Echo middleware for token bucket rate limiting.
//
// Global rate limit:
//
//	e.Use(echosentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 100,  // 100 requests per second
//	    Burst: 200,  // allow bursts up to 200
//	}))
//
// Per-IP rate limit:
//
//	e.Use(echosentinel.RateLimitByIP(100, 200))
//
// With Redis for distributed rate limiting:
//
//	e.Use(echosentinel.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    Redis:   redisClient,
//	    KeyFunc: httpserver.KeyFuncByIP(),
//	}))
func RateLimit(cfg httpserver.RateLimitConfig) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.RateLimit(cfg))
}

// RateLimitByIP returns Echo middleware that rate limits per client IP.
//
//	e.Use(echosentinel.RateLimitByIP(100, 200))  // 100/sec, burst 200
func RateLimitByIP(limit rate.Limit, burst int) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.RateLimitByIP(limit, burst))
}

// ServiceAuth returns Echo middleware for service-to-service auth.
//
// Validates Client-ID and Pass-Key headers against provided validator.
//
//	validator := httpserver.NewMemoryCredentialValidator(map[string]string{
//	    os.Getenv("CLIENT_ID"): os.Getenv("PASS_KEY"),
//	})
//	e.Use(echosentinel.ServiceAuth(httpserver.ServiceAuthConfig{
//	    Validator: validator,
//	}))
func ServiceAuth(cfg httpserver.ServiceAuthConfig) echolib.MiddlewareFunc {
	return WrapMiddleware(httpserver.ServiceAuth(cfg))
}

// RegisterHealth registers health endpoints on an Echo instance.
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
//     echosentinel.RegisterHealth(e, health)
func RegisterHealth(e *echolib.Echo, h *httpserver.HealthHandler) {
	e.GET("/ping", echolib.WrapHandler(h.PingHandler()))
	e.GET("/livez", echolib.WrapHandler(h.LiveHandler()))
	e.GET("/readyz", echolib.WrapHandler(h.ReadyHandler()))
}

// RegisterPprof registers pprof endpoints on an Echo instance.
//
// Registers /debug/pprof/* endpoints for profiling.
//
//	echosentinel.RegisterPprof(e, httpserver.PprofConfig{})
func RegisterPprof(e *echolib.Echo, cfg httpserver.PprofConfig) {
	handler := httpserver.PprofHandler(cfg)
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}
	e.Any(cfg.Prefix+"/*", echolib.WrapHandler(handler))
}

// RegisterPrometheus registers the Prometheus metrics endpoint.
//
//	echosentinel.RegisterPrometheus(e, "/metrics")
func RegisterPrometheus(e *echolib.Echo, path string) {
	if path == "" {
		path = "/metrics"
	}
	e.GET(path, echolib.WrapHandler(httpserver.PrometheusHandler()))
}
