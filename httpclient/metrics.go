package httpclient

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// metrics holds the metric instruments for HTTP client operations.
type metrics struct {
	// === Request Duration & Size Metrics ===

	// requestDuration measures the total request duration in seconds.
	// Buckets optimized for HTTP latencies per OTel semconv.
	requestDuration metric.Float64Histogram

	// requestBodySize measures the size of request bodies in bytes.
	requestBodySize metric.Int64Histogram

	// responseBodySize measures the size of response bodies in bytes.
	responseBodySize metric.Int64Histogram

	// === Connection Pool Metrics ===

	// openConnections tracks the current number of open connections.
	openConnections metric.Int64UpDownCounter

	// connectionDuration measures time to establish a connection.
	connectionDuration metric.Float64Histogram

	// === Network Timing Metrics ===

	// dnsDuration measures DNS lookup time in seconds.
	dnsDuration metric.Float64Histogram

	// tlsDuration measures TLS handshake time in seconds.
	tlsDuration metric.Float64Histogram

	// ttfb measures Time To First Byte in seconds.
	ttfb metric.Float64Histogram

	// contentTransferDuration measures response body download time in seconds.
	contentTransferDuration metric.Float64Histogram

	// === Active Request Tracking ===

	// activeRequests tracks the number of in-flight requests.
	activeRequests metric.Int64UpDownCounter

	// === Error Metrics ===

	// requestErrors counts request errors by error type.
	requestErrors metric.Int64Counter

	// === Retry Metrics ===

	// retryAttempts counts retry attempts.
	// Incremented each time a request is retried.
	retryAttempts metric.Int64Counter

	// retryExhausted counts requests that exhausted all retries.
	// A high value indicates downstream service issues.
	retryExhausted metric.Int64Counter

	// retryDuration measures total time spent in retry loop.
	// Includes all attempts and wait times.
	retryDuration metric.Float64Histogram
}

