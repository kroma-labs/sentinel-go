package httpclient

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	json "github.com/goccy/go-json"
)

// Response wraps http.Response with convenience methods for body handling,
// automatic decoding, and request debugging.
//
// Response provides:
//   - Cached body reading (body is read once and reused)
//   - Automatic JSON/XML decoding based on Content-Type
//   - Success/error status helpers
//   - cURL command generation for debugging
//   - Request timing trace information
//
// Example usage:
//
//	var users []User
//	resp, err := client.Request("GetUsers").
//	    Decode(&users).
//	    Get(ctx, "/users")
//
//	if err != nil {
//	    return err
//	}
//
//	if resp.IsSuccess() {
//	    fmt.Printf("Got %d users\n", len(users))
//	} else {
//	    body, _ := resp.String()
//	    fmt.Printf("Error: %s\n", body)
//	}
type Response struct {
	// Response embeds the standard http.Response.
	// All http.Response fields and methods are accessible directly.
	//
	// Example: resp.StatusCode, resp.Header.Get("Content-Type")
	*http.Response

	// request is the original HTTP request that produced this response.
	// Used internally for cURL generation and debugging.
	request *http.Request

	// body is the cached response body.
	// Populated on first call to Body() or String().
	// Subsequent calls return this cached value.
	body []byte

	// bodyRead tracks whether the body has been read and cached.
	// Prevents multiple reads of the response body stream.
	bodyRead bool

	// result holds the decoded success response.
	// Populated when Decode() is used and response is 2xx.
	result any

	// errorResult holds the decoded error response.
	// Populated when DecodeError() is used and response is non-2xx.
	errorResult any

	// curlCommand is the equivalent cURL command for this request.
	// Only populated if WithGenerateCurl(true) was set on the client.
	curlCommand string

	// traceInfo contains request timing information.
	// Only populated if EnableTrace() was called on the request.
	traceInfo *TraceInfo
}

// Body returns the response body as bytes.
//
// The body is read and cached on first access. Subsequent calls
// return the cached value.
func (r *Response) Body() ([]byte, error) {
	if r.bodyRead {
		return r.body, nil
	}

	defer r.Response.Body.Close()
	body, err := io.ReadAll(r.Response.Body)
	if err != nil {
		return nil, err
	}

	r.body = body
	r.bodyRead = true
	return r.body, nil
}

// String returns the response body as a string.
func (r *Response) String() (string, error) {
	body, err := r.Body()
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// Result returns the decoded success response.
//
// This is only populated if Decode() was called on the RequestBuilder
// and the response was successful (2xx).
func (r *Response) Result() any {
	return r.result
}

// Error returns the decoded error response.
//
// This is only populated if DecodeError() was called on the RequestBuilder
// and the response was not successful (non-2xx).
func (r *Response) Error() any {
	return r.errorResult
}

// IsSuccess returns true if the response status code is 2xx.
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError returns true if the response status code is 4xx or 5xx.
func (r *Response) IsError() bool {
	return r.StatusCode >= 400
}

// CurlCommand returns the cURL command equivalent for this request.
//
// This is only populated if WithGenerateCurl(true) was set on the client.
func (r *Response) CurlCommand() string {
	return r.curlCommand
}

// TraceInfo returns timing information for this request.
//
// This is only populated if EnableTrace() was called on the RequestBuilder.
func (r *Response) TraceInfo() *TraceInfo {
	return r.traceInfo
}

// decode reads the body and decodes it into the result or errorResult.
func (r *Response) decode() error {
	body, err := r.Body()
	if err != nil {
		return err
	}

	if len(body) == 0 {
		return nil
	}

	// Determine content type
	contentType := r.Header.Get("Content-Type")

	if r.IsSuccess() && r.result != nil {
		return decodeBody(body, contentType, r.result)
	}

	if r.IsError() && r.errorResult != nil {
		return decodeBody(body, contentType, r.errorResult)
	}

	return nil
}

// decodeBody decodes the body based on content type.
func decodeBody(body []byte, contentType string, target any) error {
	if strings.Contains(contentType, "application/json") {
		return json.Unmarshal(body, target)
	}
	isXML := strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml")
	if isXML {
		return xml.Unmarshal(body, target)
	}
	// Default to JSON
	return json.Unmarshal(body, target)
}

// TraceInfo contains timing information for an HTTP request.
//
// Each field represents a phase of the request lifecycle, measured as a
// human-readable duration string (e.g., "45.2ms").
//
// Example usage:
//
//	resp, err := client.Request("GetUser").
//	    EnableTrace().
//	    Get(ctx, "/users/1")
//
//	if err == nil {
//	    fmt.Println(resp.TraceInfo())
//	    // Output:
//	    // DNS Lookup:    2.1ms
//	    // TCP Connect:   15.3ms
//	    // TLS Handshake: 28.7ms
//	    // Server Time:   45.2ms
//	    // Total Time:    91.3ms
//	}
type TraceInfo struct {
	// DNSLookup is the duration of DNS name resolution.
	// This is the time from when the DNS query is sent until the IP address
	// is received. For cached DNS or IP-based URLs, this may be "0s".
	//
	// Example: "2.1ms"
	DNSLookup string

	// ConnTime is the duration to establish a TCP connection.
	// This measures the time from initiating the connection to completing
	// the TCP handshake (SYN -> SYN-ACK -> ACK).
	//
	// Example: "15.3ms"
	ConnTime string

	// TLSHandshake is the duration of the TLS handshake.
	// This includes certificate verification, cipher negotiation, and
	// session establishment. Only populated for HTTPS requests.
	//
	// Example: "28.7ms" for HTTPS, empty for HTTP
	TLSHandshake string

	// ServerTime is the duration from sending the request to receiving
	// the first byte of the response (Time to First Byte / TTFB).
	// This indicates server processing time plus network latency.
	//
	// Example: "45.2ms"
	ServerTime string

	// TotalTime is the total duration of the entire request lifecycle.
	// This includes DNS, connection, TLS, request send, server processing,
	// and response body transfer.
	//
	// Example: "91.3ms"
	TotalTime string
}

// String returns a formatted string representation of the trace info.
//
// Example output:
//
//	DNS Lookup:    2.1ms
//	TCP Connect:   15.3ms
//	TLS Handshake: 28.7ms
//	Server Time:   45.2ms
//	Total Time:    91.3ms
func (t *TraceInfo) String() string {
	if t == nil {
		return "TraceInfo: nil (EnableTrace() was not called)"
	}

	return fmt.Sprintf(
		"DNS Lookup:    %s\nTCP Connect:   %s\nTLS Handshake: %s\nServer Time:   %s\nTotal Time:    %s",
		t.DNSLookup,
		t.ConnTime,
		t.TLSHandshake,
		t.ServerTime,
		t.TotalTime,
	)
}
