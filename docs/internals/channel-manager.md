# Channel Manager Internals

## What It Is

The "channel manager" in agent-sandbox is the combination of **OpenACP** (the orchestrator) and the **agent-manager-acp** plugin. Together they manage agent sessions and bridge external channels (Telegram, Slack, etc.) to an AI coding agent via the Agent Client Protocol (ACP) over stdio.

- **OpenACP** — the orchestrator process that connects to external platforms (bot APIs, webhooks), handles user ACLs, maps chats to ACP sessions, and manages platform UX (typing indicators, reactions, formatting).
- **agent-manager** — a Node.js subprocess spawned by OpenACP that owns the agent process lifecycle: spawning the ACP agent, performing the handshake, relaying messages, and auto-approving permissions.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  External Channels                                                          │
│  (Telegram DM, Slack channel, etc.)                                         │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │ Platform API (webhooks / polling)
                                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  OpenACP (orchestrator)                                                     │
│  • Connects to platform APIs                                                │
│  • User ACL / filtering                                                     │
│  • Maps chatId → ACP sessionId                                              │
│  • Handles platform UX (typing, reactions, formatting)                      │
│  • Commands registration (setMyCommands)                                    │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │ stdio (ndjson JSON-RPC 2.0)
                                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  agent-manager (Node.js subprocess)                                         │
│  • Spawns ACP agent child process                                           │
│  • Performs initialize + auth handshake                                      │
│  • Caches initialize result                                                 │
│  • Intercepts: initialize, auth, session/new, /restart                      │
│  • Injects mcpServers into session/new                                      │
│  • Auto-approves all tool permissions (headless)                            │
│  • Surfaces agent errors (auth, rate-limit, upstream) to OpenACP            │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │ stdio (ndjson JSON-RPC 2.0)
                                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  ACP Agent (codex-acp, claude-agent-acp, pi-acp, etc.)                      │
│  • Processes prompts                                                        │
│  • Streams updates (agent_message_chunk, turn_completed)                    │
│  • Declares available commands                                              │
│  • Requests tool permissions                                                │
│  • Makes HTTPS calls through transparent gateway proxy                      │
└─────────────────────────────────────────────────────────────────────────────┘
```

## The ACP Protocol

ACP (Agent Client Protocol) is a **JSON-RPC 2.0 protocol over stdio** for communicating with AI coding agents. It's the industry standard, supported by Codex, Claude Code, Pi, Gemini, and Copilot.

- **Spec**: https://agentclientprotocol.com
- **TypeScript SDK**: `@agentclientprotocol/sdk`
- **Transport**: ndjson (newline-delimited JSON) over stdin/stdout
- **Logs**: stderr (keeps stdout clean for protocol traffic)

### Core Methods

| Method | Direction | Purpose |
|--------|-----------|---------|
| `initialize` | client → agent | Negotiate protocol version and capabilities |
| `auth/authenticate` | client → agent | Provide API credentials |
| `session/new` | client → agent | Create a new conversation session |
| `session/prompt` | client → agent | Send user message to agent |
| `session/update` | agent → client | Stream responses, status updates, errors |
| `client/requestPermission` | agent → client | Ask client to approve tool usage |

### Message Types

- **Requests** have `id` + `method` — expect a response with matching `id`
- **Notifications** have `method` but no `id` — fire-and-forget (e.g., `session/update`)
- **Responses** have `id` + `result` (or `error`) — reply to a prior request

## Relationship to agent-manager-acp Plugin

The `agent-manager-acp` plugin (`core/plugins/agent-manager-acp/`) is the packaging mechanism that builds and deploys the agent-manager into the container. It consists of:

### plugin.yaml

Declares:
- **assets**: the `agent-manager/` TypeScript source (excluding node_modules/dist)
- **options**: `acp_command` (required array) and `acp_install` (optional install shell command)
- **contributes.runtime.extra_builds**: Dockerfile steps that install the ACP adapter, compile agent-manager, and write `config.json`

### Generated Build Steps

```dockerfile
# Install the ACP adapter binary (e.g. npm install -g @zed-industries/codex-acp)
RUN --mount=type=cache,target=/root/.npm {{ .plugin.options.acp_install }}

# Copy, build, and install agent-manager
COPY agent-manager/ /opt/agent-manager-src/
RUN cd /opt/agent-manager-src && npm install && npm run build \
    && mkdir -p /opt/agent-manager && mv dist /opt/agent-manager/dist \
    && mv node_modules /opt/agent-manager/node_modules \
    && rm -rf /opt/agent-manager-src

