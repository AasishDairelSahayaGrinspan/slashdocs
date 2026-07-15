SWAG := go run github.com/swaggo/swag/cmd/swag@v1.16.6
GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT_CACHE := $(CURDIR)/bin/.cache/golangci-lint
GOLANGCI_LINT_GOCACHE := $(CURDIR)/bin/.cache/go-build
JSONNET ?= jsonnet
JB ?= jb
AIDOCS_VERSION ?= dev-local-$(shell date +%Y%m%d%H%M%S)
AIDOCS_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || true)
E2E_COMPOSE_PROJECT ?= aidocs-e2e
CLI_LDFLAGS := -X github.com/razorpay/aidocs/cli/internal/cli.Version=$(AIDOCS_VERSION) -X github.com/razorpay/aidocs/cli/internal/cli.Commit=$(AIDOCS_COMMIT)
SERVER_LDFLAGS := -X github.com/razorpay/aidocs/api/internal/server.BuildVersion=$(AIDOCS_VERSION) -X github.com/razorpay/aidocs/api/internal/server.BuildCommit=$(AIDOCS_COMMIT)

.PHONY: swagger frontend backend cli cli-assets build build-frontend build-backend build-cli install-dev dashboards check-dashboards format lint bin/golangci-lint test ci e2e e2e-browser verify-production verify-production-image clean

swagger:
	$(SWAG) init \
		-g main.go \
		-d api/cmd/aidocs-server,api/internal/server,api/internal/repo \
		-o api/internal/server/docs \
		--parseDependency \
		--parseInternal

frontend:
	cd frontend && npm ci && npm run build

backend: swagger frontend cli-assets
	@bridge_source_hash="$$(node frontend/scripts/bridge-source-hash.mjs)" && \
		if [ "$${#bridge_source_hash}" -ne 64 ]; then echo "invalid bridge source hash" >&2; exit 1; fi && \
		go build -ldflags "$(SERVER_LDFLAGS) -X github.com/razorpay/aidocs/api/internal/server.BuildBridgeSourceHash=$$bridge_source_hash" -o bin/aidocs-server ./api/cmd/aidocs-server

cli:
	go build -ldflags "$(CLI_LDFLAGS)" -o bin/aidocs ./cli

cli-assets:
	mkdir -p api/internal/server/download_dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(CLI_LDFLAGS)" -o api/internal/server/download_dist/aidocs-darwin-arm64 ./cli
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(CLI_LDFLAGS)" -o api/internal/server/download_dist/aidocs-darwin-amd64 ./cli
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(CLI_LDFLAGS)" -o api/internal/server/download_dist/aidocs-linux-arm64 ./cli
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(CLI_LDFLAGS)" -o api/internal/server/download_dist/aidocs-linux-amd64 ./cli

build: backend cli

build-frontend: frontend

build-backend: backend

build-cli: cli

install-dev:
	mkdir -p "$(HOME)/.local/bin"
	ln -sfn "$(CURDIR)/aidocs-dev" "$(HOME)/.local/bin/aidocs-dev"

dashboards:
	cd monitoring/grafana && $(JB) install && $(JSONNET) -J vendor aidocs-dashboard.libsonnet | python3 -m json.tool > aidocs-dashboard.json

check-dashboards: dashboards
	git diff --exit-code -- monitoring/grafana/aidocs-dashboard.json

format:
	gofmt -w $$(find api cli -name '*.go' -not -path 'api/internal/server/docs/*')
	cd frontend && npm exec prettier -- --write --ignore-unknown --no-error-on-unmatched-pattern \
		../.github/**/*.{yml,yaml} \
		../charts/**/*.{yaml,yml} \
		../docs/**/*.md \
		../frontend/bridge/**/*.ts \
		../frontend/public/**/*.{html,css,js,json,svg} \
		../frontend/scripts/**/*.{js,ts,json} \
		../frontend/src/**/*.{ts,tsx,css,json} \
		../frontend/e2e/**/*.ts \
		../frontend/playwright.config.ts \
		../frontend/tests/**/*.ts \
		../skills/**/*.md \
		../*.{md,json,yaml,yml}

# Install the pinned linter locally when it is missing or has the wrong version.
bin/golangci-lint:
	@mkdir -p bin
	@if [ "$$(./bin/golangci-lint version 2>/dev/null | awk '{print $$4}')" != "$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))" ]; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh | \
			sh -s -- -b ./bin $(GOLANGCI_LINT_VERSION); \
	fi

lint: bin/golangci-lint
	./bin/golangci-lint config verify
	cd api && ../bin/golangci-lint fmt --diff --config ../.golangci.yaml
	cd cli && ../bin/golangci-lint fmt --diff --config ../.golangci.yaml
	GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) GOCACHE=$(GOLANGCI_LINT_GOCACHE) \
		./bin/golangci-lint run ./api/... ./cli/...
	cd frontend && npm run lint && npm run format:check

# Handler and repository tests run against a real PostgreSQL database: a
# disposable container via testcontainers, or the server named by
# AIDOCS_TEST_DATABASE_URL (see api/internal/testdb).
test: swagger frontend
	go test ./api/... ./cli/...
	cd frontend && npm test

ci: build test lint

# Boots the docker-compose dev stack (Postgres, MinIO, Dex, aidocs) and runs
# the non-interactive end-to-end smoke test in scripts/e2e.sh. The stack is
# left running; tear it down with `scripts/e2e.sh down`.
e2e:
	scripts/e2e.sh

# Browser E2E is advisory and intentionally separate from `make ci`. Reuse a
# healthy local app + Dex stack when present; otherwise boot the isolated
# project and leave it running for inspection.
e2e-browser:
	@if curl -fsS http://localhost:8080/readyz >/dev/null 2>&1; then \
		if curl -fsS http://localhost:5556/dex/healthz >/dev/null 2>&1; then \
			echo "Using the running aidocs + Dex stack."; \
		else \
			echo "aidocs is already running on :8080, but the browser suite requires the local Dex provider on :5556." >&2; \
			exit 1; \
		fi; \
	else \
		COMPOSE_PROJECT_NAME=$(E2E_COMPOSE_PROJECT) docker compose up -d --build --wait; \
	fi
	cd frontend && npm ci
	cd frontend && npm run e2e:install
	cd frontend && npm run e2e

# Runs the production server command against fresh PostgreSQL and MinIO,
# exercises auth/import/review/webhooks/restart persistence, and renders Helm.
verify-production:
	./scripts/verify-production.sh

# Also builds and boots the Dockerfile artifact under the chart's runtime
# restrictions. This is opt-in because it requires Docker Buildx.
verify-production-image:
	AIDOCS_VERIFY_IMAGE=1 ./scripts/verify-production.sh

clean:
	rm -rf bin
