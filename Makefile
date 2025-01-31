# Go related variables
BINARY_NAME := tusd
GO := go
GOARCH := amd64
# Detect the operating system for local development and testing
ifeq ($(OS),Windows_NT)
    GOOS := windows
else
    UNAME_S := $(shell uname -s)
    ifeq ($(UNAME_S),Linux)
        GOOS := linux
    endif
    ifeq ($(UNAME_S),Darwin)
        GOOS := darwin
    endif
endif

# For container builds, override to linux
CONTAINER_GOOS := linux

CGO_ENABLED := 0
GO_FILES := $(shell find . -name '*.go' -not -path "./vendor/*")
VERSION := $(shell git describe --tags --always --dirty)
COMMIT_HASH := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Docker related variables
DOCKER_REGISTRY := docker.io
DOCKER_IMAGE_NAME := resumable-upload-service
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(DOCKER_IMAGE_NAME)
DOCKER_TAG := $(VERSION)
DOCKERFILE := Dockerfile

# Helm/K8s related variables
CHART_NAME := resumable-upload-service
CHART_PATH := charts/$(CHART_NAME)
HELM_VALUES := $(CHART_PATH)/values.yaml
NAMESPACE := default
RELEASE_NAME := resumable-upload

# Tool binaries
HELM := helm
KUBECTL := kubectl
YAMLLINT := yamllint
KUBEVAL := kubeval
GOLANGCI_LINT := golangci-lint

# Build flags
LDFLAGS := -X main.version=$(VERSION) \
           -X main.commitHash=$(COMMIT_HASH) \
           -X main.buildTime=$(BUILD_TIME) \
           -w -s

.PHONY: all
all: help

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Go development targets
.PHONY: build
build: ## Build the Go binary
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

.PHONY: build-container
build-container: ## Build for container (linux)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(CONTAINER_GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

.PHONY: build-linux
build-linux: ## Build for Linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/$(BINARY_NAME)

.PHONY: build-darwin
build-darwin: ## Build for macOS
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/$(BINARY_NAME)

.PHONY: build-windows
build-windows: ## Build for Windows
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/$(BINARY_NAME)

.PHONY: build-all
build-all: build-linux build-darwin build-windows ## Build for all platforms

.PHONY: run
run: ## Run the application locally
	$(GO) run ./cmd/$(BINARY_NAME)

.PHONY: test
test: build ## Run tests
	$(eval TUSD_BINARY := $(PWD)/bin/$(BINARY_NAME))
	TUSD_BINARY=$(TUSD_BINARY) $(GO) test -v -race ./...

.PHONY: test-coverage
test-coverage: build ## Run tests with coverage
	$(eval TUSD_BINARY := $(PWD)/bin/$(BINARY_NAME))
	TUSD_BINARY=$(TUSD_BINARY) $(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: lint-go
lint-go: ## Run golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: fmt
fmt: ## Format Go code
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: clean-go
clean-go: ## Clean Go build files
	rm -rf bin/
	rm -f coverage.out coverage.html

.PHONY: deps
deps: ## Download Go dependencies
	$(GO) mod download
	$(GO) mod verify

.PHONY: tidy
tidy: ## Tidy Go modules
	$(GO) mod tidy

.PHONY: vendor
vendor: ## Vendor Go dependencies
	$(GO) mod vendor

# Docker targets
.PHONY: docker-build
docker-build: build-container ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_HASH=$(COMMIT_HASH) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-f $(DOCKERFILE) .

.PHONY: docker-run
docker-run: ## Run Docker container locally
	docker run -p 8080:8080 $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-clean
docker-clean: ## Clean Docker images
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) || true

# Tool installation and verification targets
.PHONY: install-tools
install-tools: ## Install required tools
	@mkdir -p scripts
	@chmod +x scripts/install-tools.sh
	@./scripts/install-tools.sh

.PHONY: check-helm
check-helm: ## Check if helm is installed
	@which $(HELM) >/dev/null || (echo "helm is required but not installed. Run 'make install-tools' to install" && exit 1)

.PHONY: check-kubectl
check-kubectl: ## Check if kubectl is installed
	@which $(KUBECTL) >/dev/null || (echo "kubectl is required but not installed. Run 'make install-tools' to install" && exit 1)

.PHONY: check-yamllint
check-yamllint: ## Check if yamllint is installed
	@which $(YAMLLINT) >/dev/null || (echo "yamllint is required but not installed. Run 'make install-tools' to install" && exit 1)

.PHONY: check-kubeval
check-kubeval: ## Check if kubeval is installed
	@which $(KUBEVAL) >/dev/null || (echo "kubeval is required but not installed. Run 'make install-tools' to install" && exit 1)

