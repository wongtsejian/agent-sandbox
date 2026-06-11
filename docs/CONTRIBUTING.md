# Contributing to agent-sandbox

## Prerequisites

- Go 1.24+ (provided via flox)
- Docker with Compose v2
- (Optional) [flox](https://flox.dev) for dev environment

## Development Setup

```bash
# Clone the repo
git clone https://github.com/donbader/agent-sandbox.git
cd agent-sandbox

# Option A: Use flox (provides Go, golangci-lint, etc.)
flox activate

# Option B: Bring your own Go 1.24+
go version  # verify >= 1.24
```

## Project Structure

```
scripts/
  shim.sh                 ← POSIX shell shim (installed as `agent-sandbox`)
  install.sh              ← Installer
cmd/agent-sandbox-core/   ← Core CLI binary (cobra)
internal/
  config/               ← agent.yaml parsing
  dotenv/               ← .env file loading
  envvar/               ← environment variable resolution
  generate/             ← Build artifact generation
    v1/                 ← v1 generator (compose, dockerfile, gateway config)
    templates/          ← Go text/template files
  plugin/               ← plugin resolution, merging, rendering
core/
  gateway/              ← Gateway source (Go + goja JS runtime)
  sdk/                  ← Gateway middleware interfaces
  presets/              ← Runtime presets (YAML)
  plugins/              ← Feature plugins (YAML + TypeScript)
examples/               ← Working example configurations
tests/                  ← Integration tests
docs/                   ← Documentation
```

## Building

```bash
# Build all packages
go build ./...

# Build the core binary
go build -o core/agent-sandbox-core ./cmd/agent-sandbox-core/

# Run locally with --dev (from source)
./scripts/shim.sh --dev -C examples/local-coding generate
```

## Testing

```bash
# Unit tests
go test ./...

# Unit tests with race detector
go test -race ./...

# Integration tests (requires Docker)
go test -tags integration ./...

# Shim tests
tests/shim/test_fleet_resolution.sh
tests/shim/test_shim_e2e.sh
```

## Linting

```bash
# Go linting
golangci-lint run ./...

# Shell script linting
shellcheck scripts/shim.sh scripts/install.sh
```

## Running Examples

```bash
# Using --dev mode (builds from source, no download)
agent-sandbox --dev -C examples/local-coding generate
agent-sandbox -C examples/local-coding compose up --build
```

## Making Changes

### Commit Conventions

We use conventional commits:
- `feat:` — New functionality
- `fix:` — Bug fixes
- `docs:` — Documentation
- `refactor:` — Restructuring without behavior change
- `test:` — Adding/updating tests
- `chore:` — Dependency updates, tooling

### Branching

- Feature branches: `feat/<description>`
- Bug fixes: `fix/<description>`
- Docs: `docs/<description>`

### What Needs Tests

- All exported functions
- Plugin resolution logic
- Template rendering
- Config parsing edge cases

Test guidelines:
- Test behavior, not constants
- Don't test functions that only return hardcoded values
- Use `//go:build integration` for tests needing Docker
- Prefer fewer meaningful tests over many trivial ones

### PR Process

1. Create a feature branch
2. Make your changes with tests
3. Ensure `go test ./...` and `golangci-lint run ./...` pass
4. Push and create a PR against `main`
5. CI runs: lint, test, build, examples, integration

## Architecture Guidelines

- **Every change produces a working `generate && compose up --build`**
- **Plugin updates never require CLI upgrades** — plugins are YAML + TypeScript
- **Runtime presets are pure data** — YAML only, no Go code
- **SDK interfaces are stable** — additive changes only
- **Each plugin is self-contained** in its own directory

## Adding a New Runtime Preset

1. Create `core/presets/<name>/runtime.yaml`
2. Define base image, install commands, CMD
3. Test with an example agent.yaml using the preset
4. No Go code changes needed

## Adding a New Feature Plugin

1. Create `core/plugins/<name>/plugin.yaml`
2. Write TypeScript middleware/routes in `src/`
3. Test with `--dev` mode
4. See [Plugin Development Guide](guides/creating-plugins.md)

## Release Process

Releases are triggered by pushing a `v*` tag:

```bash
git tag v1.32.0
git push origin v1.32.0
```

The `core-release.yml` workflow:
1. Cross-compiles core binary (4 platforms)
2. Builds pre-built gateway binaries (linux/amd64 + linux/arm64)
3. Packages tarballs with presets, plugins, templates, gateway
4. Creates GitHub Release with all platform assets
