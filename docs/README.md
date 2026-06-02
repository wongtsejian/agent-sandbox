# agent-sandbox

An opinionated agent sandbox orchestrator that deploys AI coding agents inside secure Docker containers. Prioritizes minimal configuration while maintaining strong security boundaries.

**Philosophy:** One config file, one command. All infrastructure details hidden from the user.

## Requirements

1. User can choose supported runtime agent provider (codex, claude-code, pi)
2. Agent sandbox enforced (transparent proxy via separate gateway container — cannot be bypassed)
3. Minimize user configuration efforts
4. Allow user to customize packages and home directory
5. Allow agent to spin up Docker containers for development (planned)

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│  agent-sandbox CLI (user's machine)                            │
│  - Reads agent.yaml                                            │
│  - Resolves plugins (runtime + features)                       │
│  - Generates .build/ (Dockerfile, docker-compose.yml, etc.)    │
│  - Runs: docker compose up                                     │
└────────────────────────────────────────────────────────────────┘

         docker compose up
              │
    ┌─────────┴──────────────┐
    ▼                        ▼
┌──────────────────┐   ┌────────────────────────────────────────┐
│ Gateway Container│   │ Agent Container                         │
│ (separate, root) │   │                                        │
│                  │   │  ┌──────────────────────────────────┐   │
│ - TCP listener   │   │  │ Channel Manager (TypeScript)     │   │
│ - SNI extraction │   │  │ - Spawns agent as child process  │   │
│ - DNS (port 53)  │   │  │ - Loads channel plugins          │   │
│ - TLS MITM       │   │  │ - No channels → agent standalone │   │
│ - Cred injection │   │  └──────────────────────────────────┘   │
│ - Passthrough    │   │                                        │
│                  │   │  ┌──────────────────────────────────┐   │
│ Agent's default  │   │  │ Agent Runtime (child process)    │   │
│ route → gateway  │   │  │ - codex | claude-code | pi       │   │
│                  │   │  │ - Unaware of proxy or channels   │   │
│ Real credentials │   │  └──────────────────────────────────┘   │
│ stay HERE only   │   │                                        │
└──────────────────┘   └────────────────────────────────────────┘
```

**Key security property:** The agent container cannot read real credentials. All secrets live in the gateway container. The agent uses dummy tokens; the gateway intercepts requests and swaps in real credentials.

## Documents

| Doc | Description |
|-----|-------------|
| [Configuration](configuration.md) | User config, home directory, packages |
| [Plugins](plugins.md) | Runtime, credential, channel, and feature plugins |
| [Troubleshooting](troubleshooting.md) | Common issues and fixes |
| [Security](security.md) | Network model, hardening, Docker access |
| [Plugin System](plugin-system.md) | SDK interface, plugin structure, resolution logic |
| [Build & Deploy](build-and-deploy.md) | Build flow, Dockerfile, distribution |
| [Gateway Internals](gateway-internals.md) | Proxy architecture, MITM pipeline, DNS, rewriters |
| [CLI & UX](cli-and-ux.md) | Commands, UX design, DX for plugin authors |
| [Decisions](decisions.md) | Key decisions, comparison with agent-fleet, maintainability |
| [Roadmap](roadmap.md) | Phased implementation plan |
