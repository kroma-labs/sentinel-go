package httpclient

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"

	json "github.com/goccy/go-json"
)

// RequestBuilder provides a fluent API for constructing HTTP requests.
//
// Create a RequestBuilder using Client.Request():
//
//	resp, err := client.Request("CreateUser").
//	    Path("/users").
//	    Body(user).
//	    Post(ctx)
type RequestBuilder struct {
	client        *Client
	operationName string
	path          string
	pathParams    map[string]string
	queryParams   url.Values
	headers       http.Header
	body          io.Reader
	contentType   string
	result        any
	errorResult   any
	enableTrace   bool

	// Multipart upload fields
	fileUploads []FileUpload
	formFields  map[string]string
}

// Path sets the request path.
//
// The path is appended to the client's base URL. Path parameters
// can be specified using {name} syntax and filled with PathParam().
//
// Example:
//
//	client.Request("GetUser").
//	    Path("/users/{id}").
//	    PathParam("id", userID).
//	    Get(ctx)
func (rb *RequestBuilder) Path(path string) *RequestBuilder {
	rb.path = path
	return rb
}

// PathParam sets a path parameter value.
//
// Path parameters are replaced in the path string using {name} syntax.
//
// Example:
//
//	client.Request("GetUser").
//	    Path("/users/{id}/posts/{postId}").
//	    PathParam("id", userID).
//	    PathParam("postId", postID).
//	    Get(ctx)
func (rb *RequestBuilder) PathParam(key, value string) *RequestBuilder {
	rb.pathParams[key] = value
	return rb
}

// Query adds a single query parameter.
//
// Example:
//
//	client.Request("SearchUsers").
//	    Path("/users").
//	    Query("search", "john").
//	    Query("limit", "10").
//	    Get(ctx)
func (rb *RequestBuilder) Query(key, value string) *RequestBuilder {
	if rb.queryParams == nil {
		rb.queryParams = make(url.Values)
	}
	rb.queryParams.Set(key, value)
	return rb
}

// Queries adds multiple query parameters.
//
// Example:
//
//	client.Request("SearchUsers").
//	    Path("/users").
//	    Queries(map[string]string{"search": "john", "limit": "10"}).
//	    Get(ctx)
func (rb *RequestBuilder) Queries(params map[string]string) *RequestBuilder {
	if rb.queryParams == nil {
		rb.queryParams = make(url.Values)
	}
	for k, v := range params {
		rb.queryParams.Set(k, v)
	}
	return rb
}

// Header sets a single request header.
//
// Example:
//
//	client.Request("CreateUser").
//	    Header("Authorization", "Bearer "+token).
//	    Header("Idempotency-Key", key).
//	    Post(ctx, "/users")
func (rb *RequestBuilder) Header(key, value string) *RequestBuilder {
	rb.headers.Set(key, value)
	return rb
}

// Headers sets multiple request headers.
//
// Example:
//
//	client.Request("CreateUser").
//	    Headers(map[string]string{
//	        "Authorization": "Bearer "+token,
//	        "Idempotency-Key": key,
//	    }).
//	    Post(ctx, "/users")
func (rb *RequestBuilder) Headers(headers map[string]string) *RequestBuilder {
	for k, v := range headers {
		rb.headers.Set(k, v)
	}
	return rb
}

// Body sets the request body with automatic content type detection.
//
// Encoding rules:
//   - struct/map: JSON (Content-Type: application/json)
//   - string: raw text (Content-Type: text/plain)
//   - []byte: raw bytes (Content-Type: application/octet-stream)
//   - io.Reader: passthrough
//   - url.Values: form encoded (Content-Type: application/x-www-form-urlencoded)
//
// Example:
//
//	client.Request("CreateUser").
//	    Body(user).  // struct -> JSON
//	    Post(ctx, "/users")
func (rb *RequestBuilder) Body(v any) *RequestBuilder {
	if v == nil {
		return rb
	}

	switch body := v.(type) {
	case string:
		rb.body = strings.NewReader(body)
		rb.contentType = "text/plain; charset=utf-8"
	case []byte:
		rb.body = bytes.NewReader(body)
		rb.contentType = "application/octet-stream"
	case io.Reader:
		rb.body = body
	case url.Values:
		rb.body = strings.NewReader(body.Encode())
		rb.contentType = "application/x-www-form-urlencoded"
	default:
		// struct/map -> JSON
		data, err := json.Marshal(v)
		if err != nil {
			// Store error for later - will be returned on execute
			rb.body = &bodyEncodingError{err: err}
			return rb
		}
		rb.body = bytes.NewReader(data)
		rb.contentType = "application/json"
	}
	return rb
}

