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

	"github.com/kroma-labs/sentinel-go/example/sqlx/internal/config"
	"github.com/kroma-labs/sentinel-go/example/sqlx/internal/database"
	"github.com/kroma-labs/sentinel-go/example/sqlx/internal/telemetry"

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

	// 3. Open Database Connection with Sentinel SQLX
	db, err := database.New(ctx)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 4. Perform Database Operations in a Loop
	// This generates continuous metrics for demonstration
	tracer := otel.Tracer("example-app")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Initial setup
	if err := db.CreateTable(ctx); err != nil {
		log.Printf("Failed to create table: %v", err)
	}

	ticker := time.NewTicker(time.Duration(config.OperationInterval) * time.Second)
	defer ticker.Stop()

	fmt.Println("✅ SQLX Example app started!")
	fmt.Println("📊 Prometheus metrics: http://localhost:2112/metrics")
	fmt.Println("🔍 Grafana UI: http://localhost:3000")
	fmt.Println("Press Ctrl+C to stop...")

	for {
		select {
		case <-ticker.C:
			ctx, span := tracer.Start(ctx, "db-operations")

			// Insert some data
			if err := db.InsertUsers(ctx); err != nil {
				log.Printf("Failed to insert users: %v", err)
			}

			// Query data using SelectContext (sqlx feature)
			if err := db.QueryUsers(ctx); err != nil {
				log.Printf("Failed to query users: %v", err)
			}

			// Get single user using GetContext (sqlx feature)
			if _, err := db.GetUser(ctx, "Alice"); err != nil {
				log.Printf("Failed to get user: %v", err)
			}

			// Demonstrate transaction usage
			if err := db.InsertWithTransaction(ctx); err != nil {
				log.Printf("Failed transaction: %v", err)
			}

			span.End()
			log.Println("✓ Database operations completed")

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
