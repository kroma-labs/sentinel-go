package httpclient

import (
	"context"
	"errors"
	"net/http"

	"github.com/sony/gobreaker/v2"
)

// circuitBreakerTransport is a RoundTripper that wraps requests in a circuit breaker.
type circuitBreakerTransport struct {
	breaker    CircuitBreaker
	next       http.RoundTripper
	classifier BreakerClassifier
	cfg        *internalConfig
	name       string
}

// errSyntheticFailure is a sentinel error used to signal the circuit breaker
// that a request failed (e.g. 500 status) even if the underlying RoundTrip returned no error.
// It is intercepted and unwrapped by the transport before returning to the caller.
var errSyntheticFailure = errors.New("synthetic failure")

// RoundTrip implements http.RoundTripper.
func (t *circuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	res, err := t.breaker.Execute(func() (interface{}, error) {
		resp, err := t.next.RoundTrip(req) //nolint:bodyclose

		if t.classifier(resp, err) {
			if err != nil {
				return resp, err
			}
			return resp, errSyntheticFailure
		}

		return resp, nil
	})
	if err != nil {
		// Differentiate between "Circuit Open" rejection and "Actual Failure"
		if errors.Is(err, gobreaker.ErrOpenState) {
			t.cfg.Metrics.recordBreakerRequest(ctx, t.name, "rejected")
		} else {
			// This is a failure that passed through the breaker but failed execution
			t.cfg.Metrics.recordBreakerRequest(ctx, t.name, "failure")
		}

		// Unwrap synthetic failure
		if errors.Is(err, errSyntheticFailure) {
			if resp, ok := res.(*http.Response); ok {
				return resp, nil
			}
		}

		return nil, err
	}

	t.cfg.Metrics.recordBreakerRequest(ctx, t.name, "success")

	if resp, ok := res.(*http.Response); ok {
		return resp, nil
	}

	return nil, errors.New("circuit breaker returned unknown response type")
}

// newCircuitBreakerTransport creates a new circuit breaker transport.
func newCircuitBreakerTransport(next http.RoundTripper, cfg *internalConfig) http.RoundTripper {
	if cfg.BreakerConfig == nil {
		return next
	}

	// Use ServiceName as the breaker identifier.
	// If no ServiceName provided, fallback to "default-http-client".
	name := cfg.ServiceName
	if name == "" {
		name = "default-http-client"
	}

	st := gobreaker.Settings{
		Name:        name,
		MaxRequests: cfg.BreakerConfig.MaxRequests,
		Interval:    cfg.BreakerConfig.Interval,
		Timeout:     cfg.BreakerConfig.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if cfg.BreakerConfig.FailureThreshold > 0 &&
				counts.Requests < cfg.BreakerConfig.FailureThreshold {
				return false
			}
			if cfg.BreakerConfig.ConsecutiveFailures > 0 &&
				counts.ConsecutiveFailures >= cfg.BreakerConfig.ConsecutiveFailures {
				return true
			}
			if cfg.BreakerConfig.FailureRatio > 0 && counts.TotalFailures > 0 {
				ratio := float64(counts.TotalFailures) / float64(counts.Requests)
				if ratio >= cfg.BreakerConfig.FailureRatio {
					return true
				}
			}
			return false
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			if cfg.Metrics != nil {
				cfg.Metrics.recordBreakerState(context.Background(), name, int64(to))
			}
			if cfg.BreakerConfig.OnStateChange != nil {
				cfg.BreakerConfig.OnStateChange(name, from, to)
			}
		},
	}

	var cb CircuitBreaker

	if cfg.BreakerConfig.Store != nil {
		// NewDistributedCircuitBreaker returns error only if Store is nil, which we checked.
		// However, adhering to API signature:
		dcb, err := gobreaker.NewDistributedCircuitBreaker[interface{}](cfg.BreakerConfig.Store, st)
		if err != nil {
			// Fallback to local breaker if distributed creation fails.
			//
			// Rationale: Graceful Degradation.
			// It is safer to have a local circuit breaker protecting the service than no protection at all.
			// Common causes for failure include:
			//   - Invalid settings (e.g., empty Name, though we ensure a default)
			//   - Store connectivity issues during initialization (depends on Store implementation)
			//
			// If creation fails, this instance will operate independently (Local mode), which may result in
			// slightly higher total traffic to the failing service across all instances, but still provides
			// process-level overload protection.
			cb = gobreaker.NewCircuitBreaker[interface{}](st)
		} else {
			cb = dcb
		}
	} else {
		cb = gobreaker.NewCircuitBreaker[interface{}](st)
	}

	return &circuitBreakerTransport{
		breaker:    cb,
		next:       next,
		classifier: cfg.BreakerConfig.Classifier,
		cfg:        cfg,
		name:       name,
	}
}
