# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary name
BINARY_NAME=./bin/yago
BINARY_UNIX=$(BINARY_NAME)_unix

# Main package path
MAIN_PATH=./cmd/yago

# Version information
VERSION ?= $(shell git describe --tags --always --dirty)
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT ?= $(shell git rev-parse HEAD)

# Build flags
LDFLAGS=-ldflags "-X github.com/danieleborsaro/yago/internal/buildinfo.Version=$(VERSION) -X github.com/danieleborsaro/yago/internal/buildinfo.Date=$(BUILD_DATE) -X github.com/danieleborsaro/yago/internal/buildinfo.Commit=$(GIT_COMMIT)"

.PHONY: all build build-linux build-cross release-check release-snapshot release-build clean test deps help

all: deps test build

build: ## Build the binary
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)

build-linux: ## Build the binary for Linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_UNIX) $(MAIN_PATH)

build-cross: ## Build binaries for multiple platforms
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

release-check: ## Validate the GoReleaser configuration
	goreleaser check --config .goreleaser.yaml

release-snapshot: ## Build release artifacts locally without publishing
	goreleaser release --snapshot --clean --config .goreleaser.yaml

release-build: ## Build release binaries locally without packaging or publishing
	goreleaser build --snapshot --clean --config .goreleaser.yaml

test: ## Run tests
	$(GOTEST) -v -p 1 ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f $(BINARY_NAME)-*
	rm -f coverage.out coverage.html

deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

# docker-build: ## Build Docker image
# 	docker build -t yago:$(VERSION) .

# docker-run: ## Run Docker container
# 	docker run --rm -it yago:$(VERSION) --help

# Linting targets
lint: lint-go lint-yaml ## Run all linters

lint-go: ## Run Go linters
	@echo "🔍 Running golangci-lint..."
	golangci-lint run --timeout=5m
	@echo "🔍 Running go vet..."
	$(GOCMD) vet ./...
	@echo "✅ Go linting completed"

lint-yaml: ## Run YAML linter
	@echo "🔍 Running yamllint..."
	yamllint .
	@echo "✅ YAML linting completed"

lint-fix: ## Auto-fix linting issues where possible
	@echo "🔧 Auto-fixing Go format issues..."
	gofmt -s -w .
	goimports -w .
	@echo "🔧 Running go mod tidy..."
	$(GOMOD) tidy
	@echo "✅ Auto-fix completed"

# Pre-commit targets
pre-commit-install: ## Install pre-commit hooks
	@echo "📦 Installing pre-commit..."
	pip install pre-commit
	pre-commit install
	@echo "✅ Pre-commit hooks installed"

pre-commit-run: ## Run pre-commit hooks on all files
	@echo "🔍 Running pre-commit hooks..."
	pre-commit run --all-files
	@echo "✅ Pre-commit checks completed"

pre-commit-update: ## Update pre-commit hooks
	pre-commit autoupdate

# Security scanning
security-scan: ## Run security scanner
	@echo "🔒 Running security scan..."
	golangci-lint run --default=none -E gosec --timeout=5m
	@echo "✅ Security scan completed"

format: lint-fix ## Format code (alias for lint-fix)

install: build ## Install binary to $GOPATH/bin
	cp $(BINARY_NAME) $(GOPATH)/bin/

# Show help
help: ## Display this help screen
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
