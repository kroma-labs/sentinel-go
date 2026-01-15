package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthBearerInterceptor(t *testing.T) {
	t.Parallel()

	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(AuthBearerInterceptor("test-token-123")),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token-123", capturedAuth)
}

func TestAPIKeyInterceptor(t *testing.T) {
	t.Parallel()

	var capturedAPIKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(APIKeyInterceptor("X-API-Key", "my-secret-key")),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, "my-secret-key", capturedAPIKey)
}

func TestUserAgentInterceptor(t *testing.T) {
	t.Parallel()

	var capturedUA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(UserAgentInterceptor("MyApp/1.0")),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, "MyApp/1.0", capturedUA)
}

func TestMultipleInterceptors_ExecuteInOrder(t *testing.T) {
	t.Parallel()

	var order []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(func(_ *http.Request) error {
			order = append(order, "first")
			return nil
		}),
		WithRequestInterceptor(func(_ *http.Request) error {
			order = append(order, "second")
			return nil
		}),
		WithRequestInterceptor(func(_ *http.Request) error {
			order = append(order, "third")
			return nil
		}),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, []string{"first", "second", "third"}, order)
}

func TestInterceptor_ErrorStopsChain(t *testing.T) {
	t.Parallel()

	errInterceptor := errors.New("interceptor error")
	var secondCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(func(_ *http.Request) error {
			return errInterceptor
		}),
		WithRequestInterceptor(func(_ *http.Request) error {
			secondCalled = true
			return nil
		}),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.Error(t, err)
	require.ErrorIs(t, err, errInterceptor)
	assert.False(t, secondCalled, "second interceptor should not be called")
}

func TestPerRequestInterceptor(t *testing.T) {
	t.Parallel()

	var capturedHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Request-Specific")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	_, err := client.Request("Test").
		Intercept(func(req *http.Request) error {
			req.Header.Set("X-Request-Specific", "per-request-value")
			return nil
		}).
		Get(context.Background(), "/test")

	require.NoError(t, err)
	assert.Equal(t, "per-request-value", capturedHeader)
}

func TestPerRequestInterceptor_RunsAfterClientInterceptors(t *testing.T) {
	t.Parallel()

	var order []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(func(_ *http.Request) error {
			order = append(order, "client")
			return nil
		}),
	)

	_, err := client.Request("Test").
		Intercept(func(_ *http.Request) error {
			order = append(order, "request")
			return nil
		}).
		Get(context.Background(), "/test")

	require.NoError(t, err)
	assert.Equal(t, []string{"client", "request"}, order)
}

func TestResponseInterceptor(t *testing.T) {
	t.Parallel()

	var capturedStatus int
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithResponseInterceptor(func(resp *http.Response, req *http.Request) error {
			capturedStatus = resp.StatusCode
			capturedMethod = req.Method
			return nil
		}),
	)

	_, err := client.Request("Test").Post(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, capturedStatus)
	assert.Equal(t, "POST", capturedMethod)
}

func TestResponseInterceptor_ErrorReturned(t *testing.T) {
	t.Parallel()

	errResponse := errors.New("response interceptor error")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithResponseInterceptor(func(_ *http.Response, _ *http.Request) error {
			return errResponse
		}),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.Error(t, err)
	assert.ErrorIs(t, err, errResponse)
}

func TestBothRequestAndResponseInterceptors(t *testing.T) {
	t.Parallel()

	var requestCalled, responseCalled atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(func(_ *http.Request) error {
			requestCalled.Store(true)
			return nil
		}),
		WithResponseInterceptor(func(_ *http.Response, _ *http.Request) error {
			responseCalled.Store(true)
			return nil
		}),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.True(t, requestCalled.Load())
	assert.True(t, responseCalled.Load())
}

func TestCorrelationIDInterceptor(t *testing.T) {
	t.Parallel()

	var capturedCorrelationID string
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorrelationID = r.Header.Get("X-Correlation-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithRequestInterceptor(CorrelationIDInterceptor("X-Correlation-ID", func() string {
			callCount++
			return "corr-id-" + string(rune('0'+callCount))
		})),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)
	assert.Equal(t, "corr-id-1", capturedCorrelationID)

	_, err = client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)
	assert.Equal(t, "corr-id-2", capturedCorrelationID)
}
