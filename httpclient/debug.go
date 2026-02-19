package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// debugLogger is the package-level zerolog logger for debug output.
var debugLogger = zerolog.New(os.Stdout).With().Timestamp().Logger()

// generateCurlCommand creates a cURL command equivalent for the given request.
//
// The generated command can be used to reproduce the request from the command line.
// Sensitive headers like Authorization are included for debugging purposes.
//
// Example output:
//
//	curl -X POST 'https://api.example.com/users' \
//	  -H 'Content-Type: application/json' \
//	  -H 'Authorization: Bearer ***' \
//	  -d '{"name":"John"}'
func generateCurlCommand(req *http.Request, body []byte) string {
	var parts []string

	parts = append(parts, "curl")

	// Method
	if req.Method != http.MethodGet {
		parts = append(parts, "-X", req.Method)
	}

	// URL
	parts = append(parts, fmt.Sprintf("'%s'", req.URL.String()))

	// Headers (sorted for consistent output)
	headerKeys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	for _, k := range headerKeys {
		for _, v := range req.Header[k] {
			parts = append(parts, "-H", fmt.Sprintf("'%s: %s'", k, v))
		}
	}

	// Body
	if len(body) > 0 {
		// Escape single quotes in body
		bodyStr := strings.ReplaceAll(string(body), "'", "'\\''")
		parts = append(parts, "-d", fmt.Sprintf("'%s'", bodyStr))
	}

	return strings.Join(parts, " ")
}

// requestTracer captures timing information for an HTTP request.
//
// Use with EnableTrace() on RequestBuilder to collect timing data:
//
//	resp, err := client.Request("GetUser").
//	    EnableTrace().
//	    Get(ctx, "/users/1")
//	fmt.Println(resp.TraceInfo())
type requestTracer struct {
	mu         sync.Mutex
	dnsStart   time.Time
	dnsEnd     time.Time
	connStart  time.Time
	connEnd    time.Time
	tlsStart   time.Time
	tlsEnd     time.Time
	reqStart   time.Time
	firstByte  time.Time
	totalStart time.Time
}

// clientTrace creates an httptrace.ClientTrace for capturing timing info.
func (t *requestTracer) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			t.mu.Lock()
			t.dnsStart = time.Now()
			t.mu.Unlock()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			t.mu.Lock()
			t.dnsEnd = time.Now()
			t.mu.Unlock()
		},
		ConnectStart: func(_, _ string) {
			t.mu.Lock()
			t.connStart = time.Now()
			t.mu.Unlock()
		},
		ConnectDone: func(_, _ string, _ error) {
			t.mu.Lock()
			t.connEnd = time.Now()
			t.mu.Unlock()
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			t.tlsStart = time.Now()
			t.mu.Unlock()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			t.mu.Lock()
			t.tlsEnd = time.Now()
			t.mu.Unlock()
		},
		WroteRequest: func(_ httptrace.WroteRequestInfo) {
			t.mu.Lock()
			t.reqStart = time.Now()
			t.mu.Unlock()
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			t.firstByte = time.Now()
			t.mu.Unlock()
		},
	}
}

// toTraceInfo converts the captured timing data to a TraceInfo struct.
func (t *requestTracer) toTraceInfo() *TraceInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	info := &TraceInfo{}

	if !t.dnsStart.IsZero() && !t.dnsEnd.IsZero() {
		info.DNSLookup = t.dnsEnd.Sub(t.dnsStart).String()
	} else {
		info.DNSLookup = "0s"
	}

	if !t.connStart.IsZero() && !t.connEnd.IsZero() {
		info.ConnTime = t.connEnd.Sub(t.connStart).String()
	} else {
		info.ConnTime = "0s"
	}

	if !t.tlsStart.IsZero() && !t.tlsEnd.IsZero() {
		info.TLSHandshake = t.tlsEnd.Sub(t.tlsStart).String()
	} else {
		info.TLSHandshake = ""
	}

	if !t.reqStart.IsZero() && !t.firstByte.IsZero() {
		info.ServerTime = t.firstByte.Sub(t.reqStart).String()
	} else {
		info.ServerTime = "0s"
	}

	if !t.totalStart.IsZero() {
		info.TotalTime = time.Since(t.totalStart).String()
	} else {
		info.TotalTime = "0s"
	}

	return info
}

// logRequest logs the request details using zerolog.
func logRequest(logger zerolog.Logger, req *http.Request) {
	logger.Debug().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Str("host", req.Host).
		Msg("HTTP request")
}

// logResponse logs the response details using zerolog.
func logResponse(logger zerolog.Logger, resp *http.Response, duration time.Duration) {
	logger.Debug().
		Int("status", resp.StatusCode).
		Str("status_text", resp.Status).
		Dur("duration_ms", duration).
		Int64("content_length", resp.ContentLength).
		Msg("HTTP response")
}
