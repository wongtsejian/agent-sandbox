# mcp-oauth

Provides OAuth token storage for MCP (Model Context Protocol) providers via a shared volume between the gateway and agent.

## How It Works

Declares a named volume (`oauth-tokens`) mounted into both the gateway and agent containers at a configurable path. MCP providers that require OAuth can store and read token files from this shared location.

> **Note:** This plugin currently only handles the volume declaration. You must manually declare gateway services for each MCP provider endpoint in your `agent.yaml`. Full automation (dynamic service entries from provider URLs) is planned.

## Usage

```yaml
# agent.yaml
installations:
  - plugin: "@builtin/mcp-oauth"
    options:
      providers:
        notion:
          mcp_url: https://mcp.notion.com/mcp
      token_dir: "/data/oauth-tokens"

gateway:
  services:
    - url: https://mcp.notion.com
      headers:
        Authorization: Bearer ${NOTION_TOKEN}
```

```bash
# .env
NOTION_TOKEN=ntn_xxxx
```

## Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `providers` | object | yes | — | Map of provider name to config. Each provider needs at least `mcp_url`. |
| `token_dir` | string | no | `/data/oauth-tokens` | Directory for OAuth token files |

## What It Contributes

- **Gateway:** Shared volume `oauth-tokens` mounted at `token_dir`
- **Agent:** Same volume accessible for MCP client token reads
