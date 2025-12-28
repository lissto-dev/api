# Image URL to use all building/pushing image targets
IMG ?= lissto-api:latest
BUILD_DIR=build
DIST_DIR=dist
CONFIG_DIR=../controller/config/default-config.yaml
BINARY_NAME=lissto-api

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test $$(go list ./...) -coverprofile cover.out

.PHONY: coverage
coverage: fmt vet ## Run tests with coverage report.
	go test -v -coverprofile=coverage.txt ./...
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: fmt vet ## Build API binary.
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v ./cmd/server

.PHONY: build-linux
build-linux: fmt vet ## Build API binary for Linux.
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)_linux -v ./cmd/server

.PHONY: run
run: fmt vet ## Run the API from your host.
	go run ./cmd/server/main.go --config-path $(CONFIG_DIR)

.PHONY: dev
dev: ## Run in development mode with live reload (requires air)
	@echo "Starting development server..."
	@if command -v air > /dev/null; then \
		air -build.cmd 'make build' -build.bin $(BUILD_DIR)/$(BINARY_NAME) -build.args_bin '--config-path $(CONFIG_DIR) $(ARGS)' ; \
	else \
		echo "Air not found. Install with: go install github.com/air-verse/air@latest"; \
		echo "Running without live reload..."; \
		make run; \
	fi

.PHONY: docker-build
docker-build: ## Build docker image with the API.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the API.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-run
docker-run: ## Run docker container with the API.
	$(CONTAINER_TOOL) run -p 8080:8080 --env-file .env ${IMG}

##@ Cleanup

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	go clean
	@rm -rf $(BUILD_DIR)
	@rm -rf $(DIST_DIR)
	@rm -f coverage.txt coverage.html cover.out

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
GOLANGCI_LINT_VERSION ?= v2.4.0

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $$(realpath $(1)-$(3)) $(1)
endef

##@ Setup

.PHONY: deps
deps: ## Install dependencies
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

.PHONY: setup-keys
setup-keys: ## Create API keys example file
	@echo "Creating API keys example file..."
	@if [ ! -f api-keys.yaml ]; then \
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

##@ Security

.PHONY: security
security: gosec ## Run security scan
	$(GOSEC) ./...

## Tool Binaries for Security
GOSEC = $(LOCALBIN)/gosec

## Tool Versions for Security
GOSEC_VERSION ?= v2.22.4

.PHONY: gosec
gosec: $(GOSEC) ## Download gosec locally if necessary.
$(GOSEC): $(LOCALBIN)
	$(call go-install-tool,$(GOSEC),github.com/securego/gosec/v2/cmd/gosec,$(GOSEC_VERSION))

##@ Mocks

.PHONY: mocks
mocks: mockgen ## Generate mocks
	@echo "Generating mocks..."
	$(MOCKGEN) -source=internal/k8s/client.go -destination=internal/k8s/mocks/client_mock.go

## Tool Binaries for Mocks
MOCKGEN = $(LOCALBIN)/mockgen

## Tool Versions for Mocks
MOCKGEN_VERSION ?= v0.5.2

.PHONY: mockgen
mockgen: $(MOCKGEN) ## Download mockgen locally if necessary.
$(MOCKGEN): $(LOCALBIN)
	$(call go-install-tool,$(MOCKGEN),go.uber.org/mock/mockgen,$(MOCKGEN_VERSION))
