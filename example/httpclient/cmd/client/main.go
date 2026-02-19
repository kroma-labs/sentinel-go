package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kroma-labs/sentinel-go/example/httpclient/internal/api"
	"github.com/kroma-labs/sentinel-go/example/httpclient/internal/config"
	"github.com/kroma-labs/sentinel-go/example/httpclient/internal/telemetry"

	"go.opentelemetry.io/otel"
)

func main() {
	ctx := context.Background()

	// 1. Setup OpenTelemetry (Tracing + Metrics)
	shutdownTracing, shutdownMetrics, err := telemetry.Setup(ctx)
	if err != nil {
		log.Fatalf("Failed to setup OTel: %v", err)
	}
	defer func() {
		shutdownTracing(ctx)
		shutdownMetrics(ctx)
	}()

	// 2. Start Prometheus Metrics Server
	metricsServer := &http.Server{Addr: config.MetricsPort}
	go func() {
		log.Printf("Starting Prometheus metrics server on %s", config.MetricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server failed: %v", err)
		}
	}()

	// 3. Create API Client with all features enabled
	client := api.New()

	// 4. Perform HTTP Operations in a Loop
	// This generates continuous metrics for demonstration
	tracer := otel.Tracer("example-app")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(config.OperationInterval) * time.Second)
	defer ticker.Stop()

	fmt.Println("✅ HTTP Client Example started!")
	fmt.Println("📊 Prometheus metrics: http://localhost:2112/metrics")
	fmt.Println("🔍 Grafana UI: http://localhost:3001")
	fmt.Println("🌐 Mock API (httpbin): http://localhost:8080")
	fmt.Println("Press Ctrl+C to stop...")

	for {
		select {
		case <-ticker.C:
			ctx, span := tracer.Start(ctx, "http-client-operations")

			// Run all operations to generate metrics and traces
			if err := client.RunAllOperations(ctx); err != nil {
				log.Printf("Operations error: %v", err)
			}

			span.End()
			log.Println("✓ HTTP client operations completed")

		case <-sigChan:
			fmt.Println("\n🛑 Shutting down gracefully...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsServer.Shutdown(ctx); err != nil {
				log.Printf("Metrics server shutdown error: %v", err)
			}
			return
		}
	}
}
