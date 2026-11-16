# All tooling runs in Docker; nothing is installed on the host.
GO_IMAGE   ?= golang:1.27
LINT_IMAGE ?= golangci/golangci-lint:v2.15.0
WORKDIR    := /src
CACHE_VOL  := kerneltuner-gocache

VERSION    ?= $(shell git describe --tags --dirty 2>/dev/null || (s=$$(git rev-parse --short HEAD 2>/dev/null) && echo "unreleased-$$s") || echo unreleased)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG        := github.com/mirastacklabs-ai/kerneltuner/internal/cli
LDFLAGS    := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).GitCommit=$(COMMIT) -X $(PKG).BuildDate=$(DATE)

DOCKER_RUN := docker run --rm -v $(CURDIR):$(WORKDIR) -v $(CACHE_VOL):/go -w $(WORKDIR) -e GOCACHE=/go/cache
GO_RUN     := $(DOCKER_RUN) $(GO_IMAGE)

.PHONY: help build test lint fmt tidy

help: ## Show targets
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/'

build: ## Build static linux binary to bin/kerneltuner
	$(GO_RUN) env CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags '$(LDFLAGS)' -o bin/kerneltuner ./cmd/kerneltuner

test: ## Run unit tests with the race detector
	$(DOCKER_RUN) -e CGO_ENABLED=1 $(GO_IMAGE) go test -race ./...

lint: ## Run golangci-lint
	$(DOCKER_RUN) -e GOLANGCI_LINT_CACHE=/go/lint-cache $(LINT_IMAGE) golangci-lint run ./...

fmt: ## Format code
	$(GO_RUN) gofmt -s -w .

tidy: ## Tidy go.mod/go.sum
	$(GO_RUN) go mod tidy
