package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kroma-labs/sentinel-go/httpserver"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRecoveryMiddleware(t *testing.T) {
	t.Parallel()

	type args struct {
		panicValue any
		handler    http.HandlerFunc
	}

	tests := []struct {
		name           string
		args           args
		wantStatusCode int
		wantPanic      bool
	}{
		{
			name: "given handler panics with string, when recovery applied, then returns 500",
			args: args{
				panicValue: "test panic",
				handler: func(_ http.ResponseWriter, _ *http.Request) {
					panic("test panic")
				},
			},
			wantStatusCode: http.StatusInternalServerError,
			wantPanic:      false,
		},
		{
			name: "given handler panics with error, when recovery applied, then returns 500",
			args: args{
				panicValue: assert.AnError,
				handler: func(_ http.ResponseWriter, _ *http.Request) {
					panic(assert.AnError)
				},
			},
			wantStatusCode: http.StatusInternalServerError,
			wantPanic:      false,
		},
		{
			name: "given handler does not panic, when recovery applied, then proceeds normally",
			args: args{
				handler: func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			wantStatusCode: http.StatusOK,
			wantPanic:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := zerolog.Nop()
			middleware := httpserver.Recovery(logger)
			handler := middleware(tt.args.handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()

			if tt.wantPanic {
				assert.Panics(t, func() {
					handler.ServeHTTP(rec, req)
				})
			} else {
				assert.NotPanics(t, func() {
					handler.ServeHTTP(rec, req)
				})
				assert.Equal(t, tt.wantStatusCode, rec.Code)
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	t.Parallel()

	type args struct {
		method           string
		origin           string
		requestMethod    string
		requestHeaders   string
		allowedOrigins   []string
		allowedMethods   []string
		allowedHeaders   []string
		allowCredentials bool
	}

	tests := []struct {
		name            string
		args            args
		wantAllowOrigin string
		wantStatusCode  int
	}{
		{
			name: "given preflight request with matching origin, then returns CORS headers",
			args: args{
				method:         http.MethodOptions,
				origin:         "https://example.com",
				requestMethod:  "POST",
				allowedOrigins: []string{"https://example.com"},
				allowedMethods: []string{"GET", "POST"},
				allowedHeaders: []string{"Content-Type"},
			},
			wantAllowOrigin: "https://example.com",
			wantStatusCode:  http.StatusNoContent,
		},
		{
			name: "given preflight request with wildcard origin, then allows all",
			args: args{
				method:         http.MethodOptions,
				origin:         "https://any-origin.com",
				requestMethod:  "GET",
				allowedOrigins: []string{"*"},
				allowedMethods: []string{"GET"},
			},
			wantAllowOrigin: "https://any-origin.com",
			wantStatusCode:  http.StatusNoContent,
		},
		{
			name: "given preflight request with non-matching origin, then no CORS headers",
			args: args{
				method:         http.MethodOptions,
				origin:         "https://evil.com",
				requestMethod:  "GET",
				allowedOrigins: []string{"https://example.com"},
				allowedMethods: []string{"GET"},
			},
			wantAllowOrigin: "",
			wantStatusCode:  http.StatusNoContent,
		},
		{
			name: "given actual request with matching origin, then sets origin header",
			args: args{
				method:         http.MethodGet,
				origin:         "https://example.com",
				allowedOrigins: []string{"https://example.com"},
				allowedMethods: []string{"GET"},
			},
			wantAllowOrigin: "https://example.com",
			wantStatusCode:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := httpserver.CORSConfig{
				AllowedOrigins:   tt.args.allowedOrigins,
				AllowedMethods:   tt.args.allowedMethods,
				AllowedHeaders:   tt.args.allowedHeaders,
				AllowCredentials: tt.args.allowCredentials,
			}

			middleware := httpserver.CORS(cfg)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.args.method, "/", nil)
			if tt.args.origin != "" {
				req.Header.Set("Origin", tt.args.origin)
			}
			if tt.args.requestMethod != "" {
				req.Header.Set("Access-Control-Request-Method", tt.args.requestMethod)
			}
			if tt.args.requestHeaders != "" {
				req.Header.Set("Access-Control-Request-Headers", tt.args.requestHeaders)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatusCode, rec.Code)
			assert.Equal(t, tt.wantAllowOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	t.Parallel()

	type args struct {
		existingID string
	}

	tests := []struct {
		name       string
		args       args
		wantNewID  bool
		wantSameID bool
	}{
		{
			name:      "given no existing ID, when applied, then generates new ID",
			args:      args{existingID: ""},
			wantNewID: true,
		},
		{
			name:       "given existing ID, when applied, then forwards existing ID",
			args:       args{existingID: "existing-request-id-123"},
			wantSameID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			middleware := httpserver.RequestID()
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.args.existingID != "" {
				req.Header.Set("X-Request-ID", tt.args.existingID)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			responseID := rec.Header().Get("X-Request-ID")

			if tt.wantNewID {
				assert.NotEmpty(t, responseID)
			}
			if tt.wantSameID {
				assert.Equal(t, tt.args.existingID, responseID)
			}
		})
	}
}

func TestChainMiddleware(t *testing.T) {
	t.Parallel()

	type args struct {
		middlewareCount int
	}

	tests := []struct {
		name      string
		args      args
		wantOrder []string
	}{
		{
			name:      "given no middleware, when chained, then handler executes",
			args:      args{middlewareCount: 0},
			wantOrder: []string{"handler"},
		},
		{
			name:      "given one middleware, when chained, then executes in order",
			args:      args{middlewareCount: 1},
			wantOrder: []string{"m1-before", "handler", "m1-after"},
		},
		{
			name: "given multiple middleware, when chained, then executes in order",
			args: args{middlewareCount: 3},
			wantOrder: []string{
				"m1-before",
				"m2-before",
				"m3-before",
				"handler",
				"m3-after",
				"m2-after",
				"m1-after",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			order := []string{}

			makeMiddleware := func(name string) httpserver.Middleware {
				return func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						order = append(order, name+"-before")
						next.ServeHTTP(w, r)
						order = append(order, name+"-after")
					})
				}
			}

			var middlewares []httpserver.Middleware
			for i := 1; i <= tt.args.middlewareCount; i++ {
				middlewares = append(middlewares, makeMiddleware("m"+string(rune('0'+i))))
			}

			handler := httpserver.Chain(
				middlewares...)(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					order = append(order, "handler")
					w.WriteHeader(http.StatusOK)
				}),
			)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantOrder, order)
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	t.Parallel()

	type args struct {
		limit   float64
		burst   int
		numReqs int
	}

	tests := []struct {
		name            string
		args            args
		wantAllowed     int
		wantRateLimited int
	}{
		{
			name: "given burst capacity 1, when 1 request, then all allowed",
			args: args{
				limit:   1,
				burst:   1,
				numReqs: 1,
			},
			wantAllowed:     1,
			wantRateLimited: 0,
		},
		{
			name: "given burst capacity 1, when 3 requests, then 1 allowed 2 limited",
			args: args{
				limit:   1,
				burst:   1,
				numReqs: 3,
			},
			wantAllowed:     1,
			wantRateLimited: 2,
		},
		{
			name: "given burst capacity 5, when 5 requests, then all allowed",
			args: args{
				limit:   1,
				burst:   5,
				numReqs: 5,
			},
			wantAllowed:     5,
			wantRateLimited: 0,
		},
		{
			name: "given burst capacity 5, when 10 requests, then 5 allowed 5 limited",
			args: args{
				limit:   1,
				burst:   5,
				numReqs: 10,
			},
			wantAllowed:     5,
			wantRateLimited: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			middleware := httpserver.RateLimit(httpserver.RateLimitConfig{
				Limit: rate.Limit(tt.args.limit),
				Burst: tt.args.burst,
			})

			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			allowed := 0
			rateLimited := 0

			for range tt.args.numReqs {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				switch rec.Code {
				case http.StatusOK:
					allowed++
				case http.StatusTooManyRequests:
					rateLimited++
				}
			}

			assert.Equal(t, tt.wantAllowed, allowed)
			assert.Equal(t, tt.wantRateLimited, rateLimited)
		})
	}
}

func TestRateLimitByIPMiddleware(t *testing.T) {
	t.Parallel()

	type request struct {
		ip         string
		wantStatus int
	}

	tests := []struct {
		name     string
		limit    rate.Limit
		burst    int
		requests []request
	}{
		{
			name:  "given per-IP limit, when same IP exceeds, then limited",
			limit: 1,
			burst: 1,
			requests: []request{
				{ip: "192.168.1.1:1234", wantStatus: http.StatusOK},
				{ip: "192.168.1.1:1234", wantStatus: http.StatusTooManyRequests},
			},
		},
		{
			name:  "given per-IP limit, when different IPs, then separate buckets",
			limit: 1,
			burst: 1,
			requests: []request{
				{ip: "192.168.1.1:1234", wantStatus: http.StatusOK},
				{ip: "192.168.1.2:1234", wantStatus: http.StatusOK},
				{ip: "192.168.1.3:1234", wantStatus: http.StatusOK},
			},
		},
		{
			name:  "given per-IP limit with burst, when within burst, then allowed",
			limit: 1,
			burst: 3,
			requests: []request{
				{ip: "10.0.0.1:1234", wantStatus: http.StatusOK},
				{ip: "10.0.0.1:1234", wantStatus: http.StatusOK},
				{ip: "10.0.0.1:1234", wantStatus: http.StatusOK},
				{ip: "10.0.0.1:1234", wantStatus: http.StatusTooManyRequests},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			middleware := httpserver.RateLimitByIP(tt.limit, tt.burst)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i, r := range tt.requests {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = r.ip
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				assert.Equal(t, r.wantStatus, rec.Code, "request %d from %s", i, r.ip)
			}
		})
	}
}

// TestServiceAuthMiddleware is in auth_test.go

func TestHealthHandlerEndpoints(t *testing.T) {
	t.Parallel()

	type checkResult struct {
		name string
		err  error
	}

	tests := []struct {
		name            string
		version         string
		livenessChecks  []checkResult
		readinessChecks []checkResult
		wantPingStatus  int
		wantLiveStatus  int
		wantReadyStatus int
	}{
		{
			name:            "given healthy handler, when all checks pass, then all endpoints return 200",
			version:         "1.0.0",
			livenessChecks:  []checkResult{{name: "db", err: nil}},
			readinessChecks: []checkResult{{name: "cache", err: nil}},
			wantPingStatus:  http.StatusOK,
			wantLiveStatus:  http.StatusOK,
			wantReadyStatus: http.StatusOK,
		},
		{
			name:            "given failing liveness check, when checked, then livez returns 503",
			version:         "1.0.0",
			livenessChecks:  []checkResult{{name: "db", err: assert.AnError}},
			readinessChecks: []checkResult{{name: "cache", err: nil}},
			wantPingStatus:  http.StatusOK,
			wantLiveStatus:  http.StatusServiceUnavailable,
			wantReadyStatus: http.StatusOK,
		},
		{
			name:            "given failing readiness check, when checked, then readyz returns 503",
			version:         "1.0.0",
			livenessChecks:  []checkResult{{name: "db", err: nil}},
			readinessChecks: []checkResult{{name: "cache", err: assert.AnError}},
			wantPingStatus:  http.StatusOK,
			wantLiveStatus:  http.StatusOK,
			wantReadyStatus: http.StatusServiceUnavailable,
		},
		{
			name:            "given no checks, when checked, then all endpoints return 200",
			version:         "2.0.0",
			wantPingStatus:  http.StatusOK,
			wantLiveStatus:  http.StatusOK,
			wantReadyStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := httpserver.NewHealthHandler(httpserver.WithVersion(tt.version))

			for _, check := range tt.livenessChecks {
				err := check.err
				handler.AddLivenessCheck(check.name, func(_ context.Context) error {
					return err
				})
			}

			for _, check := range tt.readinessChecks {
				err := check.err
				handler.AddReadinessCheck(check.name, func(_ context.Context) error {
					return err
				})
			}

			// Test ping
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			rec := httptest.NewRecorder()
			handler.PingHandler().ServeHTTP(rec, req)
			require.Equal(t, tt.wantPingStatus, rec.Code)

			// Test livez
			req = httptest.NewRequest(http.MethodGet, "/livez", nil)
			rec = httptest.NewRecorder()
			handler.LiveHandler().ServeHTTP(rec, req)
			require.Equal(t, tt.wantLiveStatus, rec.Code)

			// Test readyz
			req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec = httptest.NewRecorder()
			handler.ReadyHandler().ServeHTTP(rec, req)
			require.Equal(t, tt.wantReadyStatus, rec.Code)
		})
	}
}

func TestKeyFuncHelpers(t *testing.T) {
	t.Parallel()

	type args struct {
		remoteAddr    string
		xForwardedFor string
		path          string
		headerName    string
		headerValue   string
	}

	tests := []struct {
		name    string
		keyFunc func() httpserver.KeyFunc
		args    args
		wantKey string
	}{
		{
			name:    "KeyFuncByIP returns RemoteAddr when no X-Forwarded-For",
			keyFunc: httpserver.KeyFuncByIP,
			args: args{
				remoteAddr: "192.168.1.1:1234",
			},
			wantKey: "192.168.1.1:1234",
		},
		{
			name:    "KeyFuncByIP returns X-Forwarded-For when present",
			keyFunc: httpserver.KeyFuncByIP,
			args: args{
				remoteAddr:    "10.0.0.1:1234",
				xForwardedFor: "203.0.113.50",
			},
			wantKey: "203.0.113.50",
		},
		{
			name:    "KeyFuncByPath returns URL path",
			keyFunc: httpserver.KeyFuncByPath,
			args: args{
				path: "/api/v1/users",
			},
			wantKey: "/api/v1/users",
		},
		{
			name:    "KeyFuncByIPAndPath returns combined key without X-Forwarded-For",
			keyFunc: httpserver.KeyFuncByIPAndPath,
			args: args{
				remoteAddr: "192.168.1.1:1234",
				path:       "/api/v1/users",
			},
			wantKey: "192.168.1.1:1234:/api/v1/users",
		},
		{
			name:    "KeyFuncByIPAndPath returns combined key with X-Forwarded-For",
			keyFunc: httpserver.KeyFuncByIPAndPath,
			args: args{
				remoteAddr:    "10.0.0.1:1234",
				xForwardedFor: "203.0.113.50",
				path:          "/api/v1/users",
			},
			wantKey: "203.0.113.50:/api/v1/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keyFunc := tt.keyFunc()

			path := tt.args.path
			if path == "" {
				path = "/"
			}

			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tt.args.remoteAddr != "" {
				req.RemoteAddr = tt.args.remoteAddr
			}
			if tt.args.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.args.xForwardedFor)
			}
			if tt.args.headerName != "" {
				req.Header.Set(tt.args.headerName, tt.args.headerValue)
			}

			key := keyFunc(req)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}

func TestKeyFuncByHeaderHelper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		headerName  string
		headerValue string
		wantKey     string
	}{
		{
			name:        "given header present, then returns header value",
			headerName:  "X-Tenant-ID",
			headerValue: "tenant-123",
			wantKey:     "tenant-123",
		},
		{
			name:       "given header absent, then returns empty string",
			headerName: "X-Tenant-ID",
			wantKey:    "",
		},
		{
			name:        "given custom header, then returns value",
			headerName:  "X-API-Key",
			headerValue: "api-key-abc",
			wantKey:     "api-key-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keyFunc := httpserver.KeyFuncByHeader(tt.headerName)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.headerValue != "" {
				req.Header.Set(tt.headerName, tt.headerValue)
			}

			key := keyFunc(req)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}
