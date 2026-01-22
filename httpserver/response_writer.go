package httpserver

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
)

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int
	wroteHeader  bool

	// For body logging (optional)
	bodyBuffer  *bytes.Buffer
	maxBodySize int
}

// wrapResponseWriter creates a new responseWriter.
func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		status:         http.StatusOK, // Default status
	}
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// Write captures bytes written and ensures header is written.
// If bodyBuffer is set, it also captures the response body up to maxBodySize.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	// Capture body for logging if enabled
	if rw.bodyBuffer != nil && rw.bodyBuffer.Len() < rw.maxBodySize {
		remaining := rw.maxBodySize - rw.bodyBuffer.Len()
		if len(b) <= remaining {
			rw.bodyBuffer.Write(b)
		} else {
			rw.bodyBuffer.Write(b[:remaining])
		}
	}

	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Status returns the captured status code.
func (rw *responseWriter) Status() int {
	return rw.status
}

// BytesWritten returns the number of bytes written to the response.
func (rw *responseWriter) BytesWritten() int {
	return rw.bytesWritten
}

// Unwrap returns the underlying ResponseWriter.
// This is useful for middleware that needs access to the original writer.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Flush implements http.Flusher.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push implements http.Pusher.
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
