package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestClassifyError(t *testing.T) {
	type args struct {
		err error
	}

	tests := []struct {
		name    string
		args    args
		wantVal string
	}{
		{
			name:    "given nil error, then returns empty",
			args:    args{err: nil},
			wantVal: "",
		},
		{
			name:    "given context cancelled, then returns cancelled",
			args:    args{err: context.Canceled},
			wantVal: ErrorTypeCancelled,
		},
		{
			name:    "given context deadline exceeded, then returns timeout",
			args:    args{err: context.DeadlineExceeded},
			wantVal: ErrorTypeTimeout,
		},
		{
			name:    "given wrapped context cancelled, then returns cancelled",
			args:    args{err: errors.Join(errors.New("request failed"), context.Canceled)},
			wantVal: ErrorTypeCancelled,
		},
		{
			name:    "given timeout in message, then returns timeout",
			args:    args{err: errors.New("connection timeout")},
			wantVal: ErrorTypeTimeout,
		},
		{
			name:    "given connection refused, then returns connection_refused",
			args:    args{err: errors.New("connection refused")},
			wantVal: ErrorTypeConnectionRefused,
		},
		{
			name:    "given connection reset, then returns connection_reset",
			args:    args{err: errors.New("connection reset by peer")},
			wantVal: ErrorTypeConnectionReset,
		},
		{
			name:    "given no such host, then returns dns_error",
			args:    args{err: errors.New("no such host")},
			wantVal: ErrorTypeDNSError,
		},
		{
			name:    "given tls error, then returns tls_error",
			args:    args{err: errors.New("tls handshake failed")},
			wantVal: ErrorTypeTLSError,
		},
		{
			name:    "given certificate error, then returns tls_error",
			args:    args{err: errors.New("certificate verify failed")},
			wantVal: ErrorTypeTLSError,
		},
		{
			name:    "given x509 error, then returns tls_error",
			args:    args{err: errors.New("x509: certificate signed by unknown authority")},
			wantVal: ErrorTypeTLSError,
		},
		{
			name:    "given eof error, then returns eof",
			args:    args{err: errors.New("unexpected eof")},
			wantVal: ErrorTypeEOF,
		},
		{
			name:    "given unknown error, then returns unknown",
			args:    args{err: errors.New("some random error")},
			wantVal: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.args.err)

			assert.Equal(t, tt.wantVal, result)
		})
	}
}

func TestClassifyError_NetErrors(t *testing.T) {
	type args struct {
		err error
	}

	tests := []struct {
		name    string
		args    args
		wantVal string
	}{
		{
			name: "given DNS error, then returns dns_error",
			args: args{
				err: &net.DNSError{Err: "no such host", Name: "example.com"},
			},
			wantVal: ErrorTypeDNSError,
		},
		{
			name: "given TLS record header error, then returns tls_error",
			args: args{
				err: &tls.RecordHeaderError{Msg: "tls: first record does not look like TLS"},
			},
			wantVal: ErrorTypeTLSError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.args.err)

			assert.Equal(t, tt.wantVal, result)
		})
	}
}

func TestErrorTypeFromStatusCode(t *testing.T) {
	type args struct {
		statusCode int
	}

	tests := []struct {
		name    string
		args    args
		wantVal string
	}{
		{name: "given 200, then returns empty", args: args{statusCode: 200}, wantVal: ""},
		{name: "given 201, then returns empty", args: args{statusCode: 201}, wantVal: ""},
		{name: "given 301, then returns empty", args: args{statusCode: 301}, wantVal: ""},
		{name: "given 400, then returns status code", args: args{statusCode: 400}, wantVal: "400"},
		{name: "given 401, then returns status code", args: args{statusCode: 401}, wantVal: "401"},
		{name: "given 404, then returns status code", args: args{statusCode: 404}, wantVal: "404"},
		{name: "given 500, then returns status code", args: args{statusCode: 500}, wantVal: "500"},
		{name: "given 503, then returns status code", args: args{statusCode: 503}, wantVal: "503"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errorTypeFromStatusCode(tt.args.statusCode)

			assert.Equal(t, tt.wantVal, result)
		})
	}
}

