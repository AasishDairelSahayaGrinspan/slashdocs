# syntax=docker/dockerfile:1

# Build on the native runner arch and cross-compile to the target arch, so the
# heavy npm/vite/Go work never runs under QEMU emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
# Build the static app and iframe bridge bundles for embedding into the Go server image.
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG AIDOCS_VERSION="dev"
ARG AIDOCS_COMMIT_SHA=""
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/api/internal/server/frontend_dist ./api/internal/server/frontend_dist
COPY --from=frontend /src/api/internal/server/bridge_dist ./api/internal/server/bridge_dist
RUN mkdir -p api/internal/server/download_dist && \
    for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
      os="${target%/*}"; arch="${target#*/}"; \
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w -X github.com/razorpay/aidocs/cli/internal/cli.Version=$AIDOCS_VERSION -X github.com/razorpay/aidocs/cli/internal/cli.Commit=$AIDOCS_COMMIT_SHA" -o "api/internal/server/download_dist/aidocs-$os-$arch" ./cli; \
    done
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X github.com/razorpay/aidocs/api/internal/server.BuildVersion=$AIDOCS_VERSION -X github.com/razorpay/aidocs/api/internal/server.BuildCommit=$AIDOCS_COMMIT_SHA" -o /out/aidocs-server ./api/cmd/aidocs-server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X github.com/razorpay/aidocs/cli/internal/cli.Version=$AIDOCS_VERSION -X github.com/razorpay/aidocs/cli/internal/cli.Commit=$AIDOCS_COMMIT_SHA" -o /out/aidocs ./cli

FROM alpine:3.23
ARG AIDOCS_COMMIT_SHA=""
ARG AIDOCS_VERSION="dev"
# CA certs are arch-independent data files; copy them from the builder rather
# than running apk under emulation in the target-arch final stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
COPY --from=builder /out/aidocs-server /usr/local/bin/aidocs-server
COPY --from=builder /out/aidocs /usr/local/bin/aidocs
EXPOSE 8080
ENV AIDOCS_PORT=8080
ENV AIDOCS_COMMIT_SHA=$AIDOCS_COMMIT_SHA
ENV AIDOCS_VERSION=$AIDOCS_VERSION
CMD ["aidocs-server"]
