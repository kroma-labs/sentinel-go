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
// This handler registers all pprof endpoints on a new ServeMux.
// Note that when mounting this handler, you must strip the prefix if it's mounting at a path
// that doesn't match the internal paths, or rely on RegisterPprof for simpler usage.
func PprofHandler(cfg PprofConfig) http.Handler {
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}

	mux := http.NewServeMux()

	// Register all endpoints on the mux
	registerPprofEndpoints(mux, cfg.Prefix)

	handler := http.Handler(mux)

	if cfg.EnableAuth && cfg.Username != "" && cfg.Password != "" {
		handler = pprofBasicAuth(cfg.Username, cfg.Password, handler)
	}

	return handler
}

// RegisterPprof registers pprof handlers on the given ServeMux.
//
// This registers all pprof endpoints directly on the provided mux.
// This is the recommended way to register pprof to avoid routing conflicts
// with pattern matching in Go 1.22+.
func RegisterPprof(mux *http.ServeMux, cfg PprofConfig) {
	if cfg.Prefix == "" {
		cfg.Prefix = "/debug/pprof"
	}

	// For direct registration, we might need to wrap individual handlers with auth
	// if auth is enabled. However, RegisterPprof typically assumes direct access.
	// If auth is needed, PprofHandler() with a sub-path mount is often better,
	// but here we will register individual endpoints.

	// If auth is enabled, we need to wrap the handlers
	if cfg.EnableAuth && cfg.Username != "" && cfg.Password != "" {
		// Helper to wrap and register
		handle := func(pattern string, h http.HandlerFunc) {
			mux.Handle(pattern, pprofBasicAuth(cfg.Username, cfg.Password, http.HandlerFunc(h)))
		}

		handle(cfg.Prefix+"/", pprof.Index)
		handle(cfg.Prefix+"/cmdline", pprof.Cmdline)
		handle(cfg.Prefix+"/profile", pprof.Profile)
		handle(cfg.Prefix+"/symbol", pprof.Symbol)
		handle(cfg.Prefix+"/trace", pprof.Trace)

		// Additional endpoints
		handle(cfg.Prefix+"/goroutine", pprof.Index)
		handle(cfg.Prefix+"/heap", pprof.Index)
		handle(cfg.Prefix+"/threadcreate", pprof.Index)
		handle(cfg.Prefix+"/block", pprof.Index)
		handle(cfg.Prefix+"/mutex", pprof.Index)
		handle(cfg.Prefix+"/allocs", pprof.Index)
	} else {
		// No auth, register directly
		registerPprofEndpoints(mux, cfg.Prefix)
	}
}

// registerPprofEndpoints registers the standard pprof endpoints on a mux with the given prefix
func registerPprofEndpoints(mux *http.ServeMux, prefix string) {
	mux.HandleFunc(prefix+"/", pprof.Index)
	mux.HandleFunc(prefix+"/cmdline", pprof.Cmdline)
	mux.HandleFunc(prefix+"/profile", pprof.Profile)
	mux.HandleFunc(prefix+"/symbol", pprof.Symbol)
	mux.HandleFunc(prefix+"/trace", pprof.Trace)

	// Additional pprof endpoints that default to Index
	mux.HandleFunc(prefix+"/goroutine", pprof.Index)
	mux.HandleFunc(prefix+"/heap", pprof.Index)
	mux.HandleFunc(prefix+"/threadcreate", pprof.Index)
	mux.HandleFunc(prefix+"/block", pprof.Index)
	mux.HandleFunc(prefix+"/mutex", pprof.Index)
	mux.HandleFunc(prefix+"/allocs", pprof.Index)
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
