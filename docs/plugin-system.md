# Plugin System

## Design

Two plugin types with clear separation:

- **RuntimePlugin** — sets base image + installs agent CLI. One per agent.
- **FeaturePlugin** — additive capabilities (credentials, channels, Docker, home). Multiple per agent.

## Interfaces

```go
package sdk

type RuntimePlugin interface {
    Name() string
    ConfigSchema() ConfigSchema
    Contribute(ctx ContributeContext) (*RuntimeContributions, error)
}

type RuntimeContributions struct {
    BaseImage string     // e.g. "node:22-slim"
    Commands  []string   // install agent CLI (RUN instructions)
    Cmd       []string   // what bridge spawns (e.g. ["codex", "--headless"])
}

type FeaturePlugin interface {
    Name() string
    ConfigSchema() ConfigSchema
    Contribute(ctx ContributeContext) (*FeatureContributions, error)
}

type FeatureContributions struct {
    Image      *ImageContribution
    Gateway    *GatewayContribution
    Bridge     *BridgeContribution
    Compose    *ComposeContribution
    Entrypoint *EntrypointContribution
}
```

Minimal interfaces. All runtime logic lives inside Contributions (gateway handler, bridge source, entrypoint hooks). CLI calls `Contribute()` at build time to generate artifacts. Gateway and bridge also call `Contribute()` at startup to get their runtime handlers.

## Feature Contributions (Grouped by Concern)

```go
type FeatureContributions struct {
    Image      *ImageContribution
    Gateway    *GatewayContribution
    Bridge     *BridgeContribution
    Compose    *ComposeContribution
    Entrypoint *EntrypointContribution
}
```

Each sub-struct is nil if the plugin doesn't contribute to that concern.

```go
// What goes into the Dockerfile (additive, cannot change base image)
type ImageContribution struct {
    Files     []File       // COPY into image (embed.FS source + dest path)
    Commands  []string     // RUN commands (no FROM/ENTRYPOINT allowed)
}

// What the gateway needs at runtime
type GatewayContribution struct {
    Hosts      []string                                    // hosts this plugin handles
    NewHandler func(cfg map[string]any) (RequestHandler, error)  // factory for runtime handler
}

type RequestHandler interface {
    HandleRequest(req *http.Request) error
}

// CLI uses Hosts to generate gateway-config.yaml.
// Gateway binary calls Contribute() at startup → uses NewHandler to create runtime handlers.
// Same pattern as bridge loading TypeScript from BridgeContribution.Source.

// Channel plugin for the bridge
type BridgeContribution struct {
    Name   string          // plugin name ("telegram", "slack")
    Source embed.FS        // TypeScript source to extract
    Config map[string]any  // runtime config passed to bridge
}

// TypeScript interface that channel plugins must implement:
//
//   export interface ChannelPlugin {
//       name: string;
//       start(config: Record<string, any>): Promise<void>;
//       stop(): Promise<void>;
//       onMessage(handler: (msg: IncomingMessage) => void): void;
//       send(msg: OutgoingMessage): Promise<void>;
//   }
//
// Bridge dynamically imports from /opt/bridge/plugins/<name>/
// and calls start() with the Config from BridgeContribution.

// Docker Compose service definition
type ComposeContribution struct {
    Services map[string]Service
    Volumes  []string
    Ports    []string
    Env      []EnvVar
}

type EnvVar struct {
    Key      string
    Value    string
    Strategy EnvStrategy  // Override | ErrorIfConflict | Append
}

// Scripts that run in entrypoint before agent starts
type EntrypointContribution struct {
    Hooks []Hook
}

type Hook struct {
    Name     string    // for logging: "[entrypoint] running: github-setup"
    Source   embed.FS  // script content
    Priority int       // execution order (lower = runs first)
}
```

## Why Grouped

- **Clear ownership**: each generator (Dockerfile, compose, gateway) only reads its own sub-struct
- **Explicit conflicts**: `EnvVar.Strategy` declares how to handle duplicates
- **Ordered execution**: `Hook.Priority` controls entrypoint hook ordering
- **Conflict detection**: same host claimed by two plugins → error at merge time
- **Nil = not contributed**: plugin only fills what it needs, rest is nil

## Module Structure

```
plugins/<name>/
  go.mod              ← independent Go module
  plugin.go           ← implements sdk.Plugin
  plugin_test.go
  hooks/              ← entrypoint scripts (optional)
  bridge/             ← TypeScript channel code (optional)
```

## Registry (Compile-Time)

```go
// cmd/agent-sandbox/plugins.go
var Runtimes = []sdk.RuntimePlugin{
    codex.New(), claudecode.New(), pi.New(),
}

var Features = []sdk.FeaturePlugin{
    github.New(), mcpoauth.New(), staticheader.New(),     // credentials
    docker.New(), telegram.New(), slack.New(),              // features/channels
    homevc.New(),                                          // home-version-control
}
```

All plugins compiled into one CLI binary and one gateway binary. No per-agent compilation. Config determines which runtime + features are active.

CLI enforces at config load time: `runtime:` must match exactly one RuntimePlugin. `features:` keys must each match a FeaturePlugin.
