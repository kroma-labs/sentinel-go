package httpclient

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockTransport_StubResponse(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().StubResponse(http.StatusOK, `{"status":"ok"}`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	resp, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := resp.String()
	assert.JSONEq(t, `{"status":"ok"}`, body)
}

func TestMockTransport_StubError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("network error")
	mock := NewMockTransport().StubError(expectedErr)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestMockTransport_StubPath(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().
		StubPath("/users", http.StatusOK, `[{"id":1}]`).
		StubPath("/posts", http.StatusOK, `[{"id":2}]`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	// Request to /users
	resp1, err := client.Request("GetUsers").Get(context.Background(), "/users")
	require.NoError(t, err)
	body1, _ := resp1.String()
	assert.Equal(t, `[{"id":1}]`, body1)

	// Request to /posts
	resp2, err := client.Request("GetPosts").Get(context.Background(), "/posts")
	require.NoError(t, err)
	body2, _ := resp2.String()
	assert.Equal(t, `[{"id":2}]`, body2)
}

func TestMockTransport_StubPathRegex(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().
		StubPathRegex(`/users/\d+`, http.StatusOK, `{"id":123}`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	resp, err := client.Request("GetUser").Get(context.Background(), "/users/123")
	require.NoError(t, err)
	body, _ := resp.String()
	assert.Equal(t, `{"id":123}`, body)

	resp2, err := client.Request("GetUser").Get(context.Background(), "/users/456")
	require.NoError(t, err)
	body2, _ := resp2.String()
	assert.Equal(t, `{"id":123}`, body2)
}

func TestMockTransport_StubMethod(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().
		StubResponse(http.StatusOK, `{"method":"default"}`).
		StubMethod("POST", http.StatusCreated, `{"method":"post"}`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	// GET uses default
	resp1, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// POST uses method stub
	resp2, err := client.Request("Test").Post(context.Background(), "/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)
}

func TestMockTransport_RequestTracking(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().StubResponse(http.StatusOK, `{}`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	_, _ = client.Request("GetUser").Get(context.Background(), "/users/1")
	_, _ = client.Request("GetUser").Get(context.Background(), "/users/2")
	_, _ = client.Request("CreateUser").Post(context.Background(), "/users")

	assert.Equal(t, 3, mock.RequestCount())

	requests := mock.Requests()
	assert.Equal(t, "/users/1", requests[0].URL.Path)
	assert.Equal(t, "/users/2", requests[1].URL.Path)
	assert.Equal(t, "POST", requests[2].Method)

	assert.Equal(t, "/users", mock.LastRequest().URL.Path)
}

func TestMockTransport_OnRequest(t *testing.T) {
	t.Parallel()

	var capturedAuth string
	mock := NewMockTransport().
		StubResponse(http.StatusOK, `{}`).
		OnRequest(func(req *http.Request) {
			capturedAuth = req.Header.Get("Authorization")
		})

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
		WithRequestInterceptor(AuthBearerInterceptor("test-token")),
	)

	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token", capturedAuth)
}

func TestMockTransport_NoStubError(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport() // No stubs

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	_, err := client.Request("Test").Get(context.Background(), "/unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no stub found")
}

func TestMockTransport_Reset(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().StubResponse(http.StatusOK, `{}`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	_, _ = client.Request("Test").Get(context.Background(), "/test")
	assert.Equal(t, 1, mock.RequestCount())

	mock.Reset()

	assert.Equal(t, 0, mock.RequestCount())

	// Now requests should fail (no stubs)
	_, err := client.Request("Test").Get(context.Background(), "/test")
	require.Error(t, err)
}

func TestMockTransport_MultipleResponseReads(t *testing.T) {
	t.Parallel()

	mock := NewMockTransport().StubResponse(http.StatusOK, `{"data":"test"}`)

	client := New(
		WithBaseURL("https://api.example.com"),
		WithMockTransport(mock),
	)

	// Multiple requests should each get their own readable body
	resp1, _ := client.Request("Test").Get(context.Background(), "/test")
	resp2, _ := client.Request("Test").Get(context.Background(), "/test")

	body1, _ := resp1.String()
	body2, _ := resp2.String()
	assert.JSONEq(t, `{"data":"test"}`, body1)
	assert.JSONEq(t, `{"data":"test"}`, body2)
}
