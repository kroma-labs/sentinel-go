package httpclient

import "net/http"

// RoundTripper represents an HTTP round tripper for testing.
type RoundTripper interface {
	RoundTrip(*http.Request) (*http.Response, error)
}
