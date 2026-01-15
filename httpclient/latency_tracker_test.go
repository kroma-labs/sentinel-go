package httpclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLatencyTracker_Record(t *testing.T) {
	type args struct {
		endpoint   string
		latencies  []time.Duration
		windowSize int
		minSamples int
	}

	tests := []struct {
		name      string
		args      args
		wantCount int
	}{
		{
			name: "given multiple samples, then tracks count correctly",
			args: args{
				endpoint: "/users",
				latencies: []time.Duration{
					10 * time.Millisecond,
					20 * time.Millisecond,
					30 * time.Millisecond,
				},
				windowSize: 100,
				minSamples: 10,
			},
			wantCount: 3,
		},
		{
			name: "given samples exceeding window, then caps at window size",
			args: args{
				endpoint: "/users",
				latencies: []time.Duration{
					10 * time.Millisecond,
					20 * time.Millisecond,
					30 * time.Millisecond,
					40 * time.Millisecond,
					50 * time.Millisecond,
				},
				windowSize: 3,
				minSamples: 1,
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewLatencyTracker(tt.args.windowSize, tt.args.minSamples)

			for _, latency := range tt.args.latencies {
				tracker.Record(tt.args.endpoint, latency)
			}

			got := tracker.Count(tt.args.endpoint)
			assert.Equal(t, tt.wantCount, got)
		})
	}
}

func TestLatencyTracker_Percentile(t *testing.T) {
	type args struct {
		endpoint   string
		latencies  []time.Duration
		percentile float64
		windowSize int
		minSamples int
	}

	tests := []struct {
		name        string
		args        args
		wantLatency time.Duration
		wantOK      bool
	}{
		{
			name: "given insufficient samples, then returns false",
			args: args{
				endpoint:   "/users",
				latencies:  []time.Duration{10 * time.Millisecond, 20 * time.Millisecond},
				percentile: 0.95,
				windowSize: 100,
				minSamples: 10,
			},
			wantLatency: 0,
			wantOK:      false,
		},
		{
			name: "given sufficient samples P50, then returns median",
			args: args{
				endpoint: "/users",
				latencies: []time.Duration{
					10 * time.Millisecond,
					20 * time.Millisecond,
					30 * time.Millisecond,
					40 * time.Millisecond,
					50 * time.Millisecond,
				},
				percentile: 0.50,
				windowSize: 100,
				minSamples: 3,
			},
			wantLatency: 30 * time.Millisecond,
			wantOK:      true,
		},
		{
			name: "given sufficient samples P90, then returns 90th percentile",
			args: args{
				endpoint: "/users",
				latencies: []time.Duration{
					10 * time.Millisecond,
					20 * time.Millisecond,
					30 * time.Millisecond,
					40 * time.Millisecond,
					50 * time.Millisecond,
					60 * time.Millisecond,
					70 * time.Millisecond,
					80 * time.Millisecond,
					90 * time.Millisecond,
					100 * time.Millisecond,
				},
				percentile: 0.90,
				windowSize: 100,
				minSamples: 5,
			},
			wantLatency: 90 * time.Millisecond, // Index 8 of 10 samples (0-based)
			wantOK:      true,
		},
		{
			name: "given unknown endpoint, then returns false",
			args: args{
				endpoint:   "/unknown",
				latencies:  nil,
				percentile: 0.95,
				windowSize: 100,
				minSamples: 10,
			},
			wantLatency: 0,
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewLatencyTracker(tt.args.windowSize, tt.args.minSamples)

			for _, latency := range tt.args.latencies {
				tracker.Record(tt.args.endpoint, latency)
			}

			gotLatency, gotOK := tracker.Percentile(tt.args.endpoint, tt.args.percentile)
			assert.Equal(t, tt.wantOK, gotOK)
			if tt.wantOK {
				assert.Equal(t, tt.wantLatency, gotLatency)
			}
		})
	}
}

func TestLatencyTracker_Reset(t *testing.T) {
	tracker := NewLatencyTracker(100, 5)
	tracker.Record("/users", 10*time.Millisecond)
	tracker.Record("/users", 20*time.Millisecond)

	assert.Equal(t, 2, tracker.Count("/users"))

	tracker.Reset()

	assert.Equal(t, 0, tracker.Count("/users"))
}

func TestLatencyTracker_PerEndpoint(t *testing.T) {
	tracker := NewLatencyTracker(100, 2)

	// Record different latencies for different endpoints
	tracker.Record("/users", 10*time.Millisecond)
	tracker.Record("/users", 20*time.Millisecond)
	tracker.Record("/users", 30*time.Millisecond)

	tracker.Record("/posts", 100*time.Millisecond)
	tracker.Record("/posts", 200*time.Millisecond)
	tracker.Record("/posts", 300*time.Millisecond)

	usersP50, usersOK := tracker.Percentile("/users", 0.50)
	postsP50, postsOK := tracker.Percentile("/posts", 0.50)

	assert.True(t, usersOK)
	assert.True(t, postsOK)

	// They should be different
	assert.Equal(t, 20*time.Millisecond, usersP50)
	assert.Equal(t, 200*time.Millisecond, postsP50)
}
