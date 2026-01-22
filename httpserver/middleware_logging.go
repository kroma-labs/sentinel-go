package httpserver

import (
	"bytes"
	"io"
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
	// This reads the entire request body into memory, which may impact
	// performance for large payloads. Consider using only in development.
	LogRequestBody bool

	// LogResponseBody enables logging of response body (use with caution).
	// This buffers the entire response body, which may impact performance
	// for large payloads. Consider using only in development.
	LogResponseBody bool

	// MaxBodyLogSize limits the size of logged bodies (default: 4KB).
	// Bodies larger than this will be truncated in the log output.
	MaxBodyLogSize int
}

const defaultMaxBodyLogSize = 4 * 1024 // 4KB

// Logger returns middleware that logs HTTP requests.
//
// Logs include:
//   - Method, path, status code
//   - Request duration
//   - Request ID (if present)
//   - Client IP
//   - Request/Response bodies (if enabled)
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

	maxBodySize := cfg.MaxBodyLogSize
	if maxBodySize <= 0 {
		maxBodySize = defaultMaxBodyLogSize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for certain paths
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Capture request body if enabled
			var requestBody []byte
			if cfg.LogRequestBody && r.Body != nil {
				requestBody, _ = io.ReadAll(io.LimitReader(r.Body, int64(maxBodySize)))
				r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(requestBody))
			}

			// Wrap response writer to capture status code and optionally body
			var wrapped *responseWriter
			var responseBody *bytes.Buffer

			if cfg.LogResponseBody {
				responseBody = &bytes.Buffer{}
				wrapped = wrapResponseWriterWithBody(w, responseBody, maxBodySize)
			} else {
				wrapped = wrapResponseWriter(w)
			}

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

			// Log request body if enabled and captured
			if cfg.LogRequestBody && len(requestBody) > 0 {
				event.Bytes("request_body", requestBody)
			}

			// Log response body if enabled and captured
			if cfg.LogResponseBody && responseBody != nil && responseBody.Len() > 0 {
				event.Bytes("response_body", responseBody.Bytes())
			}

			event.Msg("request completed")
		})
	}
}

// wrapResponseWriterWithBody creates a responseWriter that also captures the body.
func wrapResponseWriterWithBody(
	w http.ResponseWriter,
	body *bytes.Buffer,
	maxSize int,
) *responseWriter {
	rw := wrapResponseWriter(w)
	rw.bodyBuffer = body
	rw.maxBodySize = maxSize
	return rw
}
