// Package httpserver provides a production-ready HTTP server with graceful shutdown,
// observability, and middleware support.
//
// # Quick Start
//
// Create a server with your handler and start it:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/", homeHandler)
//
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHandler(mux),
//	)
//
//	if err := server.ListenAndServe(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # ServiceName Integration
//
// The server's ServiceName is automatically propagated to all observability
// components. Set it once and it appears everywhere:
//
//	server := httpserver.New(
//	    httpserver.WithServiceName("payment-api"),  // Set once
//	    httpserver.WithTracing(httpserver.TracingConfig{}),    // Gets ServiceName
//	    httpserver.WithMetrics(httpserver.MetricsConfig{}),    // Gets ServiceName
//	    httpserver.WithLogging(httpserver.LoggerConfig{...}),  // Gets ServiceName
//	    httpserver.WithHealth(&health, "1.0.0"),               // Gets ServiceName
//	    httpserver.WithHandler(mux),
//	)
//
// # Configuration
//
// Use preset configurations as a starting point:
//
//	// For production with hardened timeouts
//	server := httpserver.New(
//	    httpserver.WithConfig(httpserver.ProductionConfig()),
//	    httpserver.WithServiceName("payment-api"),
//	    httpserver.WithHandler(mux),
//	)
//
// # Rate Limiting
//
// Global rate limiting (all requests):
//
//	server := httpserver.New(
//	    httpserver.WithRateLimit(httpserver.RateLimitConfig{
//	        Limit: 100,  // 100 req/sec
//	        Burst: 200,
//	    }),
//	    httpserver.WithHandler(mux),
//	)
//
// Per-endpoint rate limiting (apply middleware to specific routes):
//
//	// Stricter limit for login
//	mux.Handle("/api/login", httpserver.RateLimitByIP(10, 20)(loginHandler))
//
//	// Different limit for API
//	mux.Handle("/api/", httpserver.RateLimit(httpserver.RateLimitConfig{
//	    Limit: 50,
//	    Burst: 100,
//	})(apiHandler))
//
// Distributed rate limiting with Redis:
//
//	rdb := redis.NewUniversalClient(&redis.UniversalOptions{
//	    Addrs: []string{"localhost:6379"},
//	})
//	mux.Handle("/api/", httpserver.RateLimitByIPRedis(rdb, 100, 200)(apiHandler))
//
// # Health Checks
//
// Register health endpoints with auto-configured ServiceName:
//
//	var health *httpserver.HealthHandler
//	server := httpserver.New(
//	    httpserver.WithServiceName("my-api"),
//	    httpserver.WithHealth(&health, "1.0.0"),
//	    httpserver.WithHandler(mux),
//	)
//
//	health.AddReadinessCheck("database", dbPingCheck)
//	health.AddReadinessCheck("redis", redisPingCheck)
//
//	mux.Handle("/ping", health.PingHandler())
//	mux.Handle("/livez", health.LiveHandler())
//	mux.Handle("/readyz", health.ReadyHandler())
//
// # Framework Adapters
//
// Use adapters for popular frameworks:
//
//	import "github.com/kroma-labs/sentinel-go/httpserver/adapters/chi"
//	import "github.com/kroma-labs/sentinel-go/httpserver/adapters/gin"
//	import "github.com/kroma-labs/sentinel-go/httpserver/adapters/echo"
//	import "github.com/kroma-labs/sentinel-go/httpserver/adapters/fiber"
//	import "github.com/kroma-labs/sentinel-go/httpserver/adapters/grpcgateway"
package httpserver
