package httpserver

import (
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog"
)

// Recovery returns middleware that recovers from panics.
//
// When a panic occurs:
//   - The panic is recovered
//   - A 500 Internal Server Error is returned
//   - The stack trace is logged (if logger provided)
//   - The request continues to the next middleware
//
// Example:
//
//	handler := httpserver.Recovery(logger)(myHandler)
func Recovery(logger zerolog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Get stack trace
					stack := debug.Stack()

					// Log the panic
					logger.Error().
						Interface("panic", rec).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("stack", string(stack)).
						Msg("panic recovered")

					// Return 500 error
					WriteError(w, http.StatusInternalServerError,
						"internal server error",
						Error{Field: "server", Message: "an unexpected error occurred"},
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
