package httpserver

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig configures the CORS middleware.
type CORSConfig struct {
	// AllowedOrigins is a list of origins that are allowed.
	// Use "*" to allow all origins (not recommended for production).
	AllowedOrigins []string

	// AllowedMethods is a list of HTTP methods allowed.
	// Default: GET, POST, PUT, DELETE, OPTIONS, HEAD, PATCH
	AllowedMethods []string

	// AllowedHeaders is a list of headers that are allowed in requests.
	AllowedHeaders []string

	// ExposedHeaders is a list of headers that are exposed to the client.
	ExposedHeaders []string

	// AllowCredentials indicates whether credentials (cookies, auth headers)
	// are allowed in cross-origin requests.
	AllowCredentials bool

	// MaxAge is the maximum age (in seconds) of the preflight cache.
	// Default: 86400 (24 hours)
	MaxAge int
}

// DefaultCORSConfig returns a permissive CORS configuration.
//
// This is suitable for development. For production, customize the
// AllowedOrigins to your specific domains.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodHead,
			http.MethodPatch,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Request-ID",
			"X-Requested-With",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
		},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORS returns middleware that handles Cross-Origin Resource Sharing.
//
// Example:
//
//	handler := httpserver.CORS(httpserver.CORSConfig{
//	    AllowedOrigins:   []string{"https://example.com"},
//	    AllowCredentials: true,
//	})(myHandler)
func CORS(cfg CORSConfig) Middleware {
	// Build origin lookup map
	allowAllOrigins := false
	origins := make(map[string]bool)
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			allowAllOrigins = true
		}
		origins[origin] = true
	}

	// Pre-compute header values
	allowMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if origin != "" {
				if allowAllOrigins {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if origins[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			// Set other CORS headers
			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if len(cfg.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
