# CLI & UX

## Commands

```bash
agent-sandbox init [--runtime codex]    # interactive scaffold
agent-sandbox generate                  # read config → write .build/ artifacts
agent-sandbox validate                  # check config + resolve plugins
agent-sandbox plugins                   # list available plugins
agent-sandbox upgrade                   # self-update
agent-sandbox compose ...               # docker compose passthrough
```

### Compose Passthrough

Auto-injects `-f .build/docker-compose.yml` and `--project-name`. All compose commands work:

```bash
agent-sandbox compose up --build        # build + start
agent-sandbox compose up --build -d     # detached
agent-sandbox compose down              # stop + remove
agent-sandbox compose logs -f coder     # stream logs
agent-sandbox compose exec coder bash   # shell into agent
agent-sandbox compose ps                # status
agent-sandbox compose restart coder     # restart container
agent-sandbox compose build coder       # rebuild image only
```

### Typical Workflow

```bash
agent-sandbox generate                  # generate artifacts
agent-sandbox compose up --build -d     # build + start detached
agent-sandbox compose logs -f           # watch logs
```

After config changes:
```bash
agent-sandbox generate                  # regenerate
agent-sandbox compose up --build -d     # rebuild + restart
```

## UX Design

### Progressive Disclosure

```yaml
# Minimal (works immediately):
name: coder
runtime: codex

# Add credentials:
features:
  - plugin: github
    token: "${GITHUB_PAT}"

# Add channels:
features:
  - plugin: telegram
    access_control:
      allowed_users: ["@me"]

# Full power:
features:
  - plugin: github
    token: "${GITHUB_PAT}"
  - plugin: docker
  - plugin: telegram
    access_control:
      allowed_users: ["@me"]
  - plugin: custom-runtime
    commands: ["apt-get update && apt-get install -y --no-install-recommends ripgrep fd-find && rm -rf /var/lib/apt/lists/*"]
    entrypoint_hooks: [./scripts/sync-dotfiles.sh]
    runtime_volumes: ["agent-home:/home/agent"]
```

### Interactive Init

Auto-detects `gh auth token`, suggests plugins based on runtime, creates `.env` with detected credentials.

### Smart Validation

```bash
$ agent-sandbox validate
⚠ runtime 'codex' typically needs 'openai' plugin for API access.
✓ Config valid (1 warning)
```

### Helpful Errors

```bash
✗ Plugin 'github' failed: token is invalid or expired
  Fix: gh auth refresh && agent-sandbox up
```

## DX (Plugin Authors)

### Creating a New Plugin

Core plugins live in `internal/plugins/<name>/`. Each plugin needs:

1. `feature.yaml` — metadata and config schema
2. `plugin.go` — typed Config struct + `Register[C]()` call
3. `plugin_test.go` — tests

### Testing

```go
func TestResolve(t *testing.T) {
    contrib, err := resolve.ResolveFeature("github-pat", ".", map[string]any{
        "token": "ghp_test",
    })
    require.NoError(t, err)
    assert.Contains(t, contrib.MITMDomains, "api.github.com")
    assert.NotEmpty(t, contrib.Rewriters)
}
```
