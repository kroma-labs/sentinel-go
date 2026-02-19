.PHONY: help run docker-up docker-down docker-logs test load-test

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Build and run the example server
	@echo "Starting HTTP server example..."
	@go run cmd/server/main.go

docker-up: ## Start all Docker services (Tempo, Prometheus, Grafana)
	@echo "Starting Docker services..."
	@docker-compose -f deployments/docker-compose.yaml up -d
	@echo "✅ Services started. Waiting for initialization..."
	@sleep 5
	@docker-compose -f deployments/docker-compose.yaml ps
	@echo ""
	@echo "📊 Grafana:    http://localhost:3002"
	@echo "📈 Prometheus: http://localhost:9057"
	@echo "🔍 Tempo:      http://localhost:3202"

docker-down: ## Stop and remove all Docker services
	@echo "Stopping Docker services..."
	@docker-compose -f deployments/docker-compose.yaml down
	@echo "✅ Services stopped"

docker-logs: ## Show logs from all Docker services
	@docker-compose -f deployments/docker-compose.yaml logs -f

docker-restart: docker-down docker-up ## Restart all Docker services

tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	@go mod tidy
	@echo "✅ Dependencies tidied"

load-test: ## Generate load using hey (install: go install github.com/rakyll/hey@latest)
	@echo "Generating load on the server..."
	@hey -n 1000 -c 10 http://localhost:8080/api/users
	@hey -n 500 -c 5 http://localhost:8080/api/random
	@hey -n 100 -c 2 http://localhost:8080/api/slow?delay=100ms
	@echo "Test PATCH..."
	@hey -n 200 -c 10 -m PATCH -d '{"name":"Patched"}' -T "application/json" http://localhost:8080/api/users/1
	@echo "Test Upload (10KB)..."
	@dd if=/dev/urandom bs=1024 count=10 > dummy.dat 2>/dev/null
	@hey -n 100 -c 5 -m POST -D dummy.dat http://localhost:8080/api/upload
	@rm dummy.dat
	@echo "Test Download (50KB)..."
	@hey -n 100 -c 5 http://localhost:8080/api/download?size=51200

all: tidy run ## Clean, download deps, and build
