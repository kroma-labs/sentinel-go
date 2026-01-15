package httpclient

import "time"

// HedgeConfig configures hedged requests for tail latency optimization.
//
// Hedged requests reduce tail latency by sending a duplicate request if the
// original request hasn't completed within a specified delay. The first
// response received is used, and any remaining requests are cancelled.
//
// This technique is based on Google's "The Tail at Scale" paper, which
// demonstrated that hedging can dramatically reduce 99th percentile latency
// with minimal overhead (typically 1-5% extra requests).
//
// IMPORTANT: Hedged requests should only be used for idempotent operations.
// Non-idempotent operations (like POST creating a resource) may result in
// duplicate side effects.
//
// Example usage:
//
//	client := httpclient.New(
//	    httpclient.WithHedging(httpclient.HedgeConfig{
//	        Delay:     50 * time.Millisecond,  // Hedge after 50ms
//	        MaxHedges: 1,                       // Send 1 hedge request
//	    }),
//	)
//
// Best practices:
//   - Set Delay to the P95 or P99 latency of your target service
//   - Use MaxHedges of 1-2 to limit overhead
//   - Only use for read operations or idempotent writes
//   - Monitor hedge success rate to tune Delay
type HedgeConfig struct {
	// Delay is how long to wait before sending a hedge request.
	//
	// If the original request hasn't completed within this duration,
	// a duplicate request (hedge) is sent.
	//
	// Recommended: Set to P95 latency of your target service.
	// Too short: Excessive hedging wastes resources.
	// Too long: Hedging won't help with tail latency.
	//
	// Default: 0 (disabled - no hedging)
	Delay time.Duration

	// MaxHedges is the maximum number of hedge requests to send.
	//
	// With MaxHedges=1, at most 2 requests are in flight (original + 1 hedge).
	// Higher values provide more resilience but increase server load.
	//
	// Recommended: 1 for most use cases, 2 for critical paths.
	//
	// Default: 0 (disabled - no hedging)
	MaxHedges int
}

// Enabled returns true if hedging is configured.
func (c HedgeConfig) Enabled() bool {
	return c.Delay > 0 && c.MaxHedges > 0
}
