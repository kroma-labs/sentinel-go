package httpclient

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"
)

// MockTransport provides a configurable http.RoundTripper for testing.
// It allows stubbing responses and verifying request expectations.
type MockTransport struct {
	mu          sync.RWMutex
	stubs       []stub
	defaultResp *http.Response
	defaultErr  error
	requests    []*http.Request
	requestHook func(*http.Request)
}

type stub struct {
	matcher  func(*http.Request) bool
	response *http.Response
	err      error
}

// NewMockTransport creates a new MockTransport for testing.
func NewMockTransport() *MockTransport {
	return &MockTransport{}
}

// StubResponse stubs all requests to return the given response.
func (m *MockTransport) StubResponse(statusCode int, body string) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultResp = &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
	return m
}

// StubError stubs all requests to return the given error.
func (m *MockTransport) StubError(err error) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultErr = err
	return m
}

// StubPath stubs requests matching the path to return the given response.
func (m *MockTransport) StubPath(path string, statusCode int, body string) *MockTransport {
	return m.StubFunc(func(req *http.Request) bool {
		return req.URL.Path == path
	}, statusCode, body)
}

// StubPathRegex stubs requests matching the path regex to return the given response.
func (m *MockTransport) StubPathRegex(pattern string, statusCode int, body string) *MockTransport {
	re := regexp.MustCompile(pattern)
	return m.StubFunc(func(req *http.Request) bool {
		return re.MatchString(req.URL.Path)
	}, statusCode, body)
}

// StubMethod stubs requests with the given method to return the given response.
func (m *MockTransport) StubMethod(method string, statusCode int, body string) *MockTransport {
	return m.StubFunc(func(req *http.Request) bool {
		return req.Method == method
	}, statusCode, body)
}

// StubFunc stubs requests matching the predicate to return the given response.
func (m *MockTransport) StubFunc(
	matcher func(*http.Request) bool,
	statusCode int,
	body string,
) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stubs = append(m.stubs, stub{
		matcher: matcher,
		response: &http.Response{
			StatusCode: statusCode,
			Status:     http.StatusText(statusCode),
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		},
	})
	return m
}

// StubFuncError stubs requests matching the predicate to return the given error.
func (m *MockTransport) StubFuncError(matcher func(*http.Request) bool, err error) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stubs = append(m.stubs, stub{
		matcher: matcher,
		err:     err,
	})
	return m
}

// OnRequest sets a hook that is called for each request.
// Useful for assertions or capturing request details.
func (m *MockTransport) OnRequest(fn func(*http.Request)) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestHook = fn
	return m
}

// RoundTrip implements http.RoundTripper.
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	hook := m.requestHook
	m.mu.Unlock()

	if hook != nil {
		hook(req)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check stubs in order (first match wins)
	for _, s := range m.stubs {
		if s.matcher(req) {
			if s.err != nil {
				return nil, s.err
			}
			// Clone response so body can be read multiple times
			return cloneResponse(s.response), nil
		}
	}

	// Return default response or error
	if m.defaultErr != nil {
		return nil, m.defaultErr
	}
	if m.defaultResp != nil {
		return cloneResponse(m.defaultResp), nil
	}

	return nil, errors.New("no stub found for request: " + req.Method + " " + req.URL.String())
}

// Requests returns all requests made through this transport.
func (m *MockTransport) Requests() []*http.Request {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]*http.Request{}, m.requests...)
}

// RequestCount returns the number of requests made.
func (m *MockTransport) RequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.requests)
}

// LastRequest returns the most recent request, or nil if none.
func (m *MockTransport) LastRequest() *http.Request {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.requests) == 0 {
		return nil
	}
	return m.requests[len(m.requests)-1]
}

// Reset clears all recorded requests and stubs.
func (m *MockTransport) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
	m.stubs = nil
	m.defaultResp = nil
	m.defaultErr = nil
	m.requestHook = nil
}

func cloneResponse(resp *http.Response) *http.Response {
	if resp == nil {
		return nil
	}

	// Read body
	var bodyBytes []byte
	if resp.Body != nil {
		bodyBytes, _ = io.ReadAll(resp.Body)
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	return &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Header:        resp.Header.Clone(),
		Body:          io.NopCloser(bytes.NewBuffer(bodyBytes)),
		ContentLength: resp.ContentLength,
		Request:       resp.Request,
	}
}

// WithMockTransport is a convenience function to create a client with a mock transport.
func WithMockTransport(mock *MockTransport) Option {
	return func(cfg *internalConfig) {
		cfg.MockTransport = mock
	}
}