.PHONY: check-golangci-lint
check-golangci-lint: ## Check if golangci-lint is installed
	@which $(GOLANGCI_LINT) >/dev/null || (echo "golangci-lint is required but not installed. Run 'make install-tools' to install" && exit 1)

.PHONY: check-required-tools
check-required-tools: check-helm check-kubectl check-yamllint check-kubeval check-golangci-lint ## Check if required tools are installed

# Helm chart validation targets
.PHONY: lint-chart
lint-chart: check-helm ## Lint the Helm chart
	$(HELM) lint $(CHART_PATH)

.PHONY: lint-yaml
lint-yaml: check-yamllint ## Lint all YAML files in the chart
	$(YAMLLINT) $(CHART_PATH)

.PHONY: validate-templates
validate-templates: check-helm ## Validate Helm templates
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) --debug

.PHONY: validate-manifests
validate-manifests: check-helm check-kubeval ## Validate generated Kubernetes manifests against schemas
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) | $(KUBEVAL) --strict

# Testing targets for different configurations
.PHONY: test-s3
test-s3: check-helm ## Test chart with S3 configuration
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) \
		--set tusd.storage.type=s3 \
		--set tusd.storage.s3.enabled=true \
		--set tusd.storage.s3.bucket=test-bucket \
		--set tusd.storage.s3.accessKeyId=test-key \
		--set tusd.storage.s3.secretAccessKey=test-secret \
		--set tusd.storage.s3.region=us-west-2

.PHONY: test-azure
test-azure: check-helm ## Test chart with Azure configuration
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) \
		--set tusd.storage.type=azure \
		--set tusd.storage.azure.enabled=true \
		--set tusd.storage.azure.container=test-container \
		--set tusd.storage.azure.storageAccount=test-account \
		--set tusd.storage.azure.storageKey=test-key

.PHONY: test-metrics
test-metrics: check-helm ## Test chart with metrics enabled
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) \
		--set tusd.monitoring.metrics.enabled=true \
		--set service.type=ClusterIP

.PHONY: test-ingress
test-ingress: check-helm ## Test chart with ingress enabled
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) \
		--set ingress.enabled=true \
		--set ingress.hosts[0].host=example.com \
		--set ingress.hosts[0].paths[0].path=/

# Validation and deployment targets
.PHONY: validate-all
validate-all: check-required-tools lint-chart lint-yaml validate-templates validate-manifests test-s3 test-azure test-metrics test-ingress ## Run all validation tests

.PHONY: quick-validate
quick-validate: check-required-tools ## Run basic validations (faster)
	@mkdir -p scripts
	@chmod +x scripts/validate.sh
	@./scripts/validate.sh $(CHART_PATH) $(RELEASE_NAME) $(NAMESPACE) --quick

.PHONY: dry-run
dry-run: check-helm ## Perform a dry-run installation
	$(HELM) install $(RELEASE_NAME) $(CHART_PATH) \
		--dry-run \
		--debug \
		--namespace $(NAMESPACE)

.PHONY: template-all
template-all: check-helm ## Generate all templates with default values
	@mkdir -p generated-manifests
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) > generated-manifests/all.yaml

# Cleanup and maintenance targets
.PHONY: clean
clean: clean-go docker-clean ## Clean up generated files
	rm -rf generated-manifests/

# Value file management
.PHONY: create-example-values
create-example-values: ## Create example values files for different environments
	@mkdir -p examples
	@echo "# Development environment values" > examples/values-dev.yaml
	@echo "tusd:" >> examples/values-dev.yaml
	@echo "  storage:" >> examples/values-dev.yaml
	@echo "    type: s3" >> examples/values-dev.yaml
	@echo "    s3:" >> examples/values-dev.yaml
	@echo "      enabled: true" >> examples/values-dev.yaml
	@echo "      bucket: dev-bucket" >> examples/values-dev.yaml
	@echo "      region: us-west-2" >> examples/values-dev.yaml
	@echo "# Production environment values" > examples/values-prod.yaml
	@echo "tusd:" >> examples/values-prod.yaml
	@echo "  storage:" >> examples/values-prod.yaml
	@echo "    type: s3" >> examples/values-prod.yaml
	@echo "    s3:" >> examples/values-prod.yaml
	@echo "      enabled: true" >> examples/values-prod.yaml
	@echo "      bucket: prod-bucket" >> examples/values-prod.yaml
	@echo "      region: us-west-2" >> examples/values-prod.yaml

# Development workflow targets
.PHONY: dev
dev: deps build ## Setup development environment

.PHONY: ci
ci: deps lint-go test validate-all ## Run CI pipeline locally

.PHONY: release
release: ci docker-build ## Build and release

.PHONY: build-and-deploy
build-and-deploy: build docker-build ## Build and deploy to Kubernetes

.PHONY: precommit
precommit: fmt build-all test tidy validate-templates validate-manifests ## precommit hook
