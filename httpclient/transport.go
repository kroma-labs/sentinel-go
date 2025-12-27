package httpclient

import (
	"fmt"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Compile-time interface check.
var _ http.RoundTripper = (*otelTransport)(nil)

// otelTransport wraps an http.RoundTripper with OpenTelemetry instrumentation.
type otelTransport struct {
	base       http.RoundTripper
	cfg        *internalConfig
	propagator propagation.TextMapPropagator
}

// newOtelTransport creates a new instrumented transport.
func newOtelTransport(base http.RoundTripper, cfg *internalConfig) *otelTransport {
	return &otelTransport{
		base: base,
		cfg:  cfg,
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	}
}

// RoundTrip implements http.RoundTripper with full tracing and metrics.
func (t *otelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	ctx := req.Context()

	// Build span name: "HTTP {method}"
	spanName := "HTTP " + req.Method

	// Create span with client kind
	ctx, span := t.cfg.Tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(t.requestAttributes(req)...),
	)
	defer span.End()

	// Inject trace context into request headers
	t.propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	// Track active requests
	baseAttrs := t.cfg.baseAttributes()
	t.cfg.Metrics.recordActiveRequestStart(ctx, baseAttrs)
	defer t.cfg.Metrics.recordActiveRequestEnd(ctx, baseAttrs)

	// Record request body size if known
	if req.ContentLength > 0 {
		t.cfg.Metrics.recordRequestBodySize(ctx, req.ContentLength, baseAttrs)
	}

	// Setup network tracing if enabled
	var nt *networkTrace
	if t.cfg.EnableNetworkTrace {
		nt = &networkTrace{}
		clientTrace := createClientTrace(nt)
		ctx = httptrace.WithClientTrace(ctx, clientTrace)
	}

	// Update request with new context
	req = req.WithContext(ctx)

	// Perform the actual request
	resp, err := t.base.RoundTrip(req)

	// Calculate duration
	duration := time.Since(start)

	// Record network trace events and metrics
	if nt != nil {
		nt.addTraceEvents(span)
		nt.recordTimingMetrics(ctx, t.cfg.Metrics, baseAttrs)
	}

	// Handle errors
	if err != nil {
		errorType := classifyError(err)
		setSpanError(span, err, errorType)
		t.cfg.Metrics.recordError(ctx, errorType, baseAttrs)
		t.cfg.Metrics.recordRequestDuration(ctx, duration, t.errorAttributes(req, errorType))
		return nil, err
	}

	// Record response attributes
	span.SetAttributes(t.responseAttributes(resp)...)

	// Set span status based on response code
	if resp.StatusCode >= 400 {
		errorType := errorTypeFromStatusCode(resp.StatusCode)
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
		span.SetAttributes(attribute.String("error.type", errorType))
	}

	// Record response body size if known
	if resp.ContentLength > 0 {
		t.cfg.Metrics.recordResponseBodySize(ctx, resp.ContentLength, baseAttrs)
	}

	// Record request duration with response attributes
	t.cfg.Metrics.recordRequestDuration(ctx, duration, t.metricsAttributes(req, resp))

	return resp, nil
}

// requestAttributes returns span attributes for the request.
func (t *otelTransport) requestAttributes(req *http.Request) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 10)

	// Add base attributes (service name)
	attrs = append(attrs, t.cfg.baseAttributes()...)

	// HTTP method (required)
	attrs = append(attrs, attribute.String("http.request.method", req.Method))

	// URL components
	if req.URL != nil {
		attrs = append(attrs, attribute.String("url.full", req.URL.String()))
		attrs = append(attrs, attribute.String("url.scheme", req.URL.Scheme))

		// Server address and port
		host := req.URL.Hostname()
		if host != "" {
			attrs = append(attrs, attribute.String("server.address", host))
		}

		port := req.URL.Port()
		if port != "" {
			if p, err := strconv.Atoi(port); err == nil {
				attrs = append(attrs, attribute.Int("server.port", p))
			}
		} else {
			// Default ports
			switch req.URL.Scheme {
			case "http":
				attrs = append(attrs, attribute.Int("server.port", 80))
			case "https":
				attrs = append(attrs, attribute.Int("server.port", 443))
			}
		}
	}

	// Request body size
	if req.ContentLength > 0 {
		attrs = append(attrs, attribute.Int64("http.request.body.size", req.ContentLength))
	}

	// User agent
	if ua := req.UserAgent(); ua != "" {
		attrs = append(attrs, attribute.String("user_agent.original", ua))
	}

	return attrs
}

// responseAttributes returns span attributes for the response.
func (t *otelTransport) responseAttributes(resp *http.Response) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 4)

	// Status code (required when available)
	attrs = append(attrs, attribute.Int("http.response.status_code", resp.StatusCode))

	// Response body size
	if resp.ContentLength > 0 {
		attrs = append(attrs, attribute.Int64("http.response.body.size", resp.ContentLength))
	}

	// Protocol version
	if resp.Proto != "" {
		// Convert "HTTP/1.1" to "1.1", "HTTP/2.0" to "2"
		version := resp.Proto
		if len(version) > 5 && version[:5] == "HTTP/" {
			version = version[5:]
		}
		if version == "2.0" {
			version = "2"
		}
		attrs = append(attrs, attribute.String("network.protocol.version", version))
	}

	return attrs
}

// metricsAttributes returns attributes for metrics recording.
func (t *otelTransport) metricsAttributes(
	req *http.Request,
	resp *http.Response,
) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 5)

	// Add base attributes
	attrs = append(attrs, t.cfg.baseAttributes()...)

	// HTTP method (required per semconv)
	attrs = append(attrs, attribute.String("http.request.method", req.Method))

	// Server address and port (required per semconv)
	if req.URL != nil {
		host := req.URL.Hostname()
		if host != "" {
			attrs = append(attrs, attribute.String("server.address", host))
		}

		port := req.URL.Port()
		if port != "" {
			if p, err := strconv.Atoi(port); err == nil {
				attrs = append(attrs, attribute.Int("server.port", p))
			}
		} else {
			switch req.URL.Scheme {
			case "http":
				attrs = append(attrs, attribute.Int("server.port", 80))
			case "https":
				attrs = append(attrs, attribute.Int("server.port", 443))
			}
		}
	}

	// Response status code (required when available)
	if resp != nil {
		attrs = append(attrs, attribute.Int("http.response.status_code", resp.StatusCode))

		// Add error.type for 4xx/5xx responses
		if resp.StatusCode >= 400 {
			attrs = append(attrs, attribute.String("error.type", strconv.Itoa(resp.StatusCode)))
		}
	}

	return attrs
}

// errorAttributes returns attributes for error metrics.
func (t *otelTransport) errorAttributes(req *http.Request, errorType string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 5)

	// Add base attributes
	attrs = append(attrs, t.cfg.baseAttributes()...)

	// HTTP method
	attrs = append(attrs, attribute.String("http.request.method", req.Method))

	// Server address and port
	if req.URL != nil {
		host := req.URL.Hostname()
		if host != "" {
			attrs = append(attrs, attribute.String("server.address", host))
		}

		port := req.URL.Port()
		if port != "" {
			if p, err := strconv.Atoi(port); err == nil {
				attrs = append(attrs, attribute.Int("server.port", p))
			}
		}
	}

	// Error type
	if errorType != "" {
		attrs = append(attrs, attribute.String("error.type", errorType))
	}

	return attrs
}
