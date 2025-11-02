.PHONY: help build run start dev clean test install

# Default target
.DEFAULT_GOAL := help

# Variables
PORT ?= 3000
BINARY_NAME := server
MAIN_FILE := main.go

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install Go dependencies
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

build: ## Build the server binary
	@echo "Building server..."
	go build -o $(BINARY_NAME) $(MAIN_FILE)
	@echo "Binary created: $(BINARY_NAME)"

run: build ## Build and run the server
	@echo "Starting server on port $(PORT)..."
	PORT=$(PORT) ./$(BINARY_NAME)

start: ## Start the server (builds first)
	@$(MAKE) run

dev: ## Run the server in development mode (with auto-reload via air if available)
	@if command -v air > /dev/null; then \
		echo "Starting server with air (auto-reload)..."; \
		PORT=$(PORT) air; \
	else \
		echo "Starting server (install 'air' for auto-reload: go install github.com/cosmtrek/air@latest)"; \
		PORT=$(PORT) go run $(MAIN_FILE); \
	fi

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@go clean
	@echo "Clean complete"

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

deps: install ## Alias for install

