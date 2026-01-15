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

func TestAdaptiveHedgeConfig_Enabled(t *testing.T) {
	type args struct {
		fallbackDelay time.Duration
		maxHedges     int
	}

	tests := []struct {
		name        string
		args        args
		wantEnabled bool
	}{
		{
			name: "given zero values, then disabled",
			args: args{
				fallbackDelay: 0,
				maxHedges:     0,
			},
			wantEnabled: false,
		},
		{
			name: "given only fallback delay, then disabled",
			args: args{
				fallbackDelay: 50 * time.Millisecond,
				maxHedges:     0,
			},
			wantEnabled: false,
		},
		{
			name: "given both set, then enabled",
			args: args{
				fallbackDelay: 50 * time.Millisecond,
				maxHedges:     1,
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := AdaptiveHedgeConfig{
				FallbackDelay: tt.args.fallbackDelay,
				MaxHedges:     tt.args.maxHedges,
			}

			got := config.Enabled()
			assert.Equal(t, tt.wantEnabled, got)
		})
	}
}

func TestAdaptiveHedgeConfig_GetDelay(t *testing.T) {
	type args struct {
		endpoint         string
		priorLatencies   []time.Duration
		targetPercentile float64
		fallbackDelay    time.Duration
		minSamples       int
	}

	tests := []struct {
		name      string
		args      args
		wantDelay time.Duration
	}{
		{
			name: "given insufficient samples, then returns fallback",
			args: args{
				endpoint:         "/users",
				priorLatencies:   []time.Duration{10 * time.Millisecond, 20 * time.Millisecond},
				targetPercentile: 0.95,
				fallbackDelay:    50 * time.Millisecond,
				minSamples:       10,
			},
			wantDelay: 50 * time.Millisecond,
		},
		{
			name: "given sufficient samples, then returns percentile",
			args: args{
				endpoint: "/users",
				priorLatencies: []time.Duration{
					10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond,
					40 * time.Millisecond, 50 * time.Millisecond,
				},
				targetPercentile: 0.80,
				fallbackDelay:    100 * time.Millisecond,
				minSamples:       3,
			},
			wantDelay: 40 * time.Millisecond, // Index 3 of 5 samples (0-based)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewLatencyTracker(100, tt.args.minSamples)
			for _, lat := range tt.args.priorLatencies {
				tracker.Record(tt.args.endpoint, lat)
			}

			config := AdaptiveHedgeConfig{
				TargetPercentile: tt.args.targetPercentile,
				FallbackDelay:    tt.args.fallbackDelay,
				MinSamples:       tt.args.minSamples,
				Tracker:          tracker,
			}

			got := config.GetDelay(tt.args.endpoint)
			assert.Equal(t, tt.wantDelay, got)
		})
	}
}

func TestRequestBuilder_AdaptiveHedge(t *testing.T) {
	type args struct {
		config      AdaptiveHedgeConfig
		priorCalls  int
		serverDelay time.Duration
		minSamples  int
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
		wantSC  int
	}{
		{
			name: "given insufficient samples, uses fallback delay",
			args: args{
				config: AdaptiveHedgeConfig{
					TargetPercentile: 0.95,
					FallbackDelay:    50 * time.Millisecond,
					MaxHedges:        1,
					MinSamples:       100, // Requires many samples
				},
				priorCalls:  0,
				serverDelay: 0,
			},
			wantErr: assert.NoError,
			wantSC:  http.StatusOK,
		},
		{
			name: "given sufficient samples, adapts delay",
			args: args{
				config: AdaptiveHedgeConfig{
					TargetPercentile: 0.95,
					FallbackDelay:    500 * time.Millisecond,
					MaxHedges:        1,
					MinSamples:       3,
				},
				priorCalls:  5, // Builds history first
				serverDelay: 0,
			},
			wantErr: assert.NoError,
			wantSC:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if tt.args.serverDelay > 0 {
						time.Sleep(tt.args.serverDelay)
					}
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			// Create a fresh tracker for each test
			tracker := NewLatencyTracker(100, tt.args.minSamples)
			tt.args.config.Tracker = tracker

			client := New(
				WithBaseURL(server.URL),
				WithRetryDisabled(),
			)

			// Build up prior latency samples
			for i := 0; i < tt.args.priorCalls; i++ {
				resp, err := client.Request("Test").
					AdaptiveHedge(tt.args.config).
					Get(context.Background(), "/test")
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			}

			// Make the actual test request
			resp, err := client.Request("Test").
				AdaptiveHedge(tt.args.config).
				Get(context.Background(), "/test")

			tt.wantErr(t, err)

			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}
		})
	}
}

func TestAdaptiveHedge_RecordsLatency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tracker := NewLatencyTracker(100, 1)
	config := AdaptiveHedgeConfig{
		TargetPercentile: 0.95,
		FallbackDelay:    50 * time.Millisecond,
		MaxHedges:        1,
		MinSamples:       1,
		Tracker:          tracker,
	}

	client := New(
		WithBaseURL(server.URL),
		WithRetryDisabled(),
	)

	// Initially no samples
	assert.Equal(t, 0, tracker.Count("Test"))

	// Make requests
	for i := 0; i < 5; i++ {
		resp, err := client.Request("Test").
			AdaptiveHedge(config).
			Get(context.Background(), "/test")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Latencies should be recorded
	assert.Equal(t, 5, tracker.Count("Test"))

	// Percentile should now be available
	_, ok := tracker.Percentile("Test", 0.95)
	assert.True(t, ok)
}

func TestDefaultAdaptiveHedgeConfig(t *testing.T) {
	config := DefaultAdaptiveHedgeConfig()

	assert.InDelta(t, 0.95, config.TargetPercentile, 0.001)
	assert.Equal(t, 100, config.WindowSize)
	assert.Equal(t, 10, config.MinSamples)
	assert.Equal(t, 50*time.Millisecond, config.FallbackDelay)
	assert.Equal(t, 1, config.MaxHedges)
}
