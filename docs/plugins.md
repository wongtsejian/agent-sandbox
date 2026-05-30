# Plugins

## Runtime Plugins

Runtime plugins set `BaseImage` and install the agent CLI. Selected by the `runtime:` field. Only one per agent.

```yaml
runtime: codex    # uses codex RuntimePlugin → sets BaseImage + installs @openai/codex
```

| Runtime | Base Image | Packages |
|---------|-----------|----------|
| `codex` | node:22-slim | git, curl, @openai/codex |
| `claude-code` | node:22-slim | git, curl, @anthropic-ai/claude-code |
| `pi` | node:22-slim | git, curl, pi-coding-agent |

For unsupported runtimes:
```yaml
plugins:
  custom-runtime:
    base_image: "python:3.12-slim"
    packages: ["git", "my-agent-cli"]
    cmd: "my-agent-cli"
```

## Feature Plugins

Additive capabilities. Multiple per agent. Listed under `features:` in config.

### Credential Features

Declare egress rules + implement `RequestHandler` for credential injection at the gateway.

| Plugin | Hosts | Injection |
|--------|-------|-----------|
| `github` | github.com, *.github.com | Header: `Authorization: token <PAT>` |
| `mcp-oauth` | user-defined MCP server URL | OAuth2 dynamic client registration + token refresh |
| `static-header` | user-defined endpoint | Static header injection (any API key) |

Note: LLM API credentials (OpenAI, Anthropic) are handled by the runtime itself (codex device flow, claude login). No dedicated plugins needed — the agent stores its own auth token in the home directory.

### mcp-oauth plugin

Generic OAuth2 plugin for any MCP server. User provides the MCP URL, plugin handles:
1. Dynamic client registration (RFC 7591)
2. Authorization flow (redirect user to auth URL)
3. Token exchange (code → access_token + refresh_token)
4. Auto-refresh before expiry
5. Inject `Authorization: Bearer <token>` on matching requests

```yaml
plugins:
  mcp-oauth:
    servers:
      - url: "https://mcp.notion.com"
        name: "notion"
      - url: "https://mcp.linear.app"
        name: "linear"
```

The plugin auto-derives egress rules from the configured URLs. User triggers auth via channel command (`/oauth notion`).

### Channel Features

Contribute both egress rules (gateway side) AND bridge plugin code (channel side). One plugin, two contributions.

| Plugin | Egress | Bridge |
|--------|--------|--------|
| `telegram` | api.telegram.org → URL rewrite (bot token) | grammy bot, long-poll |
| `slack` | slack.com → OAuth token refresh | Slack socket mode |

### Infrastructure Features

| Plugin | Contributes |
|--------|-------------|
| `docker` | DinD sidecar, docker CLI, DOCKER_HOST env, DockerHandler in gateway |
| `home-version-control` | Custom commands, entrypoint hooks, runtime volumes |

### home-version-control plugin

Gives users direct control over image build commands, startup hooks, and persistent volumes. Replaces top-level `packages:` and `home:` fields — everything goes through the plugin model.

```yaml
plugins:
  home-version-control:
    commands:
      - "apt-get install -y ripgrep fd-find"
      - "npm install -g typescript"
    entrypoint_hooks:
      - ./scripts/sync-dotfiles.sh
      - ./scripts/setup-git.sh
    runtime_volumes:
      - "agent-home:/home/agent"
```

| Field | Contribution | Behavior |
|-------|-------------|----------|
| `commands` | Image.Commands | RUN during docker build (after base packages) |
| `entrypoint_hooks` | Entrypoint.Hooks | Scripts run on every container start (before agent) |
| `runtime_volumes` | Compose.Volumes | Named volumes mounted at runtime |

The `./home/` override directory (if present) is auto-staged to `/opt/home-override/` and cp'd by a built-in hook. User's `entrypoint_hooks` run after the override copy.

```go
func (p *HomeVersionControl) Contribute(ctx sdk.ContributeContext) (*sdk.Contributions, error) {
    cfg := parseConfig(ctx.Config)
    return &sdk.Contributions{
        Image: &sdk.ImageContribution{
            Commands: cfg.Commands,
        },
        Entrypoint: &sdk.EntrypointContribution{
            Hooks: loadHooks(cfg.EntrypointHooks),
        },
        Compose: &sdk.ComposeContribution{
            Volumes: cfg.RuntimeVolumes,
        },
    }, nil
}
```
