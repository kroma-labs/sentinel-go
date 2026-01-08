package httpclient

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestDefaultConfig(t *testing.T) {
	tests := []struct {
		name            string
		wantTimeout     time.Duration
		wantMaxIdle     int
		wantMaxPerHost  int
		wantCompression bool // DisableCompression value
	}{
		{
			name:            "given default config, then returns balanced settings",
			wantTimeout:     15 * time.Second,
			wantMaxIdle:     100,
			wantMaxPerHost:  20,
			wantCompression: true, // DisableCompression = true (compression disabled)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()

			assert.Equal(t, tt.wantTimeout, cfg.Timeout)
			assert.Equal(t, tt.wantMaxIdle, cfg.MaxIdleConns)
			assert.Equal(t, tt.wantMaxPerHost, cfg.MaxIdleConnsPerHost)
			assert.Equal(t, tt.wantCompression, cfg.DisableCompression)
			assert.Equal(t, 100, cfg.MaxConnsPerHost)
			assert.Equal(t, 90*time.Second, cfg.IdleConnTimeout)
			assert.Equal(t, 5*time.Second, cfg.DialTimeout)
			assert.Equal(t, 10*time.Second, cfg.TLSHandshakeTimeout)
			assert.Equal(t, 64*1024, cfg.WriteBufferSize)
			assert.Equal(t, 64*1024, cfg.ReadBufferSize)
			assert.False(t, cfg.DisableKeepAlives)
			assert.False(t, cfg.ForceHTTP2)
		})
	}
}

func TestHighThroughputConfig(t *testing.T) {
	tests := []struct {
		name           string
		wantTimeout    time.Duration
		wantMaxIdle    int
		wantMaxPerHost int
		wantBufferSize int
	}{
		{
			name:           "given high throughput config, then has aggressive pooling",
			wantTimeout:    30 * time.Second,
			wantMaxIdle:    500,
			wantMaxPerHost: 100,
			wantBufferSize: 128 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := HighThroughputConfig()

			assert.Equal(t, tt.wantTimeout, cfg.Timeout)
			assert.Equal(t, tt.wantMaxIdle, cfg.MaxIdleConns)
			assert.Equal(t, tt.wantMaxPerHost, cfg.MaxIdleConnsPerHost)
			assert.Equal(t, tt.wantBufferSize, cfg.WriteBufferSize)
			assert.Equal(t, tt.wantBufferSize, cfg.ReadBufferSize)
			assert.Equal(t, 0, cfg.MaxConnsPerHost) // Unlimited
		})
	}
}

func TestLowLatencyConfig(t *testing.T) {
	tests := []struct {
		name            string
		wantTimeout     time.Duration
		wantDialTimeout time.Duration
		wantForceHTTP2  bool
	}{
		{
			name:            "given low latency config, then has fast timeouts",
			wantTimeout:     5 * time.Second,
			wantDialTimeout: 2 * time.Second,
			wantForceHTTP2:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LowLatencyConfig()

			assert.Equal(t, tt.wantTimeout, cfg.Timeout)
			assert.Equal(t, tt.wantDialTimeout, cfg.DialTimeout)
			assert.Equal(t, tt.wantForceHTTP2, cfg.ForceHTTP2)
			assert.Equal(t, 3*time.Second, cfg.ResponseHeaderTimeout)
			assert.Equal(t, 150*time.Millisecond, cfg.FallbackDelay)
		})
	}
}

func TestConservativeConfig(t *testing.T) {
	tests := []struct {
		name           string
		wantTimeout    time.Duration
		wantMaxIdle    int
		wantBufferSize int
	}{
		{
			name:           "given conservative config, then conserves resources",
			wantTimeout:    10 * time.Second,
			wantMaxIdle:    20,
			wantBufferSize: 4 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ConservativeConfig()

			assert.Equal(t, tt.wantTimeout, cfg.Timeout)
			assert.Equal(t, tt.wantMaxIdle, cfg.MaxIdleConns)
			assert.Equal(t, tt.wantBufferSize, cfg.WriteBufferSize)
			assert.Equal(t, 5, cfg.MaxIdleConnsPerHost)
			assert.Equal(t, 30*time.Second, cfg.IdleConnTimeout)
		})
	}
}