func TestNetworkTrace_AddTraceEvents(t *testing.T) {
	type args struct {
		dnsStart  time.Time
		dnsDone   time.Time
		dnsAddrs  []string
		connReuse bool
	}

	tests := []struct {
		name           string
		args           args
		wantEventCount int
	}{
		{
			name: "given full trace, then adds all events",
			args: func() args {
				now := time.Now()
				return args{
					dnsStart:  now,
					dnsDone:   now.Add(10 * time.Millisecond),
					dnsAddrs:  []string{"192.168.1.1"},
					connReuse: false,
				}
			}(),
			wantEventCount: 2, // dns.start, dns.done
		},
		{
			name:           "given empty trace, then adds no events",
			args:           args{},
			wantEventCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer tp.Shutdown(context.Background())

			tracer := tp.Tracer("test")
			_, span := tracer.Start(context.Background(), "test-span")

			nt := &networkTrace{
				dnsStart: tt.args.dnsStart,
				dnsDone:  tt.args.dnsDone,
				dnsAddrs: tt.args.dnsAddrs,
			}

			nt.addTraceEvents(span)
			span.End()

			spans := exporter.GetSpans()
			require.Len(t, spans, 1)

			if tt.wantEventCount > 0 {
				assert.GreaterOrEqual(t, len(spans[0].Events), tt.wantEventCount)
			} else {
				assert.Empty(t, spans[0].Events)
			}
		})
	}
}

func TestNetworkTrace_RecordTimingMetrics(t *testing.T) {
	type args struct {
		dnsStart time.Time
		dnsDone  time.Time
	}

	tests := []struct {
		name       string
		args       args
		nilMetrics bool
		wantPanic  bool
	}{
		{
			name: "given full trace and metrics, then records metrics",
			args: func() args {
				now := time.Now()
				return args{
					dnsStart: now,
					dnsDone:  now.Add(10 * time.Millisecond),
				}
			}(),
			nilMetrics: false,
			wantPanic:  false,
		},
		{
			name: "given nil metrics, then does not panic",
			args: func() args {
				now := time.Now()
				return args{
					dnsStart: now,
					dnsDone:  now.Add(10 * time.Millisecond),
				}
			}(),
			nilMetrics: true,
			wantPanic:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m *metrics
			if !tt.nilMetrics {
				reader := sdkmetric.NewManualReader()
				mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
				defer mp.Shutdown(context.Background())

				meter := mp.Meter("test")
				var err error
				m, err = newMetrics(meter)
				require.NoError(t, err)
			}

			nt := &networkTrace{
				dnsStart: tt.args.dnsStart,
				dnsDone:  tt.args.dnsDone,
			}

			if tt.wantPanic {
				assert.Panics(t, func() {
					nt.recordTimingMetrics(context.Background(), m, nil)
				})
			} else {
				assert.NotPanics(t, func() {
					nt.recordTimingMetrics(context.Background(), m, nil)
				})
			}
		})
	}
}

func TestCreateClientTrace(t *testing.T) {
	t.Run("given network trace, then creates client trace with hooks", func(t *testing.T) {
		nt := &networkTrace{}

		trace := createClientTrace(nt)

		require.NotNil(t, trace)
		assert.NotNil(t, trace.GetConn)
		assert.NotNil(t, trace.GotConn)
		assert.NotNil(t, trace.DNSStart)
		assert.NotNil(t, trace.DNSDone)
		assert.NotNil(t, trace.ConnectStart)
		assert.NotNil(t, trace.ConnectDone)
		assert.NotNil(t, trace.TLSHandshakeStart)
		assert.NotNil(t, trace.TLSHandshakeDone)
		assert.NotNil(t, trace.WroteRequest)
		assert.NotNil(t, trace.GotFirstResponseByte)
	})
}
