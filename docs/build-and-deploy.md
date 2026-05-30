# Build & Deploy

## Build Flow

```
agent-sandbox generate
  │
  ├── Detect mode: agent.yaml (single) or fleet.yaml (multi)
  ├── Load RuntimePlugin from runtime: field
  ├── Load FeaturePlugins from features: map
  ├── Merge shared features (if fleet mode)
  ├── Call Contribute() on runtime + each feature
  │
  └── Generate .build/:
        ├── gateway-src/        (go:embed → gateway Go source)
        ├── bridge/             (go:embed → bridge TypeScript)
        ├── bridge-plugins/     (from feature embed.FS)
        ├── home-override/      (from user's home/ dir)
        ├── hooks/              (from features)
        ├── Dockerfile          (multi-stage: gateway compile + runtime)
        ├── gateway-config.yaml (merged egress rules)
        ├── bridge-config.json  (channels + runtime cmd)
        └── docker-compose.yml  (services, volumes, networks)

agent-sandbox compose up --build -d
  │
  └── docker compose -f .build/docker-compose.yml up --build -d
```

## Generated Dockerfile (Multi-Stage)

```dockerfile
# Stage 1: Compile gateway
FROM golang:1.24 AS gateway-builder
COPY gateway-src/ /src/
RUN cd /src && CGO_ENABLED=0 go build -o /gateway ./cmd/gateway

# Stage 2: Runtime
FROM node:22-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates iptables gosu docker.io ripgrep \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g @openai/codex

COPY bridge/ /opt/bridge/
RUN cd /opt/bridge && npm install --production
COPY bridge-plugins/telegram/ /opt/bridge/plugins/telegram/

COPY home-override/ /opt/home-override/
COPY packages.sh /tmp/packages.sh
RUN chmod +x /tmp/packages.sh && /tmp/packages.sh && rm /tmp/packages.sh

COPY --from=gateway-builder /gateway /usr/local/bin/gateway
COPY hooks/ /opt/entrypoint-hooks/

RUN useradd -m -s /bin/bash agent && useradd -r -s /usr/sbin/nologin gateway

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["node", "/opt/bridge/src/index.js"]
```

## Distribution

- **Gateway source**: embedded in CLI via `go:embed`. Extracted to `.build/gateway-src/`. Compiled in Docker multi-stage (user doesn't need Go).
- **Bridge source**: embedded in CLI via `go:embed`. Extracted to `.build/bridge/`.
- **Channel plugins**: embedded in each plugin's Go module via `go:embed`. Extracted to `.build/bridge-plugins/`.

## Multi-Agent Topology

```
┌─ Internal Network ──────────────────────────────────────────┐
│                                                              │
│  ┌─ coder ───────────────────────────────────────────────┐  │
│  │  Gateway (github + docker + telegram rules)            │  │
│  │  Bridge → codex                                        │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌─ reviewer ────────────────────────────────────────────┐  │
│  │  Gateway (github + telegram rules)                     │  │
│  │  Bridge → claude-code                                  │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌─ dind (shared) ───────────────────────────────────────┐  │
│  │  Docker daemon                                         │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

Each agent has its own gateway instance (same binary, different config). Agents share the network (can communicate). DinD shared if multiple agents need Docker.

## Project Structure

```
agent-sandbox/
  go.work

  cmd/agent-sandbox/        ← CLI binary
    go.mod
    main.go
    cmd/                    ← cobra commands
    plugins.go              ← registry

  sdk/                      ← Plugin SDK
    go.mod
    plugin.go
    contributions.go

  gateway/                  ← Universal gateway binary
    go.mod
    cmd/gateway/main.go
    proxy.go, sni.go, mitm.go, injector_registry.go

  bridge/                   ← Bridge runtime (TypeScript)
    package.json
    src/index.ts, agent.ts, plugin-loader.ts, types.ts

  plugins/
    codex/      (go.mod, plugin.go)
    claude-code/(go.mod, plugin.go)
    github/     (go.mod, plugin.go, plugin_test.go)
    mcp-oauth/  (go.mod, plugin.go)
    static-header/ (go.mod, plugin.go)
    docker/     (go.mod, plugin.go)
    telegram/   (go.mod, plugin.go, bridge/src/telegram.ts)
    slack/      (go.mod, plugin.go, bridge/src/slack.ts)

  internal/
    compose/    ← docker-compose.yml generation
    dockerfile/ ← Dockerfile generation
    config/     ← agent.yaml + fleet.yaml parsing
    merge/      ← contribution merging + conflict detection

  templates/    ← entrypoint.sh, etc.
```
