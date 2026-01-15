package httpclient

import (
	"net/http"
)

// RequestInterceptor allows modification of requests before they are sent.
// Interceptors are executed in the order they are added.
//
// Common use cases:
//   - Adding authentication headers (Bearer tokens, API keys)
//   - Injecting correlation IDs
//   - Request logging
//   - Adding custom headers based on request context
type RequestInterceptor func(req *http.Request) error

// ResponseInterceptor allows inspection/modification of responses after receipt.
// Interceptors are executed in the order they are added.
//
// Common use cases:
//   - Response logging
//   - Metrics collection
//   - Token refresh on 401
//   - Custom error handling
type ResponseInterceptor func(resp *http.Response, req *http.Request) error

// InterceptorChain manages request and response interceptors.
type InterceptorChain struct {
	requestInterceptors  []RequestInterceptor
	responseInterceptors []ResponseInterceptor
}

// NewInterceptorChain creates an empty interceptor chain.
func NewInterceptorChain() *InterceptorChain {
	return &InterceptorChain{}
}

// AddRequestInterceptor adds a request interceptor to the chain.
func (c *InterceptorChain) AddRequestInterceptor(i RequestInterceptor) {
	c.requestInterceptors = append(c.requestInterceptors, i)
}

// AddResponseInterceptor adds a response interceptor to the chain.
func (c *InterceptorChain) AddResponseInterceptor(i ResponseInterceptor) {
	c.responseInterceptors = append(c.responseInterceptors, i)
}

// ApplyRequestInterceptors runs all request interceptors in order.
// Returns an error if any interceptor fails.
func (c *InterceptorChain) ApplyRequestInterceptors(req *http.Request) error {
	for _, interceptor := range c.requestInterceptors {
		if err := interceptor(req); err != nil {
			return err
		}
	}
	return nil
}

// ApplyResponseInterceptors runs all response interceptors in order.
// Returns an error if any interceptor fails.
func (c *InterceptorChain) ApplyResponseInterceptors(resp *http.Response, req *http.Request) error {
	for _, interceptor := range c.responseInterceptors {
		if err := interceptor(resp, req); err != nil {
			return err
		}
	}
	return nil
}

// Common interceptor helpers

// AuthBearerInterceptor creates an interceptor that adds a Bearer token.
func AuthBearerInterceptor(token string) RequestInterceptor {
	return func(req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// AuthBearerFuncInterceptor creates an interceptor that adds a Bearer token
// from a function (useful for dynamic/refreshable tokens).
func AuthBearerFuncInterceptor(tokenFunc func() (string, error)) RequestInterceptor {
	return func(req *http.Request) error {
		token, err := tokenFunc()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// APIKeyInterceptor creates an interceptor that adds an API key header.
func APIKeyInterceptor(headerName, apiKey string) RequestInterceptor {
	return func(req *http.Request) error {
		req.Header.Set(headerName, apiKey)
		return nil
	}
}

// CorrelationIDInterceptor creates an interceptor that adds a correlation ID.
func CorrelationIDInterceptor(headerName string, idFunc func() string) RequestInterceptor {
	return func(req *http.Request) error {
		req.Header.Set(headerName, idFunc())
		return nil
	}
}

// UserAgentInterceptor creates an interceptor that sets the User-Agent header.
func UserAgentInterceptor(userAgent string) RequestInterceptor {
	return func(req *http.Request) error {
		req.Header.Set("User-Agent", userAgent)
		return nil
	}
}
