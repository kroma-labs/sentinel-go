package httpclient

import (
	"errors"
	"net"
	"net/http"
	"time"
)

// ErrChaosInjected is returned when chaos injection simulates a network error.
var ErrChaosInjected = errors.New("chaos: simulated network error")

// chaosTransport wraps an http.RoundTripper to inject chaos for testing.
type chaosTransport struct {
	next   http.RoundTripper
	config ChaosConfig
}

// newChaosTransport creates a new chaos transport wrapper.
func newChaosTransport(next http.RoundTripper, cfg ChaosConfig) http.RoundTripper {
	return &chaosTransport{
		next:   next,
		config: cfg,
	}
}

// RoundTrip implements http.RoundTripper with chaos injection.
func (t *chaosTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Check for timeout injection first
	if t.config.ShouldInjectTimeout() {
		// Block until context is cancelled or deadline exceeded
		<-ctx.Done()
		return nil, ctx.Err()
	}

	// Check for error injection
	if t.config.ShouldInjectError() {
		return nil, &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: ErrChaosInjected,
		}
	}

	// Apply latency delay
	delay := t.config.Delay()
	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Forward to next transport
	return t.next.RoundTrip(req)
}
