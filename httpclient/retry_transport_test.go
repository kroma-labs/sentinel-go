package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kroma-labs/sentinel-go/httpclient/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetryTransport_RoundTrip(t *testing.T) {
	type args struct {
		method string
		url    string
		body   string
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(*mocks.RoundTripper)
		cfgOpts []Option
		wantErr assert.ErrorAssertionFunc
		wantSC  int
	}{
		{
			name: "given successful first attempt, then returns response",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("OK")),
					}, nil).Once()
			},
			cfgOpts: []Option{WithRetryConfig(DefaultRetryConfig())},
			wantErr: assert.NoError,
			wantSC:  200,
		},
		{
			name: "given retryable error then success, then returns response",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				// First attempt fails
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(nil, errors.New("connection reset by peer")).Once()
				// Second attempt succeeds
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("OK")),
					}, nil).Once()
			},
			cfgOpts: []Option{
				WithRetryConfig(RetryConfig{
					MaxRetries:      3,
					InitialInterval: 1 * time.Millisecond,
					MaxInterval:     5 * time.Millisecond,
					Multiplier:      2.0,
					JitterFactor:    0.1,
				}),
			},
			wantErr: assert.NoError,
			wantSC:  200,
		},
		{
			name: "given retries exhausted, then returns error",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				// Fail 3 times (initial + 2 retries)
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(nil, errors.New("connection reset by peer")).Times(3)
			},
			cfgOpts: []Option{
				WithRetryConfig(RetryConfig{
					MaxRetries:      2,
					InitialInterval: 1 * time.Millisecond,
					MaxInterval:     5 * time.Millisecond,
					Multiplier:      2.0,
					JitterFactor:    0.1,
				}),
			},
			wantErr: assert.Error,
			wantSC:  0,
		},
		{
			name: "given context canceled, then returns error",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(nil, context.Canceled).Once()
			},
			cfgOpts: []Option{WithRetryConfig(DefaultRetryConfig())},
			wantErr: assert.Error,
			wantSC:  0,
		},
		{
			name: "given non-retryable TLS error, then returns error without retry",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				// Only called once - no retry for permanent errors
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(nil, errors.New("x509: certificate has expired")).Once()
			},
			cfgOpts: []Option{WithRetryConfig(DefaultRetryConfig())},
			wantErr: assert.Error,
			wantSC:  0,
		},
		{
			name: "given 503 then 200, then retries and returns success",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				// First returns 503
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{
						StatusCode: http.StatusServiceUnavailable,
						Body:       io.NopCloser(bytes.NewBufferString("Service Unavailable")),
					}, nil).Once()
				// Second returns 200
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("OK")),
					}, nil).Once()
			},
			cfgOpts: []Option{
				WithRetryConfig(RetryConfig{
					MaxRetries:      1,
					InitialInterval: 1 * time.Millisecond,
					MaxInterval:     5 * time.Millisecond,
					Multiplier:      2.0,
					JitterFactor:    0.1,
				}),
			},
			wantErr: assert.NoError,
			wantSC:  200,
		},
		{
			name: "given 500 with custom classifier, then retries and returns success",
			args: args{method: "GET", url: "http://example.com", body: ""},
			mockFn: func(rt *mocks.RoundTripper) {
				// First returns 500
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
					}, nil).Once()
				// Second returns 200
				rt.EXPECT().
					RoundTrip(mock.Anything).
					Return(&http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("OK")),
					}, nil).Once()
			},
			cfgOpts: []Option{
				WithRetryConfig(RetryConfig{
					MaxRetries:      1,
					InitialInterval: 1 * time.Millisecond,
					MaxInterval:     5 * time.Millisecond,
					Multiplier:      2.0,
					JitterFactor:    0.1,
				}),
				WithRetryClassifier(func(resp *http.Response, err error) bool {
					if resp != nil && resp.StatusCode == http.StatusInternalServerError {
						return true
					}
					return err != nil
				}),
			},
			wantErr: assert.NoError,
			wantSC:  200,
		},
		{
			name: "given request with body, then preserves body on retry",
			args: args{method: "POST", url: "http://example.com", body: "test body"},
			mockFn: func(rt *mocks.RoundTripper) {
				// First attempt fails
				rt.EXPECT().
					RoundTrip(mock.MatchedBy(func(r *http.Request) bool {
						return r.Method == http.MethodPost
					})).
					Return(nil, errors.New("connection refused")).Once()
				// Second attempt succeeds
				rt.EXPECT().
					RoundTrip(mock.MatchedBy(func(r *http.Request) bool {
						if r.Body == nil {
							return false
						}
						buf := new(bytes.Buffer)
						buf.ReadFrom(r.Body)
						return buf.String() == "test body"
					})).
					Return(&http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("OK")),
					}, nil).Once()
			},
			cfgOpts: []Option{
				WithRetryConfig(RetryConfig{
					MaxRetries:      1,
					InitialInterval: 1 * time.Millisecond,
					MaxInterval:     5 * time.Millisecond,
					Multiplier:      2.0,
					JitterFactor:    0.1,
				}),
			},
			wantErr: assert.NoError,
			wantSC:  200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRT := mocks.NewRoundTripper(t)
			tt.mockFn(mockRT)

			cfg := newConfig(tt.cfgOpts...)
			rt := newRetryTransport(mockRT, cfg)

			var body io.Reader
			if tt.args.body != "" {
				body = bytes.NewBufferString(tt.args.body)
			}
			req := httptest.NewRequest(tt.args.method, tt.args.url, body)

			resp, err := rt.RoundTrip(req)

			tt.wantErr(t, err)
			if err == nil {
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSC, resp.StatusCode)
			}
		})
	}
}

func TestRetryTransport_Disabled(t *testing.T) {
	t.Run("given retry disabled, then returns base transport directly", func(t *testing.T) {
		mockRT := mocks.NewRoundTripper(t)
		cfg := newConfig(WithRetryDisabled())

		rt := newRetryTransport(mockRT, cfg)

		assert.Equal(t, mockRT, rt)
	})
}
