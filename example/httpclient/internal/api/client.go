package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kroma-labs/sentinel-go/example/httpclient/internal/config"
	"github.com/kroma-labs/sentinel-go/httpclient"
)

// User represents a user entity for API operations
type User struct {
	ID        int       `json:"id,omitempty"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// HTTPBinResponse represents the response from httpbin endpoints
type HTTPBinResponse struct {
	Args    map[string]string `json:"args,omitempty"`
	Data    string            `json:"data,omitempty"`
	Files   map[string]string `json:"files,omitempty"`
	Form    map[string]string `json:"form,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	JSON    any               `json:"json,omitempty"`
	Method  string            `json:"method,omitempty"`
	Origin  string            `json:"origin,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Client wraps the httpclient with application-specific methods
type Client struct {
	// Standard client with retry and circuit breaker
	client *httpclient.Client

	// High-throughput client for bulk operations
	highThroughputClient *httpclient.Client

	// Low-latency client for time-sensitive operations
	lowLatencyClient *httpclient.Client
}

// New creates a new API client with all features enabled
func New() *Client {
	// Standard client with balanced configuration
	standardClient := httpclient.New(
		httpclient.WithBaseURL(config.MockAPIBaseURL),
		httpclient.WithServiceName(config.ServiceName),

		// Retry configuration with exponential backoff
		httpclient.WithRetryConfig(httpclient.RetryConfig{
			MaxRetries:      3,
			InitialInterval: 500 * time.Millisecond,
			MaxInterval:     30 * time.Second,
			MaxElapsedTime:  2 * time.Minute,
			Multiplier:      2.0,
			JitterFactor:    0.5,
		}),

		// Circuit breaker for fault tolerance
		httpclient.WithBreakerConfig(httpclient.BreakerConfig{
			MaxRequests:         5,
			Interval:            10 * time.Second,
			Timeout:             30 * time.Second,
			FailureThreshold:    3,
			FailureRatio:        0.5,
			ConsecutiveFailures: 3,
		}),

		// Client-level rate limiting (100 req/s with burst of 20)
		httpclient.WithRateLimit(httpclient.RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             20,
			WaitOnLimit:       true,
		}),

		// Add default headers
		httpclient.WithDefaultHeader("User-Agent", "sentinel-httpclient-example/0.1.0"),

		// Request interceptor for auth
		httpclient.WithRequestInterceptor(httpclient.AuthBearerInterceptor("example-token-12345")),

		// Enable debug features
		httpclient.WithDebug(true),
		httpclient.WithGenerateCurl(true),
	)

	// High-throughput client for bulk operations
	highThroughputClient := httpclient.New(
		httpclient.WithBaseURL(config.MockAPIBaseURL),
		httpclient.WithServiceName(config.ServiceName+"-bulk"),
		httpclient.WithConfig(httpclient.HighThroughputConfig()),
		httpclient.WithRetryConfig(httpclient.ConservativeRetryConfig()),
	)

	// Low-latency client for time-sensitive operations
	lowLatencyClient := httpclient.New(
		httpclient.WithBaseURL(config.MockAPIBaseURL),
		httpclient.WithServiceName(config.ServiceName+"-realtime"),
		httpclient.WithConfig(httpclient.LowLatencyConfig()),
		httpclient.WithRetryConfig(httpclient.RetryConfig{
			MaxRetries: 1, // Minimal retries for low latency
		}),
	)

	return &Client{
		client:               standardClient,
		highThroughputClient: highThroughputClient,
		lowLatencyClient:     lowLatencyClient,
	}
}

// ============================================================================
// Basic CRUD Operations
// ============================================================================

// GetUsers retrieves a list of users with query parameters
func (c *Client) GetUsers(ctx context.Context, page, limit int) ([]User, error) {
	var response HTTPBinResponse

	resp, err := c.client.Request("GetUsers").
		Query("page", fmt.Sprintf("%d", page)).
		Query("limit", fmt.Sprintf("%d", limit)).
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return nil, fmt.Errorf("get users: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("get users failed with status: %d", resp.StatusCode)
	}

	log.Printf("📖 GetUsers: page=%d, limit=%d, origin=%s", page, limit, response.Origin)
	return nil, nil
}

// GetUser retrieves a single user by ID with hedging for low latency
func (c *Client) GetUser(ctx context.Context, userID string) (*User, error) {
	var response HTTPBinResponse
	var apiErr ErrorResponse

	resp, err := c.client.Request("GetUser").
		PathParam("id", userID).
		Hedge(50*time.Millisecond). // Send hedge request after 50ms
		Decode(&response).
		DecodeError(&apiErr).
		Get(ctx, "/get")
	if err != nil {
		return nil, fmt.Errorf("get user %s: %w", userID, err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("get user failed: %s - %s", apiErr.Code, apiErr.Message)
	}

	log.Printf("👤 GetUser: id=%s, origin=%s", userID, response.Origin)
	return nil, nil
}

// CreateUser creates a new user with JSON body
func (c *Client) CreateUser(ctx context.Context, user User) (*User, error) {
	var response HTTPBinResponse

	resp, err := c.client.Request("CreateUser").
		BodyJSON(user).
		Decode(&response).
		Post(ctx, "/post")
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("create user failed with status: %d", resp.StatusCode)
	}

	log.Printf("✨ CreateUser: name=%s, email=%s", user.Name, user.Email)
	if cmd := resp.CurlCommand(); cmd != "" {
		log.Printf("📝 Curl command:\n%s", cmd)
	}
	return nil, nil
}

// UpdateUser updates an existing user
func (c *Client) UpdateUser(ctx context.Context, userID string, user User) (*User, error) {
	var response HTTPBinResponse

	resp, err := c.client.Request("UpdateUser").
		PathParam("id", userID).
		BodyJSON(user).
		Decode(&response).
		Put(ctx, "/put")
	if err != nil {
		return nil, fmt.Errorf("update user %s: %w", userID, err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("update user failed with status: %d", resp.StatusCode)
	}

	log.Printf("📝 UpdateUser: id=%s, name=%s", userID, user.Name)
	return nil, nil
}

// DeleteUser deletes a user with request coalescing
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("DeleteUser").
		PathParam("id", userID).
		Coalesce(). // Deduplicate simultaneous requests for same user
		Decode(&response).
		Delete(ctx, "/delete")
	if err != nil {
		return fmt.Errorf("delete user %s: %w", userID, err)
	}

	if resp.IsError() {
		return fmt.Errorf("delete user failed with status: %d", resp.StatusCode)
	}

	log.Printf("🗑️  DeleteUser: id=%s", userID)
	return nil
}

// ============================================================================
// Advanced Features
// ============================================================================

// GetWithTrace demonstrates request tracing for timing info
func (c *Client) GetWithTrace(ctx context.Context) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("GetWithTrace").
		EnableTrace().
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("get with trace: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("get with trace failed with status: %d", resp.StatusCode)
	}

	// Log timing information
	trace := resp.TraceInfo()
	log.Printf("⏱️  GetWithTrace timing:")
	log.Printf("   DNS Lookup:    %s", trace.DNSLookup)
	log.Printf("   Connect Time:  %s", trace.ConnTime)
	log.Printf("   TLS Handshake: %s", trace.TLSHandshake)
	log.Printf("   TTFB:          %s", trace.ServerTime)
	log.Printf("   Total Time:    %s", trace.TotalTime)

	return nil
}

// RateLimitedRequest demonstrates per-request rate limiting
func (c *Client) RateLimitedRequest(ctx context.Context) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("RateLimitedExport").
		RateLimit(10). // Only 10 requests per second for this endpoint
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("rate limited request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("rate limited request failed with status: %d", resp.StatusCode)
	}

	log.Printf("🚦 RateLimitedRequest: completed")
	return nil
}

// AdaptiveHedgeRequest demonstrates adaptive hedging based on historical latency
func (c *Client) AdaptiveHedgeRequest(ctx context.Context) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("AdaptiveHedgeRequest").
		AdaptiveHedge(httpclient.AdaptiveHedgeConfig{
			TargetPercentile: 0.95,                   // Hedge at p95 latency
			MinSamples:       10,                     // Need 10 samples before using adaptive delay
			FallbackDelay:    100 * time.Millisecond, // Use this delay until enough samples
			MaxHedges:        1,
		}).
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("adaptive hedge request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("adaptive hedge request failed with status: %d", resp.StatusCode)
	}

	log.Printf("🎯 AdaptiveHedgeRequest: completed")
	return nil
}

// FormSubmission demonstrates form data submission
func (c *Client) FormSubmission(ctx context.Context, username, password string) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("FormSubmission").
		BodyForm(map[string]string{
			"username": username,
			"password": password,
		}).
		Decode(&response).
		Post(ctx, "/post")
	if err != nil {
		return fmt.Errorf("form submission: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("form submission failed with status: %d", resp.StatusCode)
	}

	log.Printf("📋 FormSubmission: username=%s", username)
	return nil
}

// CustomInterceptorRequest demonstrates per-request interceptors
func (c *Client) CustomInterceptorRequest(ctx context.Context, adminToken string) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("AdminAction").
		Intercept(func(req *http.Request) error {
			req.Header.Set("X-Admin-Token", adminToken)
			return nil
		}).
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("admin request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("admin request failed with status: %d", resp.StatusCode)
	}

	log.Printf("🔐 CustomInterceptorRequest: admin action completed")
	return nil
}

// TimeoutRequest demonstrates per-request timeout
func (c *Client) TimeoutRequest(ctx context.Context) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("SlowEndpoint").
		Timeout(10*time.Second). // Override client timeout for this request
		Decode(&response).
		Get(ctx, "/delay/2") // httpbin delays response by 2 seconds
	if err != nil {
		return fmt.Errorf("timeout request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("timeout request failed with status: %d", resp.StatusCode)
	}

	log.Printf("⏰ TimeoutRequest: completed with custom timeout")
	return nil
}

// HighThroughputRequest demonstrates using the high-throughput client
func (c *Client) HighThroughputRequest(ctx context.Context) error {
	var response HTTPBinResponse

	resp, err := c.highThroughputClient.Request("BulkOperation").
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("high throughput request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("high throughput request failed with status: %d", resp.StatusCode)
	}

	log.Printf("📈 HighThroughputRequest: completed using bulk client")
	return nil
}

// LowLatencyRequest demonstrates using the low-latency client
func (c *Client) LowLatencyRequest(ctx context.Context) error {
	var response HTTPBinResponse

	resp, err := c.lowLatencyClient.Request("RealtimeData").
		Decode(&response).
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("low latency request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("low latency request failed with status: %d", resp.StatusCode)
	}

	log.Printf("⚡ LowLatencyRequest: completed using realtime client")
	return nil
}

// TriggerFailure purposely fails to demonstrate retries and circuit breaker
func (c *Client) TriggerFailure(ctx context.Context) error {
	log.Printf("🔥 Triggering failures to test retries and circuit breaker...")

	// Loop enough times to trip the breaker (threshold is 3)
	for i := 0; i < 10; i++ {
		// Verify if we can even make a request (status 500 triggers retry)
		resp, err := c.client.Request("FailAndRetry").
			Get(ctx, "/status/500")
		if err != nil {
			log.Printf("   Attempt %d: Error: %v", i+1, err)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if resp.IsError() {
			log.Printf("   Attempt %d: Status: %d", i+1, resp.StatusCode)
		}

		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

// PatchUser partially updates an existing user using PATCH method
func (c *Client) PatchUser(ctx context.Context, userID string, fields map[string]string) error {
	var response HTTPBinResponse

	resp, err := c.client.Request("PatchUser").
		PathParam("id", userID).
		BodyJSON(fields).
		Decode(&response).
		Patch(ctx, "/patch")
	if err != nil {
		return fmt.Errorf("patch user %s: %w", userID, err)
	}

	if resp.IsError() {
		return fmt.Errorf("patch user failed with status: %d", resp.StatusCode)
	}

	log.Printf("🩹 PatchUser: id=%s, fields=%v", userID, fields)
	return nil
}

// HealthCheck performs a lightweight health check request
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.client.Request("HealthCheck").
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	log.Printf("💓 HealthCheck: status=%d", resp.StatusCode)
	return nil
}

// UploadPayload sends a large request body to generate meaningful request body size metrics
func (c *Client) UploadPayload(ctx context.Context) error {
	// Build a ~10KB payload to exercise request body size histogram
	payload := map[string]any{
		"operation": "bulk_import",
		"records":   make([]map[string]string, 0, 50),
	}
	for i := 0; i < 50; i++ {
		payload["records"] = append(payload["records"].([]map[string]string), map[string]string{
			"id":    fmt.Sprintf("record-%d", i),
			"name":  fmt.Sprintf("User %d", i),
			"email": fmt.Sprintf("user%d@example.com", i),
			"bio":   strings.Repeat("x", 100),
		})
	}

	var response HTTPBinResponse

	resp, err := c.client.Request("UploadPayload").
		BodyJSON(payload).
		Decode(&response).
		Post(ctx, "/post")
	if err != nil {
		return fmt.Errorf("upload payload: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("upload payload failed with status: %d", resp.StatusCode)
	}

	log.Printf("📦 UploadPayload: sent large body")
	return nil
}

// DownloadLargeResponse fetches a large response to generate response body size
// and content transfer duration metrics
func (c *Client) DownloadLargeResponse(ctx context.Context) error {
	resp, err := c.client.Request("DownloadLargeResponse").
		EnableTrace().
		Get(ctx, "/bytes/10240") // httpbin returns 10KB of random bytes
	if err != nil {
		return fmt.Errorf("download large response: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("download large response failed with status: %d", resp.StatusCode)
	}

	trace := resp.TraceInfo()
	log.Printf("📥 DownloadLargeResponse: size=%d bytes, transfer=%s",
		resp.ContentLength, trace.ServerTime)
	return nil
}

// ExternalHTTPSRequest demonstrates TLS metrics by calling an external HTTPS service
func (c *Client) ExternalHTTPSRequest(ctx context.Context) error {
	log.Printf("🔒 Making external HTTPS request to generate TLS metrics...")

	// Create a temporary client for external access since our main client uses a local base URL
	// We use the same options but override the BaseURL
	extClient := httpclient.New(
		httpclient.WithBaseURL("https://httpbin.org"),
		httpclient.WithServiceName("external-https-client"),
		httpclient.WithEnableTrace(true),
		httpclient.WithDebug(true),
	)

	resp, err := extClient.Request("ExternalTLS").
		EnableTrace().
		Get(ctx, "/get")
	if err != nil {
		return fmt.Errorf("external https request: %w", err)
	}

	// Log timing information to verify TLS capture
	trace := resp.TraceInfo()
	log.Printf("⏱️  External HTTPS timing:")
	log.Printf("   DNS Lookup:    %s", trace.DNSLookup)
	log.Printf("   Connect Time:  %s", trace.ConnTime)
	log.Printf("   TLS Handshake: %s", trace.TLSHandshake)
	log.Printf("   Total Time:    %s", trace.TotalTime)

	return nil
}

// RunAllOperations executes all operations to generate metrics and traces
func (c *Client) RunAllOperations(ctx context.Context) error {
	// Basic CRUD operations
	if _, err := c.GetUsers(ctx, 1, 10); err != nil {
		log.Printf("⚠️  GetUsers error: %v", err)
	}

	if _, err := c.GetUser(ctx, "123"); err != nil {
		log.Printf("⚠️  GetUser error: %v", err)
	}

	user := User{
		Name:  "John Doe",
		Email: "john@example.com",
	}
	if _, err := c.CreateUser(ctx, user); err != nil {
		log.Printf("⚠️  CreateUser error: %v", err)
	}

	updatedUser := User{
		Name:  "John Updated",
		Email: "john.updated@example.com",
	}
	if _, err := c.UpdateUser(ctx, "123", updatedUser); err != nil {
		log.Printf("⚠️  UpdateUser error: %v", err)
	}

	if err := c.DeleteUser(ctx, "456"); err != nil {
		log.Printf("⚠️  DeleteUser error: %v", err)
	}

	// Advanced features
	if err := c.GetWithTrace(ctx); err != nil {
		log.Printf("⚠️  GetWithTrace error: %v", err)
	}

	if err := c.RateLimitedRequest(ctx); err != nil {
		log.Printf("⚠️  RateLimitedRequest error: %v", err)
	}

	if err := c.FormSubmission(ctx, "testuser", "testpass"); err != nil {
		log.Printf("⚠️  FormSubmission error: %v", err)
	}

	if err := c.CustomInterceptorRequest(ctx, "admin-secret-token"); err != nil {
		log.Printf("⚠️  CustomInterceptorRequest error: %v", err)
	}

	// Different client configurations
	if err := c.HighThroughputRequest(ctx); err != nil {
		log.Printf("⚠️  HighThroughputRequest error: %v", err)
	}

	if err := c.LowLatencyRequest(ctx); err != nil {
		log.Printf("⚠️  LowLatencyRequest error: %v", err)
	}

	// PATCH and HEAD methods
	if err := c.PatchUser(ctx, "123", map[string]string{"name": "John Patched"}); err != nil {
		log.Printf("⚠️  PatchUser error: %v", err)
	}

	if err := c.HealthCheck(ctx); err != nil {
		log.Printf("⚠️  HealthCheck error: %v", err)
	}

	// Body size and content transfer metrics
	if err := c.UploadPayload(ctx); err != nil {
		log.Printf("⚠️  UploadPayload error: %v", err)
	}

	if err := c.DownloadLargeResponse(ctx); err != nil {
		log.Printf("⚠️  DownloadLargeResponse error: %v", err)
	}

	// Retry and Circuit Breaker scenarios
	if err := c.TriggerFailure(ctx); err != nil {
		log.Printf("⚠️  TriggerFailure error: %v", err)
	}

	// TLS Metrics
	if err := c.ExternalHTTPSRequest(ctx); err != nil {
		log.Printf("⚠️  ExternalHTTPSRequest error: %v", err)
	}

	return nil
}
