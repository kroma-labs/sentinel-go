package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewWrappedBody(t *testing.T) {
	type args struct {
		body io.ReadCloser
	}

	tests := []struct {
		name    string
		args    args
		wantNil bool
	}{
		{
			name:    "given nil body, then returns nil",
			args:    args{body: nil},
			wantNil: true,
		},
		{
			name:    "given valid body, then returns wrapped body",
			args:    args{body: io.NopCloser(bytes.NewReader([]byte("test")))},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer tp.Shutdown(context.Background())

			_, span := tp.Tracer("test").Start(context.Background(), "test")

			result := newWrappedBody(span, tt.args.body, nil)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
			}
		})
	}
}

func TestWrappedBody_Read(t *testing.T) {
	type args struct {
		content string
	}

	tests := []struct {
		name          string
		args          args
		wantBytesRead int
		wantSpanEnded bool
	}{
		{
			name:          "given content, then reads and tracks bytes",
			args:          args{content: "hello world"},
			wantBytesRead: 11,
			wantSpanEnded: true, // EOF ends span
		},
		{
			name:          "given empty content, then reads zero bytes",
			args:          args{content: ""},
			wantBytesRead: 0,
			wantSpanEnded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer tp.Shutdown(context.Background())

			_, span := tp.Tracer("test").Start(context.Background(), "test")

			var recordedBytes int64
			body := newWrappedBody(
				span,
				io.NopCloser(bytes.NewReader([]byte(tt.args.content))),
				func(n int64) { recordedBytes = n },
			)

			// Read all content
			buf := make([]byte, 1024)
			totalRead := 0
			for {
				n, err := body.Read(buf)
				totalRead += n
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantBytesRead, totalRead)
			assert.Equal(t, int64(tt.wantBytesRead), recordedBytes)

			// Verify span ended
			spans := exporter.GetSpans()
			if tt.wantSpanEnded {
				assert.Len(t, spans, 1)
			}
		})
	}
}

func TestWrappedBody_Close(t *testing.T) {
	t.Run("given open body, then close ends span", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		defer tp.Shutdown(context.Background())

		_, span := tp.Tracer("test").Start(context.Background(), "test")

		var closeCalled bool
		body := newWrappedBody(span, io.NopCloser(bytes.NewReader([]byte("test"))), func(_ int64) {
			closeCalled = true
		})

		err := body.Close()

		require.NoError(t, err)
		assert.True(t, closeCalled)

		// Verify span ended
		spans := exporter.GetSpans()
		assert.Len(t, spans, 1)
	})
}

func TestWrappedBody_CloseAfterEOF(t *testing.T) {
	t.Run("given EOF already reached, then close does not end span twice", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		defer tp.Shutdown(context.Background())

		_, span := tp.Tracer("test").Start(context.Background(), "test")

		closeCount := 0
		body := newWrappedBody(span, io.NopCloser(bytes.NewReader([]byte("x"))), func(_ int64) {
			closeCount++
		})

		// Read to EOF
		buf := make([]byte, 10)
		_, err := body.Read(buf)
		require.NoError(t, err)
		_, err = body.Read(buf)
		require.ErrorIs(t, err, io.EOF)

		// Close after EOF
		err = body.Close()
		require.NoError(t, err)

		// Callback should only be called once
		assert.Equal(t, 1, closeCount)
	})
}

func TestWrappedBody_ReadError(t *testing.T) {
	t.Run("given read error, then records error on span", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		defer tp.Shutdown(context.Background())

		_, span := tp.Tracer("test").Start(context.Background(), "test")

		expectedErr := errors.New("read error")
		body := newWrappedBody(span, &errorReader{err: expectedErr}, nil)

		buf := make([]byte, 10)
		_, err := body.Read(buf)

		require.ErrorIs(t, err, expectedErr)

		// Close to end span
		body.Close()

		// Verify span has error
		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		assert.NotEmpty(t, spans[0].Events) // Error should be recorded as event
	})
}

func TestWrappedBody_ReadWriteCloser(t *testing.T) {
	t.Run("given ReadWriteCloser, then preserves interface", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		defer tp.Shutdown(context.Background())

		_, span := tp.Tracer("test").Start(context.Background(), "test")

		rwc := &mockReadWriteCloser{
			reader: bytes.NewReader([]byte("readable")),
			writer: &bytes.Buffer{},
		}
		body := newWrappedBody(span, rwc, nil)

		// Should implement io.ReadWriteCloser
		_, ok := body.(io.ReadWriteCloser)
		assert.True(t, ok)

		// Test write
		writer := body.(io.ReadWriteCloser)
		n, err := writer.Write([]byte("test"))
		require.NoError(t, err)
		assert.Equal(t, 4, n)

		body.Close()
	})
}

// errorReader is a mock reader that always returns an error.
type errorReader struct {
	err error
}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, e.err
}

func (e *errorReader) Close() error {
	return nil
}

// mockReadWriteCloser implements io.ReadWriteCloser for testing.
type mockReadWriteCloser struct {
	reader io.Reader
	writer io.Writer
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	return m.writer.Write(p)
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}
