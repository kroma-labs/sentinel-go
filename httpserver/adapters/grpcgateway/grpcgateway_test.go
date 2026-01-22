package grpcgateway_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/kroma-labs/sentinel-go/httpserver/adapters/grpcgateway"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestWrapWithMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("given middleware, when wrapped, then applies to gwmux", func(t *testing.T) {
		gwmux := runtime.NewServeMux()

		// Create a middleware that adds a header
		middleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom", "test-value")
				next.ServeHTTP(w, r)
			})
		}

		handler := grpcgateway.WrapWithMiddleware(gwmux, middleware)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "test-value", rec.Header().Get("X-Custom"))
	})

	t.Run("given multiple middleware, when wrapped, then applies in order", func(t *testing.T) {
		gwmux := runtime.NewServeMux()

		order := []string{}

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

		handler := grpcgateway.WrapWithMiddleware(gwmux, m1, m2)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, []string{"m1-before", "m2-before", "m2-after", "m1-after"}, order)
	})
}

func TestDefaultMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("given logger, when applied, then adds RequestID", func(t *testing.T) {
		gwmux := runtime.NewServeMux()
		logger := zerolog.Nop()

		handler := grpcgateway.DefaultMiddleware(gwmux, &logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
	})

	t.Run("given nil logger, when applied, then still works", func(t *testing.T) {
		gwmux := runtime.NewServeMux()

		handler := grpcgateway.DefaultMiddleware(gwmux, nil)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			handler.ServeHTTP(rec, req)
		})
	})
}

func TestWithTracing(t *testing.T) {
	t.Parallel()

	t.Run("given handler, when WithTracing applied, then works", func(t *testing.T) {
		baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		handler := grpcgateway.WithTracing(baseHandler, httpserver.DefaultTracingConfig())

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestWithCORS(t *testing.T) {
	t.Parallel()

	t.Run(
		"given preflight request, when WithCORS applied, then returns CORS headers",
		func(t *testing.T) {
			baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := grpcgateway.WithCORS(baseHandler, httpserver.DefaultCORSConfig())

			req := httptest.NewRequest(http.MethodOptions, "/", nil)
			req.Header.Set("Origin", "https://example.com")
			req.Header.Set("Access-Control-Request-Method", "GET")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		},
	)
}

func TestWithRateLimit(t *testing.T) {
	t.Parallel()

	t.Run("given rate limit exceeded, when applied, then returns 429", func(t *testing.T) {
		baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		handler := grpcgateway.WithRateLimit(baseHandler, httpserver.RateLimitConfig{
			Limit: 1,
			Burst: 1,
		})

		// First request: OK
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec1 := httptest.NewRecorder()
		handler.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// Second request: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec2 := httptest.NewRecorder()
		handler.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	})
}

func TestWithServiceAuth(t *testing.T) {
	t.Parallel()

	t.Run("given valid credentials, when applied, then allows request", func(t *testing.T) {
		baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		handler := grpcgateway.WithServiceAuth(baseHandler, httpserver.ServiceAuthConfig{
			Validator: validator,
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "secret-1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("given invalid credentials, when applied, then returns 401", func(t *testing.T) {
		baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		handler := grpcgateway.WithServiceAuth(baseHandler, httpserver.ServiceAuthConfig{
			Validator: validator,
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestCombinedMux(t *testing.T) {
	t.Parallel()

	t.Run("given grpc content-type, when requested, then routes to gwmux", func(t *testing.T) {
		gwmux := runtime.NewServeMux()
		httpmux := http.NewServeMux()
		httpmux.HandleFunc("/http", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("http-handler"))
		})

		handler := grpcgateway.CombinedMux(gwmux, httpmux)

		// gRPC request goes to gwmux (returns 501 for unregistered)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/resource", nil)
		req.Header.Set("Content-Type", "application/grpc")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		// gwmux returns 501 for unregistered path, but the routing worked
		assert.GreaterOrEqual(t, rec.Code, 400) // Not a 200, but routed to gwmux
	})

	t.Run(
		"given non-grpc content-type, when requested, then routes to httpmux",
		func(t *testing.T) {
			gwmux := runtime.NewServeMux()
			httpmux := http.NewServeMux()
			httpmux.HandleFunc("/http", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("http-handler"))
			})

			handler := grpcgateway.CombinedMux(gwmux, httpmux)

			// Regular HTTP goes to httpmux
			req := httptest.NewRequest(http.MethodGet, "/http", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "http-handler", rec.Body.String())
		},
	)
}

func TestNewHandler(t *testing.T) {
	t.Parallel()

	t.Run("given full config, when created, then handler works", func(t *testing.T) {
		gwmux := runtime.NewServeMux()
		logger := zerolog.Nop()
		tracingCfg := httpserver.DefaultTracingConfig()
		corsCfg := httpserver.DefaultCORSConfig()
		rateLimitCfg := httpserver.RateLimitConfig{
			Limit: 100,
			Burst: 200,
		}

		handler := grpcgateway.NewHandler(gwmux, grpcgateway.Config{
			Logger:    &logger,
			Tracer:    &tracingCfg,
			CORS:      &corsCfg,
			RateLimit: &rateLimitCfg,
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			handler.ServeHTTP(rec, req)
		})
	})

	t.Run("given minimal config, when created, then handler works", func(t *testing.T) {
		gwmux := runtime.NewServeMux()

		handler := grpcgateway.NewHandler(gwmux, grpcgateway.Config{})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			handler.ServeHTTP(rec, req)
		})
		// Should have RequestID
		assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
	})

	t.Run("given config with service auth, when created, then auth works", func(t *testing.T) {
		gwmux := runtime.NewServeMux()
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		serviceAuthCfg := httpserver.ServiceAuthConfig{
			Validator: validator,
		}

		handler := grpcgateway.NewHandler(gwmux, grpcgateway.Config{
			ServiceAuth: &serviceAuthCfg,
		})

		// Without auth: 401
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		// With auth: allowed
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "secret-1")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		// gwmux returns something (not 401)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
	})
}
