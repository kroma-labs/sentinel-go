package httpserver

import (
	"crypto/subtle"
	"net/http"
	"net/http/pprof"
)

// PprofConfig configures pprof endpoint security.
type PprofConfig struct {
	// Prefix is the URL prefix for pprof endpoints.
	// Default: "/debug/pprof"
	Prefix string

	// EnableAuth enables basic authentication for pprof endpoints.
	EnableAuth bool

	// Username for basic auth (required when EnableAuth is true).
	Username string

	// Password for basic auth (required when EnableAuth is true).
	Password string
}

// DefaultPprofConfig returns default pprof configuration.
func DefaultPprofConfig() PprofConfig {
	return PprofConfig{
		Prefix:     "/debug/pprof",
		EnableAuth: false,
	}
}

// PprofHandler returns an http.Handler that serves pprof endpoints.
//
// Available endpoints:
//   - /debug/pprof/           - Index page
//   - /debug/pprof/cmdline    - Command line
//   - /debug/pprof/profile    - CPU profile
//   - /debug/pprof/symbol     - Symbol lookup
//   - /debug/pprof/trace      - Execution trace
//   - /debug/pprof/heap       - Heap profile
//   - /debug/pprof/goroutine  - Goroutine profile
//   - /debug/pprof/block      - Block profile
//   - /debug/pprof/mutex      - Mutex profile
//   - /debug/pprof/allocs     - Allocation profile
//   - /debug/pprof/threadcreate - Thread creation profile
//
// Example:
//
//	mux.Handle("/debug/pprof/", httpserver.PprofHandler(httpserver.PprofConfig{
//	    EnableAuth: true,
//	    Username:   "admin",
//	    Password:   "secret",
//	}))
func PprofHandler(cfg PprofConfig) http.Handler {
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Prefix+"/", pprof.Index)
	mux.HandleFunc(cfg.Prefix+"/cmdline", pprof.Cmdline)
	mux.HandleFunc(cfg.Prefix+"/profile", pprof.Profile)
	mux.HandleFunc(cfg.Prefix+"/symbol", pprof.Symbol)
	mux.HandleFunc(cfg.Prefix+"/trace", pprof.Trace)

	handler := http.Handler(mux)

	if cfg.EnableAuth && cfg.Username != "" && cfg.Password != "" {
		handler = pprofBasicAuth(cfg.Username, cfg.Password, handler)
	}

	return handler
}

// pprofBasicAuth wraps handler with HTTP Basic Authentication.
func pprofBasicAuth(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="pprof"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		usernameMatch := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1

		if !usernameMatch || !passwordMatch {
			w.Header().Set("WWW-Authenticate", `Basic realm="pprof"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RegisterPprof registers pprof handlers on the given ServeMux.
//
// Example:
//
//	mux := http.NewServeMux()
//	httpserver.RegisterPprof(mux, httpserver.DefaultPprofConfig())
func RegisterPprof(mux *http.ServeMux, cfg PprofConfig) {
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}

	handler := PprofHandler(cfg)

	mux.Handle(cfg.Prefix+"/", handler)
}
