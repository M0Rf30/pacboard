# SPDX-FileCopyrightText: The pacboard Authors
#
# SPDX-License-Identifier: AGPL-3.0-or-later

.PHONY: help web-build web-dev build run-dev test vet lint fmt check deps clean docker-build

BINARY_NAME = pacboard
MAIN_PATH   = ./cmd/pacboard
GO          = go
DOCKER_IMAGE ?= pacboard:latest

# Build metadata injected via -ldflags (main.version, main.commit, main.buildTime).
VERSION    ?= $(if $(CI_VERSION),$(CI_VERSION),$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
COMMIT     ?= $(if $(CI_COMMIT),$(CI_COMMIT),$(shell git rev-parse --short=7 HEAD 2>/dev/null || echo unknown))
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     = -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Kubernetes context and namespaces for run-dev.
KUBE_CONTEXT ?=
NAMESPACES   ?= tekton-ci

help:
	@echo "Available targets:"
	@echo "  web-build    - Build the web UI into web/dist (embedded by the binary)"
	@echo "  web-dev      - Start the Vite dev server (proxies /api and /auth to :8080)"
	@echo "  build        - Build bin/pacboard (includes the web UI)"
	@echo "  run-dev      - Run the backend against your kubeconfig with --auth-mode none (no login, no reruns)"
	@echo "  test         - Run Go tests with the race detector"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run golangci-lint"
	@echo "  fmt          - Format code (golangci-lint fmt)"
	@echo "  check        - fmt + vet + lint"
	@echo "  deps         - Download and tidy dependencies"
	@echo "  docker-build - Build the container image ($(DOCKER_IMAGE))"
	@echo "  clean        - Remove build artifacts"

web-build:
	cd web && bun install --frozen-lockfile && bun run build

web-dev:
	cd web && bun run dev

build: web-build
	CGO_ENABLED=0 $(GO) build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: bin/$(BINARY_NAME)"

# Reads the cluster through your kubeconfig (a read-only user is enough). With --auth-mode none
# nobody is logged in; export GITHUB_TOKEN and add --dev-github-token-env GITHUB_TOKEN to try reruns.
run-dev:
	$(GO) run $(MAIN_PATH) --auth-mode none --namespaces $(NAMESPACES) \
		$(if $(KUBE_CONTEXT),--kube-context $(KUBE_CONTEXT)) --listen 127.0.0.1:8080

test:
	$(GO) test -race -timeout 5m ./...

vet:
	$(GO) vet ./...

lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0" && exit 1)
	golangci-lint run ./...

fmt:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0" && exit 1)
	golangci-lint fmt

check: fmt vet lint
	@echo "All checks passed"

deps:
	$(GO) mod download
	$(GO) mod tidy

docker-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE) -f docker/Dockerfile .

clean:
	rm -rf bin/
	$(GO) clean

.DEFAULT_GOAL := help
