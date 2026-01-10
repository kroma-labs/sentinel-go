package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBuilder_Path(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{
			name:     "given simple path, then sets path",
			path:     "/users",
			wantPath: "/users",
		},
		{
			name:     "given path with leading slash, then preserves slash",
			path:     "/api/v1/users",
			wantPath: "/api/v1/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New()
			rb := client.Request("test").Path(tt.path)

			assert.Equal(t, tt.wantPath, rb.path)
		})
	}
}

func TestRequestBuilder_PathParam(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		pathParams map[string]string
		wantURL    string
	}{
		{
			name:       "given single path param, then replaces it",
			path:       "/users/{id}",
			pathParams: map[string]string{"id": "123"},
			wantURL:    "https://api.example.com/users/123",
		},
		{
			name: "given multiple path params, then replaces all",
			path: "/users/{userId}/posts/{postId}",
			pathParams: map[string]string{
				"userId": "123",
				"postId": "456",
			},
			wantURL: "https://api.example.com/users/123/posts/456",
		},
		{
			name:       "given special characters, then escapes them",
			path:       "/search/{query}",
			pathParams: map[string]string{"query": "hello world"},
			wantURL:    "https://api.example.com/search/hello%20world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(WithBaseURL("https://api.example.com"))
			rb := client.Request("test").Path(tt.path)

			for k, v := range tt.pathParams {
				rb = rb.PathParam(k, v)
			}

			url, err := rb.buildURL()
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}

func TestRequestBuilder_Query(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string]string
		wantInURL   []string
	}{
		{
			name:        "given single query param, then adds to URL",
			queryParams: map[string]string{"page": "1"},
			wantInURL:   []string{"page=1"},
		},
		{
			name: "given multiple query params, then adds all to URL",
			queryParams: map[string]string{
				"page":  "1",
				"limit": "10",
			},
			wantInURL: []string{"page=1", "limit=10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(WithBaseURL("https://api.example.com"))
			rb := client.Request("test").Path("/users")

			for k, v := range tt.queryParams {
				rb = rb.Query(k, v)
			}

			url, err := rb.buildURL()
			require.NoError(t, err)

			for _, want := range tt.wantInURL {
				assert.Contains(t, url, want)
			}
		})
	}
}

func TestRequestBuilder_Queries(t *testing.T) {
	client := New(WithBaseURL("https://api.example.com"))
	rb := client.Request("test").
		Path("/users").
		Queries(map[string]string{"page": "1", "limit": "10"})

	url, err := rb.buildURL()
	require.NoError(t, err)

	assert.Contains(t, url, "page=1")
	assert.Contains(t, url, "limit=10")
}

func TestRequestBuilder_Header(t *testing.T) {
	client := New()
	rb := client.Request("test").
		Header("Authorization", "Bearer token123").
		Header("X-Custom", "value")

	assert.Equal(t, "Bearer token123", rb.headers.Get("Authorization"))
	assert.Equal(t, "value", rb.headers.Get("X-Custom"))
}

func TestRequestBuilder_Headers(t *testing.T) {
	client := New()
	rb := client.Request("test").
		Headers(map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "value",
		})

	assert.Equal(t, "Bearer token123", rb.headers.Get("Authorization"))
	assert.Equal(t, "value", rb.headers.Get("X-Custom"))
}

func TestRequestBuilder_Body(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	tests := []struct {
		name            string
		body            any
		wantContentType string
		wantBodyPrefix  string
	}{
		{
			name:            "given struct, then encodes as JSON",
			body:            User{Name: "John", Email: "john@example.com"},
			wantContentType: "application/json",
			wantBodyPrefix:  `{"name":"John"`,
		},
		{
			name:            "given string, then uses text/plain",
			body:            "hello world",
			wantContentType: "text/plain; charset=utf-8",
			wantBodyPrefix:  "hello world",
		},
		{
			name:            "given []byte, then uses octet-stream",
			body:            []byte("binary data"),
			wantContentType: "application/octet-stream",
			wantBodyPrefix:  "binary data",
		},
		{
			name:            "given url.Values, then uses form-urlencoded",
			body:            url.Values{"key": []string{"value"}},
			wantContentType: "application/x-www-form-urlencoded",
			wantBodyPrefix:  "key=value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New()
			rb := client.Request("test").Body(tt.body)

			assert.Equal(t, tt.wantContentType, rb.contentType)

			if rb.body != nil {
				data, err := io.ReadAll(rb.body)
				require.NoError(t, err)
				assert.True(t, strings.HasPrefix(string(data), tt.wantBodyPrefix))
			}
		})
	}
}

