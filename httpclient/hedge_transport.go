package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// hedgeTransport wraps an http.RoundTripper to implement hedged requests.
type hedgeTransport struct {
	next   http.RoundTripper
	config HedgeConfig
}

// newHedgeTransport creates a new hedge transport wrapper.
func newHedgeTransport(next http.RoundTripper, cfg HedgeConfig) http.RoundTripper {
	if !cfg.Enabled() {
		return next
	}
	return &hedgeTransport{
		next:   next,
		config: cfg,
	}
}

// hedgeResult holds the result of a single request attempt.
type hedgeResult struct {
	resp *http.Response
	err  error
}

// RoundTrip implements http.RoundTripper with hedged requests.
func (t *hedgeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read and buffer the request body so we can replay it for hedges
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	results := make(chan hedgeResult, t.config.MaxHedges+1)
	var wg sync.WaitGroup

	// Start the original request
	wg.Add(1)
	go func() {
		defer wg.Done()
		t.doRequest(ctx, req, bodyBytes, results)
	}()

	// Set up timers for hedge requests
	hedgeTimers := make([]*time.Timer, t.config.MaxHedges)
	for i := range t.config.MaxHedges {
		delay := t.config.Delay * time.Duration(i+1)
		hedgeTimers[i] = time.AfterFunc(delay, func() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				t.doRequest(ctx, req, bodyBytes, results)
			}()
		})
	}

	// Wait for first result
	result := <-results

	// Cancel remaining requests and stop timers
	cancel()
	for _, timer := range hedgeTimers {
		timer.Stop()
	}

	// Wait for all goroutines to finish in background to avoid leaks
	go func() {
		wg.Wait()
		close(results)
		// Drain and close any remaining responses
		for r := range results {
			if r.resp != nil && r.resp.Body != nil {
				r.resp.Body.Close()
			}
		}
	}()

	return result.resp, result.err
}

// doRequest executes a single request and sends the result.
func (t *hedgeTransport) doRequest(
	ctx context.Context,
	original *http.Request,
	bodyBytes []byte,
	results chan<- hedgeResult,
) {
	// Clone the request with the cancellable context
	req := original.Clone(ctx)
	if bodyBytes != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	resp, err := t.next.RoundTrip(req)

	// Only send result if context hasn't been cancelled
	select {
	case <-ctx.Done():
		// Context cancelled, close response if we got one
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	case results <- hedgeResult{resp: resp, err: err}:
		// Result sent
	}
}
