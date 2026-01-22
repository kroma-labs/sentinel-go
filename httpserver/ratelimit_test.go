package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit_InMemory(t *testing.T) {
	t.Parallel()

	t.Run("given global rate limit, when burst exhausted, then returns 429", func(t *testing.T) {
		handler := okHandler()
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 1,
			Burst: 3,
		})(handler)

		// First 3 requests should succeed (burst = 3)
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
		}

		// 4th request should be rate limited
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, "4th request should be limited")
	})

	t.Run(
		"given per-key rate limit, when different keys, then separate buckets",
		func(t *testing.T) {
			handler := okHandler()
			chain := httpserver.RateLimit(httpserver.RateLimitConfig{
				Limit: 1,
				Burst: 1,
				KeyFunc: func(r *http.Request) string {
					return r.Header.Get("X-User-ID")
				},
			})(handler)

			// User A first request: OK
			req1 := httptest.NewRequest(http.MethodGet, "/", nil)
			req1.Header.Set("X-User-ID", "user-a")
			rec1 := httptest.NewRecorder()
			chain.ServeHTTP(rec1, req1)
			assert.Equal(t, http.StatusOK, rec1.Code)

			// User A second request: rate limited
			req2 := httptest.NewRequest(http.MethodGet, "/", nil)
			req2.Header.Set("X-User-ID", "user-a")
			rec2 := httptest.NewRecorder()
			chain.ServeHTTP(rec2, req2)
			assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

			// User B first request: OK (separate bucket)
			req3 := httptest.NewRequest(http.MethodGet, "/", nil)
			req3.Header.Set("X-User-ID", "user-b")
			rec3 := httptest.NewRecorder()
			chain.ServeHTTP(rec3, req3)
			assert.Equal(t, http.StatusOK, rec3.Code)
		},
	)

	t.Run(
		"given per-path rate limit, when different paths, then separate buckets",
		func(t *testing.T) {
			handler := okHandler()
			chain := httpserver.RateLimit(httpserver.RateLimitConfig{
				Limit:   1,
				Burst:   1,
				KeyFunc: httpserver.KeyFuncByPath(),
			})(handler)

			// /api/users first request: OK
			req1 := httptest.NewRequest(http.MethodGet, "/api/users", nil)
			rec1 := httptest.NewRecorder()
			chain.ServeHTTP(rec1, req1)
			assert.Equal(t, http.StatusOK, rec1.Code)

			// /api/users second request: rate limited
			req2 := httptest.NewRequest(http.MethodGet, "/api/users", nil)
			rec2 := httptest.NewRecorder()
			chain.ServeHTTP(rec2, req2)
			assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

			// /api/orders first request: OK (different path = different bucket)
			req3 := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
			rec3 := httptest.NewRecorder()
			chain.ServeHTTP(rec3, req3)
			assert.Equal(t, http.StatusOK, rec3.Code)
		},
	)

	t.Run("given token bucket, when time passes, then tokens refill", func(t *testing.T) {
		handler := okHandler()
		// Rate: 10 per second, Burst: 2
		// This means tokens refill at 10/sec = 1 token per 100ms
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 10,
			Burst: 2,
		})(handler)

		// Exhaust burst
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}

		// Should be rate limited now
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)

		// Wait for token to refill (100ms for 1 token at 10/sec)
		time.Sleep(150 * time.Millisecond)

		// Should be allowed again
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		rec = httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestRateLimit_Redis(t *testing.T) {
	t.Parallel()

	t.Run("given redis rate limit, when burst exhausted, then returns 429", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer rdb.Close()

		handler := okHandler()
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 10,
			Burst: 3,
			Redis: rdb,
		})(handler)

		// First 3 requests should succeed (burst = 3)
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
		}

		// 4th request should be rate limited
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, "4th request should be limited")
	})

	t.Run(
		"given redis per-key rate limit, when different keys, then separate buckets",
		func(t *testing.T) {
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer rdb.Close()

			handler := okHandler()
			chain := httpserver.RateLimit(httpserver.RateLimitConfig{
				Limit: 10,
				Burst: 1,
				Redis: rdb,
				KeyFunc: func(r *http.Request) string {
					return r.Header.Get("X-Client-ID")
				},
			})(handler)

			// Client A first request: OK
			req1 := httptest.NewRequest(http.MethodGet, "/", nil)
			req1.Header.Set("X-Client-ID", "client-a")
			rec1 := httptest.NewRecorder()
			chain.ServeHTTP(rec1, req1)
			assert.Equal(t, http.StatusOK, rec1.Code)

			// Client A second request: rate limited
			req2 := httptest.NewRequest(http.MethodGet, "/", nil)
			req2.Header.Set("X-Client-ID", "client-a")
			rec2 := httptest.NewRecorder()
			chain.ServeHTTP(rec2, req2)
			assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

			// Client B first request: OK (separate bucket)
			req3 := httptest.NewRequest(http.MethodGet, "/", nil)
			req3.Header.Set("X-Client-ID", "client-b")
			rec3 := httptest.NewRecorder()
			chain.ServeHTTP(rec3, req3)
			assert.Equal(t, http.StatusOK, rec3.Code)
		},
	)

	t.Run("given redis token bucket, when time passes, then tokens refill", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer rdb.Close()

		handler := okHandler()
		// Rate: 50 per second = 1 token per 20ms, Burst: 2
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 50,
			Burst: 2,
			Redis: rdb,
		})(handler)

		// Exhaust burst
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}

		// Should be rate limited now
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)

		// Wait for token to refill (use real time.Sleep since Lua uses time.Now())
		time.Sleep(30 * time.Millisecond)

		// Should be allowed again
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		rec = httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("given redis failure, then fails open (allows request)", func(t *testing.T) {
		// Create a client pointing to non-existent Redis
		rdb := redis.NewClient(&redis.Options{Addr: "localhost:59999"})
		defer rdb.Close()

		handler := okHandler()
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 10,
			Burst: 1,
			Redis: rdb,
		})(handler)

		// Should fail open (allow request) when Redis is unavailable
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "should fail open when Redis is unavailable")
	})
}

