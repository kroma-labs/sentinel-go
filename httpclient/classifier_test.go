package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultClassifier(t *testing.T) {
	type args struct {
		resp *http.Response
		err  error
	}
	tests := []struct {
		name      string
		args      args
		wantRetry bool
	}{
		{
			name: "given 200 response, then returns false",
			args: args{
				resp: &http.Response{StatusCode: http.StatusOK},
				err:  nil,
			},
			wantRetry: false,
		},
		{
			name: "given 201 response, then returns false",
			args: args{
				resp: &http.Response{StatusCode: http.StatusCreated},
				err:  nil,
			},
			wantRetry: false,
		},
		{
			name: "given 400 response, then returns false",
			args: args{
				resp: &http.Response{StatusCode: http.StatusBadRequest},
				err:  nil,
			},
			wantRetry: false,
		},
		{
			name: "given 401 response, then returns false",
			args: args{
				resp: &http.Response{StatusCode: http.StatusUnauthorized},
				err:  nil,
			},
			wantRetry: false,
		},
		{
			name: "given 404 response, then returns false",
			args: args{
				resp: &http.Response{StatusCode: http.StatusNotFound},
				err:  nil,
			},
			wantRetry: false,
		},
		{
			name: "given 429 response, then returns true",
			args: args{
				resp: &http.Response{StatusCode: http.StatusTooManyRequests},
				err:  nil,
			},
			wantRetry: true,
		},
		{
			name: "given 500 response, then returns false",
			args: args{
				resp: &http.Response{StatusCode: http.StatusInternalServerError},
				err:  nil,
			},
			wantRetry: false,
		},
		{
			name: "given 502 response, then returns true",
			args: args{
				resp: &http.Response{StatusCode: http.StatusBadGateway},
				err:  nil,
			},
			wantRetry: true,
		},
		{
			name: "given 503 response, then returns true",
			args: args{
				resp: &http.Response{StatusCode: http.StatusServiceUnavailable},
				err:  nil,
			},
			wantRetry: true,
		},
		{
			name: "given 504 response, then returns true",
			args: args{
				resp: &http.Response{StatusCode: http.StatusGatewayTimeout},
				err:  nil,
			},
			wantRetry: true,
		},
		{
			name: "given context canceled, then returns false",
			args: args{
				resp: nil,
				err:  context.Canceled,
			},
			wantRetry: false,
		},
		{
			name: "given context deadline exceeded, then returns false",
			args: args{
				resp: nil,
				err:  context.DeadlineExceeded,
			},
			wantRetry: false,
		},
		{
			name: "given connection refused error, then returns true",
			args: args{
				resp: nil,
				err:  errors.New("connection refused"),
			},
			wantRetry: true,
		},
		{
			name: "given connection reset error, then returns true",
			args: args{
				resp: nil,
				err:  errors.New("connection reset by peer"),
			},
			wantRetry: true,
		},
		{
			name: "given timeout error, then returns true",
			args: args{
				resp: nil,
				err:  &timeoutError{},
			},
			wantRetry: true,
		},
		{
			name: "given DNS error (temporary), then returns true",
			args: args{
				resp: nil,
				err: &net.DNSError{
					Err:         "lookup failed",
					IsTemporary: true,
				},
			},
			wantRetry: true,
		},
		{
			name: "given TLS certificate error, then returns false",
			args: args{
				resp: nil,
				err:  errors.New("x509: certificate has expired"),
			},
			wantRetry: false,
		},
		{
			name: "given unknown error, then returns true",
			args: args{
				resp: nil,
				err:  errors.New("some unknown error"),
			},
			wantRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultClassifier(tt.args.resp, tt.args.err)
			assert.Equal(t, tt.wantRetry, got)
		})
	}
}

