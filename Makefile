test: ## Run unit tests
	@go test -cover -timeout=30m -failfast ./...
.PHONY: test

# Runs linters without modifying files
lint:
	golangci-lint run

# Runs linters and attempts to automatically fix issues
lint-fix:
	golangci-lint run --fix
.PHONY: lint lint-fix