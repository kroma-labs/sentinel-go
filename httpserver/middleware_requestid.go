package httpserver

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// RequestIDHeader is the header key for request IDs.
const RequestIDHeader = "X-Request-ID"

// requestIDKey is the context key for request ID.
type requestIDKey struct{}

// RequestID returns middleware that generates or forwards request IDs.
//
// Behavior:
//   - If X-Request-ID header exists, use it
//   - Otherwise, generate a new UUID v4
//   - Add the ID to the response header
//   - Store the ID in the request context
//
// Example:
//
//	handler := httpserver.RequestID()(myHandler)
//
//	// Access in handler:
//	func myHandler(w http.ResponseWriter, r *http.Request) {
//	    id := httpserver.RequestIDFromContext(r.Context())
//	    log.Printf("Request ID: %s", id)
//	}
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get or generate request ID
			id := r.Header.Get(RequestIDHeader)
			if id == "" {
				id = uuid.New().String()
			}

			// Add to response header
			w.Header().Set(RequestIDHeader, id)

			// Add to context
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext extracts the request ID from the context.
//
// Returns an empty string if no request ID is present.
//
// Example:
//
//	func myHandler(w http.ResponseWriter, r *http.Request) {
//	    id := httpserver.RequestIDFromContext(r.Context())
//	    log.Printf("Processing request: %s", id)
//	}
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}
