package httpclient

import (
	"time"
)

// RetryConfig holds the retry behavior configuration.
// Use DefaultRetryConfig() for balanced defaults, then modify as needed.
//
// The retry mechanism uses exponential backoff with jitter to prevent
// "thundering herd" problems when multiple clients retry simultaneously.
//
// Key concepts:
//   - MaxRetries: Maximum number of retry attempts (0 = disabled)
//   - MaxElapsedTime: Total time budget for all retries combined.
//     If retrying would exceed this budget, the retry loop stops.
//     Example: With MaxElapsedTime=30s, if 25s have passed, no more retries.
//   - JitterFactor: Randomization factor (0.0-1.0) applied to each interval.
//     A factor of 0.5 means intervals vary ±50% (e.g., 1s becomes 0.5s-1.5s).
//     This prevents synchronized retry storms across distributed clients.
//
// Example usage:
//
//	// Use defaults
//	client := httpclient.New(
//	    httpclient.WithRetryConfig(httpclient.DefaultRetryConfig()),
//	)
//
//	// Custom configuration
//	cfg := httpclient.DefaultRetryConfig()
//	cfg.MaxRetries = 5
//	cfg.InitialInterval = 200 * time.Millisecond
//	client := httpclient.New(
//	    httpclient.WithRetryConfig(cfg),
//	)
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	// Set to 0 to disable retries entirely.
	// The initial request is not counted as a retry.
	// Default: 3
	MaxRetries uint

	// InitialInterval is the first backoff interval before any retries.
	// Subsequent intervals grow exponentially based on Multiplier.
	// Default: 500ms
	InitialInterval time.Duration

	// MaxInterval caps the backoff interval.
	// Even with exponential growth, intervals never exceed this value.
	// Default: 30s
	MaxInterval time.Duration

	// MaxElapsedTime is the total time budget for the entire retry sequence.
	// Once this time has passed since the first attempt, no more retries occur.
	// Set to 0 for no time limit (only MaxRetries applies).
	// Default: 2m
	//
	// Example: If MaxElapsedTime=30s and 25s have passed, even if MaxRetries
	// hasn't been reached, the next retry won't happen if the backoff interval
	// would push total time past 30s.
	MaxElapsedTime time.Duration

	// Multiplier controls exponential growth of backoff intervals.
	// Each retry interval = previous interval × Multiplier.
	// Default: 2.0 (intervals double each retry)
	//
	// Example with InitialInterval=500ms, Multiplier=2.0:
	//   Retry 1: 500ms → Retry 2: 1s → Retry 3: 2s → Retry 4: 4s
	Multiplier float64

	// JitterFactor adds randomization to prevent retry storms.
	// Value between 0.0 (no jitter) and 1.0 (±100% randomization).
	// Default: 0.5 (±50% randomization, recommended)
	//
	// Jitter is critical in distributed systems to prevent synchronized
	// retries from overwhelming recovering services.
	//
	// Example with JitterFactor=0.5 and interval=1s:
	// Actual wait time will be random between 0.5s and 1.5s.
	JitterFactor float64
}

// Default values for RetryConfig.
const (
	// DefaultMaxRetries is the default number of retry attempts.
	DefaultMaxRetries = 3

	// DefaultInitialInterval is the default starting backoff interval.
	DefaultInitialInterval = 500 * time.Millisecond

	// DefaultMaxInterval is the default maximum backoff interval.
	DefaultMaxInterval = 30 * time.Second

	// DefaultMaxElapsedTime is the default total retry time budget.
	DefaultMaxElapsedTime = 2 * time.Minute

	// DefaultMultiplier is the default backoff multiplier.
	DefaultMultiplier = 2.0

	// DefaultJitterFactor is the default randomization factor.
	// 0.5 means ±50% randomization, which is recommended for most use cases.
	DefaultJitterFactor = 0.5
)

// DefaultRetryConfig returns balanced defaults for general use.
//
// Configuration:
//   - 3 retries with exponential backoff (500ms → 1s → 2s)
//   - 2 minute total time budget
//   - 50% jitter for storm prevention
//   - 30s maximum interval cap
//
// This configuration is suitable for most HTTP client use cases where
// you want resilience without being too aggressive.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      DefaultMaxRetries,
		InitialInterval: DefaultInitialInterval,
		MaxInterval:     DefaultMaxInterval,
		MaxElapsedTime:  DefaultMaxElapsedTime,
		Multiplier:      DefaultMultiplier,
		JitterFactor:    DefaultJitterFactor,
	}
}

// AggressiveRetryConfig returns configuration for mission-critical operations.
//
// Configuration:
//   - 5 retries with faster start (200ms → 400ms → 800ms → 1.6s → 3.2s)
//   - 5 minute total time budget
//   - 50% jitter
//   - 60s maximum interval cap
//
// Use this for:
//   - Idempotent operations that must succeed
//   - Critical payment or transaction calls
//   - Operations where failure has high business impact
//
// Warning: More aggressive retries increase load on downstream services.
// Ensure the target service can handle the additional traffic.
func AggressiveRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      5,
		InitialInterval: 200 * time.Millisecond,
		MaxInterval:     60 * time.Second,
		MaxElapsedTime:  5 * time.Minute,
		Multiplier:      2.0,
		JitterFactor:    0.5,
	}
}

// ConservativeRetryConfig returns configuration for expensive or rate-limited services.
//
// Configuration:
//   - 2 retries with slower start (1s → 2s)
//   - 30 second total time budget
//   - 50% jitter
//   - 10s maximum interval cap
//
// Use this for:
//   - Rate-limited APIs (respects service capacity)
//   - Expensive downstream operations (billing APIs, etc.)
//   - Services where you want to fail fast rather than wait
//
// This configuration minimizes additional load on struggling services
// while still providing basic resilience.
func ConservativeRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      2,
		InitialInterval: 1 * time.Second,
		MaxInterval:     10 * time.Second,
		MaxElapsedTime:  30 * time.Second,
		Multiplier:      2.0,
		JitterFactor:    0.5,
	}
}

// NoRetryConfig returns configuration that disables retries entirely.
//
// Use this when:
//   - The operation is not idempotent
//   - You want to handle retries at a higher level
//   - Testing without retry interference
func NoRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      0,
		InitialInterval: 0,
		MaxInterval:     0,
		MaxElapsedTime:  0,
		Multiplier:      0,
		JitterFactor:    -1, // Sentinel to distinguish from uninitialized config
	}
}

// IsEnabled returns true if retries are enabled.
func (c RetryConfig) IsEnabled() bool {
	return c.MaxRetries > 0
}
