package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
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

		// Check for permanent errors that should not be retried (TLS, DNS NXDOMAIN, etc.)
		if isPermanentError(err) {
			return false
		}

		// Check for retryable network errors
		if isRetryableNetworkError(err) {
			return true
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

	// 1. Check net.Error interface (Timeout + Temporary)
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	// 2. Check net.DNSError
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		// Only retry if DNS error is explicitly temporary or timeout
		if dnsErr.IsTemporary || dnsErr.IsTimeout {
			return true
		}
		// All other DNS errors (including IsNotFound) are permanent
		return false
	}

	// 3. Check syscall retryable errors
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	// 4. Check for deadline exceeded and EOF
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, io.EOF) {
		return true
	}

	// 5. Fallback for edge cases (wrapped errors from third-party libraries)
	return containsTransientPattern(err)
}

// containsTransientPattern is a fallback for edge cases where type checks fail.
func containsTransientPattern(err error) bool {
	errStr := strings.ToLower(err.Error())
	patterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is down",
		"network unreachable",
		"i/o timeout",
		"temporary failure",
		"server closed",
		"broken pipe",
		"eof",
	}
	for _, p := range patterns {
		if strings.Contains(errStr, p) {
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

	// 1. TLS/Certificate errors
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}

	// 2. DNS not found (host doesn't exist - NXDOMAIN)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}

	// 3. Syscall permanent errors
	if errors.Is(err, syscall.EACCES) || // Permission denied
		errors.Is(err, syscall.EHOSTDOWN) { // Host is down
		return true
	}

	// 4. Fallback for edge cases
	return containsPermanentPattern(err)
}

// containsPermanentPattern is a fallback for edge cases where type checks fail.
func containsPermanentPattern(err error) bool {
	errStr := strings.ToLower(err.Error())
	patterns := []string{
		"x509:",
		"certificate",
		"tls:",
		"protocol error",
		"no route to host",
		"permission denied",
	}
	for _, p := range patterns {
		if strings.Contains(errStr, p) {
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
