package httpserver

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics provides server metrics using OpenTelemetry.
type Metrics struct {
	serviceName     string
	requestDuration metric.Float64Histogram
	requestSize     metric.Int64Histogram
	responseSize    metric.Int64Histogram
	activeRequests  metric.Int64UpDownCounter
	requestTotal    metric.Int64Counter
	responseStatus  metric.Int64Counter
}

// MetricsConfig configures the metrics middleware.
type MetricsConfig struct {
	// MeterProvider is the OTel meter provider.
	// If nil, uses otel.GetMeterProvider().
	MeterProvider metric.MeterProvider

	// serviceName is set internally by the server.
	serviceName string

	// SkipPaths are paths that should not be recorded.
	SkipPaths []string

	// Buckets for request duration histogram (in seconds).
	// Default: [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
	DurationBuckets []float64
}

// DefaultMetricsConfig returns a default metrics configuration.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		MeterProvider: otel.GetMeterProvider(),
		DurationBuckets: []float64{
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		},
	}
}

// NewMetrics creates a new Metrics instance.
func NewMetrics(cfg MetricsConfig) (*Metrics, error) {
	if cfg.MeterProvider == nil {
		cfg.MeterProvider = otel.GetMeterProvider()
	}

	meter := cfg.MeterProvider.Meter(
		"github.com/kroma-labs/sentinel-go/httpserver",
		metric.WithInstrumentationVersion("1.0.0"),
	)

	requestDuration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(cfg.DurationBuckets...),
	)
	if err != nil {
		return nil, err
	}

	requestSize, err := meter.Int64Histogram(
		"http.server.request.size",
		metric.WithDescription("Size of HTTP request bodies in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	responseSize, err := meter.Int64Histogram(
		"http.server.response.size",
		metric.WithDescription("Size of HTTP response bodies in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	activeRequests, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of active HTTP requests"),
	)
	if err != nil {
		return nil, err
	}

	requestTotal, err := meter.Int64Counter(
		"http.server.request.total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return nil, err
	}

	responseStatus, err := meter.Int64Counter(
		"http.server.response.status",
		metric.WithDescription("HTTP response status code distribution"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		serviceName:     cfg.serviceName,
		requestDuration: requestDuration,
		requestSize:     requestSize,
		responseSize:    responseSize,
		activeRequests:  activeRequests,
		requestTotal:    requestTotal,
		responseStatus:  responseStatus,
	}, nil
}

// Middleware returns middleware that records HTTP metrics.
//
// Metrics recorded:
//   - http.server.request.duration: Request latency histogram
//   - http.server.request.size: Request body size histogram
//   - http.server.response.size: Response body size histogram
//   - http.server.active_requests: In-flight request gauge
//   - http.server.request.total: Total request counter
//   - http.server.response.status: Status code distribution
//
// Example:
//
//	metrics, _ := httpserver.NewMetrics(httpserver.DefaultMetricsConfig())
//	handler := metrics.Middleware()(myHandler)
func (m *Metrics) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Track active requests
			attrs := []attribute.KeyValue{
				attribute.String("service.name", m.serviceName),
				attribute.String("http.request.method", r.Method),
				attribute.String("url.path", r.URL.Path),
			}

			m.activeRequests.Add(r.Context(), 1, metric.WithAttributes(attrs...))
			defer m.activeRequests.Add(r.Context(), -1, metric.WithAttributes(attrs...))

			// Record request size
			if r.ContentLength > 0 {
				m.requestSize.Record(r.Context(), r.ContentLength, metric.WithAttributes(attrs...))
			}

			// Wrap response writer
			wrapped := wrapResponseWriter(w)

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := wrapped.Status()
			respSize := int64(wrapped.BytesWritten())

			allAttrs := make([]attribute.KeyValue, len(attrs)+1)
			copy(allAttrs, attrs)
			allAttrs[len(attrs)] = attribute.Int("http.response.status_code", status)

			m.requestDuration.Record(r.Context(), duration, metric.WithAttributes(allAttrs...))
			m.responseSize.Record(r.Context(), respSize, metric.WithAttributes(allAttrs...))
			m.requestTotal.Add(r.Context(), 1, metric.WithAttributes(allAttrs...))
			m.responseStatus.Add(r.Context(), 1, metric.WithAttributes(allAttrs...))
		})
	}
}