func TestWithConfig(t *testing.T) {
	type args struct {
		config Config
	}

	tests := []struct {
		name        string
		args        args
		wantTimeout time.Duration
		wantMaxIdle int
	}{
		{
			name: "given custom config, then applies values",
			args: args{
				config: Config{
					Timeout:             10 * time.Second,
					MaxIdleConnsPerHost: 50,
				},
			},
			wantTimeout: 10 * time.Second,
			wantMaxIdle: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithConfig(tt.args.config))

			assert.Equal(t, tt.wantTimeout, cfg.httpConfig.Timeout)
			assert.Equal(t, tt.wantMaxIdle, cfg.httpConfig.MaxIdleConnsPerHost)
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name             string
		wantTimeout      time.Duration
		wantNetworkTrace bool
		wantProxyFromEnv bool
		wantTracerNotNil bool
		wantMeterNotNil  bool
	}{
		{
			name:             "given no options, then uses defaults",
			wantTimeout:      15 * time.Second,
			wantNetworkTrace: true,
			wantProxyFromEnv: true,
			wantTracerNotNil: true,
			wantMeterNotNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig()

			assert.Equal(t, tt.wantTimeout, cfg.httpConfig.Timeout)
			assert.Equal(t, tt.wantNetworkTrace, cfg.EnableNetworkTrace)
			assert.Equal(t, tt.wantProxyFromEnv, cfg.ProxyFromEnvironment)
			if tt.wantTracerNotNil {
				assert.NotNil(t, cfg.Tracer)
			}
			if tt.wantMeterNotNil {
				assert.NotNil(t, cfg.Meter)
			}
		})
	}
}

func TestWithServiceName(t *testing.T) {
	type args struct {
		serviceName string
	}

	tests := []struct {
		name            string
		args            args
		wantServiceName string
	}{
		{
			name:            "given service name, then sets it",
			args:            args{serviceName: "my-service"},
			wantServiceName: "my-service",
		},
		{
			name:            "given empty service name, then sets empty",
			args:            args{serviceName: ""},
			wantServiceName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithServiceName(tt.args.serviceName))

			assert.Equal(t, tt.wantServiceName, cfg.ServiceName)
		})
	}
}

func TestWithTracerProvider(t *testing.T) {
	t.Run("given custom tracer provider, then uses it", func(t *testing.T) {
		tp := sdktrace.NewTracerProvider()
		defer tp.Shutdown(context.Background())

		cfg := newConfig(WithTracerProvider(tp))

		assert.Equal(t, tp, cfg.TracerProvider)
	})
}

func TestWithMeterProvider(t *testing.T) {
	t.Run("given custom meter provider, then uses it", func(t *testing.T) {
		mp := noop.NewMeterProvider()

		cfg := newConfig(WithMeterProvider(mp))

		assert.Equal(t, mp, cfg.MeterProvider)
	})
}

func TestWithDisableNetworkTrace(t *testing.T) {
	tests := []struct {
		name             string
		wantNetworkTrace bool
	}{
		{
			name:             "given disable option, then disables network trace",
			wantNetworkTrace: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithDisableNetworkTrace())

			assert.Equal(t, tt.wantNetworkTrace, cfg.EnableNetworkTrace)
		})
	}
}

func TestWithProxyFromEnvironment(t *testing.T) {
	type args struct {
		enabled bool
	}

	tests := []struct {
		name    string
		args    args
		wantVal bool
	}{
		{
			name:    "given true, then enables proxy from env",
			args:    args{enabled: true},
			wantVal: true,
		},
		{
			name:    "given false, then disables proxy from env",
			args:    args{enabled: false},
			wantVal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithProxyFromEnvironment(tt.args.enabled))

			assert.Equal(t, tt.wantVal, cfg.ProxyFromEnvironment)
		})
	}
}

func TestBuildTransport(t *testing.T) {
	type args struct {
		maxIdleConns    int
		maxIdlePerHost  int
		idleConnTimeout time.Duration
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given custom pool settings, then builds transport",
			args: args{
				maxIdleConns:    50,
				maxIdlePerHost:  25,
				idleConnTimeout: 60 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customCfg := DefaultConfig()
			customCfg.MaxIdleConns = tt.args.maxIdleConns
			customCfg.MaxIdleConnsPerHost = tt.args.maxIdlePerHost
			customCfg.IdleConnTimeout = tt.args.idleConnTimeout

			cfg := newConfig(WithConfig(customCfg))
			transport := cfg.buildTransport()

			require.NotNil(t, transport)
			assert.Equal(t, tt.args.maxIdleConns, transport.MaxIdleConns)
			assert.Equal(t, tt.args.maxIdlePerHost, transport.MaxIdleConnsPerHost)
			assert.Equal(t, tt.args.idleConnTimeout, transport.IdleConnTimeout)
		})
	}
}

