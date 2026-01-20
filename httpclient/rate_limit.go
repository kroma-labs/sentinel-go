package httpclient

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig configures client-level rate limiting.
type RateLimitConfig struct {
	// RequestsPerSecond is the maximum sustained request rate.
	RequestsPerSecond float64

	// Burst is the maximum number of requests allowed in a burst.
	// This allows brief spikes above the rate limit.
	Burst int

	// WaitOnLimit determines behavior when rate limit is hit.
	// If true, requests wait for a token (respecting context deadline).
	// If false, requests immediately return ErrRateLimited.
	WaitOnLimit bool
}

// DefaultRateLimitConfig returns a sensible default rate limit configuration.
// 100 requests per second with a burst of 10.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 100,
		Burst:             10,
		WaitOnLimit:       true,
	}
}

// ErrRateLimited is returned when a request is rejected due to rate limiting.
var ErrRateLimited = errors.New("rate limit exceeded")

// rateLimitTransport implements http.RoundTripper with rate limiting.
type rateLimitTransport struct {
	next    http.RoundTripper
	limiter *rate.Limiter
	wait    bool
}

// newRateLimitTransport creates a rate-limited transport wrapper.
func newRateLimitTransport(next http.RoundTripper, cfg RateLimitConfig) http.RoundTripper {
	if cfg.RequestsPerSecond <= 0 {
		return next // No rate limiting
	}

	limiter := rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst)

	return &rateLimitTransport{
		next:    next,
		limiter: limiter,
		wait:    cfg.WaitOnLimit,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	if t.wait {
		// Wait for token, respecting context deadline
		if err := t.limiter.Wait(ctx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, err
			}
			return nil, ErrRateLimited
		}
	} else {
		// Fail fast if no token available
		if !t.limiter.Allow() {
			return nil, ErrRateLimited
		}
	}

	return t.next.RoundTrip(req)
}

// requestRateLimiter manages per-endpoint rate limiters.
type requestRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
}

var globalRequestLimiter = &requestRateLimiter{
	limiters: make(map[string]*rate.Limiter),
}

// getOrCreate returns a rate limiter for the given key, creating one if needed.
func (r *requestRateLimiter) getOrCreate(key string, rps float64, burst int) *rate.Limiter {
	r.mu.RLock()
	if limiter, ok := r.limiters[key]; ok {
		r.mu.RUnlock()
		return limiter
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, ok := r.limiters[key]; ok {
		return limiter
	}

	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	r.limiters[key] = limiter
	return limiter
}

// RequestRateLimitConfig configures per-request rate limiting.
type RequestRateLimitConfig struct {
	// RequestsPerSecond for this specific endpoint/operation.
	RequestsPerSecond float64

	// Burst allows brief spikes above the rate limit.
	Burst int

	// WaitOnLimit determines behavior when rate limit is hit.
	WaitOnLimit bool
}

// applyRequestRateLimit checks and applies per-request rate limiting.
// Returns an error if rate limit is exceeded and WaitOnLimit is false.
func applyRequestRateLimit(ctx context.Context, key string, cfg RequestRateLimitConfig) error {
	if cfg.RequestsPerSecond <= 0 {
		return nil // No rate limiting
	}

	burst := cfg.Burst
	if burst <= 0 {
		burst = 1 // Minimum burst of 1
	}

	limiter := globalRequestLimiter.getOrCreate(key, cfg.RequestsPerSecond, burst)

	if cfg.WaitOnLimit {
		return limiter.Wait(ctx)
	}

	if !limiter.Allow() {
		return ErrRateLimited
	}
	return nil
}

// RateLimitBehavior specifies how to handle rate limit exceeded.
type RateLimitBehavior int

const (
	// RateLimitWait waits for a token to become available (default).
	RateLimitWait RateLimitBehavior = iota
	// RateLimitFailFast immediately returns ErrRateLimited.
	RateLimitFailFast
)

// NewRateLimitConfigWithBehavior creates a rate limit config with specified behavior.
func NewRateLimitConfigWithBehavior(
	rps float64,
	burst int,
	behavior RateLimitBehavior,
) RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: rps,
		Burst:             burst,
		WaitOnLimit:       behavior == RateLimitWait,
	}
}

// RateLimiterStats provides visibility into rate limiter state.
type RateLimiterStats struct {
	// Limit is the maximum rate per second.
	Limit float64
	// Burst is the maximum burst size.
	Burst int
	// TokensAvailable is the current number of tokens.
	TokensAvailable float64
}

// GetRateLimiterStats returns stats for the client's rate limiter.
func (t *rateLimitTransport) GetStats() RateLimiterStats {
	return RateLimiterStats{
		Limit:           float64(t.limiter.Limit()),
		Burst:           t.limiter.Burst(),
		TokensAvailable: t.limiter.Tokens(),
	}
}

// ReserveN attempts to reserve n tokens without blocking.
// Returns the duration to wait before the reservation is valid.
func (t *rateLimitTransport) ReserveN(n int) time.Duration {
	r := t.limiter.ReserveN(time.Now(), n)
	if !r.OK() {
		return -1 // Cannot satisfy request
	}
	return r.Delay()
}
