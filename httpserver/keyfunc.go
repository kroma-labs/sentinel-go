package httpserver

import "net/http"

// KeyFunc extracts a rate limiting key from a request.
//
// The key determines how requests are grouped for rate limiting.
// Requests with the same key share the same rate limit bucket.
//
// # Example Usage
//
// Global rate limit (all requests share one bucket):
//
//	httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,    // 100 requests per second
//	    Burst:   200,    // Allow bursts up to 200
//	    KeyFunc: nil,    // nil = global (all requests share bucket)
//	})
//
// Per-IP rate limit (each IP has its own bucket):
//
//	httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByIP(), // Each IP gets 100 req/sec
//	})
//
// Per-endpoint rate limit (each endpoint has its own bucket):
//
//	httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByPath(), // /api/users and /api/orders limited separately
//	})
//
// Per-IP per-endpoint rate limit (most granular):
//
//	httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByIPAndPath(), // IP1:/api/users != IP1:/api/orders != IP2:/api/users
//	})
type KeyFunc func(r *http.Request) string

// KeyFuncByIP returns a KeyFunc that extracts the client IP.
//
// Uses X-Forwarded-For header if present (for reverse proxy setups),
// otherwise falls back to RemoteAddr.
//
// # Use Case
//
// Rate limit each client IP independently. For example, 100 req/sec per IP
// means one abusive client doesn't affect other clients.
//
// # Example
//
//	// Each IP can make 100 requests per second
//	mux.Handle("/api/", httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByIP(),
//	})(handler))
func KeyFuncByIP() KeyFunc {
	return func(r *http.Request) string {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return xff
		}
		return r.RemoteAddr
	}
}

// KeyFuncByPath returns a KeyFunc that uses the URL path.
//
// # Use Case
//
// Rate limit each endpoint independently. For example, /api/search might
// have a lower limit than /api/users because it's more expensive.
//
// # Example
//
//	// All requests to /api/search share 10 req/sec
//	// All requests to /api/users share separate 100 req/sec
//	mux.Handle("/api/", httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByPath(),
//	})(handler))
//
// # Note
//
// This limits ALL clients combined per endpoint. For per-client per-endpoint,
// use KeyFuncByIPAndPath or KeyFuncByClientIDAndPath.
func KeyFuncByPath() KeyFunc {
	return func(r *http.Request) string {
		return r.URL.Path
	}
}

// KeyFuncByIPAndPath returns a KeyFunc that combines client IP and path.
//
// # Use Case
//
// Most granular rate limiting for public APIs. Each client IP gets its
// own rate limit bucket per endpoint.
//
// # Example
//
//	// IP 1.2.3.4 gets 100 req/sec to /api/users
//	// IP 1.2.3.4 gets separate 100 req/sec to /api/orders
//	// IP 5.6.7.8 gets its own 100 req/sec to /api/users
//	mux.Handle("/api/", httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByIPAndPath(),
//	})(handler))
func KeyFuncByIPAndPath() KeyFunc {
	return func(r *http.Request) string {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = xff
		}
		return ip + ":" + r.URL.Path
	}
}

// KeyFuncByClientID returns a KeyFunc that uses the client_id from ServiceAuth.
//
// # Use Case
//
// Rate limit authenticated service clients. Use after ServiceAuth middleware.
//
// # Example
//
//	// Each authenticated client gets 1000 req/sec across all endpoints
//	mux.Handle("/internal/", httpserver.Chain(
//	    httpserver.ServiceAuth(authConfig),
//	    httpserver.RateLimit(httpserver.RateLimitConfig{
//	        Limit:   1000,
//	        Burst:   2000,
//	        KeyFunc: httpserver.KeyFuncByClientID(),
//	    }),
//	)(handler))
//
// # Prerequisite
//
// Must be used after ServiceAuth middleware. Without authentication,
// returns empty string (all unauthenticated requests share one bucket).
func KeyFuncByClientID() KeyFunc {
	return func(r *http.Request) string {
		return ClientIDFromContext(r.Context())
	}
}

// KeyFuncByClientIDAndPath combines client_id and path.
//
// # Use Case
//
// Rate limit authenticated clients per endpoint. For example, a client
// might have different limits for read vs write operations.
//
// # Example
//
//	// client-1 gets 100 req/sec to /api/read
//	// client-1 gets separate 10 req/sec to /api/write
//	mux.Handle("/api/", httpserver.Chain(
//	    httpserver.ServiceAuth(authConfig),
//	    httpserver.RateLimit(httpserver.RateLimitConfig{
//	        Limit:   100,
//	        Burst:   200,
//	        KeyFunc: httpserver.KeyFuncByClientIDAndPath(),
//	    }),
//	)(handler))
func KeyFuncByClientIDAndPath() KeyFunc {
	return func(r *http.Request) string {
		clientID := ClientIDFromContext(r.Context())
		return clientID + ":" + r.URL.Path
	}
}

// KeyFuncByHeader returns a KeyFunc that extracts a value from a header.
//
// # Use Case
//
// Rate limit by a custom header, such as tenant ID or API key.
//
// # Example
//
//	// Each tenant gets 100 req/sec
//	mux.Handle("/api/", httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit:   100,
//	    Burst:   200,
//	    KeyFunc: httpserver.KeyFuncByHeader("X-Tenant-ID"),
//	})(handler))
func KeyFuncByHeader(header string) KeyFunc {
	return func(r *http.Request) string {
		return r.Header.Get(header)
	}
}
