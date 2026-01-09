package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"given 200, then returns true", http.StatusOK, true},
		{"given 201, then returns true", http.StatusCreated, true},
		{"given 204, then returns true", http.StatusNoContent, true},
		{"given 299, then returns true", 299, true},
		{"given 300, then returns false", 300, false},
		{"given 400, then returns false", http.StatusBadRequest, false},
		{"given 500, then returns false", http.StatusInternalServerError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				Response: &http.Response{StatusCode: tt.statusCode},
			}
			assert.Equal(t, tt.want, resp.IsSuccess())
		})
	}
}

func TestResponse_IsError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"given 200, then returns false", http.StatusOK, false},
		{"given 300, then returns false", 300, false},
		{"given 399, then returns false", 399, false},
		{"given 400, then returns true", http.StatusBadRequest, true},
		{"given 404, then returns true", http.StatusNotFound, true},
		{"given 500, then returns true", http.StatusInternalServerError, true},
		{"given 503, then returns true", http.StatusServiceUnavailable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				Response: &http.Response{StatusCode: tt.statusCode},
			}
			assert.Equal(t, tt.want, resp.IsError())
		})
	}
}

func TestResponse_Body(t *testing.T) {
	bodyContent := "test body content"
	resp := &Response{
		Response: &http.Response{
			Body: io.NopCloser(strings.NewReader(bodyContent)),
		},
	}

	// First call reads and caches
	body, err := resp.Body()
	require.NoError(t, err)
	assert.Equal(t, bodyContent, string(body))

	// Second call returns cached value
	body2, err := resp.Body()
	require.NoError(t, err)
	assert.Equal(t, bodyContent, string(body2))
	assert.True(t, resp.bodyRead)
}

func TestResponse_String(t *testing.T) {
	bodyContent := "test body content"
	resp := &Response{
		Response: &http.Response{
			Body: io.NopCloser(strings.NewReader(bodyContent)),
		},
	}

	str, err := resp.String()
	require.NoError(t, err)
	assert.Equal(t, bodyContent, str)
}

func TestResponse_CurlCommand(t *testing.T) {
	resp := &Response{
		curlCommand: "curl -X GET 'https://api.example.com/users'",
	}

	assert.Equal(t, "curl -X GET 'https://api.example.com/users'", resp.CurlCommand())
}

func TestResponse_TraceInfo(t *testing.T) {
	traceInfo := &TraceInfo{
		DNSLookup:    "2ms",
		ConnTime:     "15ms",
		TLSHandshake: "30ms",
		ServerTime:   "100ms",
		TotalTime:    "150ms",
	}

	resp := &Response{
		traceInfo: traceInfo,
	}

	assert.Equal(t, traceInfo, resp.TraceInfo())
}

func TestTraceInfo_String(t *testing.T) {
	t.Run("given valid trace info, then returns formatted string", func(t *testing.T) {
		info := &TraceInfo{
			DNSLookup:    "2.1ms",
			ConnTime:     "15.3ms",
			TLSHandshake: "28.7ms",
			ServerTime:   "45.2ms",
			TotalTime:    "91.3ms",
		}

		str := info.String()

		assert.Contains(t, str, "DNS Lookup:    2.1ms")
		assert.Contains(t, str, "TCP Connect:   15.3ms")
		assert.Contains(t, str, "TLS Handshake: 28.7ms")
		assert.Contains(t, str, "Server Time:   45.2ms")
		assert.Contains(t, str, "Total Time:    91.3ms")
	})

	t.Run("given nil trace info, then returns nil message", func(t *testing.T) {
		var info *TraceInfo
		str := info.String()
		assert.Contains(t, str, "nil")
	})
}

func TestDecodeBody(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	tests := []struct {
		name        string
		body        []byte
		contentType string
		wantName    string
	}{
		{
			name:        "given JSON content-type, then decodes as JSON",
			body:        []byte(`{"id":1,"name":"John"}`),
			contentType: "application/json",
			wantName:    "John",
		},
		{
			name:        "given JSON with charset, then decodes as JSON",
			body:        []byte(`{"id":1,"name":"Jane"}`),
			contentType: "application/json; charset=utf-8",
			wantName:    "Jane",
		},
		{
			name:        "given no content-type, then defaults to JSON",
			body:        []byte(`{"id":1,"name":"Default"}`),
			contentType: "",
			wantName:    "Default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var user User
			err := decodeBody(tt.body, tt.contentType, &user)

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, user.Name)
		})
	}
}
