package httpclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, uint(3), cfg.MaxRetries)
	assert.Equal(t, 500*time.Millisecond, cfg.InitialInterval)
	assert.Equal(t, 30*time.Second, cfg.MaxInterval)
	assert.Equal(t, 2*time.Minute, cfg.MaxElapsedTime)
	assert.InDelta(t, 2.0, cfg.Multiplier, 0.001)
	assert.InDelta(t, 0.5, cfg.JitterFactor, 0.001)
}

func TestAggressiveRetryConfig(t *testing.T) {
	cfg := AggressiveRetryConfig()

	assert.Equal(t, uint(5), cfg.MaxRetries)
	assert.Equal(t, 200*time.Millisecond, cfg.InitialInterval)
	assert.Equal(t, 60*time.Second, cfg.MaxInterval)
	assert.Equal(t, 5*time.Minute, cfg.MaxElapsedTime)
	assert.InDelta(t, 2.0, cfg.Multiplier, 0.001)
	assert.InDelta(t, 0.5, cfg.JitterFactor, 0.001)
}

func TestConservativeRetryConfig(t *testing.T) {
	cfg := ConservativeRetryConfig()

	assert.Equal(t, uint(2), cfg.MaxRetries)
	assert.Equal(t, 1*time.Second, cfg.InitialInterval)
	assert.Equal(t, 10*time.Second, cfg.MaxInterval)
	assert.Equal(t, 30*time.Second, cfg.MaxElapsedTime)
	assert.InDelta(t, 2.0, cfg.Multiplier, 0.001)
	assert.InDelta(t, 0.5, cfg.JitterFactor, 0.001)
}

func TestNoRetryConfig(t *testing.T) {
	cfg := NoRetryConfig()

	assert.Equal(t, uint(0), cfg.MaxRetries)
	assert.Equal(t, time.Duration(0), cfg.InitialInterval)
	assert.Equal(t, time.Duration(0), cfg.MaxInterval)
	assert.Equal(t, time.Duration(0), cfg.MaxElapsedTime)
	assert.InDelta(t, 0.0, cfg.Multiplier, 0.001)
	assert.InDelta(t, 0.0, cfg.JitterFactor, 0.001)
}

func TestRetryConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config RetryConfig
		want   bool
	}{
		{
			name:   "given default config, then returns true",
			config: DefaultRetryConfig(),
			want:   true,
		},
		{
			name:   "given no retry config, then returns false",
			config: NoRetryConfig(),
			want:   false,
		},
		{
			name: "given MaxRetries > 0, then returns true",
			config: RetryConfig{
				MaxRetries: 1,
			},
			want: true,
		},
		{
			name: "given MaxRetries = 0, then returns false",
			config: RetryConfig{
				MaxRetries: 0,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsEnabled()
			assert.Equal(t, tt.want, got)
		})
	}
}
