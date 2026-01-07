package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
)

// RetryClassifier determines if a request should be retried.
// Return true to retry, false to stop immediately.
//
// The classifier receives both the response and error to make decisions.
// Common classification patterns:
//   - Network errors (timeout, connection refused): Retry
//   - 429 Too Many Requests: Retry (with Retry-After header if present)
//   - 502/503/504 Gateway errors: Retry (transient infrastructure issues)
//   - 500 Internal Server Error: Usually don't retry (server bug, not transient)
//   - 4xx Client errors: Never retry (request is invalid)
//   - Context cancelled: Never retry (intentional cancellation)
//
// Example custom classifier that retries all 5xx errors:
//
//	client := httpclient.New(
//	    httpclient.WithRetryClassifier(func(resp *http.Response, err error) bool {
//	        // Retry all 5xx errors including 500
//	        if resp != nil && resp.StatusCode >= 500 {
//	            return true
//	        }
//	        return httpclient.DefaultClassifier(resp, err)
//	    }),
//	)
//
// Example classifier that never retries:
//
//	client := httpclient.New(
//	    httpclient.WithRetryClassifier(func(resp *http.Response, err error) bool {
//	        return false
//	    }),
//	)
type RetryClassifier func(resp *http.Response, err error) bool

// DefaultClassifier applies production-safe retry rules.
//
// Retries on:
//   - Network errors (timeout, connection refused, DNS errors)
//   - 429 Too Many Requests (rate limiting)
//   - 502 Bad Gateway
//   - 503 Service Unavailable
//   - 504 Gateway Timeout
//
// Does NOT retry on:
//   - 500 Internal Server Error (server bug, unlikely to resolve with retry)
//   - 4xx Client errors (request is invalid, retry won't help)
//   - Context cancellation (intentional cancellation by caller)
//   - Permanent errors (TLS certificate errors, etc.)
//   - nil error with success response (no retry needed)
//
// This classifier is designed for safety:
//   - It avoids retrying requests that are unlikely to succeed
//   - It respects intentional cancellation
//   - It distinguishes transient errors from permanent failures
func DefaultClassifier(resp *http.Response, err error) bool {
	// Success - no retry needed
	if err == nil && resp != nil && resp.StatusCode < 400 {
		return false
	}

	// Check for context cancellation - never retry intentional cancellation
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}

		// Check for retryable network errors
		if isRetryableNetworkError(err) {
			return true
		}

		// Check for permanent errors that should not be retried
		if isPermanentError(err) {
			return false
		}

		// Unknown error - default to retry for network-level errors
		return true
	}

	// Response received - check status code
	if resp != nil {
		return isRetryableStatusCode(resp.StatusCode)
	}

	return false
}

// isRetryableStatusCode returns true for status codes that indicate
// transient failures that may succeed on retry.
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests: // 429 - Rate limited
		return true
	case http.StatusBadGateway: // 502 - Gateway error
		return true
	case http.StatusServiceUnavailable: // 503 - Service temporarily unavailable
		return true
	case http.StatusGatewayTimeout: // 504 - Gateway timeout
		return true
	default:
		return false
	}
}

// isRetryableNetworkError returns true for network errors that are
// typically transient and may succeed on retry.
func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	// Check for DNS errors (temporary resolution failures)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.Temporary()
	}

	// Check for common transient error patterns in error messages
	errStr := strings.ToLower(err.Error())
	transientPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",    // DNS failure
		"network is down", // Network unavailable
		"network unreachable",
		"i/o timeout",
		"operation timed out",
		"temporary failure",
		"server closed",
		"broken pipe",
		"connection closed",
		"eof",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isPermanentError returns true for errors that will not succeed
// on retry and should fail immediately.
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	permanentPatterns := []string{
		"certificate",       // TLS certificate errors
		"x509",              // Certificate validation errors
		"tls:",              // TLS handshake errors (permanent misconfig)
		"no route to host",  // Network configuration error
		"permission denied", // Access errors
		"protocol error",    // Protocol-level errors
	}

	for _, pattern := range permanentPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// RetryClassifierFunc is a convenience type for creating classifiers
// from simple functions.
type RetryClassifierFunc func(resp *http.Response, err error) bool

// Classify implements RetryClassifier.
func (f RetryClassifierFunc) Classify(resp *http.Response, err error) bool {
	return f(resp, err)
}

// AlwaysRetryClassifier returns a classifier that always retries on error.
// Use with caution - this may cause excessive retries.
func AlwaysRetryClassifier() RetryClassifier {
	return func(resp *http.Response, err error) bool {
		return err != nil || (resp != nil && resp.StatusCode >= 400)
	}
}

// NeverRetryClassifier returns a classifier that never retries.
// Use when you want to handle retries at a higher level.
func NeverRetryClassifier() RetryClassifier {
	return func(_ *http.Response, _ error) bool {
		return false
	}
}

// StatusCodeClassifier returns a classifier that retries on specific status codes.
// Network errors are always retried.
//
// Example:
//
//	// Retry on 500, 502, 503, 504
//	classifier := httpclient.StatusCodeClassifier(500, 502, 503, 504)
func StatusCodeClassifier(codes ...int) RetryClassifier {
	codeSet := make(map[int]bool, len(codes))
	for _, code := range codes {
		codeSet[code] = true
	}

	return func(resp *http.Response, err error) bool {
		// Always retry on retryable network errors
		if err != nil && isRetryableNetworkError(err) {
			return true
		}

		// Don't retry on permanent errors
		if err != nil && isPermanentError(err) {
			return false
		}

		// Check status code
		if resp != nil {
			return codeSet[resp.StatusCode]
		}

		return false
	}
}
