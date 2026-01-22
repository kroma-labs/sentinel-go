package echo_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kroma-labs/sentinel-go/httpserver"
	echosentinel "github.com/kroma-labs/sentinel-go/httpserver/adapters/echo"
	echolib "github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestWrapMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("given httpserver middleware, when wrapped, then works with Echo", func(t *testing.T) {
		e := echolib.New()

		// Create a simple middleware that adds a header
		middleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom", "test-value")
				next.ServeHTTP(w, r)
			})
		}

		e.Use(echosentinel.WrapMiddleware(middleware))
		e.GET("/test", func(c echolib.Context) error {
			return c.String(http.StatusOK, "hello")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "test-value", rec.Header().Get("X-Custom"))
		assert.Equal(t, "hello", rec.Body.String())
	})
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	t.Run(
		"given no request ID, when RequestID middleware applied, then generates ID",
		func(t *testing.T) {
			e := echolib.New()
			e.Use(echosentinel.RequestID())
			e.GET("/test", func(c echolib.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
		},
	)

	t.Run(
		"given existing request ID, when RequestID middleware applied, then forwards ID",
		func(t *testing.T) {
			e := echolib.New()
			e.Use(echosentinel.RequestID())
			e.GET("/test", func(c echolib.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-Request-ID", "existing-id-123")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "existing-id-123", rec.Header().Get("X-Request-ID"))
		},
	)
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	t.Run(
		"given handler panics, when Recovery middleware applied, then returns 500",
		func(t *testing.T) {
			logger := zerolog.Nop()
			e := echolib.New()
			e.Use(echosentinel.Recovery(logger))
			e.GET("/panic", func(_ echolib.Context) error {
				panic("test panic")
			})

			req := httptest.NewRequest(http.MethodGet, "/panic", nil)
			rec := httptest.NewRecorder()

			assert.NotPanics(t, func() {
				e.ServeHTTP(rec, req)
			})
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
		},
	)
}

func TestCORS(t *testing.T) {
	t.Parallel()

	t.Run(
		"given preflight request, when CORS middleware applied, then returns CORS headers",
		func(t *testing.T) {
			e := echolib.New()
			e.Use(echosentinel.CORS(httpserver.DefaultCORSConfig()))
			e.GET("/test", func(c echolib.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set("Origin", "https://example.com")
			req.Header.Set("Access-Control-Request-Method", "GET")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		},
	)
}

func TestTimeout(t *testing.T) {
	t.Parallel()

	t.Run(
		"given timeout middleware, when applied, then handler receives context",
		func(t *testing.T) {
			e := echolib.New()
			e.Use(echosentinel.Timeout(5 * time.Second))
			e.GET("/test", func(c echolib.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
		},
	)
}

func TestRateLimit(t *testing.T) {
	t.Parallel()

	t.Run("given rate limit exceeded, when applied, then returns 429", func(t *testing.T) {
		e := echolib.New()
		e.Use(echosentinel.RateLimit(httpserver.RateLimitConfig{
			Limit: 1,
			Burst: 1,
		}))
		e.GET("/test", func(c echolib.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		// First request: OK
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec1 := httptest.NewRecorder()
		e.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// Second request: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec2 := httptest.NewRecorder()
		e.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	})
}

func TestRateLimitByIP(t *testing.T) {
	t.Parallel()

	t.Run("given per-IP rate limit, when different IPs, then separate limits", func(t *testing.T) {
		e := echolib.New()
		e.Use(echosentinel.RateLimitByIP(1, 1))
		e.GET("/test", func(c echolib.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		// IP1 first request: OK
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req1.RemoteAddr = "10.0.0.1:1234"
		rec1 := httptest.NewRecorder()
		e.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		// IP1 second request: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.RemoteAddr = "10.0.0.1:1234"
		rec2 := httptest.NewRecorder()
		e.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

		// IP2 first request: OK (different IP)
		req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req3.RemoteAddr = "10.0.0.2:1234"
		rec3 := httptest.NewRecorder()
		e.ServeHTTP(rec3, req3)
		assert.Equal(t, http.StatusOK, rec3.Code)
	})
}

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	t.Run("given valid credentials, when applied, then allows request", func(t *testing.T) {
		e := echolib.New()
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		e.Use(echosentinel.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		}))
		e.GET("/test", func(c echolib.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "secret-1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("given invalid credentials, when applied, then returns 401", func(t *testing.T) {
		e := echolib.New()
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		e.Use(echosentinel.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		}))
		e.GET("/test", func(c echolib.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "wrong-secret")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestRegisterHealth(t *testing.T) {
	t.Parallel()

	t.Run("given health handler, when registered, then endpoints work", func(t *testing.T) {
		e := echolib.New()
		health := httpserver.NewHealthHandler(httpserver.WithVersion("1.0.0"))
		echosentinel.RegisterHealth(e, health)

		// Test /ping
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "pong")

		// Test /livez
		req = httptest.NewRequest(http.MethodGet, "/livez", nil)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Test /readyz
		req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestRegisterPrometheus(t *testing.T) {
	t.Parallel()

	t.Run("given prometheus registered, then metrics endpoint works", func(t *testing.T) {
		e := echolib.New()
		echosentinel.RegisterPrometheus(e, "/metrics")

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
