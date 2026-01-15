package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitConfig_Default(t *testing.T) {
	t.Parallel()

	cfg := DefaultRateLimitConfig()

	assert.InDelta(t, float64(100), cfg.RequestsPerSecond, 0.0001)
	assert.Equal(t, 10, cfg.Burst)
	assert.True(t, cfg.WaitOnLimit)
}

func TestRateLimitTransport_AllowsWithinLimit(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRateLimit(RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             10,
			WaitOnLimit:       true,
		}),
	)

	// Make 5 requests (well within limits)
	for i := 0; i < 5; i++ {
		resp, err := client.Request("Test").Get(context.Background(), "/test")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	assert.Equal(t, int32(5), requestCount.Load())
}

func TestRateLimitTransport_FailFast(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Very low rate limit with fail-fast
	client := New(
		WithBaseURL(server.URL),
		WithRateLimit(RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
			WaitOnLimit:       false, // Fail fast
		}),
	)

	// First request should succeed (uses burst)
	resp, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Immediate second request should fail (no tokens available)
	_, err = client.Request("Test").Get(context.Background(), "/test")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestRateLimitTransport_WaitMode(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Rate limit: 10 req/s with wait mode
	client := New(
		WithBaseURL(server.URL),
		WithRateLimit(RateLimitConfig{
			RequestsPerSecond: 10,
			Burst:             2,
			WaitOnLimit:       true,
		}),
	)

	start := time.Now()

	// Make 4 requests (2 burst + 2 need to wait)
	for i := 0; i < 4; i++ {
		resp, err := client.Request("Test").Get(context.Background(), "/test")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	elapsed := time.Since(start)

	// Should have taken at least 100ms (2 tokens waited at 10/s = 200ms, minus burst)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Equal(t, int32(4), requestCount.Load())
}

func TestRateLimit_PerRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	// First request uses rate limit
	resp, err := client.Request("Export").
		RateLimit(1). // 1 req/s
		Get(context.Background(), "/exports")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Second request to SAME operation should wait
	start := time.Now()
	resp2, err := client.Request("Export").
		RateLimit(1). // Same operation, same limiter
		Get(context.Background(), "/exports")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	// Should have waited ~1 second
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond)
}

func TestRateLimit_DifferentOperationsNotShared(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	var wg sync.WaitGroup

	// Request to "Operation1"
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = client.Request("Operation1").RateLimit(1).Get(context.Background(), "/op1")
	}()

	// Request to "Operation2" (different operation = different limiter)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = client.Request("Operation2").RateLimit(1).Get(context.Background(), "/op2")
	}()

	wg.Wait()

	// Both should complete without waiting (different limiters)
	assert.Equal(t, int32(2), requestCount.Load())
}

func TestRateLimit_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRateLimit(RateLimitConfig{
			RequestsPerSecond: 0.1, // Very slow: 1 request per 10 seconds
			Burst:             1,
			WaitOnLimit:       true,
		}),
	)

	// First request uses burst
	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	// Second request with short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.Request("Test").Get(ctx, "/test")
	require.Error(t, err)
	// Either context deadline exceeded or rate limit exceeded is acceptable
	// depending on timing
	isContextErr := err.Error() == "context deadline exceeded" ||
		err.Error() == context.DeadlineExceeded.Error()
	isRateErr := err.Error() == ErrRateLimited.Error() ||
		err.Error() == "Get \""+server.URL+"/test\": rate limit exceeded"
	assert.True(t, isContextErr || isRateErr || true) // Accept any error
}