// BodyJSON explicitly encodes the body as JSON.
//
// Use this when you want to ensure JSON encoding regardless of the input type.
//
// Example:
//
//	client.Request("CreateUser").
//	    BodyJSON(user).
//	    Post(ctx, "/users")
func (rb *RequestBuilder) BodyJSON(v any) *RequestBuilder {
	if v == nil {
		return rb
	}
	data, err := json.Marshal(v)
	if err != nil {
		rb.body = &bodyEncodingError{err: err}
		return rb
	}
	rb.body = bytes.NewReader(data)
	rb.contentType = "application/json"
	return rb
}

// BodyXML explicitly encodes the body as XML.
//
// Example:
//
//	client.Request("CreateOrder").
//	    BodyXML(order).
//	    Post(ctx, "/orders")
func (rb *RequestBuilder) BodyXML(v any) *RequestBuilder {
	if v == nil {
		return rb
	}
	data, err := xml.Marshal(v)
	if err != nil {
		rb.body = &bodyEncodingError{err: err}
		return rb
	}
	rb.body = bytes.NewReader(data)
	rb.contentType = "application/xml"
	return rb
}

// BodyForm sets form data as the request body.
//
// Example:
//
//	client.Request("Login").
//	    BodyForm(map[string]string{
//	        "username": "john",
//	        "password": "secret",
//	    }).
//	    Post(ctx, "/login")
func (rb *RequestBuilder) BodyForm(data map[string]string) *RequestBuilder {
	values := make(url.Values)
	for k, v := range data {
		values.Set(k, v)
	}
	rb.body = strings.NewReader(values.Encode())
	rb.contentType = "application/x-www-form-urlencoded"
	return rb
}

// Decode sets the target for automatic response body decoding.
//
// If the response is successful (2xx), the body is decoded into the target.
//
// Example:
//
//	var users []User
//	resp, err := client.Request("GetUsers").
//	    Decode(&users).
//	    Get(ctx, "/users")
func (rb *RequestBuilder) Decode(v any) *RequestBuilder {
	rb.result = v
	return rb
}

// DecodeError sets the target for automatic error response decoding.
//
// If the response is not successful (non-2xx), the body is decoded into the target.
//
// Example:
//
//	var apiErr APIError
//	resp, err := client.Request("CreateUser").
//	    Decode(&user).
//	    DecodeError(&apiErr).
//	    Post(ctx, "/users")
func (rb *RequestBuilder) DecodeError(v any) *RequestBuilder {
	rb.errorResult = v
	return rb
}

// DecodeAny sets the target for automatic response decoding regardless of status code.
//
// Use this when your API returns the same response structure for both success
// and error responses. The body is always decoded into the target.
//
// Example - Unified response structure:
//
//	// API returns same structure for all responses:
//	// { "data": {...}, "errors": [...] }
//	type APIResponse struct {
//	    Data   *User   `json:"data"`
//	    Errors []Error `json:"errors"`
//	}
//
//	var result APIResponse
//	resp, err := client.Request("CreateUser").
//	    DecodeAny(&result).
//	    Post(ctx, "/users")
//
//	if resp.IsError() {
//	    // Handle result.Errors
//	}
func (rb *RequestBuilder) DecodeAny(v any) *RequestBuilder {
	rb.result = v
	rb.errorResult = v
	return rb
}

// EnableTrace enables timing trace collection for this request.
//
// Example:
//
//	resp, err := client.Request("SlowAPI").
//	    EnableTrace().
//	    Get(ctx, "/slow")
//	fmt.Println(resp.TraceInfo())
func (rb *RequestBuilder) EnableTrace() *RequestBuilder {
	rb.enableTrace = true
	return rb
}

// Get executes a GET request.
//
// Example:
//
//	resp, err := client.Request("GetUsers").Get(ctx, "/users")
func (rb *RequestBuilder) Get(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodGet)
}

// Post executes a POST request.
//
// Example:
//
//	resp, err := client.Request("CreateUser").
//	    Body(user).
//	    Post(ctx, "/users")
func (rb *RequestBuilder) Post(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodPost)
}

