package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOtelTransport_RoundTrip(t *testing.T) {
	type args struct {
		method      string
		path        string
		body        string
		serviceName string
	}

	tests := []struct {
		name          string
		args          args
		serverStatus  int
		wantErr       assert.ErrorAssertionFunc
		wantSpanName  string
		wantSpanCount int
	}{
		{
			name: "given successful GET request, then creates span",
			args: args{
				method:      http.MethodGet,
				path:        "/test",
				serviceName: "test-service",
			},
			serverStatus:  http.StatusOK,
			wantErr:       assert.NoError,
			wantSpanName:  "HTTP GET",
			wantSpanCount: 1,
		},
		{
			name: "given POST with body, then records body size",
			args: args{
				method: http.MethodPost,
				path:   "/upload",
				body:   "test body content",
			},
			serverStatus:  http.StatusOK,
			wantErr:       assert.NoError,
			wantSpanName:  "HTTP POST",
			wantSpanCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.args.body != "" {
						io.ReadAll(r.Body)
					}
					w.WriteHeader(tt.serverStatus)
				}),
			)
			defer server.Close()

			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			mp := sdkmetric.NewMeterProvider()
			defer tp.Shutdown(context.Background())
			defer mp.Shutdown(context.Background())

			cfg := &internalConfig{
				httpConfig:         DefaultConfig(),
				TracerProvider:     tp,
				MeterProvider:      mp,
				ServiceName:        tt.args.serviceName,
				EnableNetworkTrace: true,
			}
			cfg.Tracer = tp.Tracer(scope)
			cfg.Meter = mp.Meter(scope)
			cfg.Metrics, _ = newMetrics(cfg.Meter)

			transport := newOtelTransport(http.DefaultTransport, cfg)

			var body io.Reader
			if tt.args.body != "" {
				body = strings.NewReader(tt.args.body)
			}
			req, _ := http.NewRequest(tt.args.method, server.URL+tt.args.path, body)
			if tt.args.body != "" {
				req.ContentLength = int64(len(tt.args.body))
			}

			resp, err := transport.RoundTrip(req)

			tt.wantErr(t, err)
			if err == nil {
				defer resp.Body.Close()
			}

			spans := exporter.GetSpans()
			assert.Len(t, spans, tt.wantSpanCount)
			if tt.wantSpanCount > 0 {
				assert.Equal(t, tt.wantSpanName, spans[0].Name)
			}
		})
	}
}

func TestOtelTransport_RoundTrip_TracePropagation(t *testing.T) {
	t.Run("given parent span, then propagates trace context", func(t *testing.T) {
		var receivedHeaders http.Header
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		mp := sdkmetric.NewMeterProvider()
		defer tp.Shutdown(context.Background())
		defer mp.Shutdown(context.Background())

		cfg := &internalConfig{
			httpConfig:         DefaultConfig(),
			TracerProvider:     tp,
			MeterProvider:      mp,
			EnableNetworkTrace: false,
		}
		cfg.Tracer = tp.Tracer(scope)
		cfg.Meter = mp.Meter(scope)
		cfg.Metrics, _ = newMetrics(cfg.Meter)

		transport := newOtelTransport(http.DefaultTransport, cfg)

		ctx, parentSpan := tp.Tracer(scope).Start(context.Background(), "parent")
		defer parentSpan.End()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.NotEmpty(t, receivedHeaders.Get("Traceparent"))
	})
}

func TestOtelTransport_RoundTrip_ErrorHandling(t *testing.T) {
	type args struct {
		transportErr error
	}

	tests := []struct {
		name         string
		args         args
		wantErrType  string
		wantSpanAttr bool
	}{
		{
			name:         "given connection refused, then records error type",
			args:         args{transportErr: errors.New("connection refused")},
			wantErrType:  ErrorTypeConnectionRefused,
			wantSpanAttr: true,
		},
		{
			name:         "given context cancelled, then records cancelled",
			args:         args{transportErr: context.Canceled},
			wantErrType:  ErrorTypeCancelled,
			wantSpanAttr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())

			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer tp.Shutdown(context.Background())

			cfg := &internalConfig{
				httpConfig:         DefaultConfig(),
				TracerProvider:     tp,
				MeterProvider:      mp,
				EnableNetworkTrace: false,
			}
			cfg.Tracer = tp.Tracer(scope)
			cfg.Meter = mp.Meter(scope)
			cfg.Metrics, _ = newMetrics(cfg.Meter)

			mockTransport := &mockRoundTripper{err: tt.args.transportErr}
			transport := newOtelTransport(mockTransport, cfg)

			req, _ := http.NewRequest(http.MethodGet, "http://localhost:1", nil)
			_, err := transport.RoundTrip(req)

			require.Error(t, err)

			spans := exporter.GetSpans()
			require.Len(t, spans, 1)

			if tt.wantSpanAttr {
				attrMap := make(map[string]interface{})
				for _, attr := range spans[0].Attributes {
					attrMap[string(attr.Key)] = attr.Value.AsInterface()
				}
				assert.Equal(t, tt.wantErrType, attrMap["error.type"])
			}
		})
	}
}