# Write runtime config
RUN echo '{"acp_command":[...], "cwd":"/workspace"}' > /opt/agent-manager/config.json
```

The resulting container has `/opt/agent-manager/` with the compiled JS + dependencies + config, ready to be spawned by OpenACP.

## Message Flow

### Inbound (Channel → Agent)

```
1. User sends message in Telegram/Slack
2. OpenACP receives via platform webhook/polling
3. OpenACP checks ACL, resolves chatId → sessionId
4. If no session: OpenACP sends session/new → agent-manager → agent
5. OpenACP sends session/prompt with user text → agent-manager
6. agent-manager forwards to agent process (no interception for prompts)
7. Agent processes prompt
```

### Outbound (Agent → Channel)

```
1. Agent emits session/update notifications (streaming chunks)
2. agent-manager forwards to OpenACP via stdout
3. OpenACP accumulates chunks, applies platform formatting
4. OpenACP sends formatted response to user via platform API
5. Agent emits turn_completed notification
6. Agent returns session/prompt response with stopReason
```

### Error Surfacing

The agent-manager monitors the agent's stderr for common failure patterns and surfaces them as `session/update` error notifications:

| Pattern | Error Type |
|---------|-----------|
| `401`, `403`, `unauthorized`, `forbidden`, `invalid.*key` | Authentication error |
| `429`, `rate.?limit`, `too many requests` | Rate limit |
| `500`, `502`, `503`, `504`, `service unavailable` | Upstream error |

These are sent as JSON-RPC notifications with `sessionId: "__system__"` so OpenACP can relay them to the user.

## Session Lifecycle

### Create

1. OpenACP sends `session/new` with optional `cwd`
2. agent-manager intercepts: injects `cwd` from config (if not provided) and `mcpServers: []`
3. Forwards modified request to agent
4. Agent returns `{ sessionId: "abc-123" }`
5. OpenACP maps chatId → sessionId for future messages

### Message (Prompt/Response Cycle)

1. OpenACP sends `session/prompt` with `sessionId` and `prompt` array
2. agent-manager forwards directly (no interception)
3. Agent streams `session/update` notifications back
4. Agent may request permissions via `client/requestPermission` — agent-manager auto-approves
5. Agent sends final response with `stopReason: "end_turn"`

### Terminate

- **Agent crash**: agent-manager emits exit notification to OpenACP, auto-restarts on next request
- **Stdin close**: agent-manager exits (if `exitOnClose: true`)
- **SIGTERM**: agent-manager gracefully stops the agent process, then exits
- **/restart command**: agent-manager kills and respawns the agent process

### Multi-Session Concurrency

Multiple channels share one agent-manager + agent connection:
- Each chat/channel maps to a separate ACP session
- Different sessions can be processed concurrently (async)
- Same session is serial (one prompt at a time)

## Build and Deployment

### Source Structure

```
core/plugins/agent-manager-acp/
├── plugin.yaml              ← Plugin metadata, options, build contributions
├── README.md                ← Usage docs
└── agent-manager/
    ├── Dockerfile           ← Standalone build (dev/testing)
    ├── package.json         ← Dependencies: @agentclientprotocol/sdk, pino
    ├── tsconfig.json        ← TypeScript config
    └── src/
        ├── index.ts         ← Entry point: reads config, starts agent, runs relay
        ├── agent-process.ts ← AgentProcess class: spawn, send, sendAndWait, restart
        ├── stdio-relay.ts   ← StdioRelay class: intercept, forward, error detection
        ├── logger.ts        ← Pino logger (writes to stderr, redacts secrets)
        └── __tests__/
            └── stdio-relay.test.ts
```

### Runtime Dependencies

| Package | Purpose |
|---------|---------|
| `@agentclientprotocol/sdk` | ACP TypeScript types (v0.25.0) |
| `pino` | Structured JSON logging to stderr |

### How It's Deployed

1. `agent-sandbox generate` reads `agent.yaml` with the plugin installation
2. The plugin's `extra_builds` inject Dockerfile steps that:
   - Install the ACP adapter binary (e.g., `codex-acp`)
   - Copy and compile the agent-manager TypeScript source
   - Write `/opt/agent-manager/config.json` with the ACP command and workspace path
3. At container runtime, OpenACP spawns `node /opt/agent-manager/dist/index.js`
4. agent-manager reads config, starts the agent, completes the handshake, and begins relaying

### Configuration

agent-manager reads `/opt/agent-manager/config.json` (overridable via `AGENT_MANAGER_CONFIG` env var):

```json
{
  "acp_command": ["codex-acp"],
  "cwd": "/workspace"
}
```

## Implementation Details

### Startup Sequence (index.ts)

1. Read and parse config.json
2. Spawn agent process with configured `acp_command`
3. Send `initialize` request, cache result
4. Send `auth/authenticate` (skip gracefully if agent returns -32601 "method not found")
5. Create StdioRelay with cached init result
6. Start relay (begin reading parent stdin)
7. Register SIGTERM handler for graceful shutdown

### Request Interception (stdio-relay.ts)

| Incoming Method | Behavior |
|-----------------|----------|
| `initialize` | Return cached init result immediately |
| `auth/authenticate` | Return empty success immediately |
| `session/new` | Inject `cwd` and `mcpServers`, then forward |
| `session/prompt` with `/restart` | Kill + respawn agent, return success |
| Everything else | Forward directly to agent |

### Logging

All logs go to **stderr** via pino to keep stdout clean for ACP protocol traffic. The logger redacts sensitive fields (`token`, `authorization`, `*.token`, `*.authorization`).

## What's Not Documented Here

The following are handled by OpenACP (the orchestrator) rather than agent-manager, and their implementation details are outside this plugin's source:

- Platform adapter logic (Telegram bot API, Slack webhooks)
- User ACL and permission filtering
- Session-to-chat mapping persistence
- Platform UX (typing indicators, message reactions, rich formatting)
- Command registration (e.g., Telegram `setMyCommands` from `available_commands_update`)
