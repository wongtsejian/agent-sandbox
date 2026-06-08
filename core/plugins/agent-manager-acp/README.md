# agent-manager-acp

ACP (Agent Client Protocol) proxy that spawns an agent process and exposes it over HTTP/WebSocket for channel adapters.

## How It Works

At container startup, the agent-manager:

1. Spawns the configured `acp_command` as a child process
2. Communicates with it via ACP over stdio (JSON-RPC 2.0)
3. Performs the ACP handshake (initialize + authenticate)
4. Exposes an HTTP/WebSocket server on the configured port

Channel adapters (like the telegram plugin) connect to this WebSocket endpoint to send user messages and receive agent responses.

## Usage

```yaml
installations:
  - plugin: "@builtin/agent-manager-acp"
    options:
      acp_command: ["codex-acp"]
      port: "3100"
```

## Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `acp_command` | array | yes | — | Command to spawn the agent via ACP over stdio |
| `port` | string | no | `"3100"` | HTTP/WebSocket listen port |

### Common `acp_command` values

| Runtime | Command |
|---------|---------|
| Codex | `["codex-acp"]` |
| Claude Code | `["claude-agent-acp"]` |
| Pi | `["pi-acp"]` |

## What It Contributes

- **Runtime (build):** Copies agent-manager TypeScript source, compiles it, writes `config.json` with the ACP command and working directory
- **Runtime (ports):** Exposes the ACP HTTP/WS port

## Architecture

```
Channel Adapter (sidecar)
    │ WebSocket (ws://agent:<port>/acp)
    ▼
Agent Manager (inside agent container)
    │ ACP over stdio (JSON-RPC 2.0)
    ▼
Agent Process (codex-acp, claude-agent-acp, etc.)
    │ HTTPS (transparent proxy)
    ▼
Gateway (MITM + credential injection)
```

## Protocol

The agent-manager implements the ACP server specification. Channel adapters connect via WebSocket and can:

- Create sessions (`session/new`)
- Send prompts (`session/prompt`)
- Receive streaming responses via SSE or WebSocket frames

See [ACP Protocol Reference](../../docs/reference/channel-manager-protocol.md) for the full specification.

## Dependencies

This plugin is required by channel adapters (e.g., telegram). If a plugin declares `requires: ["@builtin/agent-manager-acp"]` and this plugin is not installed, generation fails with an error.
