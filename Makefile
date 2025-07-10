test: ## Run unit tests
	@go test -cover -timeout=30m ./...
.PHONY: test

test-short: ## Run unit tests in short mode
	@go test -cover -short -timeout=5m ./...
.PHONY: test-short

# Runs linters without modifying files
lint:
	golangci-lint run

# Runs linters and attempts to automatically fix issues
lint-fix:
	golangci-lint run --fix
.PHONY: lint lint-fix