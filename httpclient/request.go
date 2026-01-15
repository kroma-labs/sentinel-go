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
	client              *Client
	operationName       string
	path                string
	pathParams          map[string]string
	queryParams         url.Values
	headers             http.Header
	body                io.Reader
	contentType         string
	result              any
	errorResult         any
	enableTrace         bool
	hedgeConfig         *HedgeConfig
	adaptiveHedgeConfig *AdaptiveHedgeConfig

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
// The content type is automatically determined based on the input type:
//   - struct/map: Encoded as JSON (Content-Type: application/json)
//   - string: Sent as plain text (Content-Type: text/plain; charset=utf-8)
//   - []byte: Sent as binary data (Content-Type: application/octet-stream)
//   - io.Reader: Passed through directly (no Content-Type set)
//   - url.Values: Encoded as form data (Content-Type: application/x-www-form-urlencoded)
//
// For explicit encoding control, use the dedicated methods:
//   - BodyJSON() - Force JSON encoding
//   - BodyXML() - Force XML encoding
//   - BodyForm() - Force form encoding
//
// Example with struct (auto-detected as JSON):
//
//	type User struct {
//	    Name  string `json:"name"`
//	    Email string `json:"email"`
//	}
//
//	var user User
//	resp, err := client.Request("CreateUser").
//	    Body(user).
//	    Post(ctx, "/users")
//
// Example with string:
//
//	resp, err := client.Request("SendMessage").
//	    Body("Hello, World!").
//	    Post(ctx, "/messages")
//
// Example with url.Values (form encoded):
//
//	form := url.Values{}
//	form.Set("username", "john")
//	form.Set("password", "secret")
//
//	resp, err := client.Request("Login").
//	    Body(form).
//	    Post(ctx, "/login")
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
		data, err := json.Marshal(v)
		if err != nil {
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
// Use this method when you want to ensure JSON encoding regardless of the input type,
// or when you want to be explicit about the encoding for code clarity.
//
// The Content-Type header is automatically set to "application/json".
//
// Example:
//
//	type CreateUserRequest struct {
//	    Name  string `json:"name"`
//	    Email string `json:"email"`
//	    Age   int    `json:"age"`
//	}
//
//	req := CreateUserRequest{
//	    Name:  "John Doe",
//	    Email: "john@example.com",
//	    Age:   30,
//	}
//
//	resp, err := client.Request("CreateUser").
//	    BodyJSON(req).
//	    Post(ctx, "/api/users")
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
// Use this method when interfacing with APIs that require XML payloads,
// such as SOAP services or legacy enterprise systems.
//
// The Content-Type header is automatically set to "application/xml".
// Make sure your struct fields have appropriate `xml` tags for proper encoding.
//
// Example:
//
//	type Order struct {
//	    XMLName xml.Name `xml:"order"`
//	    ID      string   `xml:"id"`
//	    Amount  float64  `xml:"amount"`
//	    Items   []Item   `xml:"items>item"`
//	}
//
//	order := Order{
//	    ID:     "ORD-123",
//	    Amount: 99.99,
//	    Items:  []Item{{Name: "Widget", Qty: 2}},
//	}
//
//	resp, err := client.Request("CreateOrder").
//	    BodyXML(order).
//	    Post(ctx, "/api/orders")
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
// This method encodes the provided key-value pairs as URL-encoded form data,
// commonly used for HTML form submissions and OAuth token requests.
//
// The Content-Type header is automatically set to "application/x-www-form-urlencoded".
//
// Example - Login form:
//
//	resp, err := client.Request("Login").
//	    BodyForm(map[string]string{
//	        "username": "john@example.com",
//	        "password": "secret123",
//	    }).
//	    Post(ctx, "/auth/login")
//
// Example - OAuth token request:
//
//	resp, err := client.Request("GetToken").
//	    BodyForm(map[string]string{
//	        "grant_type":    "client_credentials",
//	        "client_id":     os.Getenv("CLIENT_ID"),
//	        "client_secret": os.Getenv("CLIENT_SECRET"),
//	    }).
//	    Post(ctx, "/oauth/token")
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
// When a successful response is received (HTTP 2xx status codes),
// the response body is automatically decoded into the provided target.
// The decoding format is determined by the Content-Type header (JSON by default).
//
// If an error response is received (non-2xx), the body is not decoded into this target.
// Use DecodeError() to handle error responses, or DecodeAny() for unified response structures.
//
// Example - Fetching a list of users:
//
//	type User struct {
//	    ID    int    `json:"id"`
//	    Name  string `json:"name"`
//	    Email string `json:"email"`
//	}
//
//	var users []User
//	resp, err := client.Request("GetUsers").
//	    Decode(&users).
//	    Get(ctx, "/api/users")
//	if err != nil {
//	    return err
//	}
//	// users slice is now populated
func (rb *RequestBuilder) Decode(v any) *RequestBuilder {
	rb.result = v
	return rb
}

// DecodeError sets the target for automatic error response decoding.
//
// When an error response is received (non-2xx status codes), the response body
// is automatically decoded into the provided target. This is useful when APIs
// return structured error information.
//
// This method is typically used together with Decode() to handle both success
// and error responses with different structures.
//
// Example - Handling both success and error responses:
//
//	type User struct {
//	    ID   int    `json:"id"`
//	    Name string `json:"name"`
//	}
//
//	type APIError struct {
//	    Code    string `json:"code"`
//	    Message string `json:"message"`
//	    Details []struct {
//	        Field string `json:"field"`
//	        Error string `json:"error"`
//	    } `json:"details,omitempty"`
//	}
//
//	var user User
//	var apiErr APIError
//
//	resp, err := client.Request("GetUser").
//	    Decode(&user).
//	    DecodeError(&apiErr).
//	    Get(ctx, "/api/users/123")
//	if err != nil {
//	    return err
//	}
//	if resp.IsError() {
//	    log.Printf("API error: %s - %s", apiErr.Code, apiErr.Message)
//	}
func (rb *RequestBuilder) DecodeError(v any) *RequestBuilder {
	rb.errorResult = v
	return rb
}

// DecodeAny sets the target for automatic response decoding regardless of status code.
//
// Use this when your API returns the same response structure for both success
// and error responses. The body is always decoded into the target, regardless
// of the HTTP status code.
//
// This is common in APIs that wrap all responses in a consistent envelope structure.
//
// Example - Unified response structure:
//
//	type APIResponse struct {
//	    Success bool            `json:"success"`
//	    Data    json.RawMessage `json:"data,omitempty"`
//	    Error   *struct {
//	        Code    string `json:"code"`
//	        Message string `json:"message"`
//	    } `json:"error,omitempty"`
//	}
//
//	var response APIResponse
//	resp, err := client.Request("GetData").
//	    DecodeAny(&response).
//	    Get(ctx, "/api/data")
//	if err != nil {
//	    return err
//	}
//	if !response.Success {
//	    return fmt.Errorf("API error: %s", response.Error.Message)
//	}
func (rb *RequestBuilder) DecodeAny(v any) *RequestBuilder {
	rb.result = v
	rb.errorResult = v
	return rb
}

// EnableTrace enables timing trace collection for this request.
//
// When enabled, detailed timing information is collected during the request,
// including DNS lookup, connection establishment, TLS handshake, and time to
// first byte. This is useful for debugging performance issues.
//
// Access the collected trace data via Response.TraceInfo().
//
// Example:
//
//	resp, err := client.Request("SlowAPI").
//	    EnableTrace().
//	    Get(ctx, "/api/slow-endpoint")
//	if err != nil {
//	    return err
//	}
//
//	trace := resp.TraceInfo()
//	fmt.Printf("DNS: %v, Connect: %v, TLS: %v, TTFB: %v\n",
//	    trace.DNSLookup, trace.ConnTime, trace.TLSTime, trace.TTFB)
func (rb *RequestBuilder) EnableTrace() *RequestBuilder {
	rb.enableTrace = true
	return rb
}

// Hedge enables hedged requests for this specific request.
//
// Hedged requests reduce tail latency by sending a duplicate request if the
// original hasn't completed within the specified delay. First response wins.
//
// IMPORTANT: Only use for idempotent operations (GET, HEAD, or idempotent POST/PUT).
//
// Example:
//
//	resp, err := client.Request("GetUser").
//	    Hedge(50 * time.Millisecond).  // Send hedge after 50ms
//	    Get(ctx, "/users/123")
//
// For more control, use HedgeConfig().
func (rb *RequestBuilder) Hedge(delay time.Duration) *RequestBuilder {
	rb.hedgeConfig = &HedgeConfig{
		Delay:     delay,
		MaxHedges: 1,
	}
	return rb
}

// HedgeConfig enables hedged requests with full configuration.
//
// This allows fine-grained control over hedging behavior.
//
// Example:
//
//	resp, err := client.Request("GetUser").
//	    HedgeConfig(httpclient.HedgeConfig{
//	        Delay:     50 * time.Millisecond,
//	        MaxHedges: 2,
//	    }).
//	    Get(ctx, "/users/123")
func (rb *RequestBuilder) HedgeConfig(cfg HedgeConfig) *RequestBuilder {
	rb.hedgeConfig = &cfg
	return rb
}

// AdaptiveHedge enables adaptive hedged requests that dynamically calculate
// the hedge delay based on historical endpoint latency.
//
// After sufficient samples are collected (MinSamples), the hedge delay is
// automatically set to the TargetPercentile latency. Until then, FallbackDelay
// is used.
//
// Example - Using defaults (P95, 100 samples, 50ms fallback):
//
//	resp, err := client.Request("GetUser").
//	    AdaptiveHedge(httpclient.DefaultAdaptiveHedgeConfig()).
//	    Get(ctx, "/users/123")
//
// Example - Custom config:
//
//	resp, err := client.Request("GetUser").
//	    AdaptiveHedge(httpclient.AdaptiveHedgeConfig{
//	        TargetPercentile: 0.99,
//	        MinSamples:       20,
//	        FallbackDelay:    100 * time.Millisecond,
//	    }).
//	    Get(ctx, "/users/123")
func (rb *RequestBuilder) AdaptiveHedge(cfg AdaptiveHedgeConfig) *RequestBuilder {
	rb.adaptiveHedgeConfig = &cfg
	return rb
}

// Get executes a GET request.
//
// The path parameter is optional if already set via Path(). If provided,
// it overrides any previously set path. The path can include placeholders
// for path parameters set via PathParam().
//
// Example - Simple GET:
//
//	resp, err := client.Request("GetUsers").Get(ctx, "/users")
//
// Example - GET with query parameters:
//
//	resp, err := client.Request("SearchUsers").
//	    Query("name", "john").
//	    Query("limit", "10").
//	    Get(ctx, "/users")
//
// Example - GET with path parameters:
//
//	resp, err := client.Request("GetUser").
//	    PathParam("id", userID).
//	    Get(ctx, "/users/{id}")
func (rb *RequestBuilder) Get(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodGet)
}

