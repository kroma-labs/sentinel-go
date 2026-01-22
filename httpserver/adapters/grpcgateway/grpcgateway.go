// Package grpcgateway provides middleware adapters for grpc-gateway.
//
// This package wraps httpserver middleware for seamless integration with
// grpc-gateway's runtime.ServeMux.
//
// # Quick Start
//
//	gwmux := runtime.NewServeMux()
//
//	// Register gRPC services with gwmux...
//
//	// Wrap with middleware
//	handler := grpcgateway.WrapWithMiddleware(gwmux,
//	    httpserver.Recovery(logger),
//	    httpserver.RequestID(),
//	    httpserver.Tracing(httpserver.DefaultTracingConfig()),
//	)
//
//	// Serve
//	http.ListenAndServe(":8080", handler)
//
// # Using NewHandler for Production
//
// For production deployments, use NewHandler with the Config struct:
//
//	handler := grpcgateway.NewHandler(gwmux, grpcgateway.Config{
//	    Logger:    &logger,
//	    Tracer:    &httpserver.DefaultTracingConfig(),
//	    Metrics:   metrics,
//	    CORS:      &httpserver.DefaultCORSConfig(),
//	    RateLimit: &httpserver.RateLimitConfig{Limit: 100, Burst: 200},
//	})
//
// # Combining with HTTP Endpoints
//
// To serve both gRPC-Gateway and regular HTTP from the same port:
//
//	gwmux := runtime.NewServeMux()
//	httpmux := http.NewServeMux()
//	httpmux.Handle("/metrics", httpserver.PrometheusHandler())
//	httpmux.Handle("/ping", health.PingHandler())
//
//	handler := grpcgateway.CombinedMux(gwmux, httpmux)
package grpcgateway

import (
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/rs/zerolog"
)

// WrapWithMiddleware wraps a grpc-gateway ServeMux with httpserver middleware.
//
// The middleware is applied in order (first to last).
//
//	handler := grpcgateway.WrapWithMiddleware(gwmux,
//	    httpserver.Recovery(logger),
//	    httpserver.RequestID(),
//	    httpserver.Tracing(cfg),
//	)
func WrapWithMiddleware(mux *runtime.ServeMux, middlewares ...httpserver.Middleware) http.Handler {
	return httpserver.Chain(middlewares...)(mux)
}

// DefaultMiddleware returns a grpc-gateway handler wrapped with essential middleware.
//
// Includes:
//
//   - Recovery (panic recovery with logging)
//
//   - RequestID (X-Request-ID generation/forwarding)
//
//   - Logger (structured request logging)
//
//     handler := grpcgateway.DefaultMiddleware(gwmux, &logger)
func DefaultMiddleware(mux *runtime.ServeMux, logger *zerolog.Logger) http.Handler {
	middlewares := []httpserver.Middleware{
		httpserver.RequestID(),
	}

	if logger != nil {
		middlewares = append([]httpserver.Middleware{
			httpserver.Recovery(*logger),
		}, middlewares...)
		middlewares = append(middlewares, httpserver.Logger(httpserver.LoggerConfig{
			Logger: *logger,
		}))
	}

	return httpserver.Chain(middlewares...)(mux)
}

// WithTracing adds OpenTelemetry tracing to a handler.
//
//	handler = grpcgateway.WithTracing(handler, httpserver.DefaultTracingConfig())
func WithTracing(handler http.Handler, cfg httpserver.TracingConfig) http.Handler {
	return httpserver.Tracing(cfg)(handler)
}

// WithCORS adds CORS handling to a handler.
//
//	handler = grpcgateway.WithCORS(handler, httpserver.DefaultCORSConfig())
func WithCORS(handler http.Handler, cfg httpserver.CORSConfig) http.Handler {
	return httpserver.CORS(cfg)(handler)
}

// WithMetrics adds OpenTelemetry metrics to a handler.
//
//	metrics, _ := httpserver.NewMetrics(httpserver.DefaultMetricsConfig())
//	handler = grpcgateway.WithMetrics(handler, metrics)
func WithMetrics(handler http.Handler, m *httpserver.Metrics) http.Handler {
	return m.Middleware()(handler)
}

