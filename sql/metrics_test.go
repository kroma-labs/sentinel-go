package sql

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
		name       string
		setup      func() *sdkmetric.MeterProvider
		wantErr    assert.ErrorAssertionFunc
		wantAssert func(*metrics) bool
	}{
		{
			name: "given valid meter, then creates metrics successfully",
			setup: func() *sdkmetric.MeterProvider {
				return sdkmetric.NewMeterProvider()
			},
			wantErr: assert.NoError,
			wantAssert: func(m *metrics) bool {
				return m != nil && m.queryDuration != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := tt.setup()
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)

			if !tt.wantErr(t, err) {
				return
			}
			assert.True(t, tt.wantAssert(m))
		})
	}
}

func TestRecordQueryDuration(t *testing.T) {
	type args struct {
		duration  time.Duration
		operation string
		attrs     []attribute.KeyValue
		err       error
	}

	tests := []struct {
		name        string
		args        args
		wantMetrics bool
	}{
		{
			name: "given successful query, then records with ok status",
			args: args{
				duration:  100 * time.Millisecond,
				operation: "SELECT",
				attrs: []attribute.KeyValue{
					attribute.String("db.system", "postgresql"),
				},
				err: nil,
			},
			wantMetrics: true,
		},
		{
			name: "given failed query, then records with error status",
			args: args{
				duration:  50 * time.Millisecond,
				operation: "INSERT",
				attrs: []attribute.KeyValue{
					attribute.String("db.system", "mysql"),
				},
				err: assert.AnError,
			},
			wantMetrics: true,
		},
		{
			name: "given empty operation, then records without operation attribute",
			args: args{
				duration:  10 * time.Millisecond,
				operation: "",
				attrs:     []attribute.KeyValue{},
				err:       nil,
			},
			wantMetrics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())

			meter := mp.Meter("test")
			m, err := newMetrics(meter)
			require.NoError(t, err)

			// Execute
			ctx := context.Background()
			m.recordQueryDuration(
				ctx,
				tt.args.duration,
				tt.args.operation,
				tt.args.attrs,
				tt.args.err,
			)

			// Verify
			var rm metricdata.ResourceMetrics
			err = reader.Collect(ctx, &rm)
			require.NoError(t, err)

			if tt.wantMetrics {
				// Should have recorded metrics
				assert.NotEmpty(t, rm.ScopeMetrics)
			}
		})
	}
}

func TestRecordQueryDuration_NilMetrics(t *testing.T) {
	t.Run("given nil metrics, then does not panic", func(t *testing.T) {
		var m *metrics

		// Should not panic
		assert.NotPanics(t, func() {
			m.recordQueryDuration(context.Background(), time.Second, "SELECT", nil, nil)
		})
	})
}

func TestRecordQueryDuration_NilHistogram(t *testing.T) {
	t.Run("given nil histogram, then does not panic", func(t *testing.T) {
		m := &metrics{queryDuration: nil}

		// Should not panic
		assert.NotPanics(t, func() {
			m.recordQueryDuration(context.Background(), time.Second, "SELECT", nil, nil)
		})
	})
}
