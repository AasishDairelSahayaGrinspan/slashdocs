SWAG := go run github.com/swaggo/swag/cmd/swag@v1.16.6
JSONNET ?= jsonnet
JB ?= jb

.PHONY: swagger frontend backend cli build build-frontend build-backend build-cli dashboards check-dashboards format test clean

swagger:
	$(SWAG) init \
		-g main.go \
		-d api/cmd/aidocs-server,api/internal/server,api/internal/repo \
		-o api/internal/server/docs \
		--parseDependency \
		--parseInternal

frontend:
	cd frontend && npm ci && npm run build

backend: swagger frontend
	go build -o bin/aidocs-server ./api/cmd/aidocs-server

cli:
	go build -o bin/aidocs ./cli

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
	go test -tags testfixtures ./...

clean:
	rm -rf bin
