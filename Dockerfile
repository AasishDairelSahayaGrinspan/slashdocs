# No `# syntax=` directive: nothing here needs more than the built-in BuildKit
# syntax, and skipping it avoids an extra registry pull at build time.

# Optional registry prefix for the base images, for builds that must go through
# a private pull-through cache (for example "registry.example.com/dockerhub/").
# Empty means Docker Hub.
ARG BASE_IMAGE_PREFIX=""

# Build on the native runner arch and cross-compile to the target arch, so the
# heavy npm/vite/Go work never runs under QEMU emulation.
FROM --platform=$BUILDPLATFORM ${BASE_IMAGE_PREFIX}node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
# Build the static app and iframe bridge bundles for embedding into the Go server image.
RUN npm run build
# Export the canonical bridge source fingerprint (all bridge sources plus
# resolved production deps) for the Go stage, which has no Node runtime. This
# is the same helper `make backend` uses, so the logic lives in one place.
RUN node scripts/bridge-source-hash.mjs > /src/bridge.source.hash

FROM --platform=$BUILDPLATFORM ${BASE_IMAGE_PREFIX}golang:1.27-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG SLASHDOCS_VERSION="dev"
ARG SLASHDOCS_COMMIT_SHA=""
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/api/internal/server/frontend_dist ./api/internal/server/frontend_dist
COPY --from=frontend /src/api/internal/server/bridge_dist ./api/internal/server/bridge_dist
COPY --from=frontend /src/bridge.source.hash ./bridge.source.hash
RUN mkdir -p api/internal/server/download_dist && \
    for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
      os="${target%/*}"; arch="${target#*/}"; \
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w -X github.com/razorpay/slashdocs/cli/internal/cli.Version=$SLASHDOCS_VERSION -X github.com/razorpay/slashdocs/cli/internal/cli.Commit=$SLASHDOCS_COMMIT_SHA" -o "api/internal/server/download_dist/slashdocs-$os-$arch" ./cli; \
    done
RUN BRIDGE_SOURCE_HASH="$(cat bridge.source.hash)" && test "${#BRIDGE_SOURCE_HASH}" -eq 64 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X github.com/razorpay/slashdocs/api/internal/server.BuildVersion=$SLASHDOCS_VERSION -X github.com/razorpay/slashdocs/api/internal/server.BuildCommit=$SLASHDOCS_COMMIT_SHA -X github.com/razorpay/slashdocs/api/internal/server.BuildBridgeSourceHash=$BRIDGE_SOURCE_HASH" -o /out/slashdocs-server ./api/cmd/slashdocs-server
RUN env CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X github.com/razorpay/slashdocs/cli/internal/cli.Version=$SLASHDOCS_VERSION -X github.com/razorpay/slashdocs/cli/internal/cli.Commit=$SLASHDOCS_COMMIT_SHA" -o /out/slashdocs ./cli

FROM ${BASE_IMAGE_PREFIX}alpine:3.23
ARG SLASHDOCS_COMMIT_SHA=""
# CA certs are arch-independent data files; copy them from the builder rather
# than running apk under emulation in the target-arch final stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
COPY --from=builder /out/slashdocs-server /usr/local/bin/slashdocs-server
COPY --from=builder /out/slashdocs /usr/local/bin/slashdocs
EXPOSE 8080
ENV SLASHDOCS_PORT=8080
ENV SLASHDOCS_COMMIT_SHA=$SLASHDOCS_COMMIT_SHA
CMD ["slashdocs-server"]
