# SmarterBase Makefile
# PostgreSQL-compatible file store

.PHONY: help test build clean run

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Building
build: ## Build the smarterbase binary
	go build -o bin/smarterbase ./cmd/smarterbase

# Running
run: build ## Build and run smarterbase
	./bin/smarterbase --port 5433 --data ./data

# Testing
test: ## Run all tests
	go test -v ./...

test-race: ## Run tests with race detector
	go test -v -race ./...

# Code quality
fmt: ## Format code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

# Cleanup
clean: ## Clean build artifacts
	rm -rf bin/ data/ coverage.out

# Dependencies
deps: ## Download dependencies
	go mod download

tidy: ## Tidy dependencies
	go mod tidy

# Quick development cycle
dev: fmt vet test ## Quick dev cycle: format, vet, test

.DEFAULT_GOAL := help
