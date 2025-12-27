package httpclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewMetrics(t *testing.T) {
	tests := []struct {
		name    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "given valid meter, then creates all instruments",
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := sdkmetric.NewMeterProvider()
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)

			tt.wantErr(t, err)
			assert.NotNil(t, m)
			assert.NotNil(t, m.requestDuration)
			assert.NotNil(t, m.requestBodySize)
			assert.NotNil(t, m.responseBodySize)
			assert.NotNil(t, m.dnsDuration)
			assert.NotNil(t, m.tlsDuration)
			assert.NotNil(t, m.ttfb)
			assert.NotNil(t, m.activeRequests)
			assert.NotNil(t, m.requestErrors)
		})
	}
}

func TestRecordRequestDuration(t *testing.T) {
	type args struct {
		duration time.Duration
		attrs    []attribute.KeyValue
	}

	tests := []struct {
		name        string
		args        args
		wantMetrics bool
	}{
		{
			name: "given duration and attrs, then records metric",
			args: args{
				duration: 100 * time.Millisecond,
				attrs: []attribute.KeyValue{
					attribute.String("http.request.method", "GET"),
				},
			},
			wantMetrics: true,
		},
		{
			name: "given no attrs, then still records metric",
			args: args{
				duration: 50 * time.Millisecond,
				attrs:    nil,
			},
			wantMetrics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)
			require.NoError(t, err)

			ctx := context.Background()
			m.recordRequestDuration(ctx, tt.args.duration, tt.args.attrs)

			var rm metricdata.ResourceMetrics
			err = reader.Collect(ctx, &rm)
			require.NoError(t, err)

			if tt.wantMetrics {
				assert.NotEmpty(t, rm.ScopeMetrics)
			}
		})
	}
}

func TestRecordBodySizes(t *testing.T) {
	type args struct {
		size int64
	}

	tests := []struct {
		name       string
		args       args
		recordFunc string
	}{
		{
			name:       "given request body size, then records it",
			args:       args{size: 1024},
			recordFunc: "request",
		},
		{
			name:       "given response body size, then records it",
			args:       args{size: 2048},
			recordFunc: "response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)
			require.NoError(t, err)

			ctx := context.Background()
			if tt.recordFunc == "request" {
				m.recordRequestBodySize(ctx, tt.args.size, nil)
			} else {
				m.recordResponseBodySize(ctx, tt.args.size, nil)
			}

			var rm metricdata.ResourceMetrics
			err = reader.Collect(ctx, &rm)
			require.NoError(t, err)
			assert.NotEmpty(t, rm.ScopeMetrics)
		})
	}
}

func TestRecordNetworkTimings(t *testing.T) {
	type args struct {
		duration time.Duration
	}

	tests := []struct {
		name       string
		args       args
		metricType string
	}{
		{
			name:       "given DNS duration, then records it",
			args:       args{duration: 10 * time.Millisecond},
			metricType: "dns",
		},
		{
			name:       "given TLS duration, then records it",
			args:       args{duration: 50 * time.Millisecond},
			metricType: "tls",
		},
		{
			name:       "given TTFB, then records it",
			args:       args{duration: 100 * time.Millisecond},
			metricType: "ttfb",
		},
		{
			name:       "given connection duration, then records it",
			args:       args{duration: 20 * time.Millisecond},
			metricType: "connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)
			require.NoError(t, err)

			ctx := context.Background()
			switch tt.metricType {
			case "dns":
				m.recordDNSDuration(ctx, tt.args.duration, nil)
			case "tls":
				m.recordTLSDuration(ctx, tt.args.duration, nil)
			case "ttfb":
				m.recordTTFB(ctx, tt.args.duration, nil)
			case "connection":
				m.recordConnectionDuration(ctx, tt.args.duration, nil)
			}

			var rm metricdata.ResourceMetrics
			err = reader.Collect(ctx, &rm)
			require.NoError(t, err)
			assert.NotEmpty(t, rm.ScopeMetrics)
		})
	}
}