// timeoutError implements net.Error with Timeout() returning true.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"200 OK", http.StatusOK, false},
		{"201 Created", http.StatusCreated, false},
		{"400 Bad Request", http.StatusBadRequest, false},
		{"401 Unauthorized", http.StatusUnauthorized, false},
		{"403 Forbidden", http.StatusForbidden, false},
		{"404 Not Found", http.StatusNotFound, false},
		{"429 Too Many Requests", http.StatusTooManyRequests, true},
		{"500 Internal Server Error", http.StatusInternalServerError, false},
		{"502 Bad Gateway", http.StatusBadGateway, true},
		{"503 Service Unavailable", http.StatusServiceUnavailable, true},
		{"504 Gateway Timeout", http.StatusGatewayTimeout, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableStatusCode(tt.statusCode)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsRetryableNetworkError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "connection refused",
			err:  errors.New("dial tcp: connection refused"),
			want: true,
		},
		{
			name: "connection reset",
			err:  errors.New("connection reset by peer"),
			want: true,
		},
		{
			name: "no such host",
			err:  errors.New("dial tcp: lookup host: no such host"),
			want: true,
		},
		{
			name: "network is down",
			err:  errors.New("network is down"),
			want: true,
		},
		{
			name: "i/o timeout",
			err:  errors.New("i/o timeout"),
			want: true,
		},
		{
			name: "EOF",
			err:  errors.New("unexpected EOF"),
			want: true,
		},
		{
			name: "broken pipe",
			err:  errors.New("write: broken pipe"),
			want: true,
		},
		{
			name: "timeout net.Error",
			err:  &timeoutError{},
			want: true,
		},
		{
			name: "DNS temporary error",
			err: &net.DNSError{
				Err:         "temporary failure",
				IsTemporary: true,
			},
			want: true,
		},
		{
			name: "DNS permanent error",
			err: &net.DNSError{
				Err:         "no such host",
				IsTemporary: false,
			},
			want: false,
		},
		{
			name: "random error",
			err:  errors.New("some random error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableNetworkError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "certificate error",
			err:  errors.New("x509: certificate signed by unknown authority"),
			want: true,
		},
		{
			name: "TLS error",
			err:  errors.New("tls: handshake failure"),
			want: true,
		},
		{
			name: "no route to host",
			err:  errors.New("dial tcp: no route to host"),
			want: true,
		},
		{
			name: "permission denied",
			err:  errors.New("permission denied"),
			want: true,
		},
		{
			name: "protocol error",
			err:  errors.New("http2: protocol error"),
			want: true,
		},
		{
			name: "normal network error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPermanentError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStatusCodeClassifier(t *testing.T) {
	classifier := StatusCodeClassifier(500, 502, 503)

	tests := []struct {
		name       string
		statusCode int
		err        error
		want       bool
	}{
		{
			name:       "given 500, then returns true",
			statusCode: 500,
			err:        nil,
			want:       true,
		},
		{
			name:       "given 502, then returns true",
			statusCode: 502,
			err:        nil,
			want:       true,
		},
		{
			name:       "given 503, then returns true",
			statusCode: 503,
			err:        nil,
			want:       true,
		},
		{
			name:       "given 504 (not in list), then returns false",
			statusCode: 504,
			err:        nil,
			want:       false,
		},
		{
			name:       "given network error, then returns true",
			statusCode: 0,
			err:        errors.New("connection refused"),
			want:       true,
		},
		{
			name:       "given permanent error, then returns false",
			statusCode: 0,
			err:        errors.New("x509: certificate error"),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.statusCode != 0 {
				resp = &http.Response{StatusCode: tt.statusCode}
			}
			got := classifier(resp, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAlwaysRetryClassifier(t *testing.T) {
	classifier := AlwaysRetryClassifier()

	tests := []struct {
		name string
		resp *http.Response
		err  error
		want bool
	}{
		{
			name: "given error, then returns true",
			resp: nil,
			err:  errors.New("some error"),
			want: true,
		},
		{
			name: "given 500 response, then returns true",
			resp: &http.Response{StatusCode: http.StatusInternalServerError},
			err:  nil,
			want: true,
		},
		{
			name: "given 200 response, then returns false",
			resp: &http.Response{StatusCode: http.StatusOK},
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier(tt.resp, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNeverRetryClassifier(t *testing.T) {
	classifier := NeverRetryClassifier()

	tests := []struct {
		name string
		resp *http.Response
		err  error
		want bool
	}{
		{
			name: "given error, then returns false",
			resp: nil,
			err:  errors.New("some error"),
			want: false,
		},
		{
			name: "given 500 response, then returns false",
			resp: &http.Response{StatusCode: http.StatusInternalServerError},
			err:  nil,
			want: false,
		},
		{
			name: "given 200 response, then returns false",
			resp: &http.Response{StatusCode: http.StatusOK},
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier(tt.resp, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
