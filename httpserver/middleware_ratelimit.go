package httpserver

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Limiter is an interface for rate limiters (useful for testing).
type Limiter interface {
	Limit(ctx context.Context) (time.Duration, error)
}

// RateLimitConfig configures the rate limiting middleware.
type RateLimitConfig struct {
	// Limit is the rate limit in requests per second.
	Limit rate.Limit

	// Burst is the maximum burst size (token bucket capacity).
	Burst int

	// KeyFunc extracts a key from the request for per-key rate limiting.
	// If nil, a global rate limit is applied to all requests.
	KeyFunc KeyFunc

	// Redis enables distributed rate limiting across multiple instances.
	// If nil, an in-memory rate limiter is used (single-instance only).
	Redis redis.UniversalClient

	// RedisKeyPrefix is the prefix for Redis keys.
	// Default: "ratelimit:"
	RedisKeyPrefix string

	// WindowDuration is the sliding window duration for Redis rate limiting.
	// Default: 1 second
	WindowDuration time.Duration
}

// DefaultRateLimitConfig returns a default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Limit:          100,
		Burst:          200,
		RedisKeyPrefix: "ratelimit:",
		WindowDuration: time.Second,
	}
}

// RateLimit returns middleware that limits request rate using token bucket algorithm.
func RateLimit(cfg RateLimitConfig) Middleware {
	if cfg.RedisKeyPrefix == "" {
		cfg.RedisKeyPrefix = "ratelimit:"
	}
	if cfg.WindowDuration == 0 {
		cfg.WindowDuration = time.Second
	}

	if cfg.Redis != nil {
		return redisRateLimiter(cfg)
	}

	return memoryRateLimiter(cfg)
}

// memoryRateLimiter creates an in-memory token bucket rate limiter.
func memoryRateLimiter(cfg RateLimitConfig) Middleware {
	if cfg.KeyFunc == nil {
		limiter := rate.NewLimiter(cfg.Limit, cfg.Burst)
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !limiter.Allow() {
					WriteError(w, http.StatusTooManyRequests, "rate limit exceeded",
						Error{Field: "rate_limit", Message: "too many requests"})
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	var mu sync.RWMutex
	limiters := make(map[string]*rate.Limiter)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.KeyFunc(r)

			mu.RLock()
			limiter, exists := limiters[key]
			mu.RUnlock()

			if !exists {
				mu.Lock()
				limiter, exists = limiters[key]
				if !exists {
					limiter = rate.NewLimiter(cfg.Limit, cfg.Burst)
					limiters[key] = limiter
				}
				mu.Unlock()
			}

			if !limiter.Allow() {
				WriteError(w, http.StatusTooManyRequests, "rate limit exceeded",
					Error{Field: "rate_limit", Message: "too many requests"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tokenBucketScript is a Lua script for atomic token bucket rate limiting in Redis.
// It implements a proper token bucket algorithm:
// - Stores: tokens (remaining), last_update (timestamp in milliseconds)
// - Calculates tokens to add based on elapsed time
// - Caps tokens at burst capacity
// - Atomically checks and decrements tokens
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])       -- tokens per second
local burst = tonumber(ARGV[2])      -- max tokens (capacity)
local now = tonumber(ARGV[3])        -- current time in milliseconds
local ttl = tonumber(ARGV[4])        -- key TTL in seconds

-- Get current state
local data = redis.call('HMGET', key, 'tokens', 'last_update')
local tokens = tonumber(data[1])
local last_update = tonumber(data[2])

-- Initialize if first request
if tokens == nil then
    tokens = burst
    last_update = now
end

-- Calculate tokens to add based on elapsed time
local elapsed_ms = now - last_update
local tokens_to_add = (elapsed_ms / 1000.0) * rate
tokens = math.min(burst, tokens + tokens_to_add)

-- Try to consume one token
if tokens >= 1 then
    tokens = tokens - 1
    redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
    redis.call('EXPIRE', key, ttl)
    return 1  -- allowed
else
    -- Update timestamp even if denied (for accurate rate calculation)
    redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
    redis.call('EXPIRE', key, ttl)
    return 0  -- denied
end
`)

// redisRateLimiter creates a distributed rate limiter using Redis with proper token bucket.
func redisRateLimiter(cfg RateLimitConfig) Middleware {
	rps := float64(cfg.Limit) // requests per second
	ttl := 60                 // key TTL in seconds (cleanup inactive keys)

	if cfg.KeyFunc == nil {
		// Global limiter
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()
				key := cfg.RedisKeyPrefix + "global"
				now := time.Now().UnixMilli()

				allowed, err := tokenBucketScript.Run(ctx, cfg.Redis, []string{key}, rps, cfg.Burst, now, ttl).
					Int()
				if err != nil {
					// On error, fail open
					next.ServeHTTP(w, r)
					return
				}

				if allowed == 0 {
					WriteError(w, http.StatusTooManyRequests, "rate limit exceeded",
						Error{Field: "rate_limit", Message: "too many requests"})
					return
				}

				next.ServeHTTP(w, r)
			})
		}
	}

	// Per-key limiter
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			key := cfg.RedisKeyPrefix + cfg.KeyFunc(r)
			now := time.Now().UnixMilli()

			allowed, err := tokenBucketScript.Run(ctx, cfg.Redis, []string{key}, rps, cfg.Burst, now, ttl).
				Int()
			if err != nil {
				// On error, fail open
				next.ServeHTTP(w, r)
				return
			}

			if allowed == 0 {
				WriteError(w, http.StatusTooManyRequests, "rate limit exceeded",
					Error{Field: "rate_limit", Message: "too many requests"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitByIP returns rate limiting middleware keyed by client IP.
func RateLimitByIP(limit rate.Limit, burst int) Middleware {
	return RateLimit(RateLimitConfig{
		Limit:   limit,
		Burst:   burst,
		KeyFunc: KeyFuncByIP(),
	})
}

// RateLimitByIPRedis returns distributed rate limiting middleware keyed by client IP.
func RateLimitByIPRedis(rdb redis.UniversalClient, limit rate.Limit, burst int) Middleware {
	return RateLimit(RateLimitConfig{
		Limit:   limit,
		Burst:   burst,
		Redis:   rdb,
		KeyFunc: KeyFuncByIP(),
	})
}
