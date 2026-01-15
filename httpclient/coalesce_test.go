package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCoalesceKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method1  string
		url1     string
		body1    []byte
		method2  string
		url2     string
		body2    []byte
		wantSame bool
	}{
		{
			name:     "given_identical_requests,_then_same_key",
			method1:  "GET",
			url1:     "https://example.com/users/123",
			body1:    nil,
			method2:  "GET",
			url2:     "https://example.com/users/123",
			body2:    nil,
			wantSame: true,
		},
		{
			name:     "given_different_methods,_then_different_key",
			method1:  "GET",
			url1:     "https://example.com/users/123",
			body1:    nil,
			method2:  "POST",
			url2:     "https://example.com/users/123",
			body2:    nil,
			wantSame: false,
		},
		{
			name:     "given_different_urls,_then_different_key",
			method1:  "GET",
			url1:     "https://example.com/users/123",
			body1:    nil,
			method2:  "GET",
			url2:     "https://example.com/users/456",
			body2:    nil,
			wantSame: false,
		},
		{
			name:     "given_different_query_params,_then_different_key",
			method1:  "GET",
			url1:     "https://example.com/users?active=true",
			body1:    nil,
			method2:  "GET",
			url2:     "https://example.com/users?active=false",
			body2:    nil,
			wantSame: false,
		},
		{
			name:     "given_same_query_params_different_order,_then_same_key",
			method1:  "GET",
			url1:     "https://example.com/users?a=1&b=2",
			body1:    nil,
			method2:  "GET",
			url2:     "https://example.com/users?b=2&a=1",
			body2:    nil,
			wantSame: true,
		},
		{
			name:     "given_different_body,_then_different_key",
			method1:  "POST",
			url1:     "https://example.com/users",
			body1:    []byte(`{"name":"John"}`),
			method2:  "POST",
			url2:     "https://example.com/users",
			body2:    []byte(`{"name":"Jane"}`),
			wantSame: false,
		},
		{
			name:     "given_same_body,_then_same_key",
			method1:  "POST",
			url1:     "https://example.com/users",
			body1:    []byte(`{"name":"John"}`),
			method2:  "POST",
			url2:     "https://example.com/users",
			body2:    []byte(`{"name":"John"}`),
			wantSame: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key1 := GenerateCoalesceKey(tt.method1, tt.url1, tt.body1)
			key2 := GenerateCoalesceKey(tt.method2, tt.url2, tt.body2)

			if tt.wantSame {
				assert.Equal(t, key1, key2)
			} else {
				assert.NotEqual(t, key1, key2)
			}
		})
	}
}

func TestCoalesce_DeduplicatesSimultaneousRequests(t *testing.T) {
	t.Parallel()

	// Track number of actual server calls
	var serverCalls atomic.Int32
	var requestMu sync.Mutex
	var requestsReceived int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverCalls.Add(1)
		requestMu.Lock()
		requestsReceived++
		requestMu.Unlock()

		// Simulate some processing time
		time.Sleep(50 * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	// Launch multiple concurrent requests
	const numRequests = 10
	var wg sync.WaitGroup
	responses := make([]*Response, numRequests)
	errors := make([]error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := client.Request("GetData").
				Coalesce().
				Get(context.Background(), "/data")
			responses[idx] = resp
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All requests should succeed
	for i := 0; i < numRequests; i++ {
		require.NoError(t, errors[i], "request %d should not error", i)
		require.NotNil(t, responses[i], "response %d should not be nil", i)
		assert.Equal(t, http.StatusOK, responses[i].StatusCode)
	}

	// Only one server call should be made (others coalesced)
	assert.Equal(t, int32(1), serverCalls.Load(), "only one server call should be made")
}

func TestCoalesce_SequentialRequestsMakeNewCalls(t *testing.T) {
	t.Parallel()

	var serverCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	// First request
	resp1, err1 := client.Request("GetData").Coalesce().Get(context.Background(), "/data")
	require.NoError(t, err1)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// Second request (sequential, after first completes)
	resp2, err2 := client.Request("GetData").Coalesce().Get(context.Background(), "/data")
	require.NoError(t, err2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Both requests should result in separate server calls (no caching/staleness)
	assert.Equal(t, int32(2), serverCalls.Load(), "sequential requests should make separate calls")
}

func TestCoalesce_DifferentEndpointsNotCoalesced(t *testing.T) {
	t.Parallel()

	var serverCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	var wg sync.WaitGroup

	// Request to /data
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = client.Request("GetData").Coalesce().Get(context.Background(), "/data")
	}()

	// Request to /other (different endpoint)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = client.Request("GetOther").Coalesce().Get(context.Background(), "/other")
	}()

	wg.Wait()

	// Both requests should be made (different endpoints)
	assert.Equal(t, int32(2), serverCalls.Load(), "different endpoints should not be coalesced")
}
