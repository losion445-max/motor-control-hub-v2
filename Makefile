SHELL  := /bin/bash
.DEFAULT_GOAL := help

# ── Paths ──────────────────────────────────────────────────────────────────────
BACKEND_DIR := backend
BINARY      := $(BACKEND_DIR)/bin/server
CONFIG      ?= $(BACKEND_DIR)/config.toml

# ── Targets ───────────────────────────────────────────────────────────────────

.PHONY: help build run dev test test-v lint clean tidy

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# ── Build ──────────────────────────────────────────────────────────────────────

build:  ## Compile → backend/bin/server
	mkdir -p $(BACKEND_DIR)/bin
	cd $(BACKEND_DIR) && go build -o bin/server ./cmd/server

# ── Run ───────────────────────────────────────────────────────────────────────

run: build  ## Build and run server  [CONFIG=backend/config.toml]
	$(BINARY) -config $(CONFIG)

dev:  ## Run with race detector (no prior build needed)  [CONFIG=backend/config.toml]
	cd $(BACKEND_DIR) && go run -race ./cmd/server -config ../$(CONFIG)

# ── Test ──────────────────────────────────────────────────────────────────────

test:  ## Run all tests
	cd $(BACKEND_DIR) && go test ./... -count=1

test-v:  ## Run all tests (verbose)
	cd $(BACKEND_DIR) && go test ./... -v -count=1

test-race:  ## Run all tests with race detector
	cd $(BACKEND_DIR) && go test -race ./... -count=1

# ── Quality ───────────────────────────────────────────────────────────────────

lint:  ## Run golangci-lint (must be installed separately)
	cd $(BACKEND_DIR) && golangci-lint run ./...

tidy:  ## Tidy go.mod / go.sum
	cd $(BACKEND_DIR) && go mod tidy

# ── Cleanup ───────────────────────────────────────────────────────────────────

clean:  ## Remove compiled binary
	rm -f $(BINARY)