func TestRateLimitByIP(t *testing.T) {
	t.Parallel()

	t.Run("given RateLimitByIP, when same IP exceeds limit, then returns 429", func(t *testing.T) {
		handler := okHandler()
		chain := httpserver.RateLimitByIP(1, 1)(handler)

		// First request: OK
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		req1.RemoteAddr = "10.0.0.1:12345"
		rec1 := httptest.NewRecorder()
		chain.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// Second request from same IP: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.RemoteAddr = "10.0.0.1:12345"
		rec2 := httptest.NewRecorder()
		chain.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

		// Request from different IP: OK
		req3 := httptest.NewRequest(http.MethodGet, "/", nil)
		req3.RemoteAddr = "10.0.0.2:12345"
		rec3 := httptest.NewRecorder()
		chain.ServeHTTP(rec3, req3)
		assert.Equal(t, http.StatusOK, rec3.Code)
	})

	t.Run("given X-Forwarded-For header, then uses forwarded IP", func(t *testing.T) {
		handler := okHandler()
		chain := httpserver.RateLimitByIP(1, 1)(handler)

		// First request with X-Forwarded-For: OK
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		req1.RemoteAddr = "proxy:8080"
		req1.Header.Set("X-Forwarded-For", "client-ip-1")
		rec1 := httptest.NewRecorder()
		chain.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// Second request with same X-Forwarded-For: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.RemoteAddr = "proxy:8080"
		req2.Header.Set("X-Forwarded-For", "client-ip-1")
		rec2 := httptest.NewRecorder()
		chain.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

		// Request with different X-Forwarded-For: OK
		req3 := httptest.NewRequest(http.MethodGet, "/", nil)
		req3.RemoteAddr = "proxy:8080"
		req3.Header.Set("X-Forwarded-For", "client-ip-2")
		rec3 := httptest.NewRecorder()
		chain.ServeHTTP(rec3, req3)
		assert.Equal(t, http.StatusOK, rec3.Code)
	})
}

