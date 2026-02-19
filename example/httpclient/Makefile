.PHONY: help run docker-up docker-down docker-logs test

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Build and run the example client
	@echo "Starting HTTP client example..."
	@go run cmd/client/main.go

docker-up: ## Start all Docker services (httpbin, Tempo, Prometheus, Grafana)
	@echo "Starting Docker services..."
	@docker-compose -f deployments/docker-compose.yaml up -d
	@echo "✅ Services started. Waiting for initialization..."
	@sleep 5
	@docker-compose -f deployments/docker-compose.yaml ps
	@echo ""
	@echo "📊 Grafana:    http://localhost:3001"
	@echo "📈 Prometheus: http://localhost:9056"
	@echo "🔍 Tempo:      http://localhost:3201"
	@echo "🌐 httpbin:    http://localhost:8080"

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

all: tidy run ## Clean, download deps, and build
