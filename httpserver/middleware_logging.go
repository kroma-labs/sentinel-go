package httpserver

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// LoggerConfig configures the logging middleware.
type LoggerConfig struct {
	Logger zerolog.Logger

	// serviceName is set internally by the server.
	serviceName string

	// SkipPaths are paths that should not be logged.
	// Useful for health check endpoints that are called frequently.
	SkipPaths []string

	// LogRequestBody enables logging of request body (use with caution).
	LogRequestBody bool

	// LogResponseBody enables logging of response body (use with caution).
	LogResponseBody bool
}

// Logger returns middleware that logs HTTP requests.
//
// Logs include:
//   - Method, path, status code
//   - Request duration
//   - Request ID (if present)
//   - Client IP
//
// Example:
//
//	handler := httpserver.Logger(httpserver.LoggerConfig{
//	    Logger:    logger,
//	    SkipPaths: []string{"/livez", "/readyz", "/ping"},
//	})(myHandler)
func Logger(cfg LoggerConfig) Middleware {
	skipPaths := make(map[string]bool)
	for _, path := range cfg.SkipPaths {
		skipPaths[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for certain paths
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := wrapResponseWriter(w)

			// Process request
			next.ServeHTTP(wrapped, r)

			// Calculate duration
			duration := time.Since(start)

			// Get request ID if present
			requestID := RequestIDFromContext(r.Context())

			// Build log event
			event := cfg.Logger.Info()
			if wrapped.Status() >= 400 {
				event = cfg.Logger.Warn()
			}
			if wrapped.Status() >= 500 {
				event = cfg.Logger.Error()
			}

			event.
				Str("service", cfg.serviceName).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", wrapped.Status()).
				Dur("duration", duration).
				Int("bytes", wrapped.BytesWritten()).
				Str("remote_addr", r.RemoteAddr).
				Str("user_agent", r.UserAgent())

			if requestID != "" {
				event.Str("request_id", requestID)
			}

			event.Msg("request completed")
		})
	}
}
