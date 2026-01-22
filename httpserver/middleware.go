package httpserver

import "net/http"

// Middleware is a function that wraps an http.Handler.
//
// Middleware functions are composed together using Chain() to create
// a processing pipeline for HTTP requests.
//
// Example:
//
//	func LoggingMiddleware(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        log.Printf("Request: %s %s", r.Method, r.URL.Path)
//	        next.ServeHTTP(w, r)
//	    })
//	}
type Middleware func(http.Handler) http.Handler

// Chain composes multiple middleware into a single middleware.
//
// Middleware are applied in the order provided. The first middleware
// is the outermost (runs first on request, last on response).
//
// Example:
//
//	handler := httpserver.Chain(
//	    httpserver.Tracing(tp),
//	    httpserver.Recovery(),
//	    httpserver.Logger(logger),
//	)(myHandler)
//
// Request flow:
//
//	Tracing -> Recovery -> Logger -> myHandler -> Logger -> Recovery -> Tracing
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		// Apply in reverse order so first middleware is outermost
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// DefaultMiddleware returns a production-ready middleware stack.
//
// The stack includes (in order):
//  1. Recovery - Panic recovery
//  2. RequestID - X-Request-ID generation/forwarding
//  3. Logger - Request/response logging (if logger provided)
//
// For full observability, add Tracing() middleware manually with your
// TracerProvider.
//
// Example:
//
//	handler := httpserver.Tracing(tp)(
//	    httpserver.DefaultMiddleware(logger)(myHandler),
//	)
func DefaultMiddleware(opts ...MiddlewareOption) Middleware {
	cfg := &middlewareConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var middlewares []Middleware

	if cfg.logger != nil {
		middlewares = append(middlewares,
			Recovery(cfg.logger.Logger),
			RequestID(),
			Logger(*cfg.logger),
		)
	} else {
		middlewares = append(middlewares,
			RequestID(),
		)
	}

	return Chain(middlewares...)
}

// middlewareConfig holds options for DefaultMiddleware.
type middlewareConfig struct {
	logger *LoggerConfig
}

// MiddlewareOption configures DefaultMiddleware.
type MiddlewareOption func(*middlewareConfig)

// WithDefaultLogger adds logging middleware to DefaultMiddleware.
func WithDefaultLogger(cfg LoggerConfig) MiddlewareOption {
	return func(c *middlewareConfig) {
		c.logger = &cfg
	}
}
