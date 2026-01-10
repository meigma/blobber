# syntax=docker/dockerfile:1

# =============================================================================
# Stage 1: Builder
# =============================================================================
FROM golang:1.25-alpine AS builder

# Install git for version info and ca-certificates for HTTPS
# hadolint ignore=DL3018
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
COPY sigstore/go.mod sigstore/go.sum ./sigstore/
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# Build the binary with security flags
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
        -X github.com/meigma/blobber/cmd/blobber/cli.version=${VERSION} \
        -X github.com/meigma/blobber/cmd/blobber/cli.commit=${COMMIT} \
        -X github.com/meigma/blobber/cmd/blobber/cli.date=${DATE}" \
    -trimpath \
    -o /blobber \
    ./cmd/blobber

# =============================================================================
# Stage 2: Runtime
# =============================================================================
FROM gcr.io/distroless/static-debian12:nonroot

# OCI Image Labels
# https://github.com/opencontainers/image-spec/blob/main/annotations.md
LABEL org.opencontainers.image.title="blobber" \
      org.opencontainers.image.description="Push and pull files to OCI registries" \
      org.opencontainers.image.url="https://github.com/meigma/blobber" \
      org.opencontainers.image.source="https://github.com/meigma/blobber" \
      org.opencontainers.image.documentation="https://blobber.meigma.dev" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.vendor="Meigma"

# Copy the binary (distroless already includes CA certificates)
COPY --from=builder /blobber /usr/local/bin/blobber

# nonroot tag already runs as non-root user (65532)
ENTRYPOINT ["/usr/local/bin/blobber"]
