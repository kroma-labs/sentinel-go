package httpserver

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	json "github.com/goccy/go-json"
)

// HealthCheck is a function that checks the health of a dependency.
//
// Return nil if the dependency is healthy, or an error describing the issue.
//
// Example:
//
//	func dbHealthCheck(ctx context.Context) error {
//	    return db.PingContext(ctx)
//	}
type HealthCheck func(ctx context.Context) error

// CheckResult contains the result of a single health check.
type CheckResult struct {
	Status              string `json:"status"`
	Latency             string `json:"latency"`
	Message             string `json:"message,omitempty"`
	LastChecked         string `json:"last_checked"`
	ConsecutiveSuccess  int    `json:"consecutive_successes,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
}

// HealthResponse contains the full health response data.
type HealthResponse struct {
	Status    string                 `json:"status"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version"`
	Uptime    string                 `json:"uptime,omitempty"`
	Hostname  string                 `json:"hostname,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
}

// PingResponse contains the ping response data.
type PingResponse struct {
	Status string `json:"status"`
}

// checkState tracks the state of a health check.
type checkState struct {
	check               HealthCheck
	lastStatus          string
	lastLatency         time.Duration
	lastChecked         time.Time
	lastError           error
	consecutiveSuccess  int
	consecutiveFailures int
}

// HealthHandler manages health check endpoints.
//
// Create a HealthHandler using NewHealthHandler():
//
//	health := httpserver.NewHealthHandler(
//	    httpserver.WithServiceName("my-service"),
//	    httpserver.WithVersion("1.0.0"),
//	)
//
//	health.AddReadinessCheck("postgres", dbChecker)
//	health.AddReadinessCheck("redis", redisChecker)
//
//	mux.Handle("/ping", health.PingHandler())
//	mux.Handle("/livez", health.LiveHandler())
//	mux.Handle("/readyz", health.ReadyHandler())
type HealthHandler struct {
	serviceName string
	version     string
	startTime   time.Time
	hostname    string

	mu              sync.RWMutex
	livenessChecks  map[string]*checkState
	readinessChecks map[string]*checkState
}

// HealthOption configures the HealthHandler.
type HealthOption func(*HealthHandler)

// withHealthServiceName sets the service name for health responses.
// This is set internally by the server via WithHealth.
func withHealthServiceName(name string) HealthOption {
	return func(h *HealthHandler) {
		h.serviceName = name
	}
}

// WithVersion sets the version for health responses.
func WithVersion(version string) HealthOption {
	return func(h *HealthHandler) {
		h.version = version
	}
}

// NewHealthHandler creates a new HealthHandler.
//
// When using with httpserver, use WithHealth instead for automatic ServiceName.
//
// Example (standalone):
//
//	health := httpserver.NewHealthHandler(
//	    httpserver.WithVersion("2.1.0"),
//	)
//
// Example (with server - recommended):
//
//	var health *httpserver.HealthHandler
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHealth(&health, "2.1.0"),
//	)
//	// health is now usable with ServiceName auto-set
func NewHealthHandler(opts ...HealthOption) *HealthHandler {
	hostname, _ := os.Hostname()

	h := &HealthHandler{
		serviceName:     "unknown",
		version:         "0.0.0",
		startTime:       time.Now(),
		hostname:        hostname,
		livenessChecks:  make(map[string]*checkState),
		readinessChecks: make(map[string]*checkState),
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// AddLivenessCheck adds a health check for the liveness probe.
//
// Liveness checks determine if the process is alive and not deadlocked.
// If a liveness check fails, Kubernetes will restart the pod.
//
// Use sparingly - most applications only need basic process checks.
//
// Example:
//
//	health.AddLivenessCheck("goroutines", func(ctx context.Context) error {
//	    if runtime.NumGoroutine() > 10000 {
//	        return errors.New("too many goroutines")
//	    }
//	    return nil
//	})
func (h *HealthHandler) AddLivenessCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.livenessChecks[name] = &checkState{check: check}
}

// AddReadinessCheck adds a health check for the readiness probe.
//
// Readiness checks determine if the service can handle traffic.
// If a readiness check fails, Kubernetes stops routing traffic to the pod
// but does not restart it.
//
// Add checks for all critical dependencies (database, cache, message queues).
//
// Example:
//
//	health.AddReadinessCheck("postgres", func(ctx context.Context) error {
//	    return db.PingContext(ctx)
//	})
func (h *HealthHandler) AddReadinessCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readinessChecks[name] = &checkState{check: check}
}

// PingHandler returns an http.Handler for the /ping endpoint.
//
// This is a simple connectivity check that always returns 200 OK.
// It does not run any health checks.
func (h *HealthHandler) PingHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response[PingResponse]{
			Data: PingResponse{Status: "pong"},
		})
	})
}

// LiveHandler returns an http.Handler for the /livez endpoint.
//
// Returns 200 if all liveness checks pass, 503 otherwise.
func (h *HealthHandler) LiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handleHealthCheck(w, r, h.livenessChecks)
	})
}

// ReadyHandler returns an http.Handler for the /readyz endpoint.
//
// Returns 200 if all readiness checks pass, 503 otherwise.
func (h *HealthHandler) ReadyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handleHealthCheck(w, r, h.readinessChecks)
	})
}

// handleHealthCheck runs the specified checks and writes the response.
func (h *HealthHandler) handleHealthCheck(
	w http.ResponseWriter,
	r *http.Request,
	checks map[string]*checkState,
) {
	ctx := r.Context()
	now := time.Now()

	h.mu.Lock()
	defer h.mu.Unlock()

	results := make(map[string]CheckResult)
	var errors []Error
	allHealthy := true

	for name, state := range checks {
		start := time.Now()
		err := state.check(ctx)
		latency := time.Since(start)

		// Update state
		state.lastLatency = latency
		state.lastChecked = now

		result := CheckResult{
			Latency:     latency.String(),
			LastChecked: now.Format(time.RFC3339),
		}

		if err != nil {
			state.lastStatus = "fail"
			state.lastError = err
			state.consecutiveFailures++
			state.consecutiveSuccess = 0

			result.Status = "fail"
			result.Message = err.Error()
			result.ConsecutiveFailures = state.consecutiveFailures

			errors = append(errors, Error{
				Field:   name,
				Message: err.Error(),
			})
			allHealthy = false
		} else {
			state.lastStatus = "ok"
			state.lastError = nil
			state.consecutiveSuccess++
			state.consecutiveFailures = 0

			result.Status = "ok"
			result.Message = "connected"
			result.ConsecutiveSuccess = state.consecutiveSuccess
		}

		results[name] = result
	}

	// Build response
	status := "ok"
	statusCode := http.StatusOK
	message := "all checks passed"

	if !allHealthy {
		status = "fail"
		statusCode = http.StatusServiceUnavailable
		message = "one or more checks failed"
	}

	data := HealthResponse{
		Status:    status,
		Service:   h.serviceName,
		Version:   h.version,
		Uptime:    time.Since(h.startTime).Round(time.Second).String(),
		Hostname:  h.hostname,
		Timestamp: now.Format(time.RFC3339),
		Checks:    results,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(Response[HealthResponse]{
		Data:    data,
		Errors:  errors,
		Message: message,
	})
}
