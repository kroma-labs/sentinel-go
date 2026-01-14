package httpclient

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	gobreaker "github.com/sony/gobreaker/v2"
	gobreakerredis "github.com/sony/gobreaker/v2/redis"
)

// NewRedisStore creates a SharedDataStore backed by Redis for distributed circuit breaking.
// This uses the official sony/gobreaker/v2/redis implementation.
//
// Usage:
//
//	rdb := redis.NewUniversalClient(&redis.UniversalOptions{Addrs: []string{"localhost:6379"}})
//	store := httpclient.NewRedisStore(rdb)
func NewRedisStore(client redis.UniversalClient) gobreaker.SharedDataStore {
	return gobreakerredis.NewStoreFromClient(client)
}

// CircuitBreaker is the interface used by circuit breaker transport.
// It matches gobreaker.CircuitBreaker signature.
type CircuitBreaker interface {
	Execute(req func() (interface{}, error)) (interface{}, error)
}

// BreakerClassifier determines if a request failure should contribute to the circuit breaker trip count.
// Returns true if the error/response indicates a system failure (e.g., 500, Network Error).
type BreakerClassifier func(resp *http.Response, err error) bool

// BreakerConfig holds the configuration for the circuit breaker.
//
// Concepts:
//   - Closed: Normal state, requests allowed.
//   - Open: Failing state, requests rejected immediately.
//   - Half-Open: Probing state, limited requests allowed to test recovery.
type BreakerConfig struct {
	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is half-open (probing).
	// If 0, the circuit breaker allows 1 request.
	MaxRequests uint32

	// Interval is the cyclic period of the closed state
	// for the CircuitBreaker to clear the internal Counts.
	// If 0, the CircuitBreaker doesn't clear internal Counts during the closed state.
	Interval time.Duration

	// Timeout is the period of the open state,
	// after which the state of the CircuitBreaker becomes half-open.
	// We default this to 60s if 0.
	Timeout time.Duration

	// FailureThreshold is the minimum number of requests needed before a circuit can be tripped due to failure ratio.
	// Default: 10
	FailureThreshold uint32

	// FailureRatio is the threshold of failure ratio (0.0 - 1.0) to trip the circuit.
	// Default: 0.5 (50% failure rate)
	FailureRatio float64

	// ConsecutiveFailures is the number of consecutive failures that will trip the circuit.
	// If 0, this rule is desabled.
	ConsecutiveFailures uint32

	// Store is the shared data store for distributed circuit breaking.
	// If nil, the circuit breaker is local (in-memory).
	Store gobreaker.SharedDataStore

	// Classifier determines which errors count as failures.
	// Default: DefaultBreakerClassifier
	Classifier BreakerClassifier

	// OnStateChange is a callback invoked when the circuit breaker state changes.
	OnStateChange func(name string, from, to gobreaker.State)
}

// DistributedBreakerConfig returns a configuration for a distributed circuit breaker backed by Redis.
//
// This configuration allows multiple service instances to share the same circuit breaker state.
// If one instance trips the breaker, all instances will stop sending requests to the failing service.
func DistributedBreakerConfig(store gobreaker.SharedDataStore) BreakerConfig {
	cfg := DefaultBreakerConfig()
	cfg.Store = store
	// Distributed breakers often need slightly more relaxed thresholds to account for propagation delays
	// or broader impact. Defaults remain the same as local for now but can be customized.
	return cfg
}

// DefaultBreakerConfig returns a safe default configuration for a local (in-memory) circuit breaker.
//
// Defaults based on Hystrix and Google SRE best practices:
//   - Interval: 10s
//   - Timeout: 10s (Fail fast, recover fast)
//   - FailureThreshold: 20 (Minimum requests before triggering)
//   - MaximumRatio: 0.5 (50% failure rate)
//   - ConsecutiveFailures: 5 (Trip immediately after 5 sequential failures)
func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		MaxRequests:         1,
		Interval:            10 * time.Second,
		Timeout:             10 * time.Second,
		FailureThreshold:    20,
		FailureRatio:        0.5,
		ConsecutiveFailures: 5,
		Classifier:          DefaultBreakerClassifier,
	}
}

// DisabledBreakerConfig returns a configuration that effectively disables the circuit breaker
// by having very high thresholds.
func DisabledBreakerConfig() BreakerConfig {
	return BreakerConfig{
		MaxRequests:      0,
		Interval:         0,
		Timeout:          0,
		FailureThreshold: ^uint32(0),
		FailureRatio:     1.0,
		Classifier:       func(_ *http.Response, _ error) bool { return false },
	}
}

// DefaultBreakerClassifier classifies 5xx errors and network errors as failures.
// It ignores 429s as they should be handled by retry logic/backoff, not by tripping the breaker.
func DefaultBreakerClassifier(resp *http.Response, err error) bool {
	if err != nil {
		return isNetworkError(err)
	}

	if resp != nil {
		if resp.StatusCode >= 500 {
			return true
		}
	}

	return false
}

// isNetworkError checks for common network errors.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	return false
}
