package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHedgeConfig_Enabled(t *testing.T) {
	type args struct {
		delay     time.Duration
		maxHedges int
	}

	tests := []struct {
		name        string
		args        args
		wantEnabled bool
	}{
		{
			name: "given zero values, then disabled",
			args: args{
				delay:     0,
				maxHedges: 0,
			},
			wantEnabled: false,
		},
		{
			name: "given only delay, then disabled",
			args: args{
				delay:     50 * time.Millisecond,
				maxHedges: 0,
			},
			wantEnabled: false,
		},
		{
			name: "given only max hedges, then disabled",
			args: args{
				delay:     0,
				maxHedges: 1,
			},
			wantEnabled: false,
		},
		{
			name: "given both set, then enabled",
			args: args{
				delay:     50 * time.Millisecond,
				maxHedges: 1,
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := HedgeConfig{
				Delay:     tt.args.delay,
				MaxHedges: tt.args.maxHedges,
			}

			got := cfg.Enabled()
			assert.Equal(t, tt.wantEnabled, got)
		})
	}
}

func TestHedgeTransport_RoundTrip(t *testing.T) {
	type args struct {
		config      HedgeConfig
		serverDelay time.Duration
	}

	tests := []struct {
		name            string
		args            args
		wantErr         assert.ErrorAssertionFunc
		wantSC          int
		wantMinRequests int32
		wantMaxTime     time.Duration
	}{
		{
			name: "given disabled config, then sends one request without hedging",
			args: args{
				config:      HedgeConfig{},
				serverDelay: 0,
			},
			wantErr:         assert.NoError,
			wantSC:          http.StatusOK,
			wantMinRequests: 1,
		},
		{
			name: "given fast response, then no hedge is sent",
			args: args{
				config:      HedgeConfig{Delay: 100 * time.Millisecond, MaxHedges: 1},
				serverDelay: 0,
			},
			wantErr:         assert.NoError,
			wantSC:          http.StatusOK,
			wantMinRequests: 1,
		},
		{
			name: "given slow first response, then hedge wins and reduces latency",
			args: args{
				config:      HedgeConfig{Delay: 30 * time.Millisecond, MaxHedges: 1},
				serverDelay: 200 * time.Millisecond,
			},
			wantErr:         assert.NoError,
			wantSC:          http.StatusOK,
			wantMinRequests: 2,
			wantMaxTime:     100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestCount atomic.Int32

			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					count := requestCount.Add(1)
					// First request is slow, subsequent are fast
					if count == 1 && tt.args.serverDelay > 0 {
						time.Sleep(tt.args.serverDelay)
					}
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			transport := newHedgeTransport(http.DefaultTransport, tt.args.config)

			req, err := http.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				server.URL,
				nil,
			)
			require.NoError(t, err)

			start := time.Now()
			resp, err := transport.RoundTrip(req)
			elapsed := time.Since(start)

			tt.wantErr(t, err)

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
				resp.Body.Close()
			}

			// Wait for all goroutines to settle
			time.Sleep(tt.args.serverDelay + 50*time.Millisecond)

			assert.GreaterOrEqual(t, requestCount.Load(), tt.wantMinRequests)

			if tt.wantMaxTime > 0 {
				assert.Less(t, elapsed, tt.wantMaxTime)
			}
		})
	}
}

func TestRequestBuilder_Hedge(t *testing.T) {
	type args struct {
		hedgeDelay  time.Duration
		serverDelay time.Duration
	}

	tests := []struct {
		name            string
		args            args
		wantErr         assert.ErrorAssertionFunc
		wantSC          int
		wantMinRequests int32
		wantMaxTime     time.Duration
	}{
		{
			name: "given fast response, then no hedge is sent",
			args: args{
				hedgeDelay:  100 * time.Millisecond,
				serverDelay: 0,
			},
			wantErr:         assert.NoError,
			wantSC:          http.StatusOK,
			wantMinRequests: 1,
		},
		{
			name: "given slow first response, then hedge wins",
			args: args{
				hedgeDelay:  30 * time.Millisecond,
				serverDelay: 200 * time.Millisecond,
			},
			wantErr:         assert.NoError,
			wantSC:          http.StatusOK,
			wantMinRequests: 2,
			wantMaxTime:     100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestCount atomic.Int32

			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					count := requestCount.Add(1)
					if count == 1 && tt.args.serverDelay > 0 {
						time.Sleep(tt.args.serverDelay)
					}
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			client := New(
				WithBaseURL(server.URL),
				WithRetryDisabled(),
			)

			start := time.Now()
			resp, err := client.Request("Test").
				Hedge(tt.args.hedgeDelay).
				Get(context.Background(), "/test")
			elapsed := time.Since(start)

			tt.wantErr(t, err)

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}

			// Wait for all goroutines to settle
			time.Sleep(tt.args.serverDelay + 50*time.Millisecond)

			assert.GreaterOrEqual(t, requestCount.Load(), tt.wantMinRequests)

			if tt.wantMaxTime > 0 {
				assert.Less(t, elapsed, tt.wantMaxTime)
			}
		})
	}
}

