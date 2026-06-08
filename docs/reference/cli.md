# CLI Reference

## Commands

| Command | Description |
|---------|-------------|
| `agent-sandbox init` | Interactive project scaffold |
| `agent-sandbox generate` | Read config, generate `.build/` artifacts |
| `agent-sandbox compose ...` | Docker compose passthrough |
| `agent-sandbox audit` | Verify running sandbox meets security contract |
| `agent-sandbox upgrade` | Self-update to latest GitHub release |

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-C, --dir` | `.` | Project directory containing agent.yaml or fleet.yaml |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `AGENT_SANDBOX_RUNTIME` | Override container runtime binary (`docker` or `podman`). Takes priority over `runtime_engine` in config. |
| `AGENT_SANDBOX_CACHE` | Override core cache directory (default: `~/.cache/agent-sandbox/` or `~/Library/Caches/agent-sandbox/`) |

## generate

Reads `agent.yaml` (or `fleet.yaml` for multi-agent) and produces the `.build/` directory containing all Docker artifacts.

```bash
agent-sandbox generate
agent-sandbox -C examples/multi-agent generate
```

**Output:**
- `.build/Dockerfile` — agent container image
- `.build/docker-compose.yml` — all services
- `.build/entrypoint.sh` — iptables + CA + privilege drop
- `.build/gateway-src/` — gateway Go source (compiled during Docker build)
- `.build/schema.json` — JSON Schema for agent.yaml

## compose

Passthrough to `docker compose` (or `podman compose`) with auto-injected flags:

- `-f .build/docker-compose.yml`
- `--project-name <folder-name>`
- `--env-file .env` (if .env exists)

```bash
agent-sandbox compose up --build -d     # build + start detached
agent-sandbox compose down -v           # stop + remove volumes
agent-sandbox compose logs -f           # stream all logs
agent-sandbox compose logs agent-001    # one service
agent-sandbox compose exec -it --user agent coder codex   # exec into agent
agent-sandbox compose ps                # status
agent-sandbox compose restart coder     # restart one service
```

## audit

Runs security checks against a live running sandbox. See [Audit Reference](audit.md) for details.

```bash
agent-sandbox audit
agent-sandbox -C examples/multi-agent audit
```

Exit code is non-zero if any check fails.

## init

Interactive scaffold that creates `agent.yaml` and `.env.example`:

```bash
mkdir my-agent && cd my-agent
agent-sandbox init
```

Asks for agent name and runtime. Auto-detects `gh auth token` if available.

## upgrade

Self-updates to the latest GitHub release:

```bash
agent-sandbox upgrade
```

## Typical Workflow

```bash
# First time
agent-sandbox generate
agent-sandbox compose up --build -d
agent-sandbox audit

# After config changes
agent-sandbox generate
agent-sandbox compose up --build -d

# Tear down
agent-sandbox compose down -v
```
