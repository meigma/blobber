# Contributing to Blobber

Thank you for your interest in contributing to Blobber! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Style](#code-style)
- [Commit Guidelines](#commit-guidelines)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Documentation](#documentation)
- [Getting Help](#getting-help)

## Code of Conduct

We are committed to providing a welcoming and inclusive environment. Please be respectful and constructive in all interactions. Harassment, discrimination, or abusive behavior will not be tolerated.

## Getting Started

### Prerequisites

- **Go 1.24+** - [Installation guide](https://go.dev/doc/install)
- **Docker** - Required for integration tests
- **just** - Command runner ([installation](https://github.com/casey/just#installation))
- **golangci-lint** - Go linter ([installation](https://golangci-lint.run/welcome/install/))

Alternatively, use Nix for a reproducible development environment:

```bash
nix develop
```

### Setup

1. Fork the repository on GitHub

2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/blobber.git
   cd blobber
   ```

3. Add the upstream remote:
   ```bash
   git remote add upstream https://github.com/meigma/blobber.git
   ```

4. Verify your setup:
   ```bash
   just ci
   ```

## Development Workflow

### Creating a Branch

Create a feature branch from `master`:

```bash
git checkout master
git pull upstream master
git checkout -b feat/your-feature-name
```

Use descriptive branch names with prefixes like `feat/`, `fix/`, `docs/`, or `refactor/`.

### Available Commands

Use `just` to run common development tasks:

| Command | Description |
|---------|-------------|
| `just` | Show all available commands |
| `just build` | Build all packages |
| `just build-cli` | Build CLI binary to `bin/blobber` |
| `just test` | Run unit tests |
| `just test-v` | Run tests with verbose output |
| `just test-cover` | Run tests with coverage report |
| `just test-integration` | Run integration tests (requires Docker) |
| `just lint` | Run golangci-lint |
| `just lint-fix` | Run linter with auto-fix |
| `just fmt` | Format code |
| `just ci` | Run all CI checks locally |

### Local Testing with a Registry

Start a local OCI registry for manual testing:

```bash
just registry-start    # Start registry at localhost:5050
just registry-stop     # Stop registry
just registry-rm       # Remove registry container
```

## Code Style

### Formatting

Code must be formatted with `gofmt` and `goimports`:

```bash
just fmt
```

Imports must be organized in groups:
1. Standard library
2. External packages
3. Local packages (`github.com/meigma/blobber`)

### Linting

All code must pass `golangci-lint` with no errors:

```bash
just lint
```

The linter enforces:
- **Error handling** - All errors must be explicitly handled
- **Security** - No common security vulnerabilities (gosec)
- **Complexity** - Cyclomatic complexity <= 15, cognitive complexity <= 20
- **Code quality** - Various best practices via revive, gocritic, etc.

Use `just lint-fix` to auto-fix some issues.

### Best Practices

- Keep functions focused and concise
- Use meaningful variable and function names
- Add godoc comments for exported functions and types
- Handle errors explicitly; avoid ignoring them
- Use `context.Context` for cancellation and timeouts

## Commit Guidelines

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for automated versioning and changelog generation.

### Format

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Types and Version Impact

| Type | Version Bump | Example |
|------|--------------|---------|
| `fix:` | Patch (0.0.x) | `fix: handle nil pointer in registry client` |
| `feat:` | Minor (0.x.0) | `feat: add zstd compression support` |
| `feat!:` | Major (x.0.0) | `feat!: redesign Client API` |
| `BREAKING CHANGE:` | Major (x.0.0) | Footer indicating breaking change |

Other types (no version bump, but tracked in changelog):
- `docs:` - Documentation changes
- `chore:` - Maintenance tasks
- `test:` - Test additions or fixes
- `ci:` - CI/CD changes
- `refactor:` - Code refactoring
- `style:` - Code style changes
- `perf:` - Performance improvements

### Examples

```bash
# Bug fix
git commit -m "fix: prevent panic when image manifest is nil"

# New feature
git commit -m "feat: add support for zstd compression"

# Breaking change
git commit -m "feat!: change Push signature to accept options struct"

# With scope
git commit -m "fix(cli): correct flag parsing for --no-cache"

# With body
git commit -m "feat: add blob caching

Implements local filesystem caching to avoid repeated
downloads of the same blobs. Cache location follows
XDG conventions."
```

## Testing

### Unit Tests

Run unit tests:

```bash
just test
```

Run with verbose output:

```bash
just test-v
```

Generate coverage report:

```bash
just test-cover
# Opens coverage.html
```

### Integration Tests

Integration tests require Docker and test against a real OCI registry:

```bash
just test-integration
```

For debugging with container logs:

```bash
just test-integration-debug
```

### Writing Tests

- Use [testify](https://github.com/stretchr/testify) for assertions
- Place test files alongside source files (`foo_test.go`)
- Use table-driven tests for multiple test cases
- Tag integration tests with `//go:build integration`

Example:

```go
func TestClient_Push(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        // test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Pull Request Process

### Before Submitting

1. **Sync with upstream**:
   ```bash
   git fetch upstream
   git rebase upstream/master
   ```

2. **Run all checks**:
   ```bash
   just ci
   ```

3. **Run integration tests** (if applicable):
   ```bash
   just test-integration
   ```

### Submitting a PR

1. Push your branch to your fork:
   ```bash
   git push origin feat/your-feature-name
   ```

2. Open a Pull Request against `meigma/blobber:master`

3. Fill out the PR template with:
   - Summary of changes
   - Related issues (use `Fixes #123` to auto-close)
   - Testing performed

### PR Requirements

- All CI checks must pass
- Code must be formatted and lint-free
- Tests must pass (including any new tests for new functionality)
- Commits must follow Conventional Commits format
- Changes should be focused and atomic

### Review Process

- A maintainer will review your PR
- Address feedback by pushing additional commits
- Once approved, a maintainer will merge the PR

## Documentation

### Code Documentation

- Add godoc comments for all exported types, functions, and methods
- Keep comments concise and focused on "why" not "what"

### User Documentation

User-facing documentation lives in `/docs` (Docusaurus site):

```bash
just docs-install  # Install dependencies
just docs-dev      # Start dev server
just docs-build    # Build for production
```

Update documentation when:
- Adding new CLI commands or flags
- Changing library API
- Adding new features

## Getting Help

- **Questions**: Open a [GitHub Discussion](https://github.com/meigma/blobber/discussions)
- **Bugs**: Open a [GitHub Issue](https://github.com/meigma/blobber/issues)
- **Documentation**: Visit [blobber.meigma.dev](https://blobber.meigma.dev)

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
