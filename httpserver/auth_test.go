package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryCredentialValidator(t *testing.T) {
	t.Parallel()

	t.Run("given valid credentials, when validated, then returns nil", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
			"client-2": "secret-2",
		})

		err := validator.Validate(context.Background(), "client-1", "secret-1")
		require.NoError(t, err)
	})

	t.Run("given invalid client ID, when validated, then returns error", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})

		err := validator.Validate(context.Background(), "unknown-client", "secret-1")
		require.Error(t, err)
		assert.ErrorIs(t, err, httpserver.ErrInvalidCredentials)
	})

	t.Run("given invalid passkey, when validated, then returns error", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})

		err := validator.Validate(context.Background(), "client-1", "wrong-secret")
		require.Error(t, err)
		assert.ErrorIs(t, err, httpserver.ErrInvalidCredentials)
	})

	t.Run("given empty credentials, when validated, then returns error", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})

		err := validator.Validate(context.Background(), "", "")
		require.Error(t, err)
	})
}

func TestServiceAuthMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("given valid credentials, when request made, then proceeds", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		})

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "secret-1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("given missing credentials, when request made, then returns 401", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		})

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("given invalid credentials, when request made, then returns 401", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator: validator,
		})

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Client-ID", "client-1")
		req.Header.Set("Pass-Key", "wrong-secret")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("given custom headers, when request made, then uses custom headers", func(t *testing.T) {
		validator := httpserver.NewMemoryCredentialValidator(map[string]string{
			"client-1": "secret-1",
		})
		middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
			Validator:      validator,
			ClientIDHeader: "X-Client-ID",
			PassKeyHeader:  "X-Pass-Key",
		})

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Client-ID", "client-1")
		req.Header.Set("X-Pass-Key", "secret-1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run(
		"given valid credentials, when ClientIDFromContext called, then returns client ID",
		func(t *testing.T) {
			validator := httpserver.NewMemoryCredentialValidator(map[string]string{
				"client-1": "secret-1",
			})
			middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
				Validator: validator,
			})

			var capturedClientID string
			handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedClientID = httpserver.ClientIDFromContext(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Client-ID", "client-1")
			req.Header.Set("Pass-Key", "secret-1")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, "client-1", capturedClientID)
		},
	)
}

func TestKeyFuncByClientID(t *testing.T) {
	t.Parallel()

	t.Run(
		"given request through ServiceAuth, when key func called, then returns client ID",
		func(t *testing.T) {
			validator := httpserver.NewMemoryCredentialValidator(map[string]string{
				"test-client": "secret",
			})
			middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
				Validator: validator,
			})
			keyFunc := httpserver.KeyFuncByClientID()

			var capturedKey string
			handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedKey = keyFunc(r)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Client-ID", "test-client")
			req.Header.Set("Pass-Key", "secret")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, "test-client", capturedKey)
		},
	)

	t.Run(
		"given request without auth context, when key func called, then returns empty string",
		func(t *testing.T) {
			keyFunc := httpserver.KeyFuncByClientID()

			req := httptest.NewRequest(http.MethodGet, "/", nil)

			key := keyFunc(req)
			assert.Empty(t, key)
		},
	)
}

func TestKeyFuncByClientIDAndPath(t *testing.T) {
	t.Parallel()

	t.Run(
		"given request through ServiceAuth, when key func called, then returns combined key",
		func(t *testing.T) {
			validator := httpserver.NewMemoryCredentialValidator(map[string]string{
				"test-client": "secret",
			})
			middleware := httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
				Validator: validator,
			})
			keyFunc := httpserver.KeyFuncByClientIDAndPath()

			var capturedKey string
			handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedKey = keyFunc(r)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
			req.Header.Set("Client-ID", "test-client")
			req.Header.Set("Pass-Key", "secret")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, "test-client:/api/resource", capturedKey)
		},
	)
}

func TestKeyFuncByHeader(t *testing.T) {
	t.Parallel()

	t.Run(
		"given request with custom header, when key func called, then returns header value",
		func(t *testing.T) {
			keyFunc := httpserver.KeyFuncByHeader("X-Custom-Key")

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Custom-Key", "my-custom-value")

			key := keyFunc(req)
			assert.Equal(t, "my-custom-value", key)
		},
	)

	t.Run(
		"given request without header, when key func called, then returns empty string",
		func(t *testing.T) {
			keyFunc := httpserver.KeyFuncByHeader("X-Custom-Key")

			req := httptest.NewRequest(http.MethodGet, "/", nil)

			key := keyFunc(req)
			assert.Empty(t, key)
		},
	)
}
