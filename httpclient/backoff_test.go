package httpclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinearBackOff(t *testing.T) {
	type args struct {
		initialInterval time.Duration
		increment       time.Duration
		maxInterval     time.Duration
		jitterFactor    float64
	}
	tests := []struct {
		name             string
		args             args
		attempts         int
		wantMinIntervals []time.Duration
		wantMaxIntervals []time.Duration
	}{
		{
			name: "given default config, then increases linearly",
			args: args{
				initialInterval: 500 * time.Millisecond,
				increment:       500 * time.Millisecond,
				maxInterval:     30 * time.Second,
				jitterFactor:    0, // No jitter for predictable testing
			},
			attempts: 5,
			wantMinIntervals: []time.Duration{
				500 * time.Millisecond, // Attempt 1
				1 * time.Second,        // Attempt 2
				1500 * time.Millisecond,
				2 * time.Second,
				2500 * time.Millisecond,
			},
			wantMaxIntervals: []time.Duration{
				500 * time.Millisecond,
				1 * time.Second,
				1500 * time.Millisecond,
				2 * time.Second,
				2500 * time.Millisecond,
			},
		},
		{
			name: "given max interval, then caps at max",
			args: args{
				initialInterval: 1 * time.Second,
				increment:       1 * time.Second,
				maxInterval:     3 * time.Second,
				jitterFactor:    0,
			},
			attempts: 5,
			wantMinIntervals: []time.Duration{
				1 * time.Second, // Attempt 1
				2 * time.Second, // Attempt 2
				3 * time.Second, // Attempt 3 (capped)
				3 * time.Second, // Attempt 4 (capped)
				3 * time.Second, // Attempt 5 (capped)
			},
			wantMaxIntervals: []time.Duration{
				1 * time.Second,
				2 * time.Second,
				3 * time.Second,
				3 * time.Second,
				3 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &LinearBackOff{
				InitialInterval: tt.args.initialInterval,
				Increment:       tt.args.increment,
				MaxInterval:     tt.args.maxInterval,
				JitterFactor:    tt.args.jitterFactor,
			}
			b.Reset()

			for i := 0; i < tt.attempts; i++ {
				interval := b.NextBackOff()
				assert.GreaterOrEqual(
					t,
					interval,
					tt.wantMinIntervals[i],
					"attempt %d", i+1,
				)
				assert.LessOrEqual(t, interval, tt.wantMaxIntervals[i], "attempt %d", i+1)
			}
		})
	}
}

func TestLinearBackOff_Reset(t *testing.T) {
	b := NewLinearBackOff()

	// Make some attempts
	_ = b.NextBackOff()
	_ = b.NextBackOff()
	_ = b.NextBackOff()

	// Reset
	b.Reset()

	// First attempt should be back to initial
	b.JitterFactor = 0
	interval := b.NextBackOff()
	assert.Equal(t, b.InitialInterval, interval)
}

func TestDecorrelatedJitterBackOff(t *testing.T) {
	tests := []struct {
		name     string
		base     time.Duration
		cap      time.Duration
		attempts int
	}{
		{
			name:     "given default config, then intervals are within bounds",
			base:     500 * time.Millisecond,
			cap:      30 * time.Second,
			attempts: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &DecorrelatedJitterBackOff{
				Base: tt.base,
				Cap:  tt.cap,
			}
			b.Reset()

			for i := 0; i < tt.attempts; i++ {
				interval := b.NextBackOff()
				assert.GreaterOrEqual(t, interval, tt.base, "attempt %d", i+1)
				assert.LessOrEqual(t, interval, tt.cap, "attempt %d", i+1)
			}
		})
	}
}

func TestDecorrelatedJitterBackOff_Reset(t *testing.T) {
	b := NewDecorrelatedJitterBackOff()

	// Make some attempts
	_ = b.NextBackOff()
	_ = b.NextBackOff()

	// Reset
	b.Reset()

	// After reset, sleep should be back to base
	assert.Equal(t, b.Base, b.sleep)
}

