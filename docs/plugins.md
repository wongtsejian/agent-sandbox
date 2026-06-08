# Plugins

Available plugins that extend agent-sandbox. All builtin plugins use the `@builtin/` prefix.

## Runtime Presets

Runtime presets define the agent's base image, packages, and default command. Selected via `runtime.image`:

```yaml
runtime:
  image: "@builtin/codex"
```

| Preset | Base Image | Agent | Use Case |
|--------|-----------|-------|----------|
| `@builtin/codex` | node:24-slim | OpenAI Codex | AI coding with OpenAI models |
| `@builtin/claude-code` | node:24-slim | Anthropic Claude Code | AI coding with Claude models |
| `@builtin/pi` | node:24-slim | Pi Coding Agent | Pi-based coding |

For custom runtimes not shipped with the CLI:

```yaml
runtime:
  image: python:3.12-slim
  extra_builds:
    - "RUN pip install my-agent-cli"
  entrypoint: ["my-agent-cli", "--headless"]
```

## Feature Plugins

### @builtin/github-pat

Injects a GitHub PAT into all requests to `github.com` and `api.github.com` via the gateway.

```yaml
installations:
  - plugin: "@builtin/github-pat"
    options:
      token: "${GITHUB_PAT}"
```

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `token` | string | yes | GitHub PAT. Use `${ENV_VAR}` to reference `.env` |

The gateway adds HTTP Basic auth to all GitHub HTTPS requests. Git CLI, `gh`, and any HTTPS-based GitHub access is authenticated without the token entering the agent environment.

---

### @builtin/home-override

Mounts a local directory into the agent container as `/home/agent/`.

```yaml
installations:
  - plugin: "@builtin/home-override"
    options:
      home_directory: "./home"
      volume: true
```

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `home_directory` | string | yes | — | Local directory (relative to project root) |
| `volume` | boolean | no | `false` | Persist home across restarts via named Docker volume |

**Without volume:** Bind-mounts the directory directly. Host and container share changes.
**With volume:** Contents seeded into a named volume on first run. Survives container restarts.

---

### @builtin/ssh

SSH server inside the agent container for remote development access.

```yaml
installations:
  - plugin: "@builtin/ssh"
    options:
      port: 2222
      authorized_keys: "./ssh_key.pub"
```

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `port` | integer | no | `2222` | SSH port exposed on the host |
| `authorized_keys` | string | yes | — | Path to public key file (relative to project root) |

Key-only auth. No passwords. Connect with `ssh -p 2222 agent@localhost`.

---

### @builtin/mcp-oauth

OAuth token storage for MCP (Model Context Protocol) providers via a shared volume.

```yaml
installations:
  - plugin: "@builtin/mcp-oauth"
    options:
      providers:
        notion:
          mcp_url: https://mcp.notion.com/mcp
      token_dir: "/data/oauth-tokens"
```

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `providers` | object | yes | — | Map of provider name to MCP config (`mcp_url` required per provider) |
| `token_dir` | string | no | `/data/oauth-tokens` | Directory for OAuth token files |

You must also declare the provider endpoints as gateway services in your `agent.yaml`.

---

### @builtin/agent-manager-acp

ACP proxy that spawns an agent process and exposes it over HTTP/WebSocket for channel adapters.

```yaml
installations:
  - plugin: "@builtin/agent-manager-acp"
    options:
      acp_command: ["codex-acp"]
      port: "3100"
```

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `acp_command` | array | yes | — | Command to spawn as the agent process |
| `port` | string | no | `"3100"` | HTTP/WebSocket listen port |

Performs ACP handshake (initialize + authenticate) at startup. Channel adapters (like telegram) connect via WebSocket to send/receive messages. See the [ACP protocol reference](reference/channel-manager-protocol.md) for details.

---

## Local Plugins

Project-local plugins are referenced with a `./` path:

```yaml
installations:
  - plugin: ./plugins/my-plugin
    options:
      key: value
```

See [Creating Plugins](guides/creating-plugins.md) for how to build your own.

## Planned Plugins

| Plugin | Purpose | Status |
|--------|---------|--------|
| `@builtin/docker` | DinD sidecar with Docker API validation proxy | Planned |
| `@builtin/slack` | Slack channel adapter (like telegram) | Planned |
