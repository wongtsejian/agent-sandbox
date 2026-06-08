# agent-sandbox

An opinionated agent sandbox orchestrator that deploys AI coding agents inside secure Docker containers. Prioritizes minimal configuration while maintaining strong security boundaries.

**Philosophy:** One config file, one command. All infrastructure details hidden from the user.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ Host                                                          │
│                                                               │
│  agent-sandbox CLI                                            │
│  - Reads agent.yaml / fleet.yaml                              │
│  - Resolves plugins (@builtin/ and local)                     │
│  - Generates .build/ (Dockerfile, compose, gateway)           │
│  - Runs: docker compose up                                    │
│                                                               │
│  ┌─────────────────┐       ┌───────────────────────────────┐ │
│  │ Gateway          │◄──────│ Agent Container                │ │
│  │  TCP proxy       │ DNAT │  iptables → gateway:8443       │ │
│  │  DNS (port 53)   │      │  CA cert trusted               │ │
│  │  TLS MITM        │      │  Runs as unprivileged user     │ │
│  │  Cred injection  │      │  Agent runtime (codex, etc.)   │ │
│  │  Log redaction   │      │  Optional: agent-manager + ACP │ │
│  │                  │      │  Optional: channel sidecars    │ │
│  │  Real credentials│      │                               │ │
│  │  stay HERE only  │      │  Dummy tokens only             │ │
│  └─────────────────┘       └───────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

**Key security property:** The agent container cannot read real credentials. All secrets live in the gateway container. The agent uses dummy tokens; the gateway intercepts requests and swaps in real credentials.

## User Documentation

| Doc | Description |
|-----|-------------|
| [Getting Started](getting-started.md) | Install, configure, and run your first agent |
| [Configuration](configuration.md) | agent.yaml, fleet.yaml, and .env reference |
| [Plugins](plugins.md) | Available plugins and their options |
| [Security](security.md) | Isolation model, threat mitigations |
| [Troubleshooting](troubleshooting.md) | Common issues and fixes |

## Guides

| Guide | Description |
|-------|-------------|
| [Fleet Mode](guides/fleet-mode.md) | Multi-agent setup with shared credentials |
| [Creating Plugins](guides/creating-plugins.md) | Build your own plugin (plugin.yaml, middleware, templates) |

## Reference

| Doc | Description |
|-----|-------------|
| [CLI](reference/cli.md) | Commands, flags, environment variables |
| [Audit](reference/audit.md) | Security contract verification checks |
| [ACP Protocol](reference/channel-manager-protocol.md) | Agent Client Protocol specification |
| [Docker API Proxy](reference/docker-api-proxy.md) | Planned Docker API validation design |
| [ADRs](reference/adr/) | Architecture Decision Records |

## Internals (Contributors)

| Doc | Description |
|-----|-------------|
| [Build Pipeline](internals/build-pipeline.md) | Generate flow, Dockerfile templates, core fetching |
| [Gateway](internals/gateway.md) | Proxy architecture, MITM pipeline, DNS, middleware SDK |
| [Plugin System](internals/plugin-system.md) | Resolution, rendering, compilation, fleet merging |
| [Logging](internals/logging.md) | Structured logging standards (Go + TypeScript) |
| [Decisions](internals/decisions.md) | Key decisions, comparison with agent-fleet |
| [Roadmap](internals/roadmap.md) | Phased implementation plan |
