package httpclient

import (
	"math/rand/v2"
	"time"
)

// ChaosConfig configures chaos injection for testing resilience patterns.
//
// Chaos injection allows you to simulate failures in development/testing environments
// to verify that retry logic, circuit breakers, and fallbacks work correctly.
//
// Example usage:
//
//	client := httpclient.New(
//	    httpclient.WithChaos(httpclient.ChaosConfig{
//	        LatencyMs:    200,   // Add 200ms delay
//	        ErrorRate:    0.1,   // 10% of requests fail
//	    }),
//	)
type ChaosConfig struct {
	// LatencyMs adds a fixed delay (in milliseconds) to all requests.
	// This simulates network latency or slow upstream services.
	// Default: 0 (no added latency)
	LatencyMs int

	// LatencyJitterMs adds random jitter (0 to JitterMs) on top of LatencyMs.
	// The actual delay will be: LatencyMs + rand(0, LatencyJitterMs)
	// This creates more realistic latency patterns.
	// Default: 0 (no jitter)
	LatencyJitterMs int

	// ErrorRate is the probability (0.0-1.0) of injecting a connection error.
	// When an error is injected, the request fails with a simulated network error.
	// Default: 0.0 (no errors injected)
	ErrorRate float64

	// TimeoutRate is the probability (0.0-1.0) of simulating a request timeout.
	// When triggered, the request blocks until the context deadline is exceeded.
	// Default: 0.0 (no timeouts simulated)
	TimeoutRate float64
}

// Delay returns the total delay to apply, including jitter.
func (c ChaosConfig) Delay() time.Duration {
	delay := time.Duration(c.LatencyMs) * time.Millisecond
	if c.LatencyJitterMs > 0 {
		jitter := time.Duration(rand.IntN(c.LatencyJitterMs)) * time.Millisecond //nolint:gosec
		delay += jitter
	}
	return delay
}

// ShouldInjectError returns true if an error should be injected based on ErrorRate.
func (c ChaosConfig) ShouldInjectError() bool {
	if c.ErrorRate <= 0 {
		return false
	}
	return rand.Float64() < c.ErrorRate //nolint:gosec
}

// ShouldInjectTimeout returns true if a timeout should be simulated based on TimeoutRate.
func (c ChaosConfig) ShouldInjectTimeout() bool {
	if c.TimeoutRate <= 0 {
		return false
	}
	return rand.Float64() < c.TimeoutRate //nolint:gosec
}
