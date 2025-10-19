# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=lirgo-api
BINARY_UNIX=$(BINARY_NAME)_unix

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Directories
CMD_DIR=cmd/server
BUILD_DIR=build
DIST_DIR=dist

.PHONY: all build clean test coverage run dev deps fmt vet lint help

# Default target
all: clean deps fmt vet test build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v ./$(CMD_DIR)

# Build for Linux
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_UNIX) -v ./$(CMD_DIR)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -rf $(DIST_DIR)
	@rm -f coverage.txt coverage.html

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.txt ./...
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run $(CMD_DIR)/main.go

# Development mode with live reload (requires air)
dev:
	@echo "Starting development server..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Air not found. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Running without live reload..."; \
		$(GOCMD) run $(CMD_DIR)/main.go; \
	fi

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Install development tools
install-tools:
	@echo "Installing development tools..."
	$(GOGET) github.com/cosmtrek/air@latest
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Create API keys example file
setup-keys:
	@echo "Creating API keys example file..."
	@if [ ! -f api-keys.yaml ]; then \
		cp api-keys.yaml api-keys.example.yaml 2>/dev/null || echo "api-keys.yaml not found"; \
		echo "api_keys:" > api-keys.example.yaml; \
		echo "  - role: admin" >> api-keys.example.yaml; \
		echo "    api_key: \"your-admin-key-here\"" >> api-keys.example.yaml; \
		echo "    name: \"Admin User\"" >> api-keys.example.yaml; \
		echo "  - role: developer" >> api-keys.example.yaml; \
		echo "    api_key: \"your-developer-key-here\"" >> api-keys.example.yaml; \
		echo "    name: \"Developer User\"" >> api-keys.example.yaml; \
		echo "  - role: user" >> api-keys.example.yaml; \
		echo "    api_key: \"your-user-key-here\"" >> api-keys.example.yaml; \
		echo "    name: \"Regular User\"" >> api-keys.example.yaml; \
		echo "Created api-keys.example.yaml"; \
	fi

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .

# Docker run
docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 --env-file .env $(BINARY_NAME):$(VERSION)

# Generate mocks (requires mockgen)
mocks:
	@echo "Generating mocks..."
	@if command -v mockgen > /dev/null; then \
		mockgen -source=internal/k8s/client.go -destination=internal/k8s/mocks/client_mock.go; \
	else \
		echo "mockgen not found. Install with: go install github.com/golang/mock/mockgen@latest"; \
	fi

# Security scan (requires gosec)
security:
	@echo "Running security scan..."
	@if command -v gosec > /dev/null; then \
		gosec ./...; \
	else \
		echo "gosec not found. Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi

# Help
help:
	@echo "Available commands:"
	@echo "  build          - Build the application"
	@echo "  build-linux    - Build for Linux"
	@echo "  clean          - Clean build artifacts"
	@echo "  test           - Run tests"
	@echo "  coverage       - Run tests with coverage"
	@echo "  run            - Run the application"
	@echo "  dev            - Run in development mode with live reload"
	@echo "  deps           - Install dependencies"
	@echo "  fmt            - Format code"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run linter"
	@echo "  install-tools  - Install development tools"
	@echo "  setup-keys     - Create API keys example file"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Run Docker container"
	@echo "  mocks          - Generate mocks"
	@echo "  security       - Run security scan"
	@echo "  help           - Show this help"
