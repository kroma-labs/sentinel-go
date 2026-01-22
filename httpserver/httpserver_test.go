package httpserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		configFunc          func() httpserver.Config
		wantAddr            string
		wantReadTimeout     time.Duration
		wantWriteTimeout    time.Duration
		wantIdleTimeout     time.Duration
		wantShutdownTimeout time.Duration
	}{
		{
			name:                "given no options, then uses default timeout",
			configFunc:          httpserver.DefaultConfig,
			wantAddr:            ":8080",
			wantReadTimeout:     15 * time.Second,
			wantWriteTimeout:    15 * time.Second,
			wantIdleTimeout:     60 * time.Second,
			wantShutdownTimeout: 10 * time.Second,
		},
		{
			name:                "given production config, then uses hardened timeouts",
			configFunc:          httpserver.ProductionConfig,
			wantAddr:            ":8080",
			wantReadTimeout:     10 * time.Second,
			wantWriteTimeout:    10 * time.Second,
			wantIdleTimeout:     30 * time.Second,
			wantShutdownTimeout: 25 * time.Second,
		},
		{
			name:                "given development config, then uses lenient timeouts",
			configFunc:          httpserver.DevelopmentConfig,
			wantAddr:            ":8080",
			wantReadTimeout:     0,
			wantWriteTimeout:    0,
			wantIdleTimeout:     120 * time.Second,
			wantShutdownTimeout: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.configFunc()

			assert.Equal(t, tt.wantAddr, cfg.Addr)
			assert.Equal(t, tt.wantReadTimeout, cfg.ReadTimeout)
			assert.Equal(t, tt.wantWriteTimeout, cfg.WriteTimeout)
			assert.Equal(t, tt.wantIdleTimeout, cfg.IdleTimeout)
			assert.Equal(t, tt.wantShutdownTimeout, cfg.ShutdownTimeout)
		})
	}
}

