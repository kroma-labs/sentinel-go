package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChaosConfig_Delay(t *testing.T) {
	type args struct {
		latencyMs       int
		latencyJitterMs int
	}

	tests := []struct {
		name    string
		args    args
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name: "given zero config, then returns zero delay",
			args: args{
				latencyMs:       0,
				latencyJitterMs: 0,
			},
			wantMin: 0,
			wantMax: 0,
		},
		{
			name: "given fixed latency, then returns exact delay",
			args: args{
				latencyMs:       100,
				latencyJitterMs: 0,
			},
			wantMin: 100 * time.Millisecond,
			wantMax: 100 * time.Millisecond,
		},
		{
			name: "given latency with jitter, then returns delay in range",
			args: args{
				latencyMs:       100,
				latencyJitterMs: 50,
			},
			wantMin: 100 * time.Millisecond,
			wantMax: 150 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ChaosConfig{
				LatencyMs:       tt.args.latencyMs,
				LatencyJitterMs: tt.args.latencyJitterMs,
			}

			got := cfg.Delay()

			assert.GreaterOrEqual(t, got, tt.wantMin)
			if tt.wantMax > tt.wantMin {
				assert.Less(t, got, tt.wantMax)
			} else {
				assert.Equal(t, tt.wantMin, got)
			}
		})
	}
}

func TestChaosConfig_ShouldInjectError(t *testing.T) {
	type args struct {
		errorRate float64
	}

	tests := []struct {
		name       string
		args       args
		wantInject bool
	}{
		{
			name:       "given zero rate, then never injects",
			args:       args{errorRate: 0},
			wantInject: false,
		},
		{
			name:       "given negative rate, then never injects",
			args:       args{errorRate: -0.5},
			wantInject: false,
		},
		{
			name:       "given full rate, then always injects",
			args:       args{errorRate: 1.0},
			wantInject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ChaosConfig{ErrorRate: tt.args.errorRate}

			// Run multiple times for probabilistic tests
			for i := 0; i < 100; i++ {
				got := cfg.ShouldInjectError()
				assert.Equal(t, tt.wantInject, got)
			}
		})
	}
}

func TestChaosConfig_ShouldInjectTimeout(t *testing.T) {
	type args struct {
		timeoutRate float64
	}

	tests := []struct {
		name       string
		args       args
		wantInject bool
	}{
		{
			name:       "given zero rate, then never injects",
			args:       args{timeoutRate: 0},
			wantInject: false,
		},
		{
			name:       "given full rate, then always injects",
			args:       args{timeoutRate: 1.0},
			wantInject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ChaosConfig{TimeoutRate: tt.args.timeoutRate}

			for i := 0; i < 100; i++ {
				got := cfg.ShouldInjectTimeout()
				assert.Equal(t, tt.wantInject, got)
			}
		})
	}
}

func TestChaosTransport_RoundTrip(t *testing.T) {
	type args struct {
		config         ChaosConfig
		contextTimeout time.Duration
	}

	tests := []struct {
		name        string
		args        args
		wantErr     assert.ErrorAssertionFunc
		wantErrType error
		wantSC      int
		wantMinTime time.Duration
	}{
		{
			name: "given latency config, then adds delay and returns success",
			args: args{
				config: ChaosConfig{LatencyMs: 50},
			},
			wantErr:     assert.NoError,
			wantSC:      http.StatusOK,
			wantMinTime: 50 * time.Millisecond,
		},
		{
			name: "given error rate 1.0, then returns chaos error",
			args: args{
				config: ChaosConfig{ErrorRate: 1.0},
			},
			wantErr:     assert.Error,
			wantErrType: ErrChaosInjected,
		},
		{
			name: "given timeout rate 1.0, then returns deadline exceeded",
			args: args{
				config:         ChaosConfig{TimeoutRate: 1.0},
				contextTimeout: 50 * time.Millisecond,
			},
			wantErr:     assert.Error,
			wantErrType: context.DeadlineExceeded,
		},
		{
			name: "given no chaos config, then passes through normally",
			args: args{
				config: ChaosConfig{},
			},
			wantErr: assert.NoError,
			wantSC:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			transport := newChaosTransport(http.DefaultTransport, tt.args.config)

			ctx := context.Background()
			if tt.args.contextTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.args.contextTimeout)
				defer cancel()
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)

			start := time.Now()
			resp, err := transport.RoundTrip(req)
			elapsed := time.Since(start)

			tt.wantErr(t, err)

			if err != nil && tt.wantErrType != nil {
				require.ErrorIs(t, err, tt.wantErrType)
			}

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
				resp.Body.Close()
			}

			if tt.wantMinTime > 0 {
				assert.GreaterOrEqual(t, elapsed, tt.wantMinTime)
			}
		})
	}
}

func TestChaosTransport_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := newChaosTransport(http.DefaultTransport, ChaosConfig{LatencyMs: 1000})

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	resp, err := transport.RoundTrip(req)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resp)
}

func TestWithChaos_Integration(t *testing.T) {
	type args struct {
		config ChaosConfig
	}

	tests := []struct {
		name        string
		args        args
		wantErr     assert.ErrorAssertionFunc
		wantSC      int
		wantMinTime time.Duration
	}{
		{
			name: "given latency chaos, then request is delayed",
			args: args{
				config: ChaosConfig{LatencyMs: 30},
			},
			wantErr:     assert.NoError,
			wantSC:      http.StatusOK,
			wantMinTime: 30 * time.Millisecond,
		},
		{
			name: "given error chaos, then request fails",
			args: args{
				config: ChaosConfig{ErrorRate: 1.0},
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			client := New(
				WithBaseURL(server.URL),
				WithChaos(tt.args.config),
				WithRetryDisabled(),
			)

			start := time.Now()
			resp, err := client.Request("Test").Get(context.Background(), "/test")
			elapsed := time.Since(start)

			tt.wantErr(t, err)

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}

			if tt.wantMinTime > 0 {
				assert.GreaterOrEqual(t, elapsed, tt.wantMinTime)
			}
		})
	}
}
