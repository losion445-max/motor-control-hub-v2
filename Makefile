SHELL  := /bin/bash
.DEFAULT_GOAL := help

# ── Paths ──────────────────────────────────────────────────────────────────────
BACKEND_DIR  := backend
FRONTEND_DIR := frontend
BINARY       := $(BACKEND_DIR)/bin/server
CONFIG       ?= $(BACKEND_DIR)/config.toml

# ── Hardware overrides (optional) ──────────────────────────────────────────────
# Pass on the command line to override config.toml without editing it:
#   make run PORT=/dev/ttyUSB1
#   make run PORT=/dev/ttyUSB0 BAUD=115200
PORT ?=
BAUD ?=

# ── Targets ───────────────────────────────────────────────────────────────────

.PHONY: help build run dev ui start stop install frontend-build \
        test test-v test-race lint tidy clean

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ── Helpers ───────────────────────────────────────────────────────────────────

# Writes /tmp/mch-run.toml with PORT/BAUD overrides applied, echoes the path.
# If neither PORT nor BAUD is set, echoes CONFIG as-is (no temp file created).
define resolve_config
$(shell \
  if [ -n "$(PORT)" ] || [ -n "$(BAUD)" ]; then \
    cp $(CONFIG) /tmp/mch-run.toml; \
    [ -n "$(PORT)" ] && sed -i 's|serial_port\s*=.*|serial_port = "$(PORT)"|' /tmp/mch-run.toml; \
    [ -n "$(BAUD)" ] && sed -i 's|baud_rate\s*=.*|baud_rate = $(BAUD)|' /tmp/mch-run.toml; \
    echo /tmp/mch-run.toml; \
  else \
    echo $(CONFIG); \
  fi \
)
endef

# ── Build ──────────────────────────────────────────────────────────────────────

build:  ## Compile backend → backend/bin/server
	mkdir -p $(BACKEND_DIR)/bin
	cd $(BACKEND_DIR) && go build -o bin/server ./cmd/server

frontend-build:  ## Build frontend → frontend/dist/
	cd $(FRONTEND_DIR) && npm run build

# ── Install ────────────────────────────────────────────────────────────────────

install:  ## npm install for the frontend
	cd $(FRONTEND_DIR) && npm install

# ── Run ───────────────────────────────────────────────────────────────────────

run: build  ## Build + run backend        [PORT=/dev/ttyUSBx  BAUD=19200]
	$(BINARY) -config $(call resolve_config)

dev:  ## Run backend with race detector  [PORT=/dev/ttyUSBx  BAUD=19200]
	cd $(BACKEND_DIR) && go run -race ./cmd/server -config ../$(call resolve_config)

ui:  ## Run frontend dev server → http://localhost:5173
	cd $(FRONTEND_DIR) && npm run dev

# Starts backend in background, then runs frontend dev server in foreground.
# Ctrl-C kills the frontend only. Use 'make stop' to kill the backend.
start: build  ## Start backend (bg) + frontend dev server (fg)  [PORT=…]
	@echo "→ backend  : http://localhost:8080  (ws://localhost:8080/ws)"
	@echo "→ frontend : http://localhost:5173"
	@echo "   Ctrl-C stops frontend. Run 'make stop' to kill backend."
	@$(BINARY) -config $(call resolve_config) & echo $$! > /tmp/mch-backend.pid
	cd $(FRONTEND_DIR) && npm run dev

stop:  ## Kill the backend started by 'make start'
	@if [ -f /tmp/mch-backend.pid ]; then \
		kill "$$(cat /tmp/mch-backend.pid)" 2>/dev/null && echo "backend stopped"; \
		rm /tmp/mch-backend.pid; \
	else \
		echo "no PID file — backend not running via 'make start'"; \
	fi

# ── Test ──────────────────────────────────────────────────────────────────────

test:  ## Run all backend tests
	cd $(BACKEND_DIR) && go test ./... -count=1

test-v:  ## Run all backend tests (verbose)
	cd $(BACKEND_DIR) && go test ./... -v -count=1

test-race:  ## Run all backend tests with race detector
	cd $(BACKEND_DIR) && go test -race ./... -count=1

# ── Quality ───────────────────────────────────────────────────────────────────

lint:  ## Run golangci-lint
	cd $(BACKEND_DIR) && golangci-lint run ./...

tidy:  ## Tidy go.mod / go.sum
	cd $(BACKEND_DIR) && go mod tidy

# ── Cleanup ───────────────────────────────────────────────────────────────────

clean:  ## Remove backend binary and frontend dist
	rm -f $(BINARY)
	rm -rf $(FRONTEND_DIR)/dist
