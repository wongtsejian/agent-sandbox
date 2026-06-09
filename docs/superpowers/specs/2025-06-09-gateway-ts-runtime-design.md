# Gateway TypeScript Plugin Runtime & Release Architecture

## Problem

The current architecture has three fragility points:

1. **Gateway compiles at user's `docker build` time** — broken core releases aren't caught until someone runs `compose up`.
2. **Two independent release trains** (CLI + Core) — incompatible changes ship without cross-testing.
3. **Caching hides dev state** — edits to local plugin files have no effect until a new core release is tagged. No way to point at local source during development.

## Solution

Pre-built gateway binary with embedded TypeScript runtime for plugins.

## Release Architecture

### One Release Artifact: Core

The core release (`core-vX.Y.Z`) ships:
- Pre-compiled gateway binary (linux/amd64, linux/arm64)
- Plugin source (TypeScript files + plugin.yaml)
- Presets (runtime.yaml)
- Templates (Dockerfile.tmpl, compose.tmpl)

The CLI release (`vX.Y.Z`) ships only the CLI binary. The CLI's only job:
- Download/cache the core release
- Read `agent.yaml`
- Generate `.build/` (Dockerfiles, compose, config) from templates
- Shell out to `docker compose`

**No per-project compilation. No Go source in Docker build. No gateway code in the CLI.**

### Gateway Dockerfile (end state)

```dockerfile
FROM alpine:3.21
COPY gateway /usr/local/bin/gateway
COPY plugins/ /etc/gateway/plugins/
COPY config.yaml /etc/gateway/config.yaml
CMD ["gateway"]
```

## Plugin Runtime

### Gateway Embeds a JavaScript Runtime

- `goja` — pure Go JavaScript engine (no CGO)
- `esbuild` — Go-native TypeScript transpiler

At startup:
1. Gateway reads `config.yaml` to discover enabled plugins
2. For each plugin, reads `plugin.yaml` to find declared routes/middleware
3. For each `.ts` handler, uses esbuild to bundle it (resolves all local imports into a single JS output)
4. Executes the bundled JS in a sandboxed goja VM with injected host APIs

This means TypeScript files can freely import from sibling files (e.g., `import { generateCodeVerifier } from "./pkce"`) — esbuild resolves these at load time into a self-contained bundle.

### Plugin Structure (on disk)

```
plugins/
  mcp-oauth/
    plugin.yaml       ← metadata, options schema, route/middleware declarations
    src/
      oauth.ts        ← middleware handler
      login.ts        ← route handler
      callback.ts     ← route handler
      pkce.ts         ← shared helper (imported by other .ts files)
```

### Plugin Declaration (plugin.yaml)

```yaml
name: mcp-oauth
options:
  providers:
    type: object
    required: true
  token_dir:
    type: string
    default: "/data/oauth-tokens"

contributes:
  gateway:
    services:
      - url: "https://mcp.notion.com/mcp"
    volumes:
      - "oauth-tokens:/data/oauth-tokens"
    middlewares:
      - script: "./src/oauth.ts"
        domains: ["mcp.notion.com"]
    routes:
      - path: "/login"
        handler: "./src/login.ts"
      - path: "/callback"
        handler: "./src/callback.ts"
```

### Handler Signature

Each `.ts` file exports a single default handler function:

```typescript
// src/login.ts
export default async function(ctx: RequestContext, options: PluginOptions) {
  // ...
}
```

The gateway injects the route path, resolved plugin options, and host APIs into each handler.

### Host APIs

| API | Purpose |
|-----|---------|
| `ctx.request` | Read/modify headers, URL, method, body |
| `ctx.abort(status, body, headers?)` | Short-circuit with custom response |
| `ctx.env(key)` | Read environment variables |
| `gw.http.fetch(url, opts)` | Outbound HTTP requests (SSRF-safe) |
| `gw.fs.read(path)` / `gw.fs.write(path, data)` | File I/O (scoped to plugin's data dir) |
| `gw.crypto.sha256(data)` | SHA-256 hash |
| `gw.crypto.hmac(key, data)` | HMAC |
| `gw.crypto.randomBytes(n)` | Cryptographic random bytes |
| `gw.crypto.base64url.encode(data)` / `.decode(str)` | Base64url encoding |
| `gw.secrets.register(value)` | Mark value for log redaction |
| `gw.log.info(msg, fields)` / `.error(...)` / `.debug(...)` | Structured logging |

## Development & Testing Workflow

### `--core` Flag

```bash
# Production (downloads from release, uses cache)
agent-sandbox -C examples/local-coding generate

# Development (point at local core directory)
agent-sandbox -C examples/local-coding generate --core=./core

# Or point anywhere
agent-sandbox -C examples/local-coding generate --core=/path/to/my-fork/core
```

When `--core` is set, the CLI uses that directory directly — no download, no cache. Otherwise, resolves `core_version` from `agent.yaml` and fetches from GitHub Releases.

### Core Release CI

The core release workflow:
1. Compile gateway binary for all target platforms
2. Start gateway with all bundled plugins loaded (`--dry-run` or test config)
3. Verify TS transpilation succeeds for all plugins
4. Run smoke tests (hit test endpoints, verify middleware chains)
5. Package and publish release

If any plugin has a syntax error, missing import, or runtime failure, the release fails.

## Migration Path

### Phase 1: Dual Support

Gateway supports both Go (compiled-in) and TS (loaded at runtime). Plugin declarations accept either:

```yaml
# Old style (backward compatible)
middlewares:
  - custom: "./middlewares/oauth.go"

# New style
middlewares:
  - script: "./src/oauth.ts"
```

This phase delivers:
- Pre-built gateway binary with embedded TS runtime
- `--core` flag on CLI
- Updated core-release CI (builds binary, smoke tests plugins)
- One plugin migrated to TS as proof of concept

### Phase 2: Migrate Bundled Plugins

Convert `mcp-oauth`, `github-pat`, etc. one by one. Each plugin migration is a separate PR with TS rewrite + removal of Go source.

### Phase 3: Remove Go Custom Middleware Support

Once all bundled plugins are TypeScript:
- Remove `custom: "*.go"` path
- Remove blank import / `middlewares/custom` package
- Remove per-project compilation code from generator
- Remove `gateway-src/` from build output

End state: gateway binary is fully self-contained, no Go compilation at Docker build time.

## Documentation Updates

Incrementally updated with each phase:

| Document | Change |
|----------|--------|
| `docs/plugins.md` | Rewrite for TypeScript plugin authoring (SDK APIs, plugin.yaml schema, dev workflow) |
| `AGENTS.md` | Update Plugin Architecture section, structure diagram, conventions |
| `docs/internals/gateway.md` | Describe TS runtime, plugin loading, host APIs |
| `core/plugins/*/README.md` | Update per plugin as each migrates to TS |
| `docs/configuration.md` | Update plugin.yaml schema reference for `script:` field |
| `docs/roadmap.md` | Update with migration phases |

## Key Properties of End State

- **One binary to release** — core release ships gateway binary, no source compilation at user side
- **No version coupling** — CLI generates config, gateway reads config; decoupled by config file contract
- **CI catches everything** — gateway binary tested with all plugins at release time
- **No caching issues in dev** — `--core=./core` gives instant local feedback
- **Plugins are pure data** — TS + YAML, update independently of CLI and gateway binary releases
- **Novel middleware supported** — TypeScript gives full programmatic power without recompilation
