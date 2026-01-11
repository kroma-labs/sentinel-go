package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http/httptrace"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Error type classifications for the error.type attribute.
const (
	ErrorTypeTimeout           = "timeout"
	ErrorTypeConnectionRefused = "connection_refused"
	ErrorTypeDNSError          = "dns_error"
	ErrorTypeTLSError          = "tls_error"
	ErrorTypeCancelled         = "cancelled"
	ErrorTypeConnectionReset   = "connection_reset"
	ErrorTypeEOF               = "eof"
	ErrorTypeUnknown           = "unknown"
)

// networkTrace holds timing data collected from httptrace.ClientTrace.
type networkTrace struct {
	// DNS timing
	dnsStart time.Time
	dnsDone  time.Time

	// Connection timing
	connectStart time.Time
	connectDone  time.Time

	// TLS timing
	tlsStart time.Time
	tlsDone  time.Time

	// Request/Response timing
	getConnTime       time.Time
	gotConnTime       time.Time
	wroteRequestTime  time.Time
	firstResponseTime time.Time

	// Connection info
	connReused  bool
	connRemote  string
	connLocal   string
	connIdle    bool
	protocolVer string

	// DNS info
	dnsAddrs []string
}

// createClientTrace creates an httptrace.ClientTrace that populates networkTrace.
func createClientTrace(nt *networkTrace) *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(_ string) {
			nt.getConnTime = time.Now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			nt.gotConnTime = time.Now()
			nt.connReused = info.Reused
			nt.connIdle = info.WasIdle
			if info.Conn != nil {
				if addr := info.Conn.RemoteAddr(); addr != nil {
					nt.connRemote = addr.String()
				}
				if addr := info.Conn.LocalAddr(); addr != nil {
					nt.connLocal = addr.String()
				}
			}
		},
		DNSStart: func(_ httptrace.DNSStartInfo) {
			nt.dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			nt.dnsDone = time.Now()
			if info.Addrs != nil {
				nt.dnsAddrs = make([]string, 0, len(info.Addrs))
				for _, addr := range info.Addrs {
					nt.dnsAddrs = append(nt.dnsAddrs, addr.String())
				}
			}
		},
		ConnectStart: func(_, _ string) {
			nt.connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			nt.connectDone = time.Now()
		},
		TLSHandshakeStart: func() {
			nt.tlsStart = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, _ error) {
			nt.tlsDone = time.Now()
			nt.protocolVer = state.NegotiatedProtocol
		},
		WroteRequest: func(_ httptrace.WroteRequestInfo) {
			nt.wroteRequestTime = time.Now()
		},
		GotFirstResponseByte: func() {
			nt.firstResponseTime = time.Now()
		},
	}
}

// addTraceEvents adds span events for network timing.
func (nt *networkTrace) addTraceEvents(span trace.Span) {
	// DNS events
	if !nt.dnsStart.IsZero() && !nt.dnsDone.IsZero() {
		span.AddEvent("dns.start", trace.WithTimestamp(nt.dnsStart))
		span.AddEvent("dns.done", trace.WithTimestamp(nt.dnsDone),
			trace.WithAttributes(
				attribute.Float64(
					"dns.duration_ms",
					float64(nt.dnsDone.Sub(nt.dnsStart).Milliseconds()),
				),
				attribute.StringSlice("dns.addresses", nt.dnsAddrs),
			))
	}

	// Connect events
	if !nt.connectStart.IsZero() && !nt.connectDone.IsZero() {
		span.AddEvent("connect.start", trace.WithTimestamp(nt.connectStart))
		span.AddEvent("connect.done", trace.WithTimestamp(nt.connectDone),
			trace.WithAttributes(
				attribute.Float64(
					"connect.duration_ms",
					float64(nt.connectDone.Sub(nt.connectStart).Milliseconds()),
				),
			))
	}

	// TLS events
	if !nt.tlsStart.IsZero() && !nt.tlsDone.IsZero() {
		span.AddEvent("tls.start", trace.WithTimestamp(nt.tlsStart))
		span.AddEvent("tls.done", trace.WithTimestamp(nt.tlsDone),
			trace.WithAttributes(
				attribute.Float64(
					"tls.duration_ms",
					float64(nt.tlsDone.Sub(nt.tlsStart).Milliseconds()),
				),
				attribute.String("tls.protocol", nt.protocolVer),
			))
	}

	// Connection acquired event
	if !nt.gotConnTime.IsZero() {
		span.AddEvent("got_conn", trace.WithTimestamp(nt.gotConnTime),
			trace.WithAttributes(
				attribute.Bool("connection.reused", nt.connReused),
				attribute.Bool("connection.was_idle", nt.connIdle),
				attribute.String("network.peer.address", nt.connRemote),
			))
	}

	// Request written event
	if !nt.wroteRequestTime.IsZero() {
		span.AddEvent("wrote_request", trace.WithTimestamp(nt.wroteRequestTime))
	}

	// First response byte event
	if !nt.firstResponseTime.IsZero() {
		var ttfbMs float64
		if !nt.wroteRequestTime.IsZero() {
			ttfbMs = float64(nt.firstResponseTime.Sub(nt.wroteRequestTime).Milliseconds())
		}
		span.AddEvent("got_first_response_byte", trace.WithTimestamp(nt.firstResponseTime),
			trace.WithAttributes(
				attribute.Float64("ttfb_ms", ttfbMs),
			))
	}
}

