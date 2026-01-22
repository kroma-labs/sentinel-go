package fiber_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kroma-labs/sentinel-go/httpserver"
	fibersentinel "github.com/kroma-labs/sentinel-go/httpserver/adapters/fiber"
	"github.com/stretchr/testify/require"
)

func TestWrapMiddleware(t *testing.T) {
	t.Run("given httpserver middleware, when wrapped, then works with Fiber", func(t *testing.T) {
		app := fiber.New()

		// Create a simple middleware that adds a header
		middleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom", "test-value")
				next.ServeHTTP(w, r)
			})
		}

		app.Use(fibersentinel.WrapMiddleware(middleware))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("hello")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "test-value", resp.Header.Get("X-Custom"))
	})
}

func TestRequestID(t *testing.T) {
	t.Run(
		"given no request ID, when RequestID middleware applied, then generates ID",
		func(t *testing.T) {
			app := fiber.New()
			app.Use(fibersentinel.RequestID())
			app.Get("/test", func(c *fiber.Ctx) error {
				return c.SendString("ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotEmpty(t, resp.Header.Get("X-Request-ID"))
		},
	)

	t.Run(
		"given existing request ID, when RequestID middleware applied, then forwards ID",
		func(t *testing.T) {
			app := fiber.New()
			app.Use(fibersentinel.RequestID())
			app.Get("/test", func(c *fiber.Ctx) error {
				return c.SendString("ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-Request-ID", "existing-id-123")
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "existing-id-123", resp.Header.Get("X-Request-ID"))
		},
	)
}

func TestRecovery(t *testing.T) {
	// Note: Fiber's adaptor doesn't bridge panics from net/http middleware to fasthttp.
	// The httpserver.Recovery middleware works, but the panic happens in fasthttp layer
	// before it can be caught. For Fiber, use fiber's built-in recover middleware
	// or wrap handlers explicitly.
	t.Skip(
		"Fiber adaptor doesn't bridge panics from net/http to fasthttp - use Fiber's native recovery",
	)
}

func TestCORS(t *testing.T) {
	t.Run(
		"given preflight request, when CORS middleware applied, then returns CORS headers",
		func(t *testing.T) {
			app := fiber.New()
			app.Use(fibersentinel.CORS(httpserver.DefaultCORSConfig()))
			app.Get("/test", func(c *fiber.Ctx) error {
				return c.SendString("ok")
			})

			req := httptest.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set("Origin", "https://example.com")
			req.Header.Set("Access-Control-Request-Method", "GET")
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, "https://example.com", resp.Header.Get("Access-Control-Allow-Origin"))
		},
	)
}

func TestTimeout(t *testing.T) {
	t.Run("given timeout middleware, when applied, then handler executes", func(t *testing.T) {
		app := fiber.New()
		app.Use(fibersentinel.Timeout(5 * time.Second))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestRateLimit(t *testing.T) {
	t.Run("given rate limit exceeded, when applied, then returns 429", func(t *testing.T) {
		app := fiber.New()
		app.Use(fibersentinel.RateLimit(httpserver.RateLimitConfig{
			Limit: 1,
			Burst: 1,
		}))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		// First request: OK
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp1, err := app.Test(req1)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp1.StatusCode)

		// Second request: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp2, err := app.Test(req2)
		require.NoError(t, err)
		require.Equal(t, http.StatusTooManyRequests, resp2.StatusCode)
	})
}

func TestRateLimitByIP(t *testing.T) {
	t.Run("given per-IP rate limit, when different IPs, then separate limits", func(t *testing.T) {
		app := fiber.New()
		app.Use(fibersentinel.RateLimitByIP(1, 1))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		// IP1 first request: OK
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req1.Header.Set("X-Forwarded-For", "10.0.0.1")
		resp1, err := app.Test(req1)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp1.StatusCode)

		// IP1 second request: rate limited
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.Header.Set("X-Forwarded-For", "10.0.0.1")
		resp2, err := app.Test(req2)
		require.NoError(t, err)
		require.Equal(t, http.StatusTooManyRequests, resp2.StatusCode)

		// IP2 first request: OK (different IP)
		req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req3.Header.Set("X-Forwarded-For", "10.0.0.2")
		resp3, err := app.Test(req3)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp3.StatusCode)
	})
}

func TestServiceAuth(t *testing.T) {
	t.Run("given valid credentials, when applied, then allows request", func(t *testing.T) {
		app := fiber.New()
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		app.Use(fibersentinel.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		}))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "secret-1")
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("given invalid credentials, when applied, then returns 401", func(t *testing.T) {
		app := fiber.New()
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		app.Use(fibersentinel.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		}))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "wrong-secret")
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestRegisterHealth(t *testing.T) {
	t.Run("given health handler, when registered, then endpoints work", func(t *testing.T) {
		app := fiber.New()
		health := httpserver.NewHealthHandler(httpserver.WithVersion("1.0.0"))
		fibersentinel.RegisterHealth(app, health)

		// Test /ping
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Test /livez
		req = httptest.NewRequest(http.MethodGet, "/livez", nil)
		resp, err = app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Test /readyz
		req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err = app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestRegisterPrometheus(t *testing.T) {
	t.Run("given prometheus registered, then metrics endpoint works", func(t *testing.T) {
		app := fiber.New()
		fibersentinel.RegisterPrometheus(app, "/metrics")

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
