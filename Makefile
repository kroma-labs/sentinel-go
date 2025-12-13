.PHONY: dep install test

default: test

help:
	@echo 'Management commands:'
	@echo
	@echo 'Usage:'
	@echo '    make bench           Run benchmarks.'
	@echo '    make coverage-html   Generate test coverage report.'
	@echo '    make dep             Update dependencies.'
	@echo '    make help            Show this message.'
	@echo '    make lint            Run linters on the project.'
	@echo '    make mock            Generate mocks for interfaces.'
	@echo '    make test            Run tests.'
	@echo

dep:
	@go mod tidy

install:
	@go install github.com/vektra/mockery/v3@v3.5.0
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0

clean:
	@test ! -e bin/${BIN_NAME} || rm bin/${BIN_NAME}
	@go clean ./...

test:
	@go test -race -v \
		-coverpkg=$$(go list ./... | grep -v -E '/(mocks|cmd)($$|/)' | tr '\n' ',' | sed 's/,$$//') \
		-coverprofile=cover.out ./...
	@go tool cover -func=cover.out

coverage-html:
	@go test -race -v \
		-coverpkg=$$(go list ./... | grep -v -E '/(mocks|cmd)($$|/)' | tr '\n' ',' | sed 's/,$$//') \
		-coverprofile=cover.out ./...
	@go tool cover -html=cover.out && rm -rf cover.out

bench:
	# -run=^B negates all tests
	@go test -bench=. -run=^B -benchtime 10s -benchmem ./...

lint: install
	@golangci-lint fmt
	@golangci-lint run --fix

mock: install
	@mockery --config .mockery.yaml