// newMetrics creates and registers metric instruments.
func newMetrics(meter metric.Meter) (*metrics, error) {
	m := &metrics{}
	var err error

	// Request duration histogram with OTel semconv recommended buckets
	m.requestDuration, err = meter.Float64Histogram(
		"http.client.request.duration",
		metric.WithDescription("Duration of HTTP client requests in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10,
		),
	)
	if err != nil {
		return nil, err
	}

	// Request body size histogram
	m.requestBodySize, err = meter.Int64Histogram(
		"http.client.request.body.size",
		metric.WithDescription("Size of HTTP client request bodies in bytes"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(
			0, 100, 1024, 10*1024, 100*1024, 1024*1024, 10*1024*1024,
		),
	)
	if err != nil {
		return nil, err
	}

	// Response body size histogram
	m.responseBodySize, err = meter.Int64Histogram(
		"http.client.response.body.size",
		metric.WithDescription("Size of HTTP client response bodies in bytes"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(
			0, 100, 1024, 10*1024, 100*1024, 1024*1024, 10*1024*1024,
		),
	)
	if err != nil {
		return nil, err
	}

	// Open connections counter
	m.openConnections, err = meter.Int64UpDownCounter(
		"http.client.open_connections",
		metric.WithDescription("Number of open HTTP client connections"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	// Connection duration histogram
	m.connectionDuration, err = meter.Float64Histogram(
		"http.client.connection.duration",
		metric.WithDescription("Time to establish HTTP connection in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5,
		),
	)
	if err != nil {
		return nil, err
	}

	// DNS duration histogram
	m.dnsDuration, err = meter.Float64Histogram(
		"http.client.dns.duration",
		metric.WithDescription("DNS lookup duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
		),
	)
	if err != nil {
		return nil, err
	}

	// TLS handshake duration histogram
	m.tlsDuration, err = meter.Float64Histogram(
		"http.client.tls.duration",
		metric.WithDescription("TLS handshake duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
		),
	)
	if err != nil {
		return nil, err
	}

	// Time to first byte histogram
	m.ttfb, err = meter.Float64Histogram(
		"http.client.ttfb",
		metric.WithDescription("Time to first response byte in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5,
		),
	)
	if err != nil {
		return nil, err
	}

	// Content transfer duration histogram
	m.contentTransferDuration, err = meter.Float64Histogram(
		"http.client.content_transfer.duration",
		metric.WithDescription("Response body download duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		),
	)
	if err != nil {
		return nil, err
	}

	// Active requests counter
	m.activeRequests, err = meter.Int64UpDownCounter(
		"http.client.active_requests",
		metric.WithDescription("Number of active HTTP client requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	// Request errors counter
	m.requestErrors, err = meter.Int64Counter(
		"http.client.request.error",
		metric.WithDescription("Number of HTTP client request errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	// Retry attempts counter
	m.retryAttempts, err = meter.Int64Counter(
		"http.client.retry.attempts",
		metric.WithDescription("Number of HTTP client retry attempts"),
		metric.WithUnit("{attempt}"),
	)
	if err != nil {
		return nil, err
	}

	// Retry exhausted counter
	m.retryExhausted, err = meter.Int64Counter(
		"http.client.retry.exhausted",
		metric.WithDescription("Number of requests that exhausted all retries"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	// Retry duration histogram
	m.retryDuration, err = meter.Float64Histogram(
		"http.client.retry.duration",
		metric.WithDescription("Total time spent in retry loop in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120,
		),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// recordRequestDuration records the duration of an HTTP request.
func (m *metrics) recordRequestDuration(
	ctx context.Context,
	duration time.Duration,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.requestDuration == nil {
		return
	}
	m.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// recordRequestBodySize records the size of a request body.
func (m *metrics) recordRequestBodySize(
	ctx context.Context,
	size int64,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.requestBodySize == nil {
		return
	}
	m.requestBodySize.Record(ctx, size, metric.WithAttributes(attrs...))
}

// recordResponseBodySize records the size of a response body.
func (m *metrics) recordResponseBodySize(
	ctx context.Context,
	size int64,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.responseBodySize == nil {
		return
	}
	m.responseBodySize.Record(ctx, size, metric.WithAttributes(attrs...))
}

// recordConnectionOpened records a connection being opened.
//
//nolint:unused // Reserved for future connection tracking via httptrace
func (m *metrics) recordConnectionOpened(ctx context.Context, attrs []attribute.KeyValue) {
	if m == nil || m.openConnections == nil {
		return
	}
	m.openConnections.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// recordConnectionClosed records a connection being closed.
//
//nolint:unused // Reserved for future connection tracking via httptrace
func (m *metrics) recordConnectionClosed(ctx context.Context, attrs []attribute.KeyValue) {
	if m == nil || m.openConnections == nil {
		return
	}
	m.openConnections.Add(ctx, -1, metric.WithAttributes(attrs...))
}

// recordConnectionDuration records the time to establish a connection.
func (m *metrics) recordConnectionDuration(
	ctx context.Context,
	duration time.Duration,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.connectionDuration == nil {
		return
	}
	m.connectionDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// recordDNSDuration records the DNS lookup duration.
func (m *metrics) recordDNSDuration(
	ctx context.Context,
	duration time.Duration,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.dnsDuration == nil {
		return
	}
	m.dnsDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// recordTLSDuration records the TLS handshake duration.
func (m *metrics) recordTLSDuration(
	ctx context.Context,
	duration time.Duration,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.tlsDuration == nil {
		return
	}
	m.tlsDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// recordTTFB records Time To First Byte.
func (m *metrics) recordTTFB(
	ctx context.Context,
	duration time.Duration,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.ttfb == nil {
		return
	}
	m.ttfb.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// recordContentTransferDuration records response body download duration.
//
//nolint:unused // Reserved for future response body tracking
func (m *metrics) recordContentTransferDuration(
	ctx context.Context,
	duration time.Duration,
	attrs []attribute.KeyValue,
) {
	if m == nil || m.contentTransferDuration == nil {
		return
	}
	m.contentTransferDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// recordActiveRequestStart records a request starting.
func (m *metrics) recordActiveRequestStart(ctx context.Context, attrs []attribute.KeyValue) {
	if m == nil || m.activeRequests == nil {
		return
	}
	m.activeRequests.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// recordActiveRequestEnd records a request completing.
func (m *metrics) recordActiveRequestEnd(ctx context.Context, attrs []attribute.KeyValue) {
	if m == nil || m.activeRequests == nil {
		return
	}
	m.activeRequests.Add(ctx, -1, metric.WithAttributes(attrs...))
}

// recordError records a request error.
func (m *metrics) recordError(ctx context.Context, errorType string, attrs []attribute.KeyValue) {
	if m == nil || m.requestErrors == nil {
		return
	}
	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+1)
	allAttrs = append(allAttrs, attrs...)
	allAttrs = append(allAttrs, attribute.String("error.type", errorType))
	m.requestErrors.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}

// recordRetryAttempt records a retry attempt.
func (m *metrics) recordRetryAttempt(ctx context.Context, attrs []attribute.KeyValue, attempt int) {
	if m == nil || m.retryAttempts == nil {
		return
	}
	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+1)
	allAttrs = append(allAttrs, attrs...)
	allAttrs = append(allAttrs, attribute.Int("retry.attempt", attempt))
	m.retryAttempts.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}

// recordRetryExhausted records when all retries have been exhausted.
func (m *metrics) recordRetryExhausted(ctx context.Context, attrs []attribute.KeyValue) {
	if m == nil || m.retryExhausted == nil {
		return
	}
	m.retryExhausted.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// recordRetryDuration records the total time spent in a retry loop.
func (m *metrics) recordRetryDuration(
	ctx context.Context,
	attrs []attribute.KeyValue,
	duration time.Duration,
) {
	if m == nil || m.retryDuration == nil {
		return
	}
	m.retryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
