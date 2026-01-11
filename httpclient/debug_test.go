package httpclient

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCurlCommand(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		url          string
		headers      http.Header
		body         []byte
		wantContains []string
	}{
		{
			name:    "given GET request, then generates basic curl",
			method:  http.MethodGet,
			url:     "https://api.example.com/users",
			headers: nil,
			body:    nil,
			wantContains: []string{
				"curl",
				"'https://api.example.com/users'",
			},
		},
		{
			name:   "given POST request, then includes -X POST",
			method: http.MethodPost,
			url:    "https://api.example.com/users",
			headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
			body: []byte(`{"name":"John"}`),
			wantContains: []string{
				"curl",
				"-X", "POST",
				"-H", "'Content-Type: application/json'",
				"-d", `'{"name":"John"}'`,
			},
		},
		{
			name:   "given multiple headers, then includes all",
			method: http.MethodGet,
			url:    "https://api.example.com/users",
			headers: http.Header{
				"Authorization": []string{"Bearer token123"},
				"Accept":        []string{"application/json"},
			},
			body: nil,
			wantContains: []string{
				"-H", "'Accept: application/json'",
				"-H", "'Authorization: Bearer token123'",
			},
		},
		{
			name:    "given body with single quotes, then escapes them",
			method:  http.MethodPost,
			url:     "https://api.example.com/data",
			headers: nil,
			body:    []byte(`{"message":"it's working"}`),
			wantContains: []string{
				"-d",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, tt.url, nil)
			req.Header = tt.headers

			result := generateCurlCommand(req, tt.body)

			for _, want := range tt.wantContains {
				assert.Contains(t, result, want)
			}
		})
	}
}

func TestRequestTracer_ToTraceInfo(t *testing.T) {
	t.Run("given empty tracer, then returns zero values", func(t *testing.T) {
		tracer := &requestTracer{}
		info := tracer.toTraceInfo()

		assert.Equal(t, "0s", info.DNSLookup)
		assert.Equal(t, "0s", info.ConnTime)
		assert.Empty(t, info.TLSHandshake)
		assert.Equal(t, "0s", info.ServerTime)
		assert.Equal(t, "0s", info.TotalTime)
	})
}
