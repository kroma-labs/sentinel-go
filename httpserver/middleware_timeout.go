package httpserver

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns middleware that limits request processing time.
//
// If the handler takes longer than the timeout, the request context is
// cancelled and a 503 Service Unavailable response is returned.
//
// Note: The handler must respect context cancellation for this to work
// effectively.
//
// Example:
//
//	handler := httpserver.Timeout(30 * time.Second)(myHandler)
func Timeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create context with timeout
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Create channel to signal completion
			done := make(chan struct{})

			// Wrap response writer to prevent writes after timeout
			wrapped := &timeoutWriter{
				ResponseWriter: w,
				done:           done,
			}

			// Run handler in goroutine
			go func() {
				defer close(done)
				next.ServeHTTP(wrapped, r.WithContext(ctx))
			}()

			// Wait for completion or timeout
			select {
			case <-done:
				// Handler completed normally
			case <-ctx.Done():
				// Timeout occurred
				wrapped.timedOut = true
				WriteError(w, http.StatusServiceUnavailable,
					"request timeout",
					Error{Field: "server", Message: "request processing timed out"},
				)
			}
		})
	}
}

// timeoutWriter prevents writes after timeout.
type timeoutWriter struct {
	http.ResponseWriter
	done     chan struct{}
	timedOut bool
	wrote    bool
}

func (tw *timeoutWriter) WriteHeader(code int) {
	if tw.timedOut || tw.wrote {
		return
	}
	tw.wrote = true
	tw.ResponseWriter.WriteHeader(code)
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	if tw.timedOut {
		return 0, context.DeadlineExceeded
	}
	if !tw.wrote {
		tw.WriteHeader(http.StatusOK)
	}
	return tw.ResponseWriter.Write(b)
}
