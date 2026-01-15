package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeout_PerRequestTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		serverDelay    time.Duration
		requestTimeout time.Duration
		wantError      bool
	}{
		{
			name:           "given_request_completes_before_timeout,_then_success",
			serverDelay:    10 * time.Millisecond,
			requestTimeout: 1 * time.Second,
			wantError:      false,
		},
		{
			name:           "given_request_exceeds_timeout,_then_timeout_error",
			serverDelay:    100 * time.Millisecond,
			requestTimeout: 10 * time.Millisecond,
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					time.Sleep(tt.serverDelay)
					w.WriteHeader(http.StatusOK)
				}),
			)
			defer server.Close()

			client := New(WithBaseURL(server.URL))

			_, err := client.Request("GetData").
				Timeout(tt.requestTimeout).
				Get(context.Background(), "/data")

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "context deadline exceeded")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTimeout_ShortestTimeoutWins(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	// Context with 50ms deadline (shortest)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Request timeout is 2 seconds (longer than context)
	// The shortest timeout (context 50ms) should win
	_, err := client.Request("GetData").
		Timeout(2*time.Second).
		Get(ctx, "/data")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestTimeout_NoTimeoutSetSucceeds(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	resp, err := client.Request("GetData").
		Get(context.Background(), "/data")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
