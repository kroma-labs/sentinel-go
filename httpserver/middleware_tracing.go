package httpserver

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingConfig configures the tracing middleware.
type TracingConfig struct {
	// TracerProvider is the OTel tracer provider.
	// If nil, uses otel.GetTracerProvider().
	TracerProvider trace.TracerProvider

	// Propagator is the context propagator.
	// If nil, uses otel.GetTextMapPropagator().
	Propagator propagation.TextMapPropagator

	// serviceName is set internally by the server.
	serviceName string

	// SkipPaths are paths that should not be traced.
	SkipPaths []string

	// SpanNameFormatter formats the span name.
	// Default: "HTTP {method} {path}"
	SpanNameFormatter func(r *http.Request) string
}

// DefaultTracingConfig returns a default tracing configuration.
func DefaultTracingConfig() TracingConfig {
	return TracingConfig{
		TracerProvider: otel.GetTracerProvider(),
		Propagator:     otel.GetTextMapPropagator(),
		SpanNameFormatter: func(r *http.Request) string {
			return "HTTP " + r.Method + " " + r.URL.Path
		},
	}
}

// Tracing returns middleware that adds OpenTelemetry tracing to requests.
//
// Features:
//   - Extracts trace context from incoming requests (W3C TraceContext)
//   - Creates server spans with standard HTTP attributes
//   - Records errors on 5xx responses
//   - Propagates span context to downstream handlers
//
// Example:
//
//	handler := httpserver.Tracing(httpserver.TracingConfig{
//	    TracerProvider: tp,
//	    ServiceName:    "api-gateway",
//	})(myHandler)
func Tracing(cfg TracingConfig) Middleware {
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = otel.GetTracerProvider()
	}
	if cfg.Propagator == nil {
		cfg.Propagator = otel.GetTextMapPropagator()
	}
	if cfg.SpanNameFormatter == nil {
		cfg.SpanNameFormatter = func(r *http.Request) string {
			return "HTTP " + r.Method + " " + r.URL.Path
		}
	}

	tracer := cfg.TracerProvider.Tracer(
		"github.com/kroma-labs/sentinel-go/httpserver",
		trace.WithInstrumentationVersion("1.0.0"),
	)

	skipPaths := make(map[string]bool)
	for _, path := range cfg.SkipPaths {
		skipPaths[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip tracing for certain paths
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Extract trace context from request headers
			ctx := cfg.Propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start server span
			spanName := cfg.SpanNameFormatter(r)
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.ServiceName(cfg.serviceName),
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.URLScheme(r.URL.Scheme),
					semconv.ServerAddress(r.Host),
					semconv.UserAgentOriginal(r.UserAgent()),
					semconv.ClientAddress(r.RemoteAddr),
				),
			)
			defer span.End()

			// Add request ID if available
			if requestID := RequestIDFromContext(ctx); requestID != "" {
				span.SetAttributes(attribute.String("request.id", requestID))
			}

			// Wrap response writer to capture status code
			wrapped := wrapResponseWriter(w)

			// Process request with updated context
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record response attributes
			status := wrapped.Status()
			span.SetAttributes(semconv.HTTPResponseStatusCode(status))

			// Mark span as error for 5xx responses
			if status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
		})
	}
}