func TestConstantBackOffWithJitter(t *testing.T) {
	tests := []struct {
		name         string
		interval     time.Duration
		jitterFactor float64
		wantMin      time.Duration
		wantMax      time.Duration
	}{
		{
			name:         "given no jitter, then returns exact interval",
			interval:     1 * time.Second,
			jitterFactor: 0,
			wantMin:      1 * time.Second,
			wantMax:      1 * time.Second,
		},
		{
			name:         "given 50% jitter, then returns interval within range",
			interval:     1 * time.Second,
			jitterFactor: 0.5,
			wantMin:      500 * time.Millisecond,
			wantMax:      1500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ConstantBackOffWithJitter{
				Interval:     tt.interval,
				JitterFactor: tt.jitterFactor,
			}

			for i := 0; i < 10; i++ {
				interval := b.NextBackOff()
				assert.GreaterOrEqual(t, interval, tt.wantMin, "attempt %d", i+1)
				assert.LessOrEqual(t, interval, tt.wantMax, "attempt %d", i+1)
			}
		})
	}
}

func TestTieredRetryBackOff(t *testing.T) {
	tests := []struct {
		name       string
		tiers      []RetryTier
		maxDelay   time.Duration
		attempts   int
		wantTiers  []int
		wantDelays []time.Duration
	}{
		{
			name: "given 2 tiers, then progresses through tiers correctly",
			tiers: []RetryTier{
				{MaxRetries: 3, Delay: 1 * time.Minute},
				{MaxRetries: 2, Delay: 2 * time.Minute},
			},
			maxDelay:  10 * time.Minute,
			attempts:  8,
			wantTiers: []int{1, 1, 1, 2, 2, 3, 3, 3},
			wantDelays: []time.Duration{
				1 * time.Minute, // Tier 1, attempt 1
				1 * time.Minute, // Tier 1, attempt 2
				1 * time.Minute, // Tier 1, attempt 3
				2 * time.Minute, // Tier 2, attempt 4
				2 * time.Minute, // Tier 2, attempt 5
				1 * time.Minute, // Exponential: 2^(6-5-1) = 2^0 = 1 min
				2 * time.Minute, // Exponential: 2^(7-5-1) = 2^1 = 2 min
				4 * time.Minute, // Exponential: 2^(8-5-1) = 2^2 = 4 min
			},
		},
		{
			name: "given exponential hits max, then caps at max delay",
			tiers: []RetryTier{
				{MaxRetries: 2, Delay: 1 * time.Minute},
			},
			maxDelay:  5 * time.Minute,
			attempts:  7,
			wantTiers: []int{1, 1, 2, 2, 2, 2, 2},
			wantDelays: []time.Duration{
				1 * time.Minute, // Tier 1
				1 * time.Minute, // Tier 1
				1 * time.Minute, // Exp: 2^0 = 1 min
				2 * time.Minute, // Exp: 2^1 = 2 min
				4 * time.Minute, // Exp: 2^2 = 4 min
				5 * time.Minute, // Exp: 2^3 = 8 min, capped at 5
				5 * time.Minute, // Capped at 5
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create struct directly to avoid constructor defaulting jitter
			total := 0
			for _, tier := range tt.tiers {
				total += tier.MaxRetries
			}
			b := &TieredRetryBackOff{
				Tiers:             tt.tiers,
				MaxDelay:          tt.maxDelay,
				JitterFactor:      0, // Disable jitter for deterministic testing
				totalFixedRetries: total,
			}

			for i := 0; i < tt.attempts; i++ {
				interval := b.NextBackOff()
				tier := b.CurrentTier()

				assert.Equal(t, tt.wantTiers[i], tier, "attempt %d tier", i+1)
				assert.Equal(t, tt.wantDelays[i], interval, "attempt %d delay", i+1)
			}
		})
	}
}

func TestTieredRetryBackOff_Reset(t *testing.T) {
	b := DefaultTieredRetryBackOff()
	b.JitterFactor = 0 // Disable jitter for deterministic test

	// Make some attempts
	_ = b.NextBackOff()
	_ = b.NextBackOff()
	_ = b.NextBackOff()
	require.Equal(t, 3, b.attempt)

	// Reset
	b.Reset()

	// Should be back to initial state
	assert.Equal(t, 0, b.attempt)
	assert.Equal(t, 1, b.CurrentTier())
}

