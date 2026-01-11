package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNew(t *testing.T) {
	type args struct {
		config      *Config
		serviceName string
	}

	tests := []struct {
		name        string
		args        args
		wantTimeout time.Duration
	}{
		{
			name:        "given no options, then uses default timeout",
			args:        args{},
			wantTimeout: 15 * time.Second,
		},
		{
			name: "given custom config, then uses that timeout",
			args: args{
				config: &Config{Timeout: 10 * time.Second},
			},
			wantTimeout: 10 * time.Second,
		},
		{
			name: "given service name, then creates instrumented client",
			args: args{
				serviceName: "test-service",
			},
			wantTimeout: 15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.args.config != nil {
				opts = append(opts, WithConfig(*tt.args.config))
			}
			if tt.args.serviceName != "" {
				opts = append(opts, WithServiceName(tt.args.serviceName))
			}

			client := New(opts...)

			assert.NotNil(t, client)
			assert.NotNil(t, client.HTTP().Transport)
			assert.Equal(t, tt.wantTimeout, client.HTTP().Timeout)

			// Transport can be either retryTransport (wrapping otelTransport) or
			// otelTransport directly (if retries disabled)
			_, isRetry := client.HTTP().Transport.(*retryTransport)
			_, isOtel := client.HTTP().Transport.(*otelTransport)
			assert.True(t, isRetry || isOtel, "expected retryTransport or otelTransport")
		})
	}
}

func TestNew_RequestExecution(t *testing.T) {
	type args struct {
		serverStatus int
	}

	tests := []struct {
		name           string
		args           args
		wantStatusCode int
		wantSpanCount  int
	}{
		{
			name:           "given server returns 200, then succeeds",
			args:           args{serverStatus: http.StatusOK},
			wantStatusCode: http.StatusOK,
			wantSpanCount:  1,
		},
		{
			name:           "given server returns 404, then records status",
			args:           args{serverStatus: http.StatusNotFound},
			wantStatusCode: http.StatusNotFound,
			wantSpanCount:  1,
		},
		{
			name:           "given server returns 500, then records status",
			args:           args{serverStatus: http.StatusInternalServerError},
			wantStatusCode: http.StatusInternalServerError,
			wantSpanCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tt.args.serverStatus)
				}),
			)
			defer server.Close()

			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			mp := sdkmetric.NewMeterProvider()
			defer tp.Shutdown(context.Background())
			defer mp.Shutdown(context.Background())

			client := New(
				WithTracerProvider(tp),
				WithMeterProvider(mp),
				WithServiceName("test-service"),
			)

			req, err := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
			require.NoError(t, err)

			resp, err := client.HTTP().Do(req)
			require.NoError(t, err)

			// Must consume and close body before checking spans
			// (span ends when body is closed, not immediately after RoundTrip)
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			assert.Equal(t, tt.wantStatusCode, resp.StatusCode)

			spans := exporter.GetSpans()
			assert.Len(t, spans, tt.wantSpanCount)
			if tt.wantSpanCount > 0 {
				assert.Equal(t, "HTTP GET", spans[0].Name)
			}
		})
	}
}

func TestNewTransport(t *testing.T) {
	type args struct {
		serviceName string
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given base transport, then wraps with instrumentation",
			args: args{serviceName: "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewTransport(
				http.DefaultTransport,
				WithServiceName(tt.args.serviceName),
			)

			assert.NotNil(t, transport)
			_, ok := transport.(*otelTransport)
			assert.True(t, ok)
		})
	}
}

func TestNewWithTransport(t *testing.T) {
	type args struct {
		maxIdlePerHost int
		serviceName    string
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given custom transport, then creates client with it",
			args: args{
				maxIdlePerHost: 50,
				serviceName:    "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseTransport := &http.Transport{
				MaxIdleConnsPerHost: tt.args.maxIdlePerHost,
			}

			client := NewWithTransport(
				baseTransport,
				WithServiceName(tt.args.serviceName),
			)

			assert.NotNil(t, client)
			assert.NotNil(t, client.HTTP().Transport)
		})
	}
}

func TestWrapClient(t *testing.T) {
	type args struct {
		timeout      time.Duration
		hasTransport bool
		serviceName  string
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given client with transport, then wraps it",
			args: args{
				timeout:      15 * time.Second,
				hasTransport: true,
				serviceName:  "test",
			},
		},
		{
			name: "given client without transport, then uses default",
			args: args{
				timeout:      20 * time.Second,
				hasTransport: false,
				serviceName:  "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Timeout: tt.args.timeout}
			if tt.args.hasTransport {
				client.Transport = http.DefaultTransport
			}

			wrapped := WrapClient(client, WithServiceName(tt.args.serviceName))

			assert.Equal(t, client, wrapped.HTTP())
			assert.NotNil(t, wrapped.HTTP().Transport)
			_, ok := wrapped.HTTP().Transport.(*otelTransport)
			assert.True(t, ok)
		})
	}
}
