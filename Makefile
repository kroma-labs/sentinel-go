.PHONY: dep install test

default: test

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dep: ## Update dependencies
	@go mod tidy

install: ## Install development tools
	@go install github.com/vektra/mockery/v3@v3.5.0
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0

clean: ## Clean build artifacts
	@test ! -e bin/${BIN_NAME} || rm bin/${BIN_NAME}
	@go clean ./...

test: ## Run tests with coverage
	@go test -race -v \
		-coverpkg=$$(go list ./... | grep -v -E '/(mocks|cmd)($$|/)' | tr '\n' ',' | sed 's/,$$//') \
		-coverprofile=cover.out ./...
	@go tool cover -func=cover.out

coverage-html: ## Generate test coverage HTML report
	@go test -race -v \
		-coverpkg=$$(go list ./... | grep -v -E '/(mocks|cmd)($$|/)' | tr '\n' ',' | sed 's/,$$//') \
		-coverprofile=cover.out ./...
	@go tool cover -html=cover.out && rm -rf cover.out

bench: ## Run benchmarks
	# -run=^B negates all tests
	@go test -bench=. -run=^B -benchtime 10s -benchmem ./...

lint: install ## Run linters and format code
	@golangci-lint fmt
	@golangci-lint run --fix

mock: install ## Generate mocks for interfaces
	@mockery --config .mockery.yaml
