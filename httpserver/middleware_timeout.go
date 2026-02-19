package httpserver

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Timeout returns middleware that limits request processing time.
//
// If the handler takes longer than the timeout, the request context is
// cancelled and a 503 Service Unavailable response is returned.
//
// Note: The handler must respect context cancellation for this to work
// effectively. The handler runs in a separate goroutine; all writes to
// the underlying ResponseWriter are serialized via a mutex.
//
// Example:
//
//	handler := httpserver.Timeout(30 * time.Second)(myHandler)
func Timeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})

			wrapped := &timeoutWriter{
				ResponseWriter: w,
			}

			go func() {
				defer close(done)
				next.ServeHTTP(wrapped, r.WithContext(ctx))
			}()

			select {
			case <-done:
				// Handler completed normally
			case <-ctx.Done():
				wrapped.markTimedOut()
				WriteError(w, http.StatusServiceUnavailable,
					"request timeout",
					Error{Field: "server", Message: "request processing timed out"},
				)
			}
		})
	}
}

// timeoutWriter prevents writes after timeout.
//
// All field access is synchronized via mu because the handler goroutine
// calls Write/WriteHeader while the main goroutine may set timedOut.
type timeoutWriter struct {
	http.ResponseWriter
	mu       sync.Mutex
	timedOut bool
	wrote    bool
}

// markTimedOut atomically sets the timedOut flag.
func (tw *timeoutWriter) markTimedOut() {
	tw.mu.Lock()
	tw.timedOut = true
	tw.mu.Unlock()
}

func (tw *timeoutWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.timedOut || tw.wrote {
		return
	}
	tw.wrote = true
	tw.ResponseWriter.WriteHeader(code)
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	if tw.timedOut {
		tw.mu.Unlock()
		return 0, context.DeadlineExceeded
	}
	if !tw.wrote {
		tw.wrote = true
		tw.ResponseWriter.WriteHeader(http.StatusOK)
	}
	tw.mu.Unlock()
	return tw.ResponseWriter.Write(b)
}