func TestRequestBuilder_BodyJSON(t *testing.T) {
	type User struct {
		Name string `json:"name"`
	}

	client := New()
	rb := client.Request("test").BodyJSON(User{Name: "John"})

	assert.Equal(t, "application/json", rb.contentType)

	data, err := io.ReadAll(rb.body)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"name":"John"`)
}

func TestRequestBuilder_BodyXML(t *testing.T) {
	type User struct {
		Name string `xml:"name"`
	}

	client := New()
	rb := client.Request("test").BodyXML(User{Name: "John"})

	assert.Equal(t, "application/xml", rb.contentType)

	data, err := io.ReadAll(rb.body)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<name>John</name>")
}

func TestRequestBuilder_BodyForm(t *testing.T) {
	client := New()
	rb := client.Request("test").BodyForm(map[string]string{
		"username": "john",
		"password": "secret",
	})

	assert.Equal(t, "application/x-www-form-urlencoded", rb.contentType)

	data, err := io.ReadAll(rb.body)
	require.NoError(t, err)

	bodyStr := string(data)
	assert.Contains(t, bodyStr, "username=john")
	assert.Contains(t, bodyStr, "password=secret")
}

func TestRequestBuilder_Decode(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"name":"John"}`))
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	var user User
	resp, err := client.Request("GetUser").
		Decode(&user).
		Get(context.Background(), "/users/1")

	require.NoError(t, err)
	assert.True(t, resp.IsSuccess())
	assert.Equal(t, 1, user.ID)
	assert.Equal(t, "John", user.Name)
}

func TestRequestBuilder_HTTPMethods(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		execFunc   func(rb *RequestBuilder, ctx context.Context) (*Response, error)
		wantMethod string
	}{
		{
			name:   "Get",
			method: http.MethodGet,
			execFunc: func(rb *RequestBuilder, ctx context.Context) (*Response, error) {
				return rb.Get(ctx, "/test")
			},
			wantMethod: http.MethodGet,
		},
		{
			name:   "Post",
			method: http.MethodPost,
			execFunc: func(rb *RequestBuilder, ctx context.Context) (*Response, error) {
				return rb.Post(ctx, "/test")
			},
			wantMethod: http.MethodPost,
		},
		{
			name:   "Put",
			method: http.MethodPut,
			execFunc: func(rb *RequestBuilder, ctx context.Context) (*Response, error) {
				return rb.Put(ctx, "/test")
			},
			wantMethod: http.MethodPut,
		},
		{
			name:   "Patch",
			method: http.MethodPatch,
			execFunc: func(rb *RequestBuilder, ctx context.Context) (*Response, error) {
				return rb.Patch(ctx, "/test")
			},
			wantMethod: http.MethodPatch,
		},
		{
			name:   "Delete",
			method: http.MethodDelete,
			execFunc: func(rb *RequestBuilder, ctx context.Context) (*Response, error) {
				return rb.Delete(ctx, "/test")
			},
			wantMethod: http.MethodDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedMethod string
			handler := func(_ http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
			}
			server := httptest.NewServer(http.HandlerFunc(handler))
			defer server.Close()

			client := New(WithBaseURL(server.URL))
			_, err := tt.execFunc(client.Request("test"), context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.wantMethod, receivedMethod)
		})
	}
}

func TestRequestBuilder_DebugWithCurl(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithGenerateCurl(true),
	)

	resp, err := client.Request("test").
		Header("Authorization", "Bearer secret").
		Get(context.Background(), "/api")

	require.NoError(t, err)
	assert.NotEmpty(t, resp.CurlCommand())
	assert.Contains(t, resp.CurlCommand(), "curl")
	assert.Contains(t, resp.CurlCommand(), server.URL)
}

