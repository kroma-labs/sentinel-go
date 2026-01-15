package httpclient

import "time"

// AdaptiveHedgeConfig configures adaptive hedged requests.
//
// Adaptive hedging dynamically calculates the hedge delay based on historical
// latency data for the endpoint. This eliminates the need to manually configure
// P95 latency values.
//
// Example usage:
//
//	resp, err := client.Request("GetUser").
//	    AdaptiveHedge(httpclient.DefaultAdaptiveHedgeConfig()).
//	    Get(ctx, "/users/123")
//
// After sufficient samples are collected (MinSamples), the hedge delay is
// automatically set to the TargetPercentile latency. Until then, FallbackDelay
// is used.
type AdaptiveHedgeConfig struct {
	// TargetPercentile is the percentile to use for hedge delay (0-1).
	// For example, 0.95 means hedge after P95 latency.
	//
	// Default: 0.95 (P95)
	TargetPercentile float64

	// WindowSize is the number of latency samples to keep per endpoint.
	// Larger windows provide more stable estimates but use more memory.
	//
	// Default: 100
	WindowSize int

	// MinSamples is the minimum number of samples required before
	// adaptive delay calculation kicks in.
	//
	// Default: 10
	MinSamples int

	// FallbackDelay is used when insufficient samples are available.
	//
	// Default: 50ms
	FallbackDelay time.Duration

	// MaxHedges is the maximum number of hedge requests.
	//
	// Default: 1
	MaxHedges int

	// Tracker is the latency tracker to use. If nil, uses DefaultLatencyTracker().
	Tracker *LatencyTracker
}

// DefaultAdaptiveHedgeConfig returns reasonable defaults for adaptive hedging.
func DefaultAdaptiveHedgeConfig() AdaptiveHedgeConfig {
	return AdaptiveHedgeConfig{
		TargetPercentile: 0.95,
		WindowSize:       100,
		MinSamples:       10,
		FallbackDelay:    50 * time.Millisecond,
		MaxHedges:        1,
	}
}

// Enabled returns true if the config is valid for adaptive hedging.
func (c AdaptiveHedgeConfig) Enabled() bool {
	return c.FallbackDelay > 0 && c.MaxHedges > 0
}

// GetTracker returns the configured tracker or the default.
func (c AdaptiveHedgeConfig) GetTracker() *LatencyTracker {
	if c.Tracker != nil {
		return c.Tracker
	}
	return DefaultLatencyTracker()
}

// GetDelay calculates the hedge delay for an endpoint.
// Returns the percentile-based delay if enough samples exist, otherwise FallbackDelay.
func (c AdaptiveHedgeConfig) GetDelay(endpoint string) time.Duration {
	tracker := c.GetTracker()
	if delay, ok := tracker.Percentile(endpoint, c.TargetPercentile); ok {
		return delay
	}
	return c.FallbackDelay
}
