package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/rs/zerolog"

	"github.com/kroma-labs/sentinel-go/example/httpserver/internal/config"
	"github.com/kroma-labs/sentinel-go/example/httpserver/internal/handlers"
	"github.com/kroma-labs/sentinel-go/example/httpserver/internal/telemetry"
	"github.com/kroma-labs/sentinel-go/httpserver"
)

func main() {
	ctx := context.Background()

	// 1. Setup structured logger
	logger := zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger()

	// 2. Setup OpenTelemetry (Tracing + Metrics)
	shutdownTracing, shutdownMetrics, err := telemetry.Setup(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to setup OTel")
	}
	defer func() {
		shutdownTracing(ctx)
		shutdownMetrics(ctx)
	}()

	// 3. Create Router with handlers
	mux := http.NewServeMux()

	// 4. Register Prometheus Metrics Handler (using httpserver feature)
	// PRO TIP: In production, you often want metrics on a separate internal-only port
	// to prevent exposing them to the public internet. For this example, we serve
	// them on the main server for simplicity.
	mux.Handle("GET /metrics", httpserver.PrometheusHandler())

	// 5. Register Pprof Handlers for profiling
	// This enables /debug/pprof endpoints for CPU, memory, and block profiling.
	// In production, this should internally only or protected by auth.
	httpserver.RegisterPprof(mux, httpserver.DefaultPprofConfig())

	// Home route
	mux.HandleFunc("GET /home", handlers.HomeHandler())

	// Users API routes
	mux.HandleFunc("GET /api/users", handlers.ListUsersHandler())
	mux.HandleFunc("GET /api/users/{id}", handlers.GetUserHandler())
	mux.HandleFunc("POST /api/users", handlers.CreateUserHandler())
	mux.HandleFunc("PUT /api/users/{id}", handlers.UpdateUserHandler())
	mux.HandleFunc("DELETE /api/users/{id}", handlers.DeleteUserHandler())

	// Test endpoints
	mux.HandleFunc("GET /api/slow", handlers.SlowHandler())
	mux.HandleFunc("GET /api/error", handlers.ErrorHandler())
	mux.HandleFunc("GET /api/random", handlers.RandomHandler())

	// 6. Create Health Handler
	var health *httpserver.HealthHandler

	// 7. Create Sentinel HTTP Server with all features
	//
	// MIDDLEWARE USAGE:
	// There are TWO ways to apply middleware:
	//
	// Option A: Use httpserver.New() options (RECOMMENDED)
	// =====================================================
	// Pass middleware configuration via options. The server automatically
	// wraps your handler with the middleware in the correct order.
	//
	// Option B: Manual Chain() usage
	// ==============================
	// wrappedHandler := httpserver.Chain(
	//     httpserver.RequestID(),
	//     httpserver.Recovery(logger),
	//     httpserver.Logger(loggerConfig),
	// )(mux)  // <-- Note: you call it with your handler
	//
	// Then pass wrappedHandler to WithHandler()
	//
	// Below we use Option A (recommended)

	server := httpserver.New(
		// Base configuration
		httpserver.WithConfig(httpserver.ProductionConfig()),
		httpserver.WithServiceName(config.ServiceName),
		httpserver.WithLogger(logger),
		httpserver.WithHandler(mux), // Pass the raw mux, middleware is applied by options below

		// Health checks - creates health handler automatically
		httpserver.WithHealth(&health, config.ServiceVersion),

		// Middleware (applied in order: RequestID -> Recovery)
		httpserver.WithMiddleware(
			httpserver.Recovery(logger),
			httpserver.RequestID(),
		),

		// Observability middleware (applied in order: Tracing -> Metrics -> Logging -> RateLimit)
		httpserver.WithTracing(httpserver.TracingConfig{
			SkipPaths: []string{"/ping", "/livez", "/readyz", "/metrics"},
		}),
		httpserver.WithMetrics(func() httpserver.MetricsConfig {
			cfg := httpserver.DefaultMetricsConfig()
			cfg.SkipPaths = []string{"/ping", "/livez", "/readyz", "/metrics"}
			return cfg
		}()),
		httpserver.WithLogging(httpserver.LoggerConfig{
			Logger:    logger,
			SkipPaths: []string{"/ping", "/livez", "/readyz", "/metrics"},
		}),

		// Rate limiting (100 req/s with burst of 200)
		httpserver.WithRateLimit(httpserver.RateLimitConfig{
			Limit: 100,
			Burst: 200,
		}),
	)

	// 8. Register health check endpoints
	// Note: These routes are NOT automatically registered. You must add them to your mux.
	mux.Handle("GET /ping", health.PingHandler())
	mux.Handle("GET /livez", health.LiveHandler())
	mux.Handle("GET /readyz", health.ReadyHandler())

	// 9. Print startup info
	fmt.Println("✅ HTTP Server Example started!")
	fmt.Printf("🌐 Server:     http://localhost%s\n", config.Addr)
	fmt.Printf("📊 Metrics:    http://localhost%s/metrics (served on main port)\n", config.Addr)
	fmt.Printf("🔥 Pprof:      http://localhost%s/debug/pprof\n", config.Addr)
	fmt.Println("🔍 Grafana:    http://localhost:3002")
	fmt.Println("")
	fmt.Println("Available endpoints:")
	fmt.Println("  GET  /home          - Home")
	fmt.Println("  GET  /api/users     - List users")
	fmt.Println("  GET  /api/users/{id} - Get user")
	fmt.Println("  POST /api/users     - Create user")
	fmt.Println("  PUT  /api/users/{id} - Update user")
	fmt.Println("  DELETE /api/users/{id} - Delete user")
	fmt.Println("  GET  /api/slow      - Slow endpoint")
	fmt.Println("  GET  /api/error     - Error endpoint")
	fmt.Println("  GET  /api/random    - Random data")
	fmt.Println("  GET  /ping          - Ping")
	fmt.Println("  GET  /livez         - Liveness probe")
	fmt.Println("  GET  /readyz        - Readiness probe")
	fmt.Println("  GET  /metrics       - Prometheus metrics")
	fmt.Println("  GET  /debug/pprof   - Pprof profiling")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop...")

	// 10. Start server (blocks until shutdown)
	if err := server.ListenAndServe(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server error")
	}
}