// WithRateLimit adds rate limiting to a handler.
//
//	handler = grpcgateway.WithRateLimit(handler, httpserver.RateLimitConfig{
//	    Limit: 100,
//	    Burst: 200,
//	})
func WithRateLimit(handler http.Handler, cfg httpserver.RateLimitConfig) http.Handler {
	return httpserver.RateLimit(cfg)(handler)
}

// WithServiceAuth adds service-to-service authentication to a handler.
//
//	validator := httpserver.NewMemoryCredentialValidator(credentials)
//	handler = grpcgateway.WithServiceAuth(handler, httpserver.ServiceAuthConfig{
//	    Validator: validator,
//	})
func WithServiceAuth(handler http.Handler, cfg httpserver.ServiceAuthConfig) http.Handler {
	return httpserver.ServiceAuth(cfg)(handler)
}

// CombinedMux creates a handler that routes between grpc-gateway and HTTP handlers.
//
// Requests with Content-Type "application/grpc" or "application/grpc-web" go to gwmux.
// All other requests go to httpmux.
//
// This is useful for serving both gRPC-Gateway (JSON) and regular HTTP endpoints
// (health checks, metrics, etc.) from the same port.
//
//	gwmux := runtime.NewServeMux()
//	httpmux := http.NewServeMux()
//	httpmux.Handle("/metrics", httpserver.PrometheusHandler())
//	httpmux.Handle("/ping", health.PingHandler())
//
//	handler := grpcgateway.CombinedMux(gwmux, httpmux)
func CombinedMux(gwmux *runtime.ServeMux, httpmux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType == "application/grpc" || contentType == "application/grpc-web" {
			gwmux.ServeHTTP(w, r)
			return
		}
		httpmux.ServeHTTP(w, r)
	})
}

// Config holds configuration for NewHandler.
type Config struct {
	// Logger enables Recovery and Logger middleware.
	Logger *zerolog.Logger

	// Tracer enables OpenTelemetry tracing.
	Tracer *httpserver.TracingConfig

	// Metrics enables OpenTelemetry metrics.
	Metrics *httpserver.Metrics

	// CORS enables CORS handling.
	CORS *httpserver.CORSConfig

	// RateLimit enables rate limiting.
	RateLimit *httpserver.RateLimitConfig

	// ServiceAuth enables service-to-service authentication.
	ServiceAuth *httpserver.ServiceAuthConfig
}

// NewHandler creates a production-ready grpc-gateway handler.
//
// Applies middleware in the following order:
//  1. Recovery (if Logger provided)
//  2. RequestID
//  3. Tracing (if Tracer provided)
//  4. Metrics (if Metrics provided)
//  5. RateLimit (if RateLimit provided)
//  6. ServiceAuth (if ServiceAuth provided)
//  7. CORS (if CORS provided)
//  8. Logger (if Logger provided)
//
// Example:
//
//	handler := grpcgateway.NewHandler(gwmux, grpcgateway.Config{
//	    Logger:    &logger,
//	    Tracer:    &httpserver.DefaultTracingConfig(),
//	    Metrics:   metrics,
//	    RateLimit: &httpserver.RateLimitConfig{Limit: 100, Burst: 200},
//	})
func NewHandler(mux *runtime.ServeMux, cfg Config) http.Handler {
	var middlewares []httpserver.Middleware

	if cfg.Logger != nil {
		middlewares = append(middlewares, httpserver.Recovery(*cfg.Logger))
	}

	middlewares = append(middlewares, httpserver.RequestID())

	if cfg.Tracer != nil {
		middlewares = append(middlewares, httpserver.Tracing(*cfg.Tracer))
	}

	if cfg.Metrics != nil {
		middlewares = append(middlewares, cfg.Metrics.Middleware())
	}

	if cfg.RateLimit != nil {
		middlewares = append(middlewares, httpserver.RateLimit(*cfg.RateLimit))
	}

	if cfg.ServiceAuth != nil {
		middlewares = append(middlewares, httpserver.ServiceAuth(*cfg.ServiceAuth))
	}

	if cfg.CORS != nil {
		middlewares = append(middlewares, httpserver.CORS(*cfg.CORS))
	}

	if cfg.Logger != nil {
		middlewares = append(middlewares, httpserver.Logger(httpserver.LoggerConfig{
			Logger: *cfg.Logger,
		}))
	}

	return httpserver.Chain(middlewares...)(mux)
}