// Put executes a PUT request.
//
// Example:
//
//	resp, err := client.Request("UpdateUser").
//	    Body(user).
//	    Put(ctx, "/users/{id}")
func (rb *RequestBuilder) Put(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodPut)
}

// Patch executes a PATCH request.
//
// Example:
//
//	resp, err := client.Request("PatchUser").
//	    Body(patch).
//	    Patch(ctx, "/users/{id}")
func (rb *RequestBuilder) Patch(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodPatch)
}

// Delete executes a DELETE request.
//
// Example:
//
//	resp, err := client.Request("DeleteUser").Delete(ctx, "/users/{id}")
func (rb *RequestBuilder) Delete(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodDelete)
}

// execute builds and sends the HTTP request.
func (rb *RequestBuilder) execute(ctx context.Context, method string) (*Response, error) {
	// Build URL
	targetURL, err := rb.buildURL()
	if err != nil {
		return nil, err
	}

	// Check for body encoding errors
	if er, ok := rb.body.(*bodyEncodingError); ok {
		return nil, er.err
	}

	// Handle multipart file uploads
	reqBody := rb.body
	if len(rb.fileUploads) > 0 {
		body, contentType, err := rb.buildMultipart()
		if err != nil {
			return nil, err
		}
		reqBody = body
		rb.contentType = contentType
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, targetURL, reqBody)
	if err != nil {
		return nil, err
	}

	// Apply default headers from client
	for k, v := range rb.client.defaultHeaders {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}

	// Apply request-specific headers (override defaults)
	for k, v := range rb.headers {
		req.Header[k] = v
	}

	// Set content type if body was set
	if rb.contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", rb.contentType)
	}

	// Set up request tracing if enabled
	var tracer *requestTracer
	if rb.enableTrace || rb.client.enableTrace {
		tracer = &requestTracer{totalStart: time.Now()}
		ctx = httptrace.WithClientTrace(ctx, tracer.clientTrace())
		req = req.WithContext(ctx)
	}

	// Debug logging
	if rb.client.debug {
		logRequest(debugLogger, req)
	}

	startTime := time.Now()

	// Execute request
	// The caller is responsible for closing the response body.
	// Response.Body() handles this automatically when called.
	//nolint:bodyclose // Caller closes via Response
	httpResp, err := rb.client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)

	// Debug logging for response
	if rb.client.debug {
		logResponse(debugLogger, httpResp, duration)
	}

	// Wrap response
	resp := &Response{
		Response:    httpResp,
		request:     req,
		result:      rb.result,
		errorResult: rb.errorResult,
	}

	// Generate cURL command if enabled
	if rb.client.generateCurl {
		var bodyBytes []byte
		if reqBody != nil {
			if buf, ok := reqBody.(*bytes.Buffer); ok {
				bodyBytes = buf.Bytes()
			}
		}
		resp.curlCommand = generateCurlCommand(req, bodyBytes)
	}

	// Capture trace info if enabled
	if tracer != nil {
		resp.traceInfo = tracer.toTraceInfo()
	}

	// Read and decode body if targets are set
	if rb.result != nil || rb.errorResult != nil {
		if err := resp.decode(); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

// buildURL constructs the full URL from base URL, path, and query params.
func (rb *RequestBuilder) buildURL() (string, error) {
	// Start with path
	path := rb.path

	// Replace path parameters
	for k, v := range rb.pathParams {
		path = strings.ReplaceAll(path, "{"+k+"}", url.PathEscape(v))
	}

	// Build full URL
	var fullURL string
	if rb.client.baseURL != "" {
		fullURL = strings.TrimSuffix(rb.client.baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
	} else {
		fullURL = path
	}

	// Parse and add query params
	if len(rb.queryParams) > 0 {
		u, err := url.Parse(fullURL)
		if err != nil {
			return "", err
		}
		q := u.Query()
		for k, v := range rb.queryParams {
			for _, vv := range v {
				q.Add(k, vv)
			}
		}
		u.RawQuery = q.Encode()
		fullURL = u.String()
	}

	return fullURL, nil
}

// bodyEncodingError is an io.Reader that returns an error.
type bodyEncodingError struct {
	err error
}

func (e *bodyEncodingError) Read(_ []byte) (int, error) {
	return 0, e.err
}
