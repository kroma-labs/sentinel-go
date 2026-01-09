package httpclient

import (
	"math/rand/v2"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// Ensure our backoff strategies implement the backoff.BackOff interface.
var (
	_ backoff.BackOff = (*LinearBackOff)(nil)
	_ backoff.BackOff = (*DecorrelatedJitterBackOff)(nil)
	_ backoff.BackOff = (*ConstantBackOffWithJitter)(nil)
	_ backoff.BackOff = (*TieredRetryBackOff)(nil)
)

// LinearBackOff increases interval by a fixed increment plus jitter.
// Use when you want predictable growth without exponential explosion.
//
// Interval calculation: base + (attempt × increment) ± jitter
//
// Example with Initial=1s, Increment=500ms, JitterFactor=0.3:
//
//	Attempt 1: 1.0s ± 0.3s = [0.7s, 1.3s]
//	Attempt 2: 1.5s ± 0.45s = [1.05s, 1.95s]
//	Attempt 3: 2.0s ± 0.6s = [1.4s, 2.6s]
//
// This provides more gradual backoff than exponential, which is useful
// when you want to give services more time to recover without waiting
// extremely long periods.
type LinearBackOff struct {
	// InitialInterval is the first backoff interval.
	// Default: 500ms
	InitialInterval time.Duration

	// Increment is the fixed amount added to each subsequent interval.
	// Default: 500ms
	Increment time.Duration

	// MaxInterval caps the backoff interval.
	// Default: 30s
	MaxInterval time.Duration

	// JitterFactor adds randomization (0.0-1.0).
	// Default: 0.5 (±50% randomization)
	// A small jitter is always recommended to prevent retry storms.
	JitterFactor float64

	// currentInterval tracks the current base interval before jitter.
	currentInterval time.Duration
	// attempt tracks the current attempt number.
	attempt int
}

// NewLinearBackOff creates a LinearBackOff with sensible defaults.
//
// Defaults:
//   - InitialInterval: 500ms
//   - Increment: 500ms
//   - MaxInterval: 30s
//   - JitterFactor: 0.5
func NewLinearBackOff() *LinearBackOff {
	return &LinearBackOff{
		InitialInterval: 500 * time.Millisecond,
		Increment:       500 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		JitterFactor:    0.5,
	}
}

// Reset resets the backoff to initial state.
func (b *LinearBackOff) Reset() {
	b.currentInterval = b.InitialInterval
	b.attempt = 0
}

// NextBackOff returns the next backoff interval with jitter applied.
func (b *LinearBackOff) NextBackOff() time.Duration {
	if b.currentInterval == 0 {
		b.currentInterval = b.InitialInterval
	}

	// Calculate interval with jitter
	interval := applyJitter(b.currentInterval, b.JitterFactor)

	// Increment for next call
	b.attempt++
	b.currentInterval = b.InitialInterval + time.Duration(b.attempt)*b.Increment

	// Cap at MaxInterval
	if b.currentInterval > b.MaxInterval {
		b.currentInterval = b.MaxInterval
	}

	return interval
}

// DecorrelatedJitterBackOff uses AWS-style decorrelated jitter.
// Each interval is random between base and previous×3, providing
// better distribution than standard jitter in high-contention scenarios.
//
// Formula: sleep = random_between(base, min(cap, previous_sleep × 3))
//
// This algorithm was introduced by AWS and is particularly effective
// when many clients are retrying simultaneously, as it spreads retries
// more evenly across time.
//
// See: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
type DecorrelatedJitterBackOff struct {
	// Base is the minimum backoff interval.
	// Default: 500ms
	Base time.Duration

	// Cap is the maximum backoff interval.
	// Default: 30s
	Cap time.Duration

	// sleep is the previous sleep duration (internal state).
	sleep time.Duration
}

// NewDecorrelatedJitterBackOff creates a DecorrelatedJitterBackOff with defaults.
//
// Defaults:
//   - Base: 500ms
//   - Cap: 30s
func NewDecorrelatedJitterBackOff() *DecorrelatedJitterBackOff {
	return &DecorrelatedJitterBackOff{
		Base: 500 * time.Millisecond,
		Cap:  30 * time.Second,
	}
}

// Reset resets the backoff to initial state.
func (b *DecorrelatedJitterBackOff) Reset() {
	b.sleep = b.Base
}

// NextBackOff returns the next backoff interval using decorrelated jitter.
func (b *DecorrelatedJitterBackOff) NextBackOff() time.Duration {
	if b.sleep == 0 {
		b.sleep = b.Base
	}

	// Calculate upper bound: min(cap, sleep * 3)
	upperBound := b.sleep * 3
	if upperBound > b.Cap {
		upperBound = b.Cap
	}

	// Random value between base and upperBound
	// sleep = random_between(base, upperBound)
	b.sleep = randomBetween(b.Base, upperBound)

	return b.sleep
}

// ConstantBackOffWithJitter provides a fixed interval with randomization.
// Use when you want consistent wait times but still need jitter for
// storm prevention.
//
// Example with Interval=1s, JitterFactor=0.25:
// Each wait will be random between 0.75s and 1.25s.
type ConstantBackOffWithJitter struct {
	// Interval is the base backoff interval.
	// Default: 1s
	Interval time.Duration

	// JitterFactor adds randomization (0.0-1.0).
	// Default: 0.5 (±50% randomization)
	JitterFactor float64
}

// NewConstantBackOffWithJitter creates a ConstantBackOffWithJitter with defaults.
//
// Defaults:
//   - Interval: 1s
//   - JitterFactor: 0.5
func NewConstantBackOffWithJitter() *ConstantBackOffWithJitter {
	return &ConstantBackOffWithJitter{
		Interval:     1 * time.Second,
		JitterFactor: 0.5,
	}
}

// Reset is a no-op for constant backoff.
func (b *ConstantBackOffWithJitter) Reset() {
	// No state to reset
}

// NextBackOff returns the interval with jitter applied.
func (b *ConstantBackOffWithJitter) NextBackOff() time.Duration {
	return applyJitter(b.Interval, b.JitterFactor)
}

// applyJitter applies randomization to an interval.
// JitterFactor of 0.5 means the result will be in range [interval*0.5, interval*1.5].
func applyJitter(interval time.Duration, jitterFactor float64) time.Duration {
	if jitterFactor <= 0 {
		return interval
	}

	// Clamp jitter factor to [0, 1]
	if jitterFactor > 1 {
		jitterFactor = 1
	}

	delta := float64(interval) * jitterFactor
	minInterval := float64(interval) - delta
	maxInterval := float64(interval) + delta

	// Random value in [minInterval, maxInterval]
	//nolint:gosec // intentional weak rand for jitter (not cryptographic)
	return time.Duration(
		minInterval + rand.Float64()*(maxInterval-minInterval),
	)
}

// randomBetween returns a random duration between minDur and maxDur (inclusive).
//
//nolint:gosec // intentional weak rand for jitter (not cryptographic)
func randomBetween(minDur, maxDur time.Duration) time.Duration {
	if minDur >= maxDur {
		return minDur
	}
	return minDur + time.Duration(
		rand.Int64N(int64(maxDur-minDur)),
	)
}

// ExponentialBackOffFromConfig creates a cenkalti/backoff ExponentialBackOff
// from a RetryConfig, ensuring jitter is always applied.
func ExponentialBackOffFromConfig(cfg RetryConfig) *backoff.ExponentialBackOff {
	// Ensure minimum jitter factor for storm prevention
	jitterFactor := cfg.JitterFactor
	if jitterFactor <= 0 {
		jitterFactor = DefaultJitterFactor // Always apply some jitter
	}

	return &backoff.ExponentialBackOff{
		InitialInterval:     cfg.InitialInterval,
		RandomizationFactor: jitterFactor,
		Multiplier:          cfg.Multiplier,
		MaxInterval:         cfg.MaxInterval,
	}
}

// RetryTier defines a single tier in a tiered retry strategy.
// Each tier has a fixed delay and maximum number of retries before
// moving to the next tier.
type RetryTier struct {
	// MaxRetries is the maximum number of retries in this tier
	// before moving to the next tier.
	MaxRetries int

	// Delay is the fixed delay duration for this tier.
	// Jitter will be applied to this value.
	Delay time.Duration
}

// TieredRetryBackOff implements a tiered retry strategy.
// It progresses through configurable tiers of fixed delays,
// then falls back to exponential backoff for the final tier.
//
// This is useful for scenarios where you want:
//   - Fast retries initially (e.g., 1 min intervals)
//   - Slower retries as the problem persists (e.g., 2 min intervals)
//   - Exponential backoff as a last resort
//
// Example configuration:
//
//	tiers := []httpclient.RetryTier{
//	    {MaxRetries: 5, Delay: 1 * time.Minute},  // Tier 1: 5 retries at 1 min
//	    {MaxRetries: 5, Delay: 2 * time.Minute},  // Tier 2: 5 retries at 2 min
//	}
//	backoff := httpclient.NewTieredRetryBackOff(tiers, 10*time.Minute, 0.5)
//
// Behavior:
//   - Attempts 1-5: ~1 min delay (±50% jitter)
//   - Attempts 6-10: ~2 min delay (±50% jitter)
//   - Attempts 11+: Exponential backoff starting at 2^(attempt-10) min,
//     capped at MaxDelay (10 min)
type TieredRetryBackOff struct {
	// Tiers defines the fixed-delay tiers.
	// Each tier specifies max retries and delay before moving to next tier.
	Tiers []RetryTier

	// MaxDelay is the maximum delay for the exponential backoff tier.
	// This caps the final tier's delay to prevent excessive wait times.
	MaxDelay time.Duration

	// JitterFactor adds randomization (0.0-1.0) to all delays.
	// Default: 0.5 (±50% randomization)
	JitterFactor float64

	// attempt tracks the current attempt number (1-indexed).
	attempt int

	// totalFixedRetries caches the sum of all tier retries.
	totalFixedRetries int
}

// NewTieredRetryBackOff creates a TieredRetryBackOff with the specified tiers.
//
// Parameters:
//   - tiers: List of retry tiers (fixed delay phases)
//   - maxDelay: Maximum delay for exponential backoff tier
//   - jitterFactor: Randomization factor (0.0-1.0), use 0.5 for ±50%
//
// Example:
//
//	backoff := httpclient.NewTieredRetryBackOff(
//	    []httpclient.RetryTier{
//	        {MaxRetries: 5, Delay: 1 * time.Minute},
//	        {MaxRetries: 5, Delay: 2 * time.Minute},
//	    },
//	    10 * time.Minute, // Max delay for exponential tier
//	    0.5,              // ±50% jitter
//	)
func NewTieredRetryBackOff(
	tiers []RetryTier,
	maxDelay time.Duration,
	jitterFactor float64,
) *TieredRetryBackOff {
	// Calculate total fixed retries
	total := 0
	for _, tier := range tiers {
		total += tier.MaxRetries
	}

	if jitterFactor <= 0 {
		jitterFactor = DefaultJitterFactor
	}

	return &TieredRetryBackOff{
		Tiers:             tiers,
		MaxDelay:          maxDelay,
		JitterFactor:      jitterFactor,
		totalFixedRetries: total,
	}
}

// DefaultTieredRetryBackOff returns a commonly-used tiered retry configuration.
//
// Configuration:
//   - Tier 1: 5 retries at 1 minute intervals
//   - Tier 2: 5 retries at 2 minute intervals
//   - Final tier: Exponential backoff up to 10 minutes
//   - Jitter: ±50%
func DefaultTieredRetryBackOff() *TieredRetryBackOff {
	return NewTieredRetryBackOff(
		[]RetryTier{
			{MaxRetries: 5, Delay: 1 * time.Minute},
			{MaxRetries: 5, Delay: 2 * time.Minute},
		},
		10*time.Minute,
		0.5,
	)
}

// Reset resets the backoff to initial state.
func (b *TieredRetryBackOff) Reset() {
	b.attempt = 0
}

// NextBackOff returns the next backoff interval based on the current tier.
func (b *TieredRetryBackOff) NextBackOff() time.Duration {
	b.attempt++

	// Determine which tier we're in
	delay := b.calculateDelay()

	// Apply jitter
	return applyJitter(delay, b.JitterFactor)
}

// calculateDelay returns the base delay for the current attempt.
func (b *TieredRetryBackOff) calculateDelay() time.Duration {
	// Check fixed tiers
	attemptInTiers := b.attempt
	for _, tier := range b.Tiers {
		if attemptInTiers <= tier.MaxRetries {
			return tier.Delay
		}
		attemptInTiers -= tier.MaxRetries
	}

	// We're in the exponential backoff tier
	// Calculate: 2^(attempt - totalFixedRetries) minutes
	exponentialAttempt := b.attempt - b.totalFixedRetries
	if exponentialAttempt < 1 {
		exponentialAttempt = 1
	}

	// Calculate exponential delay: 2^(exponentialAttempt-1) * base
	// We use 1 minute as the base for exponential calculation
	baseDelay := time.Minute
	delay := baseDelay << (exponentialAttempt - 1) // 2^(n-1) * base

	// Cap at maxDelay
	if delay > b.MaxDelay {
		delay = b.MaxDelay
	}

	return delay
}

// CurrentTier returns the current tier number (1-indexed).
// Returns len(Tiers)+1 for the exponential backoff tier.
func (b *TieredRetryBackOff) CurrentTier() int {
	attemptInTiers := b.attempt
	for i, tier := range b.Tiers {
		if attemptInTiers <= tier.MaxRetries {
			return i + 1
		}
		attemptInTiers -= tier.MaxRetries
	}
	return len(b.Tiers) + 1 // Exponential tier
}
