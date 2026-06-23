# SPDX-FileCopyrightText: GitHub, Inc.
# SPDX-License-Identifier: MIT

GO ?= go
BINARY := seclab-taskflow-agent
PKG := ./...

.PHONY: build test test-e2e lint fmt vet tidy sync-embedded clean

build: ## Build the CLI binary into ./bin
	$(GO) build -o bin/$(BINARY) ./cmd/$(BINARY)

test: ## Run unit tests
	$(GO) test $(PKG)

test-e2e: ## Run end-to-end tests (requires a Python venv with the package)
	$(GO) test -tags e2e ./internal/e2e/...

vet: ## Run go vet
	$(GO) vet $(PKG)

fmt: ## Format the code
	$(GO) fmt $(PKG)

lint: ## Run golangci-lint if installed, else fall back to vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; running go vet instead"; \
		$(GO) vet $(PKG); \
	fi

tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

# Regenerate the embedded snapshot of the Python package's bundled toolbox and
# personality YAML files. Run after changing those source files.
sync-embedded: ## Refresh internal/loader/embedded from the Python source tree
	rm -rf internal/loader/embedded/seclab_taskflow_agent
	mkdir -p internal/loader/embedded/seclab_taskflow_agent/toolboxes \
	         internal/loader/embedded/seclab_taskflow_agent/personalities
	cp ../src/seclab_taskflow_agent/toolboxes/*.yaml \
	   internal/loader/embedded/seclab_taskflow_agent/toolboxes/
	cp ../src/seclab_taskflow_agent/personalities/*.yaml \
	   internal/loader/embedded/seclab_taskflow_agent/personalities/

clean: ## Remove build artifacts
	rm -rf bin