func TestRequestBuilder_EnableTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	resp, err := client.Request("test").
		EnableTrace().
		Get(context.Background(), "/api")

	require.NoError(t, err)
	assert.NotNil(t, resp.TraceInfo())
	assert.NotEmpty(t, resp.TraceInfo().TotalTime)

	// Test String() output
	str := resp.TraceInfo().String()
	assert.Contains(t, str, "DNS Lookup")
	assert.Contains(t, str, "Total Time")
}

func TestRequestBuilder_DefaultHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(
		WithBaseURL(server.URL),
		WithDefaultHeader("X-API-Key", "secret123"),
		WithDefaultHeader("Accept", "application/json"),
	)

	_, err := client.Request("test").Get(context.Background(), "/api")

	require.NoError(t, err)
	assert.Equal(t, "secret123", receivedHeaders.Get("X-API-Key"))
	assert.Equal(t, "application/json", receivedHeaders.Get("Accept"))
}

func TestRequestBuilder_DecodeError(t *testing.T) {
	type APIError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"INVALID","message":"Bad request"}`))
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	var apiErr APIError
	resp, err := client.Request("test").
		DecodeError(&apiErr).
		Get(context.Background(), "/api")

	require.NoError(t, err)
	assert.True(t, resp.IsError())
	assert.Equal(t, "INVALID", apiErr.Code)
	assert.Equal(t, "Bad request", apiErr.Message)
}

func TestRequestBuilder_DecodeAny(t *testing.T) {
	type APIResponse struct {
		Data   map[string]any `json:"data,omitempty"`
		Errors []string       `json:"errors,omitempty"`
	}

	t.Run("given 200 response, then decodes into same struct", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"id":1,"name":"John"},"errors":null}`))
		}))
		defer server.Close()

		client := New(WithBaseURL(server.URL))

		var result APIResponse
		resp, err := client.Request("test").
			DecodeAny(&result).
			Get(context.Background(), "/api")

		require.NoError(t, err)
		assert.True(t, resp.IsSuccess())
		assert.NotNil(t, result.Data)
		assert.Equal(t, "John", result.Data["name"])
		assert.Empty(t, result.Errors)
	})

	t.Run("given 400 response, then decodes into same struct", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"data":null,"errors":["Invalid input","Missing field"]}`))
		}))
		defer server.Close()

		client := New(WithBaseURL(server.URL))

		var result APIResponse
		resp, err := client.Request("test").
			DecodeAny(&result).
			Get(context.Background(), "/api")

		require.NoError(t, err)
		assert.True(t, resp.IsError())
		assert.Nil(t, result.Data)
		assert.Len(t, result.Errors, 2)
		assert.Equal(t, "Invalid input", result.Errors[0])
	})
}

func TestRequestBuilder_NilBody(t *testing.T) {
	client := New()
	rb := client.Request("test").Body(nil)

	assert.Nil(t, rb.body)
	assert.Empty(t, rb.contentType)
}

func TestRequestBuilder_BodyWithReader(t *testing.T) {
	content := "raw reader content"
	reader := bytes.NewReader([]byte(content))

	client := New()
	rb := client.Request("test").Body(reader)

	// io.Reader passthrough - no content type set
	assert.Equal(t, reader, rb.body)
	assert.Empty(t, rb.contentType)
}

func TestRequestBuilder_BodyWithWrapperTypes(t *testing.T) {
	t.Run("given struct, then uses JSON encoding", func(t *testing.T) {
		type User struct {
			Name string `json:"name"`
		}

		client := New()
		rb := client.Request("test").Body(User{Name: "John"})

		assert.Equal(t, "application/json", rb.contentType)
	})

	t.Run("given map, then uses JSON encoding", func(t *testing.T) {
		client := New()
		rb := client.Request("test").Body(map[string]string{"key": "value"})

		assert.Equal(t, "application/json", rb.contentType)
	})
}
