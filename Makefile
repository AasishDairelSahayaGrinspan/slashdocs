SWAG := go run github.com/swaggo/swag/cmd/swag@v1.16.6
JSONNET ?= jsonnet
JB ?= jb
AIDOCS_VERSION ?= dev-local-$(shell date +%Y%m%d%H%M%S)
AIDOCS_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || true)
CLI_LDFLAGS := -X github.com/razorpay/aidocs/cli/internal.Version=$(AIDOCS_VERSION) -X github.com/razorpay/aidocs/cli/internal.Commit=$(AIDOCS_COMMIT)
SERVER_LDFLAGS := -X github.com/razorpay/aidocs/api/internal/server.BuildVersion=$(AIDOCS_VERSION) -X github.com/razorpay/aidocs/api/internal/server.BuildCommit=$(AIDOCS_COMMIT)

.PHONY: swagger frontend backend cli cli-assets build build-frontend build-backend build-cli dashboards check-dashboards format test clean

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
	go build -ldflags "$(SERVER_LDFLAGS)" -o bin/aidocs-server ./api/cmd/aidocs-server

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

dashboards:
	cd monitoring/grafana && $(JB) install && $(JSONNET) -J vendor aidocs-dashboard.libsonnet | python3 -m json.tool > aidocs-dashboard.json

check-dashboards: dashboards
	git diff --exit-code -- monitoring/grafana/aidocs-dashboard.json

format:
	gofmt -w $$(find api cli -name '*.go' -not -path 'api/internal/server/docs/*')
	cd frontend && npm exec prettier -- --write --ignore-unknown --no-error-on-unmatched-pattern \
		../.github/**/*.{yml,yaml} \
		../api/internal/server/bridge.js \
		../charts/**/*.{yaml,yml} \
		../docs/**/*.md \
		../frontend/public/**/*.{html,css,js,json,svg} \
		../frontend/scripts/**/*.{js,ts,json} \
		../frontend/src/**/*.{ts,tsx,css,json} \
		../skills/**/*.md \
		../*.{md,json,yaml,yml}

# testfixtures enables the in-memory seeded repo + HTTP handler tests, which
# live behind a build tag so the fixtures never ship in the production binary.
test: swagger frontend
	cd frontend && npm test
	go test -tags testfixtures ./...

clean:
	rm -rf bin