func TestRequestBuilder_HedgeConfig(t *testing.T) {
	type args struct {
		config       HedgeConfig
		serverDelay1 time.Duration
		serverDelay2 time.Duration
	}

	tests := []struct {
		name            string
		args            args
		wantErr         assert.ErrorAssertionFunc
		wantSC          int
		wantMinRequests int32
		wantMaxTime     time.Duration
	}{
		{
			name: "given multiple hedges with slow initial requests, then later hedge wins",
			args: args{
				config: HedgeConfig{
					Delay:     20 * time.Millisecond,
					MaxHedges: 2,
				},
				serverDelay1: 200 * time.Millisecond, // First request slow
				serverDelay2: 200 * time.Millisecond, // Second request also slow
			},
			wantErr:         assert.NoError,
			wantSC:          http.StatusOK,
			wantMinRequests: 2,
			wantMaxTime:     100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestCount atomic.Int32

			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					count := requestCount.Add(1)
					if count <= 2 {
						time.Sleep(tt.args.serverDelay1)
					}
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			client := New(
				WithBaseURL(server.URL),
				WithRetryDisabled(),
			)

			start := time.Now()
			resp, err := client.Request("Test").
				HedgeConfig(tt.args.config).
				Get(context.Background(), "/test")
			elapsed := time.Since(start)

			tt.wantErr(t, err)

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}

			// Wait for goroutines
			time.Sleep(250 * time.Millisecond)

			assert.GreaterOrEqual(t, requestCount.Load(), tt.wantMinRequests)

			if tt.wantMaxTime > 0 {
				assert.Less(t, elapsed, tt.wantMaxTime)
			}
		})
	}
}

func TestHedge_DoesNotBreakNormalFlow(t *testing.T) {
	type args struct {
		method      string
		body        any
		hedgeDelay  time.Duration
		serverDelay time.Duration
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
		wantSC  int
	}{
		{
			name: "given GET request with hedge, then succeeds normally",
			args: args{
				method:      "GET",
				hedgeDelay:  50 * time.Millisecond,
				serverDelay: 0,
			},
			wantErr: assert.NoError,
			wantSC:  http.StatusOK,
		},
		{
			name: "given POST request with body and hedge, then succeeds normally",
			args: args{
				method:      "POST",
				body:        map[string]string{"key": "value"},
				hedgeDelay:  50 * time.Millisecond,
				serverDelay: 0,
			},
			wantErr: assert.NoError,
			wantSC:  http.StatusCreated,
		},
		{
			name: "given slow response without hedge, then completes normally",
			args: args{
				method:      "GET",
				hedgeDelay:  0, // No hedging
				serverDelay: 100 * time.Millisecond,
			},
			wantErr: assert.NoError,
			wantSC:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.args.serverDelay > 0 {
						time.Sleep(tt.args.serverDelay)
					}
					if r.Method == http.MethodPost {
						w.WriteHeader(http.StatusCreated)
					} else {
						w.WriteHeader(http.StatusOK)
					}
				}),
			)
			defer server.Close()

			client := New(
				WithBaseURL(server.URL),
				WithRetryDisabled(),
			)

			rb := client.Request("Test")
			if tt.args.hedgeDelay > 0 {
				rb = rb.Hedge(tt.args.hedgeDelay)
			}
			if tt.args.body != nil {
				rb = rb.Body(tt.args.body)
			}

			var resp *Response
			var err error
			switch tt.args.method {
			case "GET":
				resp, err = rb.Get(context.Background(), "/test")
			case "POST":
				resp, err = rb.Post(context.Background(), "/test")
			}

			tt.wantErr(t, err)

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}
		})
	}
}