func TestOtelTransport_RequestAttributes(t *testing.T) {
	type args struct {
		method      string
		url         string
		serviceName string
		bodySize    int64
		userAgent   string
	}

	tests := []struct {
		name       string
		args       args
		wantMethod string
		wantScheme string
		wantHost   string
		wantPort   int64
	}{
		{
			name: "given HTTPS with custom port, then extracts all attrs",
			args: args{
				method:      http.MethodPost,
				url:         "https://api.example.com:8443/users",
				serviceName: "test-service",
				bodySize:    1024,
				userAgent:   "test-agent/1.0",
			},
			wantMethod: "POST",
			wantScheme: "https",
			wantHost:   "api.example.com",
			wantPort:   8443,
		},
		{
			name: "given HTTP without port, then uses default 80",
			args: args{
				method: http.MethodGet,
				url:    "http://example.com/path",
			},
			wantMethod: "GET",
			wantScheme: "http",
			wantHost:   "example.com",
			wantPort:   80,
		},
		{
			name: "given HTTPS without port, then uses default 443",
			args: args{
				method: http.MethodGet,
				url:    "https://example.com/path",
			},
			wantMethod: "GET",
			wantScheme: "https",
			wantHost:   "example.com",
			wantPort:   443,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &internalConfig{ServiceName: tt.args.serviceName}
			transport := &otelTransport{cfg: cfg}

			req, _ := http.NewRequest(tt.args.method, tt.args.url, nil)
			if tt.args.bodySize > 0 {
				req.ContentLength = tt.args.bodySize
			}
			if tt.args.userAgent != "" {
				req.Header.Set("User-Agent", tt.args.userAgent)
			}

			attrs := transport.requestAttributes(req)

			attrMap := make(map[string]interface{})
			for _, attr := range attrs {
				attrMap[string(attr.Key)] = attr.Value.AsInterface()
			}

			assert.Equal(t, tt.wantMethod, attrMap["http.request.method"])
			assert.Equal(t, tt.wantScheme, attrMap["url.scheme"])
			assert.Equal(t, tt.wantHost, attrMap["server.address"])
			assert.Equal(t, tt.wantPort, attrMap["server.port"])
		})
	}
}

func TestOtelTransport_ResponseAttributes(t *testing.T) {
	type args struct {
		statusCode    int
		contentLength int64
		proto         string
	}

	tests := []struct {
		name           string
		args           args
		wantStatusCode int64
		wantVersion    string
	}{
		{
			name: "given HTTP/2 response, then extracts version as '2'",
			args: args{
				statusCode:    http.StatusOK,
				contentLength: 2048,
				proto:         "HTTP/2.0",
			},
			wantStatusCode: 200,
			wantVersion:    "2",
		},
		{
			name: "given HTTP/1.1 response, then extracts version as '1.1'",
			args: args{
				statusCode: http.StatusOK,
				proto:      "HTTP/1.1",
			},
			wantStatusCode: 200,
			wantVersion:    "1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &otelTransport{cfg: &internalConfig{}}
			resp := &http.Response{
				StatusCode:    tt.args.statusCode,
				ContentLength: tt.args.contentLength,
				Proto:         tt.args.proto,
			}

			attrs := transport.responseAttributes(resp)

			attrMap := make(map[string]interface{})
			for _, attr := range attrs {
				attrMap[string(attr.Key)] = attr.Value.AsInterface()
			}

			assert.Equal(t, tt.wantStatusCode, attrMap["http.response.status_code"])
			assert.Equal(t, tt.wantVersion, attrMap["network.protocol.version"])
		})
	}
}

func TestOtelTransport_MetricsRecording(t *testing.T) {
	t.Run("given successful request, then records duration metric", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		tp := sdktrace.NewTracerProvider()
		defer mp.Shutdown(context.Background())
		defer tp.Shutdown(context.Background())

		cfg := &internalConfig{
			httpConfig:         DefaultConfig(),
			TracerProvider:     tp,
			MeterProvider:      mp,
			EnableNetworkTrace: false,
		}
		cfg.Tracer = tp.Tracer(scope)
		cfg.Meter = mp.Meter(scope)
		cfg.Metrics, _ = newMetrics(cfg.Meter)

		transport := newOtelTransport(http.DefaultTransport, cfg)

		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		resp.Body.Close()

		var rm metricdata.ResourceMetrics
		err = reader.Collect(context.Background(), &rm)
		require.NoError(t, err)
		assert.NotEmpty(t, rm.ScopeMetrics)
	})
}

// mockRoundTripper is a mock http.RoundTripper for testing.
type mockRoundTripper struct {
	resp *http.Response
	err  error
}

func (m *mockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return m.resp, m.err
}