func TestBaseAttributes(t *testing.T) {
	type args struct {
		serviceName string
	}

	tests := []struct {
		name      string
		args      args
		wantLen   int
		wantKey   string
		wantValue string
	}{
		{
			name:      "given service name, then returns attribute",
			args:      args{serviceName: "test-service"},
			wantLen:   1,
			wantKey:   "http.client.name",
			wantValue: "test-service",
		},
		{
			name:    "given no service name, then returns empty",
			args:    args{serviceName: ""},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithServiceName(tt.args.serviceName))

			attrs := cfg.baseAttributes()

			assert.Len(t, attrs, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantKey, string(attrs[0].Key))
				assert.Equal(t, tt.wantValue, attrs[0].Value.AsString())
			}
		})
	}
}

func TestWithRetryConfig(t *testing.T) {
	retryCfg := AggressiveRetryConfig()
	cfg := newConfig(WithRetryConfig(retryCfg))
	assert.Equal(t, retryCfg, cfg.RetryConfig)
}

func TestWithRetryDisabled(t *testing.T) {
	cfg := newConfig(WithRetryDisabled())
	assert.False(t, cfg.RetryConfig.IsEnabled())
}

func TestWithRetryClassifier(t *testing.T) {
	classifier := func(_ *http.Response, _ error) bool { return true }
	cfg := newConfig(WithRetryClassifier(classifier))
	require.NotNil(t, cfg.RetryClassifier)
	assert.True(t, cfg.RetryClassifier(nil, nil))
}

func TestWithRetryBackOff(t *testing.T) {
	b := backoff.NewConstantBackOff(1 * time.Second)
	cfg := newConfig(WithRetryBackOff(b))
	assert.Equal(t, b, cfg.RetryBackOff)
}

func TestWithTieredRetry(t *testing.T) {
	tiers := []RetryTier{{MaxRetries: 1, Delay: time.Minute}}
	cfg := newConfig(WithTieredRetry(tiers, 5*time.Minute))
	require.IsType(t, &TieredRetryBackOff{}, cfg.RetryBackOff)
}

func TestWithTieredRetry_Default(t *testing.T) {
	cfg := newConfig(WithTieredRetry(nil, 5*time.Minute))
	require.IsType(t, &TieredRetryBackOff{}, cfg.RetryBackOff)
	tb := cfg.RetryBackOff.(*TieredRetryBackOff)
	assert.Len(t, tb.Tiers, 2)
}

func TestWithFilter(t *testing.T) {
	filter := func(_ *http.Request) bool { return false }
	cfg := newConfig(WithFilter(filter))
	require.Len(t, cfg.Filters, 1)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	assert.False(t, cfg.Filters[0](req))
}

func TestWithSpanNameFormatter(t *testing.T) {
	formatter := func(_ string, _ *http.Request) string { return "custom-span" }
	cfg := newConfig(WithSpanNameFormatter(formatter))
	require.NotNil(t, cfg.SpanNameFormatter)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	assert.Equal(t, "custom-span", cfg.SpanNameFormatter("GET", req))
}

func TestWithSpanOptions(t *testing.T) {
	opts := trace.WithAttributes(attribute.String("key", "value"))
	cfg := newConfig(WithSpanOptions(opts))
	assert.Len(t, cfg.SpanStartOptions, 1)
}

func TestWithMetricAttributesFn(t *testing.T) {
	fn := func(_ *http.Request) []attribute.KeyValue {
		return []attribute.KeyValue{attribute.String("custom", "val")}
	}
	cfg := newConfig(WithMetricAttributesFn(fn))
	assert.NotNil(t, cfg.MetricAttributesFn)
}

func TestWithPropagators(t *testing.T) {
	prop := propagation.NewCompositeTextMapPropagator()
	cfg := newConfig(WithPropagators(prop))
	assert.Equal(t, prop, cfg.Propagators)
}

func TestWithTLSConfig(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	cfg := newConfig(WithTLSConfig(tlsConfig))
	assert.Equal(t, tlsConfig, cfg.TLSConfig)
}

func TestWithProxyURL(t *testing.T) {
	proxyURL, _ := url.Parse("http://proxy.example.com:8080")
	cfg := newConfig(WithProxyURL(proxyURL))
	assert.NotNil(t, cfg)
	assert.Equal(t, proxyURL, cfg.ProxyURL)
}
