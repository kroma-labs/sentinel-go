// Package httpclient provides body wrapping utilities for response body tracking.
package httpclient

import (
	"io"
	"sync/atomic"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// wrappedBody wraps an http.Response.Body to:
// 1. Track the number of bytes read
// 2. Record errors on the span
// 3. End the span when the body is closed or EOF is reached
//
// This ensures spans accurately reflect the full request lifecycle,
// including body consumption time for streaming responses.
type wrappedBody struct {
	span   trace.Span
	body   io.ReadCloser
	read   atomic.Int64
	closed atomic.Bool

	// onClose is called with total bytes read when body is closed
	onClose func(bytesRead int64)
}

// newWrappedBody creates a wrapped body that ends the span on close/EOF.
//
// The onClose callback is invoked with the total bytes read when the body
// is closed. This can be used to record response body size metrics.
func newWrappedBody(
	span trace.Span,
	body io.ReadCloser,
	onClose func(bytesRead int64),
) io.ReadCloser {
	if body == nil {
		return nil
	}

	wb := &wrappedBody{
		span:    span,
		body:    body,
		onClose: onClose,
	}

	// Preserve io.ReadWriteCloser interface for protocol upgrade responses
	// (e.g., WebSocket upgrade where body implements io.Writer)
	if _, ok := body.(io.ReadWriteCloser); ok {
		return &readWriteCloserWrapper{wrappedBody: wb}
	}

	return wb
}

// Read reads from the underlying body, tracking bytes and errors.
func (w *wrappedBody) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)
	w.read.Add(int64(n))

	switch err {
	case nil:
		// Normal read, continue
	case io.EOF:
		// End of body reached - this is success, end span normally
		w.endSpan()
	default:
		// Read error - record on span
		w.span.RecordError(err)
		w.span.SetStatus(codes.Error, err.Error())
	}

	return n, err
}

// Close closes the underlying body and ends the span.
func (w *wrappedBody) Close() error {
	w.endSpan()

	if w.body != nil {
		return w.body.Close()
	}
	return nil
}

// endSpan ends the span exactly once and calls the onClose callback.
func (w *wrappedBody) endSpan() {
	// Ensure we only end the span once (Close could be called after EOF)
	if w.closed.CompareAndSwap(false, true) {
		if w.onClose != nil {
			w.onClose(w.read.Load())
		}
		w.span.End()
	}
}

// readWriteCloserWrapper extends wrappedBody for protocol upgrade responses
// that implement io.ReadWriteCloser (e.g., WebSocket).
type readWriteCloserWrapper struct {
	*wrappedBody
}

var _ io.ReadWriteCloser = (*readWriteCloserWrapper)(nil)

// Write delegates to the underlying body's Write method.
// Errors during writes are recorded on the span.
func (w *readWriteCloserWrapper) Write(p []byte) (int, error) {
	writer, ok := w.body.(io.Writer)
	if !ok {
		return 0, io.ErrClosedPipe
	}

	n, err := writer.Write(p)
	if err != nil {
		w.span.RecordError(err)
		w.span.SetStatus(codes.Error, err.Error())
	}
	return n, err
}
