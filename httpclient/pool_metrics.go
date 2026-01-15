package httpclient

import (
	"net/http"
	"time"
)

// =============================================================================
// Pool Stats Types
// =============================================================================

// PoolStats provides a snapshot of connection pool configuration.
// This is useful for debugging and monitoring connection pool settings.
//
// Example usage:
//
//	client := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithMaxIdleConns(100),
//	)
//
//	stats := client.PoolStats()
//	fmt.Printf("Max idle conns: %d\n", stats.MaxIdleConns)
//	fmt.Printf("Max conns per host: %d\n", stats.MaxConnsPerHost)
//	fmt.Printf("Idle conn timeout: %s\n", stats.IdleConnTimeout)
type PoolStats struct {
	// MaxIdleConns is the maximum idle connections across all hosts.
	// Zero means use Go's default (currently 100).
	MaxIdleConns int

	// MaxIdleConnsPerHost is the maximum idle connections per host.
	// Zero means use Go's default (currently 2).
	MaxIdleConnsPerHost int

	// MaxConnsPerHost is the maximum total connections per host.
	// Zero means unlimited.
	MaxConnsPerHost int

	// IdleConnTimeout is how long idle connections are kept before closing.
	// Zero means connections are kept indefinitely.
	IdleConnTimeout time.Duration

	// DisableKeepAlives indicates if HTTP keep-alives are disabled.
	DisableKeepAlives bool
}

// =============================================================================
// Client Methods
// =============================================================================

// PoolStats returns the current connection pool configuration.
// This is useful for debugging and verifying pool settings.
//
// Returns empty PoolStats if transport is not accessible.
func (c *Client) PoolStats() PoolStats {
	if c.httpClient == nil || c.httpClient.Transport == nil {
		return PoolStats{}
	}

	transport := unwrapTransport(c.httpClient.Transport)
	if transport == nil {
		return PoolStats{}
	}

	return PoolStats{
		MaxIdleConns:        transport.MaxIdleConns,
		MaxIdleConnsPerHost: transport.MaxIdleConnsPerHost,
		MaxConnsPerHost:     transport.MaxConnsPerHost,
		IdleConnTimeout:     transport.IdleConnTimeout,
		DisableKeepAlives:   transport.DisableKeepAlives,
	}
}

// =============================================================================
// Internal Utilities
// =============================================================================

// unwrapTransport traverses the transport chain to find the base http.Transport.
// This handles wrapped transports (OTel, circuit breaker, retry, etc.).
func unwrapTransport(rt http.RoundTripper) *http.Transport {
	for {
		switch t := rt.(type) {
		case *http.Transport:
			return t
		case interface{ Unwrap() http.RoundTripper }:
			rt = t.Unwrap()
		default:
			return nil
		}
	}
}
