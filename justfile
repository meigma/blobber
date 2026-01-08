# Blobber development commands
set shell := ["bash", "-euo", "pipefail", "-c"]
set dotenv-load

# Default recipe - show available commands
default:
    @just --list

# ---------- Build ----------

# Build all packages
[group('build')]
build:
    go build ./...

# Build the CLI binary
[group('build')]
build-cli:
    go build -o bin/blobber ./cmd/blobber

# ---------- Test ----------

# Run all tests
[group('test')]
test:
    go test ./...

# Run tests with verbose output
[group('test')]
test-v:
    go test -v ./...

# Run tests with coverage
[group('test')]
test-cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run integration tests (requires Docker)
[group('test')]
test-integration:
    go test -tags=integration -v -timeout=10m ./...

# Run integration tests with verbose container logs
[group('test')]
test-integration-debug:
    TESTCONTAINERS_DEBUG=true go test -tags=integration -v -timeout=10m ./...

# ---------- Lint & Format ----------

# Run golangci-lint
[group('lint')]
lint:
    golangci-lint run

# Run golangci-lint with auto-fix
[group('lint')]
lint-fix:
    golangci-lint run --fix

# Format code with gofmt and goimports
[group('lint')]
fmt:
    gofmt -w .
    goimports -w -local github.com/meigma/blobber .

# Check formatting without making changes
[group('lint')]
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

# ---------- Development ----------

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ coverage.out coverage.html

# ---------- Registry ----------

# Start local test registry
[group('registry')]
registry-start:
    @docker start blobber-registry 2>/dev/null || \
        docker run -d -p 5050:5000 --name blobber-registry registry:2
    @echo "Registry running at localhost:5050"

# Stop local test registry
[group('registry')]
registry-stop:
    @docker stop blobber-registry 2>/dev/null || true
    @echo "Registry stopped"

# Remove local test registry
[group('registry')]
registry-rm: registry-stop
    @docker rm blobber-registry 2>/dev/null || true
    @echo "Registry removed"

# Show registry logs
[group('registry')]
registry-logs:
    docker logs -f blobber-registry

# ---------- CI ----------

# Run all CI checks (lint, test, build)
[group('ci')]
ci: lint test build
    @echo "All CI checks passed"

# ---------- Docs ----------

# Install docs dependencies
[group('docs')]
docs-install:
    cd docs && npm install

# Start docs development server
[group('docs')]
docs-dev:
    cd docs && npm start

# Build docs for production
[group('docs')]
docs-build:
    cd docs && npm run build

# Serve production docs build locally
[group('docs')]
docs-serve:
    cd docs && npm run serve

# ---------- Release ----------

# Check goreleaser configuration
[group('release')]
release-check:
    goreleaser check

# Build a snapshot release locally (no publish)
[group('release')]
release-snapshot:
    goreleaser release --snapshot --clean

# ---------- Private Helpers ----------

[private]
_ensure-registry:
    @docker start blobber-registry 2>/dev/null || \
        docker run -d -p 5050:5000 --name blobber-registry registry:2
