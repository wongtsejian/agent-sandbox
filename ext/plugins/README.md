# External Plugins

This directory holds external plugins that are not bundled with the CLI binary.
Each plugin has its own version and release cycle.

## Structure

```
ext/plugins/
  <plugin-name>/
    feature.yaml          ← plugin metadata and config schema
    gateway/              ← optional Go gateway handler (compiled during Docker build)
    channel/              ← optional TypeScript channel plugin (copied into image)
    CHANGELOG.md          ← version history
```

## Resolution Order

1. `ext/plugins/<name>/` in the user's project directory
2. Embedded core plugins (shipped with CLI binary)

External plugins override embedded ones with the same name.

## Versioning

Each plugin is versioned independently. Future releases will support:
- Plugin registry for discovery
- Version pinning in agent.yaml
- Automatic fetching during `agent-sandbox generate`

## Creating a Plugin

### Minimal: Config-Only Feature

A feature that only contributes Dockerfile commands and compose config (no gateway, no channel):

```yaml
# ext/plugins/my-tools/feature.yaml
name: my-tools
description: "Install custom dev tools"

config_schema:
  packages:
    type: array
    items: string
    description: "APT packages to install"
```

### With Gateway Handler

A feature that intercepts traffic to specific hosts (e.g., credential injection):

```
ext/plugins/my-api/
  feature.yaml
  gateway/
    handler.go
    go.mod
```

```yaml
# feature.yaml
name: my-api
description: "Inject API key for my-service.com"

config_schema:
  api_key:
    type: string
    required: true
    env: true

gateway:
  hosts:
    - "api.my-service.com"
  mode: mitm

bridge: false
compose: {}
```

The gateway handler implements the `RequestHandler` interface from the SDK. It's compiled during Docker build — you don't need Go installed locally.

### With Channel Plugin

A feature that adds a messaging channel (gateway + TypeScript):

```
ext/plugins/my-channel/
  feature.yaml
  gateway/
    handler.go
    go.mod
  channel/
    channel.ts
    package.json
```

```yaml
# feature.yaml
name: my-channel
description: "My custom messaging channel"

config_schema:
  token:
    type: string
    required: true
    env: true

gateway:
  hosts:
    - "api.my-platform.com"
  mode: mitm

bridge: true
compose: {}
```

The `channel/channel.ts` must export a default class implementing the `Channel` interface (see `docs/plugin-system.md` for the protocol).

## Using Your Plugin

Reference it in `agent.yaml`:

```yaml
name: my-agent
runtime: codex
features:
  - plugin: my-api
    api_key: "${MY_API_KEY}"
```

The CLI resolves `my-api` from `ext/plugins/my-api/` automatically.

## Examples

See `internal/plugins/` for complete examples of core plugins:
- `custom-runtime/` — config-only feature (commands, hooks, volumes)
- `github-pat/` — gateway credential injection
- `telegram/` — gateway + channel (full messaging integration)
