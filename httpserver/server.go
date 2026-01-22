package httpserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
)

// Server wraps http.Server with graceful shutdown, signal handling,
// and lifecycle logging.
//
// Create a Server using New():
//
//	server := httpserver.New(
//	    httpserver.WithConfig(httpserver.ProductionConfig()),
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHandler(mux),
//	)
//
//	// Blocks until shutdown signal (SIGTERM, SIGINT) or context cancellation
//	if err := server.ListenAndServe(ctx); err != nil {
//	    log.Fatal(err)
//	}
type Server struct {
	httpServer  *http.Server
	config      Config
	logger      zerolog.Logger
	serviceName string
}

// New creates a new Server with the provided options.
//
// At minimum, you must provide a handler using WithHandler().
// If no config is provided, DefaultConfig() is used.
//
// Example:
//
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHandler(mux),
//	    httpserver.WithMiddleware(
//	        httpserver.Recovery(logger),
//	        httpserver.RequestID(),
//	    ),
//	)
func New(opts ...Option) *Server {
	// Start with defaults
	cfg := DefaultConfig()

	// Apply options
	for _, opt := range opts {
		opt(&cfg)
	}

	// Set defaults for missing values
	if cfg.ServiceName == "" {
		cfg.ServiceName = "http-server"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}

	// Set up logger
	logger := cfg.Logger
	if logger.GetLevel() == zerolog.Disabled {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	// Build middleware stack, injecting ServiceName automatically
	var middlewares []Middleware

	// Add tracing if configured
	if cfg.TracingConfig != nil {
		tracingCfg := *cfg.TracingConfig
		tracingCfg.serviceName = cfg.ServiceName
		middlewares = append(middlewares, Tracing(tracingCfg))
	}

	// Add metrics if configured
	if cfg.MetricsConfig != nil {
		metricsCfg := *cfg.MetricsConfig
		metricsCfg.serviceName = cfg.ServiceName
		metrics, _ := NewMetrics(metricsCfg)
		if metrics != nil {
			middlewares = append(middlewares, metrics.Middleware())
		}
	}

	// Add logging if configured
	if cfg.LoggerConfig != nil {
		loggerCfg := *cfg.LoggerConfig
		loggerCfg.serviceName = cfg.ServiceName
		middlewares = append(middlewares, Logger(loggerCfg))
	}

	// Add global rate limiting if configured
	if cfg.RateLimitConfig != nil {
		middlewares = append(middlewares, RateLimit(*cfg.RateLimitConfig))
	}

	// Create health handler if configured
	if cfg.HealthHandler != nil {
		*cfg.HealthHandler = NewHealthHandler(
			withHealthServiceName(cfg.ServiceName),
			WithVersion(cfg.HealthVersion),
		)
	}

	// Add user-provided middleware
	middlewares = append(middlewares, cfg.Middleware...)

	// Wrap handler with middleware
	handler := cfg.Handler
	if handler != nil && len(middlewares) > 0 {
		handler = Chain(middlewares...)(handler)
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		TLSConfig:         cfg.TLSConfig,
	}

	return &Server{
		httpServer:  httpServer,
		config:      cfg,
		logger:      logger,
		serviceName: cfg.ServiceName,
	}
}

// ListenAndServe starts the server and blocks until shutdown.
//
// The server will shut down gracefully when:
//   - The provided context is cancelled
//   - SIGTERM or SIGINT is received
//
// During shutdown:
//  1. Server stops accepting new connections
//  2. Waits up to ShutdownTimeout for in-flight requests
//  3. Returns nil on clean shutdown, or error if shutdown times out
//
// Example:
//
//	ctx := context.Background()
//	if err := server.ListenAndServe(ctx); err != nil {
//	    log.Fatal(err)
//	}
func (s *Server) ListenAndServe(ctx context.Context) error {
	return s.serve(ctx, false, "", "")
}

// ListenAndServeTLS starts the server with TLS and blocks until shutdown.
//
// This is equivalent to ListenAndServe but uses the provided certificate
// and key files for TLS.
//
// Example:
//
//	ctx := context.Background()
//	if err := server.ListenAndServeTLS(ctx, "cert.pem", "key.pem"); err != nil {
//	    log.Fatal(err)
//	}
func (s *Server) ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error {
	return s.serve(ctx, true, certFile, keyFile)
}

// serve is the internal implementation for both HTTP and HTTPS.
func (s *Server) serve(ctx context.Context, useTLS bool, certFile, keyFile string) error {
	if s.config.Handler == nil {
		return errors.New("httpserver: handler is required (use WithHandler)")
	}

	// Create a channel to receive shutdown signals
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(shutdownChan)

	// Channel to receive server errors
	serverErrChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		s.logger.Info().
			Str("addr", s.httpServer.Addr).
			Bool("tls", useTLS).
			Str("service", s.serviceName).
			Msg("server starting")

		var err error
		if useTLS {
			err = s.httpServer.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		// ErrServerClosed is expected during graceful shutdown
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- err
		}
		close(serverErrChan)
	}()

	// Wait for shutdown signal, context cancellation, or server error
	select {
	case err := <-serverErrChan:
		if err != nil {
			s.logger.Error().Err(err).Msg("server error")
			return err
		}
	case sig := <-shutdownChan:
		s.logger.Info().
			Str("signal", sig.String()).
			Msg("shutdown signal received")
	case <-ctx.Done():
		s.logger.Info().
			Err(ctx.Err()).
			Msg("context cancelled, shutting down")
	}

	// Graceful shutdown
	return s.shutdown(ctx)
}

// shutdown performs graceful shutdown of the server.
func (s *Server) shutdown(ctx context.Context) error {
	s.logger.Info().
		Dur("timeout", s.config.ShutdownTimeout).
		Msg("starting graceful shutdown")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(
		ctx,
		s.config.ShutdownTimeout,
	)
	defer cancel()

	// Attempt graceful shutdown
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error().
			Err(err).
			Msg("graceful shutdown failed, forcing close")

		// Force close if graceful shutdown fails
		if closeErr := s.httpServer.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("force close failed")
		}
		return err
	}

	s.logger.Info().Msg("server stopped gracefully")
	return nil
}

// Shutdown initiates graceful shutdown of the server.
//
// This is useful when you want to trigger shutdown programmatically
// instead of waiting for signals.
//
// Example:
//
//	// In another goroutine or signal handler
//	if err := server.Shutdown(ctx); err != nil {
//	    log.Printf("shutdown error: %v", err)
//	}
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server's listen address.
//
// This is useful when using ":0" to let the OS pick a random port.
// Note: This returns the configured address, not the actual bound address.
// For the actual address after binding, check the listener.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// ServiceName returns the configured service name.
func (s *Server) ServiceName() string {
	return s.serviceName
}
