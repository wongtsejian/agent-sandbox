# Decisions

## Key Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | Two plugin types (Runtime + Feature) | Runtime is singular (base image). Features are additive. Config reads naturally. |
| 2 | Universal gateway binary | Build once, configure per-agent. No per-agent compilation. |
| 3 | Bridge always entrypoint | Agent is always a child process. No WrapCmd hack. |
| 4 | Runtime is a plugin (auto-enabled) | New runtime = new plugin. No CLI hardcoding. |
| 5 | Allow-all egress default | Dev agents need unrestricted installs. MITM only where needed. |
| 6 | Gateway inside each container | Self-contained. Per-agent config without routing complexity. |
| 7 | Compile-time plugin import | Single binary. Type-safe. No runtime discovery. |
| 8 | Home override via /opt staging | Volume hides /home/agent. Staging + entrypoint cp ensures configs win. |
| 9 | Channels are bridge sub-plugins | Messaging is bridge's concern. Plugin embeds TypeScript via go:embed. |
| 10 | All credentials through gateway | Even bridge gets dummy tokens. Real creds never in container env. |
| 11 | Gateway + bridge source via go:embed | CLI is self-contained. Docker multi-stage compiles gateway. No pre-built downloads. |
| 12 | Optional fleet.yaml | Single agent first-class. Multi-agent additive. |
| 13 | UDP restricted | DNS redirected to gateway resolver. All other UDP dropped. Prevents tunneling. |

## Comparison with agent-fleet

| Aspect | agent-fleet | agent-sandbox |
|--------|-------------|---------------|
| Config | fleet.yaml + agent.yaml | One agent.yaml (fleet optional) |
| Egress rules | User writes manually | Auto-derived from plugins |
| Runtime | Provider + render.sh | Plugin (auto-enabled by config) |
| Extensibility | Shell scripts | Go modules (declarative) |
| Home customization | user_base + init_scripts + Dockerfile | `home/` dir (auto-override) |
| Packages | Write Dockerfile template | YAML list or packages.sh |
| Docker access | Egress rule provider | `docker: true` |
| Deploy | generate → compose up | `up` |
| Egress default | Deny | Allow all |
| Gateway | Sidecar container | Inside agent container |

## Open Questions

1. **Plugin versioning** — SDK breaking changes? Version pinning?
2. **Custom egress restrictions** — Option for default-deny mode?
3. **External plugins** — Beyond built-in? Go module proxy?
4. **Resource limits** — CPU/memory per agent? Configurable?
5. **Health checks** — Detect agent crash vs idle?
6. **Auth flow** — Codex device flow, claude login — how to handle first-time auth?

## Maintainability

### Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| CLI binary size (go:embed gateway + bridge) | ~50-80MB binary | Accept it. Single binary distribution is worth the size. |
| SDK interface changes break all plugins | All plugins need update | Semantic versioning. Keep interface minimal. Add fields (additive), not methods. |
| Multi-stage build adds time | First build ~60s | Docker layer cache. Gateway stage rarely changes. Subsequent builds ~5s. |
| Two languages (Go + TypeScript) | Higher maintenance | Clear boundary: Go = build-time + proxy. TypeScript = messaging. No overlap. |
| Gateway fix requires rebuild | Slow rollout | `agent-sandbox rebuild`. Docker cache means only gateway stage rebuilds (~10s). |

### Interface Stability

```go
// sdk v1 — keep forever
type Plugin interface {
    Name() string
    ConfigSchema() ConfigSchema
    Contribute(ctx ContributeContext) (*Contributions, error)
}

// New capabilities → new fields (non-breaking)
type ImageContribution struct {
    Files    []File    // v1.0
    Commands []string  // v1.0
    // ...
    CacheFrom []string // v1.3 — added later, nil = not used
}
```

Rule: never remove or rename fields. Only add. Plugins compiled against sdk v1.0 work with sdk v1.x.

### Upgrade Path

| Change | User action |
|--------|-------------|
| New plugin available | `agent-sandbox upgrade` |
| Plugin/gateway/bridge fix | `agent-sandbox upgrade` → `agent-sandbox rebuild` |
| Config schema change | Edit agent.yaml → `agent-sandbox up` |

All upgrades: upgrade CLI → rebuild containers. No migration scripts. No state to migrate.

### What NOT to Build (YAGNI)

- Plugin marketplace (until >20 community plugins exist)
- Hot reload (rebuild is fast enough)
- Web UI dashboard (CLI + Telegram is sufficient)
- Multi-host orchestration (use k8s)
- Plugin dependency resolution (keep plugins independent)
- Config migration tool (simple YAML, manual edit is fine)