func TestRateLimitByIPRedis(t *testing.T) {
	t.Parallel()

	t.Run(
		"given RateLimitByIPRedis, when same IP exceeds limit, then returns 429",
		func(t *testing.T) {
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer rdb.Close()

			handler := okHandler()
			chain := httpserver.RateLimitByIPRedis(rdb, 1, 1)(handler)

			// First request: OK
			req1 := httptest.NewRequest(http.MethodGet, "/", nil)
			req1.RemoteAddr = "10.0.0.1:12345"
			rec1 := httptest.NewRecorder()
			chain.ServeHTTP(rec1, req1)
			assert.Equal(t, http.StatusOK, rec1.Code)

			// Second request from same IP: rate limited
			req2 := httptest.NewRequest(http.MethodGet, "/", nil)
			req2.RemoteAddr = "10.0.0.1:12345"
			rec2 := httptest.NewRecorder()
			chain.ServeHTTP(rec2, req2)
			assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
		},
	)
}

func TestKeyFuncs(t *testing.T) {
	t.Parallel()

	t.Run("KeyFuncByIP returns RemoteAddr when no X-Forwarded-For", func(t *testing.T) {
		keyFunc := httpserver.KeyFuncByIP()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:1234"

		key := keyFunc(req)
		assert.Equal(t, "192.168.1.1:1234", key)
	})

	t.Run("KeyFuncByIP returns X-Forwarded-For when present", func(t *testing.T) {
		keyFunc := httpserver.KeyFuncByIP()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "proxy:8080"
		req.Header.Set("X-Forwarded-For", "client-real-ip")

		key := keyFunc(req)
		assert.Equal(t, "client-real-ip", key)
	})

	t.Run("KeyFuncByPath returns URL path", func(t *testing.T) {
		keyFunc := httpserver.KeyFuncByPath()
		req := httptest.NewRequest(http.MethodGet, "/api/users/123", nil)

		key := keyFunc(req)
		assert.Equal(t, "/api/users/123", key)
	})

	t.Run("KeyFuncByIPAndPath returns combined key", func(t *testing.T) {
		keyFunc := httpserver.KeyFuncByIPAndPath()
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		req.RemoteAddr = "192.168.1.1:1234"

		key := keyFunc(req)
		assert.Equal(t, "192.168.1.1:1234:/api/users", key)
	})

	t.Run("KeyFuncByHeader returns header value", func(t *testing.T) {
		keyFunc := httpserver.KeyFuncByHeader("X-Tenant-ID")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant-ID", "tenant-123")

		key := keyFunc(req)
		assert.Equal(t, "tenant-123", key)
	})
}

func TestTokenBucketAlgorithmProperties(t *testing.T) {
	t.Parallel()

	t.Run("given burst=5, then allows exactly 5 requests initially", func(t *testing.T) {
		handler := okHandler()
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 1,
			Burst: 5,
		})(handler)

		allowedCount := 0
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				allowedCount++
			}
		}

		assert.Equal(t, 5, allowedCount, "should allow exactly 5 requests (burst capacity)")
	})

	t.Run("given rate=100/sec burst=10, then refills at correct rate", func(t *testing.T) {
		handler := okHandler()
		// Rate: 100 per second = 1 token per 10ms
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 100,
			Burst: 10,
		})(handler)

		// Exhaust burst
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
		}

		// Should be rate limited
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)

		// Wait for 1 token to refill (10ms at 100/sec)
		time.Sleep(15 * time.Millisecond)

		// Should be allowed (1 token available)
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		rec = httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Should be rate limited again (just used the token)
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		rec = httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	})

	t.Run("given tokens refill, then capped at burst capacity", func(t *testing.T) {
		handler := okHandler()
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 1000, // very high rate
			Burst: 3,    // low burst
		})(handler)

		// Use 2 tokens, leaving 1
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}

		// Wait for tokens to refill (should be capped at burst=3)
		time.Sleep(50 * time.Millisecond)

		// Should allow exactly 3 requests (burst capacity, even if more time passed)
		allowedCount := 0
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				allowedCount++
			}
		}

		assert.Equal(t, 3, allowedCount, "should be capped at burst capacity of 3")
	})
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
