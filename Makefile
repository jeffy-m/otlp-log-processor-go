.PHONY: help unit acceptance test lint lint-fix lint-tools docker-build compose-up compose-down compose-logs acceptance-compose setup

help:
	@echo "Targets:"
	@echo "  unit        - Run unit tests (no tags)"
	@echo "  lint        - Run golangci-lint checks"
	@echo "  lint-fix    - Run golangci-lint with --fix"
	@echo "  lint-tools  - Install golangci-lint v2.4.0"
	@echo "  setup       - Install dev tools (golangci-lint, lefthook)"
	@echo "  docker-build- Build the container image locally"
	@echo "  compose-up  - Start the demo stack (app + telemetry)"
	@echo "  compose-down- Stop the demo stack"
	@echo "  compose-logs- Tail app logs"
	@echo "  test        - Alias for unit"

unit:
	go test ./...

test: unit

lint-tools:
	@echo "Installing golangci-lint v2.4.0..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
	@echo "Done. Ensure $(go env GOPATH)/bin is on your PATH."

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found. Run: make lint-tools" >&2; exit 1; }
	golangci-lint run

lint-fix:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found. Run: make lint-tools" >&2; exit 1; }
	golangci-lint run --fix

# Install developer tools and optional git hooks manager
setup:
	@$(MAKE) lint-tools
	@command -v lefthook >/dev/null 2>&1 || { \
		echo "Installing lefthook..."; \
		go install github.com/evilmartians/lefthook@latest; \
	}
	@lefthook install
	@echo "Lefthook hooks installed (pre-commit runs 'make lint')."

docker-build:
	docker build -t otlp-log-processor:local .

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down -v

compose-logs:
	docker compose logs -f app