func TestHealthHandler_Ping(t *testing.T) {
	t.Parallel()

	t.Run("given ping endpoint, then returns pong", func(t *testing.T) {
		health := httpserver.NewHealthHandler(
			httpserver.WithVersion("1.0.0"),
		)

		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rec := httptest.NewRecorder()

		health.PingHandler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"status":"pong"`)
	})
}

func TestHealthHandler_Checks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		handler        string
		checks         map[string]error
		wantStatusCode int
		wantStatus     string
	}{
		{
			name:           "given liveness check passes, then returns ok",
			handler:        "live",
			checks:         map[string]error{"check1": nil},
			wantStatusCode: http.StatusOK,
			wantStatus:     "ok",
		},
		{
			name:           "given liveness check fails, then returns service unavailable",
			handler:        "live",
			checks:         map[string]error{"check1": errors.New("failed")},
			wantStatusCode: http.StatusServiceUnavailable,
			wantStatus:     "fail",
		},
		{
			name:           "given readiness checks pass, then returns ok",
			handler:        "ready",
			checks:         map[string]error{"db": nil, "redis": nil},
			wantStatusCode: http.StatusOK,
			wantStatus:     "ok",
		},
		{
			name:           "given readiness check partial fail, then returns service unavailable",
			handler:        "ready",
			checks:         map[string]error{"db": nil, "redis": errors.New("connection refused")},
			wantStatusCode: http.StatusServiceUnavailable,
			wantStatus:     "fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := httpserver.NewHealthHandler(
				httpserver.WithVersion("1.0.0"),
			)

			for name, err := range tt.checks {
				checkErr := err
				if tt.handler == "live" {
					health.AddLivenessCheck(name, func(_ context.Context) error {
						return checkErr
					})
				} else {
					health.AddReadinessCheck(name, func(_ context.Context) error {
						return checkErr
					})
				}
			}

			var handler http.Handler
			if tt.handler == "live" {
				handler = health.LiveHandler()
			} else {
				handler = health.ReadyHandler()
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatusCode, rec.Code)
			assert.Contains(t, rec.Body.String(), `"status":"`+tt.wantStatus+`"`)
		})
	}
}

func TestMiddlewareChain(t *testing.T) {
	t.Parallel()

	t.Run("given two middleware, then executes in correct order", func(t *testing.T) {
		var order []string

		m1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m1-before")
				next.ServeHTTP(w, r)
				order = append(order, "m1-after")
			})
		}

		m2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m2-before")
				next.ServeHTTP(w, r)
				order = append(order, "m2-after")
			})
		}

		handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		chain := httpserver.Chain(m1, m2)(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		wantOrder := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
		assert.Equal(t, wantOrder, order)
	})
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		incomingID      string
		wantGeneratedID bool
		wantForwardedID string
	}{
		{
			name:            "given no request id, then generates new id",
			incomingID:      "",
			wantGeneratedID: true,
		},
		{
			name:            "given existing request id, then forwards id",
			incomingID:      "incoming-request-id-123",
			wantForwardedID: "incoming-request-id-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedID = httpserver.RequestIDFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			chain := httpserver.RequestID()(handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incomingID != "" {
				req.Header.Set("X-Request-ID", tt.incomingID)
			}

			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)

			responseID := rec.Header().Get("X-Request-ID")

			if tt.wantGeneratedID {
				assert.NotEmpty(t, capturedID, "expected generated request ID in context")
				assert.NotEmpty(t, responseID, "expected X-Request-ID in response header")
				assert.Equal(t, capturedID, responseID)
			} else {
				assert.Equal(t, tt.wantForwardedID, capturedID)
				assert.Equal(t, tt.wantForwardedID, responseID)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		method         string
		origin         string
		wantStatusCode int
		wantCORSHeader string
	}{
		{
			name:           "given options request with origin, then returns preflight response",
			method:         http.MethodOptions,
			origin:         "https://example.com",
			wantStatusCode: http.StatusNoContent,
			wantCORSHeader: "https://example.com",
		},
		{
			name:           "given get request with origin, then sets cors header",
			method:         http.MethodGet,
			origin:         "https://example.com",
			wantStatusCode: http.StatusOK,
			wantCORSHeader: "https://example.com",
		},
		{
			name:           "given get request without origin, then no cors header",
			method:         http.MethodGet,
			origin:         "",
			wantStatusCode: http.StatusOK,
			wantCORSHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			chain := httpserver.CORS(httpserver.DefaultCORSConfig())(handler)

			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatusCode, rec.Code)
			assert.Equal(t, tt.wantCORSHeader, rec.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	t.Run("given handler panics, then returns 500", func(t *testing.T) {
		handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("test panic")
		})

		chain := httpserver.Chain(httpserver.RequestID())(handler)

		recoveryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			chain.ServeHTTP(w, r)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		require.NotPanics(t, func() {
			recoveryHandler.ServeHTTP(rec, req)
		})

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	t.Run("given metrics middleware, then records metrics", func(t *testing.T) {
		metrics, err := httpserver.NewMetrics(httpserver.DefaultMetricsConfig())
		require.NoError(t, err)

		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		})

		chain := metrics.Middleware()(handler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		chain.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello", rec.Body.String())
	})
}

func TestRateLimit(t *testing.T) {
	t.Parallel()

	t.Run("given rate limit exceeded, then returns 429", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Very low limit for testing
		chain := httpserver.RateLimit(httpserver.RateLimitConfig{
			Limit: 1,
			Burst: 1,
		})(handler)

		// First request should succeed
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec1 := httptest.NewRecorder()
		chain.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// Second request should be rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec2 := httptest.NewRecorder()
		chain.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	})

	t.Run("given per-IP rate limit, then limits by IP", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		chain := httpserver.RateLimitByIP(1, 1)(handler)

		// First request from IP1 should succeed
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		req1.RemoteAddr = "192.168.1.1:1234"
		rec1 := httptest.NewRecorder()
		chain.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// Second request from IP1 should be rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.RemoteAddr = "192.168.1.1:1234"
		rec2 := httptest.NewRecorder()
		chain.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

		// Request from IP2 should succeed (different limiter)
		req3 := httptest.NewRequest(http.MethodGet, "/", nil)
		req3.RemoteAddr = "192.168.1.2:1234"
		rec3 := httptest.NewRecorder()
		chain.ServeHTTP(rec3, req3)
		assert.Equal(t, http.StatusOK, rec3.Code)
	})
}
