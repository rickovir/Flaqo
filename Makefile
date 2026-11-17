# Root Makefile for the Feature Flag Platform monorepo.
#
# Each application remains independently buildable. This file only
# provides convenient shortcuts.

SHELL := /bin/bash
.DEFAULT_GOAL := help

API_DIR := apps/api
DASHBOARD_DIR := apps/dashboard
SDK_DIR := packages/sdk

# Load local configuration if present. .env is gitignored.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# API (Go)
# ---------------------------------------------------------------------------

.PHONY: api-build
api-build: ## Build the Go API
	cd $(API_DIR) && go build -o bin/server ./cmd/server

.PHONY: api-run
api-run: ## Run the Go API locally
	cd $(API_DIR) && go run ./cmd/server

.PHONY: api-test
api-test: ## Run Go tests
	cd $(API_DIR) && go test ./...

.PHONY: api-test-race
api-test-race: ## Run Go tests with the race detector
	cd $(API_DIR) && go test -race ./...

.PHONY: api-vet
api-vet: ## Run go vet
	cd $(API_DIR) && go vet ./...

.PHONY: api-fmt
api-fmt: ## Format Go sources
	cd $(API_DIR) && gofmt -w $$(find . -name '*.go' -not -path './sql/generated/*')

.PHONY: api-fmt-check
api-fmt-check: ## Verify Go formatting without changing files
	cd $(API_DIR) && test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './sql/generated/*'))" \
		|| (echo "Files need formatting:" && gofmt -l $$(find . -name '*.go' -not -path './sql/generated/*') && exit 1)

.PHONY: api-tidy
api-tidy: ## Tidy Go module dependencies
	cd $(API_DIR) && go mod tidy

# ---------------------------------------------------------------------------
# Migrations
# ---------------------------------------------------------------------------

MIGRATE ?= $(shell go env GOPATH)/bin/migrate
MIGRATIONS_DIR := $(API_DIR)/migrations

.PHONY: migrate-up
migrate-up: ## Apply migrations using MIGRATION_DATABASE_URL
	@test -n "$(MIGRATION_DATABASE_URL)" || (echo "MIGRATION_DATABASE_URL is not set" && exit 1)
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATION_DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back one migration
	@test -n "$(MIGRATION_DATABASE_URL)" || (echo "MIGRATION_DATABASE_URL is not set" && exit 1)
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATION_DATABASE_URL)" down 1

.PHONY: migrate-version
migrate-version: ## Show the current migration version
	@test -n "$(MIGRATION_DATABASE_URL)" || (echo "MIGRATION_DATABASE_URL is not set" && exit 1)
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(MIGRATION_DATABASE_URL)" version

# ---------------------------------------------------------------------------
# Dashboard (Next.js)
# ---------------------------------------------------------------------------

.PHONY: dashboard-install
dashboard-install: ## Install dashboard dependencies
	cd $(DASHBOARD_DIR) && npm install

.PHONY: dashboard-build
dashboard-build: ## Build the dashboard
	cd $(DASHBOARD_DIR) && npm run build

.PHONY: dashboard-lint
dashboard-lint: ## Lint the dashboard
	cd $(DASHBOARD_DIR) && npm run lint

.PHONY: dashboard-test
dashboard-test: ## Test the dashboard
	cd $(DASHBOARD_DIR) && npm test

# ---------------------------------------------------------------------------
# SDK
# ---------------------------------------------------------------------------

.PHONY: sdk-install
sdk-install: ## Install SDK dependencies
	cd $(SDK_DIR) && npm install

.PHONY: sdk-build
sdk-build: ## Build the SDK
	cd $(SDK_DIR) && npm run build

.PHONY: sdk-test
sdk-test: ## Test the SDK
	cd $(SDK_DIR) && npm test

.PHONY: sdk-typecheck
sdk-typecheck: ## Type check the SDK
	cd $(SDK_DIR) && npm run typecheck

# ---------------------------------------------------------------------------
# Demo app
# ---------------------------------------------------------------------------

DEMO_DIR := apps/demo-app

.PHONY: demo-install
demo-install: ## Install the demo app dependencies
	cd $(DEMO_DIR) && npm install

.PHONY: demo-start
demo-start: ## Run the demo app (needs SDK_KEY)
	cd $(DEMO_DIR) && npm start

# ---------------------------------------------------------------------------
# Everything
# ---------------------------------------------------------------------------

.PHONY: test
test: api-test dashboard-test sdk-test ## Run every test suite

.PHONY: test-race
test-race: api-test-race ## Run the Go race tests

.PHONY: check
check: api-fmt-check api-vet api-test-race ## Run all static checks and race tests

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(API_DIR)/bin $(DASHBOARD_DIR)/.next $(SDK_DIR)/dist