// Post executes a POST request.
//
// POST is typically used to create new resources or submit data for processing.
// The request body should be set via Body(), BodyJSON(), BodyXML(), or BodyForm().
//
// Example - Create a resource:
//
//	type CreateUserRequest struct {
//	    Name  string `json:"name"`
//	    Email string `json:"email"`
//	}
//
//	req := CreateUserRequest{Name: "John", Email: "john@example.com"}
//	resp, err := client.Request("CreateUser").
//	    Body(req).
//	    Post(ctx, "/users")
//
// Example - Submit form data:
//
//	resp, err := client.Request("Login").
//	    BodyForm(map[string]string{"username": "john", "password": "secret"}).
//	    Post(ctx, "/auth/login")
func (rb *RequestBuilder) Post(ctx context.Context, path ...string) (*Response, error) {
	if len(path) > 0 {
		rb.path = path[0]
	}
	return rb.execute(ctx, http.MethodPost)
}

// Put executes a PUT request.
//
// PUT is typically used to replace an existing resource entirely.
// The request body should contain the complete updated resource.
//
// Example:
//
//	type User struct {
//	    ID    int    `json:"id"`
//	    Name  string `json:"name"`
//	    Email string `json:"email"`
//	}
//
//	user := User{ID: 123, Name: "John Updated", Email: "john.new@example.com"}
//	resp, err := client.Request("UpdateUser").
//	    PathParam("id", "123").
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
// PATCH is typically used to partially update a resource, sending only the
// fields that need to be changed rather than the entire resource.
//
// Example:
//
//	patch := map[string]any{"name": "Updated Name"}
//	resp, err := client.Request("PatchUser").
//	    PathParam("id", "123").
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
// DELETE is used to remove a resource. DELETE requests typically don't have
// a request body, though some APIs may accept one for additional parameters.
//
// Example:
//
//	resp, err := client.Request("DeleteUser").
//	    PathParam("id", userID).
//	    Delete(ctx, "/users/{id}")
//	if err != nil {
//	    return err
//	}
//	if resp.StatusCode() == http.StatusNoContent {
//	    log.Println("User deleted successfully")
//	}
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
	var bodyBytes []byte
	if len(rb.fileUploads) > 0 {
		body, contentType, err := rb.buildMultipart()
		if err != nil {
			return nil, err
		}
		reqBody = body
		rb.contentType = contentType
	}

	// Read body for potential replay (needed for hedging)
	if reqBody != nil && rb.hedgeConfig != nil && rb.hedgeConfig.Enabled() {
		bodyBytes, err = io.ReadAll(reqBody)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(bodyBytes)
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

	// Determine endpoint key for latency tracking
	endpoint := rb.operationName
	if endpoint == "" {
		endpoint = req.URL.Path
	}

	// Execute request (with or without hedging)
	var httpResp *http.Response
	switch {
	case rb.adaptiveHedgeConfig != nil && rb.adaptiveHedgeConfig.Enabled():
		// Adaptive hedging: calculate delay from historical data
		delay := rb.adaptiveHedgeConfig.GetDelay(endpoint)
		hedgeCfg := &HedgeConfig{
			Delay:     delay,
			MaxHedges: rb.adaptiveHedgeConfig.MaxHedges,
		}
		//nolint:bodyclose // Caller closes via Response
		httpResp, err = rb.executeWithHedgingConfig(ctx, req, bodyBytes, hedgeCfg)
	case rb.hedgeConfig != nil && rb.hedgeConfig.Enabled():
		//nolint:bodyclose // Caller closes via Response
		httpResp, err = rb.executeWithHedging(ctx, req, bodyBytes)
	default:
		// The caller is responsible for closing the response body.
		// Response.Body() handles this automatically when called.
		//nolint:bodyclose // Caller closes via Response
		httpResp, err = rb.client.httpClient.Do(req)
	}

	duration := time.Since(startTime)

	// Record latency for adaptive hedging (only on success)
	if httpResp != nil && rb.adaptiveHedgeConfig != nil {
		rb.adaptiveHedgeConfig.GetTracker().Record(endpoint, duration)
	}

	if err != nil {
		return nil, err
	}

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
		var curlBody []byte
		if reqBody != nil {
			if buf, ok := reqBody.(*bytes.Buffer); ok {
				curlBody = buf.Bytes()
			}
		}
		resp.curlCommand = generateCurlCommand(req, curlBody)
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

// executeWithHedging executes the request with hedging support using the RequestBuilder's config.
func (rb *RequestBuilder) executeWithHedging(
	ctx context.Context,
	originalReq *http.Request,
	bodyBytes []byte,
) (*http.Response, error) {
	return rb.executeWithHedgingConfig(ctx, originalReq, bodyBytes, rb.hedgeConfig)
}

// executeWithHedgingConfig executes the request with hedging support using the provided config.
func (rb *RequestBuilder) executeWithHedgingConfig(
	ctx context.Context,
	originalReq *http.Request,
	bodyBytes []byte,
	cfg *HedgeConfig,
) (*http.Response, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		resp *http.Response
		err  error
	}
	results := make(chan result, cfg.MaxHedges+1)

	// Function to execute a single request
	doRequest := func() {
		req := originalReq.Clone(ctx)
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := rb.client.httpClient.Do(req)
		select {
		case <-ctx.Done():
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		case results <- result{resp: resp, err: err}:
		}
	}

	// Start original request
	go doRequest()

	// Set up timers for hedge requests
	hedgeTimers := make([]*time.Timer, cfg.MaxHedges)
	for i := range cfg.MaxHedges {
		delay := cfg.Delay * time.Duration(i+1)
		hedgeTimers[i] = time.AfterFunc(delay, doRequest)
	}

	// Wait for first result
	res := <-results

	// Cancel remaining and stop timers
	cancel()
	for _, timer := range hedgeTimers {
		timer.Stop()
	}

	// Drain remaining results in background
	go func() {
		for r := range results {
			if r.resp != nil && r.resp.Body != nil {
				r.resp.Body.Close()
			}
		}
	}()

	return res.resp, res.err
}

// buildURL constructs the full URL from base URL, path, and query params.
func (rb *RequestBuilder) buildURL() (string, error) {
	// Start with path
	path := rb.path

	// Replace path parameters
	for k, v := range rb.pathParams {
		path = strings.ReplaceAll(path, "{"+k+"}", url.PathEscape(v))
	}

	// Build full URL using url.JoinPath for proper path handling
	var fullURL string
	var err error
	if rb.client.baseURL != "" {
		fullURL, err = url.JoinPath(rb.client.baseURL, path)
		if err != nil {
			return "", err
		}
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
