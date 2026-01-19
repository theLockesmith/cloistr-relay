.PHONY: test test-unit test-integration test-coverage test-verbose clean

# Run all unit tests
test: test-unit

# Run unit tests (no database required)
test-unit:
	go test ./internal/...

# Run integration tests (requires relay running via docker-compose)
test-integration:
	INTEGRATION_TEST=1 go test -v ./tests/... -timeout 120s

# Run integration tests with full setup/teardown
test-integration-full:
	./scripts/run-integration-tests.sh --cleanup

# Run all tests with verbose output
test-verbose:
	go test -v ./internal/... ./tests/...

# Run tests with coverage report
test-coverage:
	go test -cover ./internal/...

# Generate detailed coverage report
test-coverage-html:
	go test -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run specific package tests
test-config:
	go test -v ./internal/config

test-handlers:
	go test -v ./internal/handlers

test-auth:
	go test -v ./internal/auth

# Run tests in Docker (useful when Go is not installed locally)
docker-test:
	docker build -t coldforge-relay-test --target test .
	docker run --rm coldforge-relay-test go test ./internal/...

docker-test-verbose:
	docker build -t coldforge-relay-test --target test .
	docker run --rm coldforge-relay-test go test -v ./internal/...

docker-test-coverage:
	docker build -t coldforge-relay-test --target test .
	docker run --rm coldforge-relay-test go test -cover ./internal/...

# Clean test artifacts
clean:
	rm -f coverage.out coverage.html

# Help target
help:
	@echo "Available targets:"
	@echo "  test                 - Run all unit tests"
	@echo "  test-unit            - Run unit tests only"
	@echo "  test-integration     - Run integration tests (requires relay running)"
	@echo "  test-integration-full - Run integration tests with setup/teardown"
	@echo "  test-verbose         - Run all tests with verbose output"
	@echo "  test-coverage        - Run tests with coverage report"
	@echo "  test-coverage-html   - Generate HTML coverage report"
	@echo "  test-config          - Run config package tests"
	@echo "  test-handlers        - Run handlers package tests"
	@echo "  test-auth            - Run auth package tests"
	@echo "  docker-test          - Run tests in Docker container"
	@echo "  docker-test-verbose  - Run tests in Docker with verbose output"
	@echo "  docker-test-coverage - Run tests in Docker with coverage"
	@echo "  clean                - Remove test artifacts"
