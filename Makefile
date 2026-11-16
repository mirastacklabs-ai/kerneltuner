# All tooling runs in Docker; nothing is installed on the host.
GO_IMAGE   ?= golang:1.27
LINT_IMAGE ?= golangci/golangci-lint:v2.15.0
GORELEASER_IMAGE ?= goreleaser/goreleaser:v2.20.2
WORKDIR    := /src
CACHE_VOL  := kerneltuner-gocache

VERSION    ?= $(shell git describe --tags --dirty 2>/dev/null || (s=$$(git rev-parse --short HEAD 2>/dev/null) && echo "unreleased-$$s") || echo unreleased)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG        := github.com/mirastacklabs-ai/kerneltuner/internal/cli
LDFLAGS    := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).GitCommit=$(COMMIT) -X $(PKG).BuildDate=$(DATE)

DOCKER_RUN := docker run --rm -v $(CURDIR):$(WORKDIR) -v $(CACHE_VOL):/go -w $(WORKDIR) -e GOCACHE=/go/cache
GO_RUN     := $(DOCKER_RUN) $(GO_IMAGE)

.PHONY: help build test lint fmt tidy vet cover release-check release-snapshot

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

vet: ## Run go vet
	$(GO_RUN) go vet ./...

cover: ## Run tests with coverage (coverage.out)
	$(DOCKER_RUN) -e CGO_ENABLED=1 $(GO_IMAGE) go test -race -coverprofile=coverage.out ./...
	$(GO_RUN) go tool cover -func=coverage.out | tail -n 1

# GoReleaser needs git metadata; in a linked worktree .git points at the common
# git dir, so mount that at its host path too.
GIT_COMMON := $(shell git rev-parse --path-format=absolute --git-common-dir 2>/dev/null)
GR_GIT_MOUNT := $(if $(GIT_COMMON),-v $(GIT_COMMON):$(GIT_COMMON):ro)
GR_RUN := docker run --rm -v $(CURDIR):$(WORKDIR) $(GR_GIT_MOUNT) -v $(CACHE_VOL):/go -w $(WORKDIR) \
	-e GOCACHE=/go/cache -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' \
	$(GORELEASER_IMAGE)

release-check: ## Validate .goreleaser.yaml
	$(GR_RUN) check

release-snapshot: ## Build a local snapshot release into dist/ (no publish)
	$(GR_RUN) release --snapshot --clean
