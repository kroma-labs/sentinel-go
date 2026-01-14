package httpclient

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kroma-labs/sentinel-go/httpclient/mocks"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

type NetError struct {
	Msg string
}

func (e *NetError) Error() string   { return e.Msg }
func (e *NetError) Timeout() bool   { return false }
func (e *NetError) Temporary() bool { return false }

func TestDefaultBreakerConfig(t *testing.T) {
	cfg := DefaultBreakerConfig()
	assert.Equal(t, uint32(1), cfg.MaxRequests)
	assert.Equal(t, 10*time.Second, cfg.Interval)
	assert.Equal(t, 10*time.Second, cfg.Timeout)
	assert.Equal(t, uint32(20), cfg.FailureThreshold)
	assert.InEpsilon(t, 0.5, cfg.FailureRatio, 0.001)
	assert.Equal(t, uint32(5), cfg.ConsecutiveFailures)
	assert.NotNil(t, cfg.Classifier)
}

func TestConfigFunctions(t *testing.T) {
	t.Run("DefaultBreakerConfig", func(t *testing.T) {
		localCfg := DefaultBreakerConfig()
		assert.Nil(t, localCfg.Store)
		assert.Equal(t, uint32(5), localCfg.ConsecutiveFailures)
	})

	t.Run("DistributedBreakerConfig", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		store := NewRedisStore(rdb)

		distCfg := DistributedBreakerConfig(store)
		assert.Equal(t, store, distCfg.Store)
		assert.Equal(t, 10*time.Second, distCfg.Interval)
	})

	t.Run("DisabledBreakerConfig", func(t *testing.T) {
		disabledCfg := DisabledBreakerConfig()
		assert.Equal(t, uint32(0), disabledCfg.MaxRequests)
		assert.InEpsilon(t, float64(1.0), disabledCfg.FailureRatio, 0.001)
	})
}

func TestBreakerTransport_RoundTrip(t *testing.T) {
	type args struct {
		resp *http.Response
		err  error
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(*mocks.CircuitBreaker, *mocks.RoundTripper)
		wantErr assert.ErrorAssertionFunc
		wantSC  int
		errType error
	}{
		{
			name: "given successful execution, then returns response and no error",
			args: args{
				resp: &http.Response{StatusCode: http.StatusOK},
				err:  nil,
			},
			mockFn: func(cb *mocks.CircuitBreaker, rt *mocks.RoundTripper) {
				cb.EXPECT().
					Execute(mock.Anything).
					RunAndReturn(func(req func() (interface{}, error)) (interface{}, error) {
						// Execute wrapper will call this, which calls roundtripper
						return req()
					}).Once()

				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{StatusCode: http.StatusOK}, nil).Once()
			},
			wantErr: assert.NoError,
			wantSC:  200,
		},
		{
			name: "given circuit open (rejected), then returns ErrOpenState",
			args: args{
				resp: nil,
				err:  nil,
			},
			mockFn: func(cb *mocks.CircuitBreaker, _ *mocks.RoundTripper) {
				cb.EXPECT().
					Execute(mock.Anything).
					Return(nil, gobreaker.ErrOpenState).Once()
			},
			wantErr: assert.Error,
			errType: gobreaker.ErrOpenState,
		},
		{
			name: "given execution failure (500), then returns response",
			args: args{
				resp: &http.Response{StatusCode: http.StatusInternalServerError},
				err:  nil,
			},
			mockFn: func(cb *mocks.CircuitBreaker, rt *mocks.RoundTripper) {
				cb.EXPECT().
					Execute(mock.Anything).
					RunAndReturn(func(req func() (interface{}, error)) (interface{}, error) {
						return req()
					}).Once()

				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{StatusCode: http.StatusInternalServerError}, nil).Once()
			},
			wantErr: assert.NoError, // Transport handles the error and returns the 500 response
			wantSC:  500,
		},
		{
			name: "given network error, then returns error",
			args: args{
				resp: nil,
				err:  &NetError{Msg: "network error"},
			},
			mockFn: func(cb *mocks.CircuitBreaker, rt *mocks.RoundTripper) {
				cb.EXPECT().
					Execute(mock.Anything).
					RunAndReturn(func(req func() (interface{}, error)) (interface{}, error) {
						return req()
					}).Once()

				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(nil, &NetError{Msg: "network error"}).Once()
			},
			wantErr: assert.Error,
			errType: &NetError{Msg: "network error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBreaker := mocks.NewCircuitBreaker(t)
			mockRT := mocks.NewRoundTripper(t)

			tt.mockFn(mockBreaker, mockRT)

			meter := noop.NewMeterProvider().Meter("test")
			m, _ := newMetrics(meter)

			breakerCfg := DefaultBreakerConfig()
			cfg := &internalConfig{
				BreakerConfig: &breakerCfg,
				Metrics:       m,
				ServiceName:   "test-service",
			}

			// Using mock RoundTripper now
			tr := &circuitBreakerTransport{
				breaker:    mockBreaker,
				next:       mockRT,
				classifier: DefaultBreakerClassifier,
				cfg:        cfg,
				name:       "test-service",
			}

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
			resp, err := tr.RoundTrip(req)

			tt.wantErr(t, err)

			if err != nil && tt.errType != nil {
				if errors.Is(tt.errType, gobreaker.ErrOpenState) {
					assert.ErrorIs(t, err, gobreaker.ErrOpenState)
				} else {
					assert.Equal(t, tt.errType.Error(), err.Error())
				}
			}

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}
		})
	}
}
