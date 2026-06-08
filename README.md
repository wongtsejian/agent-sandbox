# agent-sandbox

Deploy AI coding agents in Docker containers with transparent egress proxy, credential injection, and messaging channels.

## Features

- **Data-driven plugins** — runtime presets (codex, claude-code, pi) and feature plugins configured via YAML
- **Transparent gateway** — all agent traffic routes through a proxy for credential injection and MITM
- **Secret isolation** — real credentials never enter the agent container
- **Multi-agent** — run multiple agents from a single `fleet.yaml` with shared config
- **Security audit** — verify the sandbox contract with `agent-sandbox audit`
- **One command** — `agent-sandbox generate && agent-sandbox compose up --build`

## Quickstart

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/donbader/agent-sandbox/main/install.sh | bash

# Scaffold a project
mkdir my-agent && cd my-agent
agent-sandbox init

# Or write config manually
cat > agent.yaml << 'EOF'
name: coder
core_version: latest

runtime:
  image: "@builtin/codex"

gateway:
  services:
    - url: https://api.openai.com
      headers:
        Authorization: Bearer ${OPENAI_API_KEY}

installations:
  - plugin: "@builtin/github-pat"
    options:
      token: "${GITHUB_PAT}"
  - plugin: "@builtin/home-override"
    options:
      home_directory: "./home"
      volume: true
EOF

# Add secrets
echo "OPENAI_API_KEY=sk-..." > .env
echo "GITHUB_PAT=ghp_..." >> .env

# Generate and run
agent-sandbox generate
agent-sandbox compose up --build -d
```

## Commands

```bash
agent-sandbox init              # interactive project scaffold
agent-sandbox generate          # agent.yaml → .build/ (Dockerfile, docker-compose.yml, gateway)
agent-sandbox compose ...       # docker compose passthrough (auto-injects -f and --env-file)
agent-sandbox audit             # verify running sandbox meets security contract
agent-sandbox upgrade           # self-update to latest release
```

## Architecture

```
┌───────────────────────────────────────────────────────────┐
│ Host                                                      │
│                                                           │
│  ┌─────────────────┐       ┌────────────────────────────┐ │
│  │ Gateway         │◄──────│ Agent Container            │ │
│  │  MITM proxy     │ DNAT  │  iptables → gateway:8443   │ │
│  │  DNS (port 53)  │       │  CA cert installed         │ │
│  │  Cred injection │       │  Runs as unprivileged user │ │
│  │  Log redaction  │       │  Agent runtime (codex etc) │ │
│  └─────────────────┘       └────────────────────────────┘ │
│         │                                                 │
│         ▼                                                 │
│  Real APIs (OpenAI, GitHub, etc.)                         │
└───────────────────────────────────────────────────────────┘
```

All outbound HTTPS from the agent is transparently redirected to the gateway via iptables DNAT. The gateway terminates TLS, injects real credentials, and forwards to the upstream API. The agent never sees actual secrets.

## Documentation

- [Getting Started](docs/getting-started.md) — install, configure, run
- [Configuration](docs/configuration.md) — agent.yaml and fleet.yaml reference
- [Plugins](docs/plugins.md) — available plugins and their options
- [Security](docs/security.md) — isolation model and threat mitigations
- [Troubleshooting](docs/troubleshooting.md) — common issues and fixes

**Guides:**

- [Fleet Mode](docs/guides/fleet-mode.md) — multi-agent setup
- [Creating Plugins](docs/guides/creating-plugins.md) — build your own plugin

**Reference:**

- [CLI](docs/reference/cli.md) — commands, flags, environment variables
- [Audit](docs/reference/audit.md) — security contract verification
- [Plugin YAML Schema](docs/reference/plugin-yaml.md) — plugin.yaml specification

See [examples/](examples/) for working setups.

## License

MIT
