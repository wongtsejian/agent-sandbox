# Agent Instructions

## Project

agent-sandbox — an opinionated agent sandbox orchestrator. Deploys AI coding agents inside Docker containers with transparent egress proxy, credential injection, and messaging channels.

## Tech Stack

- Language: Go 1.24+
- Build: Go workspace (go.work)
- CLI: cobra
- Config: yaml.v3
- Tests: go test + testify
- Lint: golangci-lint

## Structure

```
sdk/                    ← Plugin interfaces (RuntimePlugin, FeaturePlugin)
cmd/agent-sandbox/      ← CLI binary
internal/
  config/               ← agent.yaml + fleet.yaml parsing
  generate/             ← Dockerfile + docker-compose.yml generation
  compose/              ← compose passthrough
gateway/                ← Transparent proxy (separate Go module, embedded in CLI)
bridge/                 ← TypeScript bridge runtime (embedded in CLI)
plugins/
  codex/                ← RuntimePlugin: codex
  claude-code/          ← RuntimePlugin: claude-code
  github/               ← FeaturePlugin: GitHub PAT injection
  telegram/             ← FeaturePlugin: Telegram channel
  docker/               ← FeaturePlugin: DinD + DockerHandler
  home-version-control/ ← FeaturePlugin: packages, hooks, volumes
docs/                   ← Design documents
```

## Conventions

- Conventional commits: feat:, fix:, docs:, chore:, refactor:, test:
- Tests for all exported functions
- golangci-lint must pass
- Each plugin is self-contained in its own directory
- SDK interfaces are stable — additive changes only

## Commands

```bash
make build          # build CLI binary
make test           # run all tests
make lint           # golangci-lint
make check          # lint + test
```

## Design Docs

See docs/ for architecture, plugin system, configuration, and security docs.
Refer to docs/migration-plan.md for the phased implementation plan.

## Key Principles

- Every phase produces a working `agent-sandbox generate && agent-sandbox compose up --build`
- RuntimePlugin: one per agent, sets base image
- FeaturePlugin: multiple per agent, additive capabilities
- Gateway handles all credential injection (MITM where needed, passthrough otherwise)
- Bridge spawns agent as child process, loads channel plugins dynamically
- Ephemeral by default — containers start fresh every restart
