package httpclient

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// retryTransport wraps an http.RoundTripper with retry logic.
// It uses the provided backoff strategy and classifier to determine
// when and how to retry failed requests.
type retryTransport struct {
	base       http.RoundTripper
	cfg        *internalConfig
	classifier RetryClassifier
}

// newRetryTransport creates a new retry transport wrapper.
func newRetryTransport(base http.RoundTripper, cfg *internalConfig) http.RoundTripper {
	// If retries are disabled, return the base transport directly
	if !cfg.RetryConfig.IsEnabled() {
		return base
	}

	classifier := cfg.RetryClassifier
	if classifier == nil {
		classifier = DefaultClassifier
	}

	return &retryTransport{
		base:       base,
		cfg:        cfg,
		classifier: classifier,
	}
}

// RoundTrip implements http.RoundTripper with automatic retries.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	cfg := t.cfg.RetryConfig

	// Capture request body for potential retries
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Get or create span for retry events
	span := trace.SpanFromContext(ctx)

	// Create backoff strategy
	b := t.getBackoff()

	var (
		resp      *http.Response
		lastErr   error
		attempt   int
		startTime = time.Now()
	)

	// Use cenkalti/backoff retry with context
	retryOpts := []backoff.RetryOption{
		backoff.WithBackOff(b),
		backoff.WithMaxTries(cfg.MaxRetries + 1), // +1 because initial attempt is counted
	}

	if cfg.MaxElapsedTime > 0 {
		retryOpts = append(retryOpts, backoff.WithMaxElapsedTime(cfg.MaxElapsedTime))
	}

	// Add notify callback for retry events
	retryOpts = append(retryOpts, backoff.WithNotify(func(err error, next time.Duration) {
		attempt++
		t.recordRetryEvent(span, attempt, err, next)
		t.cfg.Metrics.recordRetryAttempt(ctx, t.cfg.baseAttributes(), attempt)
	}))

	resp, lastErr = backoff.Retry(ctx, func() (*http.Response, error) {
		// Clone request with fresh body for each attempt
		reqClone := t.cloneRequest(req, bodyBytes)

		// Execute request
		resp, err := t.base.RoundTrip(reqClone)

		// Check if we should retry
		if t.classifier(resp, err) {
			// Close response body before retry to prevent leaks
			if resp != nil && resp.Body != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			return nil, err
		}

		// Not retryable - return as permanent error to stop retrying
		if err != nil {
			return nil, backoff.Permanent(err)
		}

		return resp, nil
	}, retryOpts...)

	// Record final retry metrics
	totalDuration := time.Since(startTime)
	if attempt > 0 {
		span.SetAttributes(
			attribute.Int("http.retry_count", attempt),
			attribute.Bool("http.retry_success", lastErr == nil),
		)

		if lastErr != nil {
			t.cfg.Metrics.recordRetryExhausted(ctx, t.cfg.baseAttributes())
		}
	}
	t.cfg.Metrics.recordRetryDuration(ctx, t.cfg.baseAttributes(), totalDuration)

	return resp, lastErr
}

// cloneRequest creates a copy of the request with a fresh body.
func (t *retryTransport) cloneRequest(req *http.Request, bodyBytes []byte) *http.Request {
	// Clone the request
	clone := req.Clone(req.Context())

	// Reset body if we captured it
	if bodyBytes != nil {
		clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		clone.ContentLength = int64(len(bodyBytes))
	} else if req.GetBody != nil {
		// Use GetBody if available (standard library's preferred way)
		var err error
		clone.Body, err = req.GetBody()
		if err != nil {
			// Fall back to original body reference
			clone.Body = req.Body
		}
	}

	return clone
}

// getBackoff returns the configured backoff strategy.
func (t *retryTransport) getBackoff() backoff.BackOff {
	// Use custom backoff if provided
	if t.cfg.RetryBackOff != nil {
		t.cfg.RetryBackOff.Reset()
		return t.cfg.RetryBackOff
	}

	// Create exponential backoff from config
	return ExponentialBackOffFromConfig(t.cfg.RetryConfig)
}

// recordRetryEvent adds a span event for the retry attempt.
func (t *retryTransport) recordRetryEvent(
	span trace.Span,
	attempt int,
	err error,
	nextDelay time.Duration,
) {
	if !span.IsRecording() {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.Int("retry.attempt", attempt),
		attribute.Int64("retry.delay_ms", nextDelay.Milliseconds()),
	}

	if err != nil {
		// Classify the retry reason
		reason := "unknown"
		errStr := err.Error()

		if isRetryableNetworkError(err) {
			reason = "network_error"
		} else if len(errStr) > 0 {
			reason = errStr
			if len(reason) > 50 {
				reason = reason[:50] + "..."
			}
		}

		attrs = append(attrs, attribute.String("retry.reason", reason))

		// Record error on span
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	span.AddEvent("http.retry", trace.WithAttributes(attrs...))
}
