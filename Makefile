.PHONY: build run dev clean install test test-unit test-integration coverage lint security docker-up docker-down docs help

# Colors for output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(BLUE)Kaunta Development Commands$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'

# ============================================================================
# TESTING
# ============================================================================

test: test-unit test-integration ## Run all tests (unit + integration)

test-unit: ## Run unit tests only (no database)
	@echo "$(BLUE)Running unit tests...$(NC)"
	@go test -v -race ./... --tags='!integration'

test-integration: docker-up ## Run integration tests with database
	@echo "$(BLUE)Running integration tests...$(NC)"
	@go test -v -race -tags=integration ./...

test-short: ## Run fast tests (skip long-running tests)
	@go test -v -short ./...

coverage: ## Generate test coverage report
	@echo "$(BLUE)Generating coverage report...$(NC)"
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

# ============================================================================
# BUILD & DEPLOYMENT
# ============================================================================

build: ## Build the kaunta binary
	@echo "$(BLUE)Building Kaunta...$(NC)"
	@if command -v bun >/dev/null 2>&1; then \
		echo "$(BLUE)Using Bun for frontend build...$(NC)"; \
		bun install --frozen-lockfile && bun run build; \
	elif command -v deno >/dev/null 2>&1; then \
		echo "$(BLUE)Using Deno for frontend build...$(NC)"; \
		deno run -A build.ts; \
	else \
		echo "$(YELLOW)Warning: Neither Bun nor Deno found, skipping frontend build$(NC)"; \
	fi
	@go generate ./internal/database
	@CGO_ENABLED=0 go build -v -ldflags="-w -s" -o kaunta ./cmd/kaunta
	@echo "$(GREEN)Build complete: ./kaunta$(NC)"

run: build ## Run the application
	@echo "$(BLUE)Starting Kaunta...$(NC)"
	@DATABASE_URL="postgres://postgres:postgres@localhost:5432/kaunta_test?sslmode=disable" PORT=3000 ./kaunta

dev: ## Development mode with hot reload (requires air)
	air

docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	@docker build --platform linux/amd64 -t ghcr.io/seuros/kaunta:latest -t ghcr.io/seuros/kaunta:$$(cat VERSION) .
	@echo "$(GREEN)Docker image built$(NC)"

# ============================================================================
# CODE QUALITY
# ============================================================================

lint: ## Run linters (golangci-lint)
	@echo "$(BLUE)Running linters...$(NC)"
	@golangci-lint run ./... --timeout=5m

lint-fix: ## Fix linting issues automatically
	@golangci-lint run ./... --timeout=5m --fix

security: ## Run security scanner (gosec)
	@echo "$(BLUE)Running security scan...$(NC)"
	@gosec -fmt=json -out gosec-report.json ./...
	@echo "$(GREEN)Security scan complete$(NC)"

fmt: ## Format code
	@echo "$(BLUE)Formatting code...$(NC)"
	@go fmt ./...

vet: ## Run go vet
	@echo "$(BLUE)Running go vet...$(NC)"
	@go vet ./...

# ============================================================================
# DATABASE
# ============================================================================

docker-up: ## Start PostgreSQL container for testing
	@docker-compose -f docker-compose.test.yml up -d
	@echo "$(GREEN)PostgreSQL container started$(NC)"
	@sleep 2

docker-down: ## Stop PostgreSQL container
	@docker-compose -f docker-compose.test.yml down
	@echo "$(GREEN)PostgreSQL container stopped$(NC)"

docker-logs: ## Show PostgreSQL container logs
	@docker-compose -f docker-compose.test.yml logs -f postgres

migrate-up: docker-up ## Run database migrations up
	@echo "$(BLUE)Running migrations up...$(NC)"
	@migrate -path ./internal/database/migrations -database "postgres://postgres:postgres@localhost:5432/kaunta_test?sslmode=disable" up
	@echo "$(GREEN)Migrations complete$(NC)"

migrate-down: ## Run database migrations down
	@migrate -path ./internal/database/migrations -database "postgres://postgres:postgres@localhost:5432/kaunta_test?sslmode=disable" down
	@echo "$(GREEN)Migrations reversed$(NC)"

# ============================================================================
# DEPENDENCIES & CLEANUP
# ============================================================================

install: ## Install dependencies
	@echo "$(BLUE)Installing dependencies...$(NC)"
	@go mod download
	@go mod tidy
	@echo "$(GREEN)Dependencies installed$(NC)"

tools: ## Install development tools
	@echo "$(BLUE)Installing development tools...$(NC)"
	@go install github.com/air-verse/air@latest
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "$(GREEN)Development tools installed$(NC)"

clean: ## Clean build artifacts and caches
	@echo "$(BLUE)Cleaning up...$(NC)"
	@rm -f kaunta
	@rm -f coverage*.out
	@rm -f gosec-report.json
	@go clean -testcache
	@docker-compose -f docker-compose.test.yml down -v
	@echo "$(GREEN)Clean complete$(NC)"

# ============================================================================
# CI/CD
# ============================================================================

ci: lint test coverage ## Run full CI pipeline (lint, test, coverage)
	@echo "$(GREEN)✓ CI pipeline complete$(NC)"

ci-local: docker-up ci ## Run full CI locally with database
	@echo "$(GREEN)✓ Local CI pipeline complete$(NC)"

# ============================================================================
# DOCS
# ============================================================================

docs: ## Open TESTING.md
	@open TESTING.md 2>/dev/null || cat TESTING.md

# ============================================================================
# DEFAULT
# ============================================================================

.DEFAULT_GOAL := help