func TestRecordActiveRequests(t *testing.T) {
	t.Run("given start and end, then updates counter", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		defer mp.Shutdown(context.Background())

		meter := mp.Meter("test")
		m, err := newMetrics(meter)
		require.NoError(t, err)

		ctx := context.Background()
		m.recordActiveRequestStart(ctx, nil)
		m.recordActiveRequestEnd(ctx, nil)

		var rm metricdata.ResourceMetrics
		err = reader.Collect(ctx, &rm)
		require.NoError(t, err)
		assert.NotEmpty(t, rm.ScopeMetrics)
	})
}

func TestRecordError(t *testing.T) {
	type args struct {
		errorType string
		attrs     []attribute.KeyValue
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given error type, then records with attribute",
			args: args{
				errorType: "timeout",
				attrs:     []attribute.KeyValue{attribute.String("server.address", "example.com")},
			},
		},
		{
			name: "given error type without attrs, then records",
			args: args{
				errorType: "connection_refused",
				attrs:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)
			require.NoError(t, err)

			ctx := context.Background()
			m.recordError(ctx, tt.args.errorType, tt.args.attrs)

			var rm metricdata.ResourceMetrics
			err = reader.Collect(ctx, &rm)
			require.NoError(t, err)
			assert.NotEmpty(t, rm.ScopeMetrics)
		})
	}
}

func TestMetricsNilSafety(t *testing.T) {
	tests := []struct {
		name       string
		methodName string
	}{
		{
			name:       "given nil metrics, then recordRequestDuration does not panic",
			methodName: "requestDuration",
		},
		{
			name:       "given nil metrics, then recordRequestBodySize does not panic",
			methodName: "requestBodySize",
		},
		{
			name:       "given nil metrics, then recordResponseBodySize does not panic",
			methodName: "responseBodySize",
		},
		{
			name:       "given nil metrics, then recordDNSDuration does not panic",
			methodName: "dnsDuration",
		},
		{
			name:       "given nil metrics, then recordTLSDuration does not panic",
			methodName: "tlsDuration",
		},
		{name: "given nil metrics, then recordTTFB does not panic", methodName: "ttfb"},
		{
			name:       "given nil metrics, then recordActiveRequestStart does not panic",
			methodName: "activeStart",
		},
		{
			name:       "given nil metrics, then recordActiveRequestEnd does not panic",
			methodName: "activeEnd",
		},
		{name: "given nil metrics, then recordError does not panic", methodName: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m *metrics
			ctx := context.Background()

			assert.NotPanics(t, func() {
				switch tt.methodName {
				case "requestDuration":
					m.recordRequestDuration(ctx, time.Second, nil)
				case "requestBodySize":
					m.recordRequestBodySize(ctx, 100, nil)
				case "responseBodySize":
					m.recordResponseBodySize(ctx, 100, nil)
				case "dnsDuration":
					m.recordDNSDuration(ctx, time.Second, nil)
				case "tlsDuration":
					m.recordTLSDuration(ctx, time.Second, nil)
				case "ttfb":
					m.recordTTFB(ctx, time.Second, nil)
				case "activeStart":
					m.recordActiveRequestStart(ctx, nil)
				case "activeEnd":
					m.recordActiveRequestEnd(ctx, nil)
				case "error":
					m.recordError(ctx, "test", nil)
				}
			})
		})
	}
}

func TestMetricsNilHistogramSafety(t *testing.T) {
	t.Run("given metrics with nil histograms, then does not panic", func(t *testing.T) {
		m := &metrics{}
		ctx := context.Background()

		assert.NotPanics(t, func() {
			m.recordRequestDuration(ctx, time.Second, nil)
			m.recordRequestBodySize(ctx, 100, nil)
			m.recordResponseBodySize(ctx, 100, nil)
			m.recordConnectionDuration(ctx, time.Second, nil)
			m.recordDNSDuration(ctx, time.Second, nil)
			m.recordTLSDuration(ctx, time.Second, nil)
			m.recordTTFB(ctx, time.Second, nil)
			m.recordActiveRequestStart(ctx, nil)
			m.recordActiveRequestEnd(ctx, nil)
			m.recordError(ctx, "test", nil)
		})
	})
}
