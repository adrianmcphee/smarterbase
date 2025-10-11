# SmarterBase Makefile
# Quick commands for common development tasks

.PHONY: help test test-race test-coverage lint fmt build clean install-hooks run-example

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Testing
test: ## Run all tests
	go test -v -short ./...

test-race: ## Run tests with race detector
	go test -v -race -short ./...

test-coverage: ## Run tests with coverage report
	go test -v -short -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

test-coverage-html: ## Generate HTML coverage report
	go test -v -short -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Code quality
lint: ## Run linter (requires golangci-lint)
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run --config=.golangci.yml; \
	else \
		echo "golangci-lint not found. Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

fmt: ## Format code
	go fmt ./...
	goimports -w .

vet: ## Run go vet
	go vet ./...

# Building
build: ## Build all packages
	go build -v ./...

build-examples: ## Build all examples
	@for dir in examples/*/; do \
		if [ -f "$$dir/main.go" ]; then \
			echo "Building example: $$(basename $$dir)"; \
			(cd "$$dir" && go build -v .); \
		fi \
	done

# Cleanup
clean: ## Clean build artifacts and test data
	go clean ./...
	rm -f coverage*.out
	rm -rf ./data
	find examples -name "data" -type d -exec rm -rf {} +

# Dependencies
deps: ## Download dependencies
	go mod download

tidy: ## Tidy dependencies
	go mod tidy

# Git hooks
install-hooks: ## Install git hooks
	./scripts/install-hooks.sh

# Examples
run-example: ## Run example (usage: make run-example EXAMPLE=user-management)
	@if [ -z "$(EXAMPLE)" ]; then \
		echo "Error: EXAMPLE not specified"; \
		echo "Usage: make run-example EXAMPLE=user-management"; \
		echo "Available examples:"; \
		ls -1 examples/; \
		exit 1; \
	fi
	@if [ ! -d "examples/$(EXAMPLE)" ]; then \
		echo "Error: Example '$(EXAMPLE)' not found"; \
		echo "Available examples:"; \
		ls -1 examples/; \
		exit 1; \
	fi
	cd examples/$(EXAMPLE) && go run main.go

# Documentation
godoc: ## Start godoc server
	@echo "Starting godoc server at http://localhost:6060"
	@echo "View docs at: http://localhost:6060/pkg/github.com/adrianmcphee/smarterbase/"
	godoc -http=:6060

# CI simulation
ci: deps tidy fmt vet lint test-race build build-examples ## Run all CI checks locally

# Quick development cycle
dev: fmt vet test ## Quick dev cycle: format, vet, test

# Release (for maintainers)
release-check: ## Check if ready for release
	@echo "Checking release readiness..."
	@echo "1. Running tests..."
	@make test-race
	@echo "2. Checking coverage..."
	@make test-coverage
	@echo "3. Running linter..."
	@make lint
	@echo "4. Building all packages..."
	@make build build-examples
	@echo ""
	@echo "✅ All checks passed! Ready for release."
	@echo ""
	@echo "Next steps:"
	@echo "  1. Update version in relevant files"
	@echo "  2. Commit with conventional commit message (feat:/fix:)"
	@echo "  3. Push to main branch"
	@echo "  4. Release workflow will automatically create GitHub release"

# Help for new contributors
onboard: ## Setup for new contributors
	@echo "🚀 Welcome to SmarterBase!"
	@echo ""
	@echo "Setting up your development environment..."
	@echo ""
	@echo "1. Installing git hooks..."
	@make install-hooks
	@echo ""
	@echo "2. Downloading dependencies..."
	@make deps
	@echo ""
	@echo "3. Running tests..."
	@make test
	@echo ""
	@echo "✅ Setup complete!"
	@echo ""
	@echo "Quick commands:"
	@echo "  make test          - Run tests"
	@echo "  make dev           - Format, vet, and test"
	@echo "  make lint          - Run linter"
	@echo "  make run-example   - Run an example"
	@echo ""
	@echo "For more commands, run: make help"

.DEFAULT_GOAL := help