func TestTieredRetryBackOff_WithJitter(t *testing.T) {
	b := NewTieredRetryBackOff(
		[]RetryTier{
			{MaxRetries: 3, Delay: 1 * time.Minute},
		},
		10*time.Minute,
		0.5, // 50% jitter
	)

	// With 50% jitter, 1 minute should be between 30s and 90s
	for i := 0; i < 10; i++ {
		b.Reset()
		interval := b.NextBackOff()

		assert.GreaterOrEqual(t, interval, 30*time.Second, "attempt %d", i+1)
		assert.LessOrEqual(t, interval, 90*time.Second, "attempt %d", i+1)
	}
}

func TestDefaultTieredRetryBackOff(t *testing.T) {
	b := DefaultTieredRetryBackOff()

	assert.Len(t, b.Tiers, 2)
	assert.Equal(t, 5, b.Tiers[0].MaxRetries)
	assert.Equal(t, 1*time.Minute, b.Tiers[0].Delay)
	assert.Equal(t, 5, b.Tiers[1].MaxRetries)
	assert.Equal(t, 2*time.Minute, b.Tiers[1].Delay)
	assert.Equal(t, 10*time.Minute, b.MaxDelay)
	assert.InDelta(t, 0.5, b.JitterFactor, 0.001)
	assert.Equal(t, 10, b.totalFixedRetries)
}

func TestNewTieredRetryBackOff_DefaultJitter(t *testing.T) {
	// When jitter is 0 or negative, should use default
	b := NewTieredRetryBackOff(
		[]RetryTier{{MaxRetries: 3, Delay: 1 * time.Minute}},
		10*time.Minute,
		0, // Should default to 0.5
	)

	assert.InDelta(t, DefaultJitterFactor, b.JitterFactor, 0.001)
}

func TestApplyJitter(t *testing.T) {
	tests := []struct {
		name         string
		interval     time.Duration
		jitterFactor float64
		wantMin      time.Duration
		wantMax      time.Duration
	}{
		{
			name:         "given 0 jitter, then returns exact interval",
			interval:     1 * time.Second,
			jitterFactor: 0,
			wantMin:      1 * time.Second,
			wantMax:      1 * time.Second,
		},
		{
			name:         "given negative jitter, then returns exact interval",
			interval:     1 * time.Second,
			jitterFactor: -0.5,
			wantMin:      1 * time.Second,
			wantMax:      1 * time.Second,
		},
		{
			name:         "given 100% jitter, then returns 0 to 2x interval",
			interval:     1 * time.Second,
			jitterFactor: 1.0,
			wantMin:      0,
			wantMax:      2 * time.Second,
		},
		{
			name:         "given jitter > 1, then clamps to 1",
			interval:     1 * time.Second,
			jitterFactor: 2.0, // Should be clamped to 1.0
			wantMin:      0,
			wantMax:      2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times due to randomness
			for i := 0; i < 20; i++ {
				result := applyJitter(tt.interval, tt.jitterFactor)
				assert.GreaterOrEqual(t, result, tt.wantMin)
				assert.LessOrEqual(t, result, tt.wantMax)
			}
		})
	}
}

func TestExponentialBackOffFromConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	b := ExponentialBackOffFromConfig(cfg)

	assert.Equal(t, cfg.InitialInterval, b.InitialInterval)
	assert.InDelta(t, cfg.JitterFactor, b.RandomizationFactor, 0.001)
	assert.InDelta(t, cfg.Multiplier, b.Multiplier, 0.001)
	assert.Equal(t, cfg.MaxInterval, b.MaxInterval)
}

func TestExponentialBackOffFromConfig_DefaultJitter(t *testing.T) {
	cfg := RetryConfig{
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		JitterFactor:    0, // Should default to 0.5
	}

	b := ExponentialBackOffFromConfig(cfg)

	assert.InDelta(t, DefaultJitterFactor, b.RandomizationFactor, 0.001)
}