// recordTimingMetrics records network timing metrics.
func (nt *networkTrace) recordTimingMetrics(
	ctx context.Context,
	m *metrics,
	attrs []attribute.KeyValue,
) {
	if m == nil {
		return
	}

	// Track new connection opened (not reused from pool)
	if !nt.connReused && !nt.connectStart.IsZero() {
		m.recordConnectionOpened(ctx, attrs)
	}

	// DNS duration
	if !nt.dnsStart.IsZero() && !nt.dnsDone.IsZero() {
		m.recordDNSDuration(ctx, nt.dnsDone.Sub(nt.dnsStart), attrs)
	}

	// Connection duration
	if !nt.connectStart.IsZero() && !nt.connectDone.IsZero() {
		m.recordConnectionDuration(ctx, nt.connectDone.Sub(nt.connectStart), attrs)
	}

	// TLS duration
	if !nt.tlsStart.IsZero() && !nt.tlsDone.IsZero() {
		m.recordTLSDuration(ctx, nt.tlsDone.Sub(nt.tlsStart), attrs)
	}

	// TTFB (Time To First Byte)
	if !nt.wroteRequestTime.IsZero() && !nt.firstResponseTime.IsZero() {
		m.recordTTFB(ctx, nt.firstResponseTime.Sub(nt.wroteRequestTime), attrs)
	}
}

// classifyError returns an error.type classification for the given error.
func classifyError(err error) string {
	if err == nil {
		return ""
	}

	// Check for context cancellation
	if errors.Is(err, context.Canceled) {
		return ErrorTypeCancelled
	}

	// Check for deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTypeTimeout
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorTypeTimeout
		}
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ErrorTypeDNSError
	}

	// Check for TLS errors (certificate errors, handshake failures, etc.)
	var tlsRecordErr *tls.RecordHeaderError
	if errors.As(err, &tlsRecordErr) {
		return ErrorTypeTLSError
	}

	// Check for TLS certificate verification errors
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return ErrorTypeTLSError
	}

	// Check for syscall-level errors
	if errors.Is(err, syscall.ECONNREFUSED) {
		return ErrorTypeConnectionRefused
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return ErrorTypeConnectionReset
	}
	if errors.Is(err, io.EOF) {
		return ErrorTypeEOF
	}

	// Fallback: check error message for common patterns (for wrapped errors and edge cases)
	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "timeout") {
		return ErrorTypeTimeout
	}
	if strings.Contains(errStr, "connection refused") {
		return ErrorTypeConnectionRefused
	}
	if strings.Contains(errStr, "connection reset") {
		return ErrorTypeConnectionReset
	}
	if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "dns") {
		return ErrorTypeDNSError
	}
	if strings.Contains(errStr, "tls") || strings.Contains(errStr, "certificate") ||
		strings.Contains(errStr, "x509") {
		return ErrorTypeTLSError
	}
	if strings.Contains(errStr, "eof") {
		return ErrorTypeEOF
	}

	return ErrorTypeUnknown
}

// errorTypeFromStatusCode returns error.type for HTTP status codes.
// Per OTel semconv, the status code itself is used as the error type for 4xx/5xx.
func errorTypeFromStatusCode(statusCode int) string {
	if statusCode >= 400 {
		return strconv.Itoa(statusCode)
	}
	return ""
}

// setSpanError records an error on the span with proper status and attributes.
func setSpanError(span trace.Span, err error, errorType string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	if errorType != "" {
		span.SetAttributes(attribute.String("error.type", errorType))
	}
}
