# syntax=docker/dockerfile:1

# Build on the native runner arch and cross-compile to the target arch, so the
# heavy npm/vite/Go work never runs under QEMU emulation.
FROM --platform=$BUILDPLATFORM c.rzp.io/razorpay/rzp-docker-image-inventory-multi-arch:rzp-golden-image-base-node-22-alpine3.23 AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
# Build the static frontend bundle for embedding into the Go server image.
RUN npx tsc -b && npx vite build

FROM --platform=$BUILDPLATFORM c.rzp.io/razorpay/rzp-docker-image-inventory-multi-arch:rzp-golden-image-base-golang-1.26-alpine3.23 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/frontend/dist ./api/internal/server/frontend_dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/aidocs-server ./api/cmd/aidocs-server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/aidocs ./cli

FROM c.rzp.io/razorpay/rzp-docker-image-inventory-multi-arch:rzp-golden-image-base-alpine3.23
ARG AIDOCS_COMMIT_SHA=""
# CA certs are arch-independent data files; copy them from the builder rather
# than running apk under emulation in the target-arch final stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
COPY --from=builder /out/aidocs-server /usr/local/bin/aidocs-server
COPY --from=builder /out/aidocs /usr/local/bin/aidocs
EXPOSE 8080
ENV AIDOCS_PORT=8080
ENV AIDOCS_COMMIT_SHA=$AIDOCS_COMMIT_SHA
CMD ["aidocs-server"]
