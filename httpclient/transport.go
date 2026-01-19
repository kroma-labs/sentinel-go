package httpclient

import (
	"errors"
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
	// Use custom propagators if configured, otherwise default to W3C TraceContext + Baggage
	propagator := cfg.Propagators
	if propagator == nil {
		propagator = propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)
	}

	return &otelTransport{
		base:       base,
		cfg:        cfg,
		propagator: propagator,
	}
}

// RoundTrip implements http.RoundTripper with full tracing and metrics.
func (t *otelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check filters - skip tracing if any filter returns false
	for _, f := range t.cfg.Filters {
		if !f(req) {
			return t.base.RoundTrip(req)
		}
	}

	start := time.Now()
	ctx := req.Context()

	// Build span name using formatter or default
	var spanName string
	if t.cfg.SpanNameFormatter != nil {
		spanName = t.cfg.SpanNameFormatter(req.Method, req)
	} else {
		spanName = "HTTP " + req.Method
	}

	// Build span options
	spanOpts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(t.requestAttributes(req)...),
	}
	spanOpts = append(spanOpts, t.cfg.SpanStartOptions...)

	// Create span
	ctx, span := t.cfg.Tracer.Start(ctx, spanName, spanOpts...)
	// Note: span.End() is NOT deferred here - it will be called when:
	// 1. Transport error occurs (immediately)
	// 2. Response body is closed or EOF is reached (via wrappedBody)

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

	// Setup network tracing
	var nt *networkTrace
	if t.cfg.ClientTrace != nil {
		// Use custom client trace factory
		ctx = httptrace.WithClientTrace(ctx, t.cfg.ClientTrace(ctx))
	} else if t.cfg.EnableNetworkTrace {
		// Use built-in network tracing
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

	// Handle errors - end span immediately on transport failure
	if err != nil {
		errorType := classifyError(err)
		setSpanError(span, err, errorType)
		t.cfg.Metrics.recordError(ctx, errorType, baseAttrs)
		t.cfg.Metrics.recordRequestDuration(ctx, duration, t.errorAttributes(req, errorType))
		span.End() // End span immediately on error
		return nil, err
	}

	// Guard against nil response with nil error (should not happen in valid Transport)
	if resp == nil {
		err = errors.New("transport returned nil response with nil error")
		setSpanError(span, err, "internal_error")
		span.End()
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

	// Wrap response body to end span on close/EOF
	// This ensures span duration includes body consumption time for streaming
	if resp.Body != nil {
		// Capture whether this was a new connection for closure tracking
		wasNewConnection := nt != nil && !nt.connReused && !nt.connectStart.IsZero()

		resp.Body = newWrappedBody(span, resp.Body, func(bytesRead int64) {
			// Record actual response body size if it differs from Content-Length
			if resp.ContentLength <= 0 && bytesRead > 0 {
				t.cfg.Metrics.recordResponseBodySize(ctx, bytesRead, baseAttrs)
			}

			// Decrement open connections counter when request using new connection completes
			if wasNewConnection {
				t.cfg.Metrics.recordConnectionClosed(ctx, baseAttrs)
			}
		})
	} else {
		// No body to read, end span immediately
		span.End()

		// Still need to track connection closure for bodyless responses
		if nt != nil && !nt.connReused && !nt.connectStart.IsZero() {
			t.cfg.Metrics.recordConnectionClosed(ctx, baseAttrs)
		}
	}

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
