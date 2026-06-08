# Audit

The `agent-sandbox audit` command verifies that a running sandbox meets the security contract. It checks container isolation, secret handling, network interception, and certificate trust.

## Usage

```bash
# Single agent
agent-sandbox audit

# Fleet (audits each agent)
agent-sandbox -C examples/multi-agent audit
```

The sandbox must be running (`agent-sandbox compose up -d`) before auditing. Exit code is non-zero if any check fails.

## Checks

| Check | What it verifies | How |
|-------|-----------------|-----|
| Agent container running | Container is up and responsive | `docker inspect` state |
| Gateway container running | Gateway container is up | `docker inspect` state |
| Agent can reach external HTTPS | TLS proxy chain works end-to-end | `curl https://httpbin.org` from agent |
| Secret isolation | Real credentials from `.env` are not in agent env | Compares `.env` values against `env` output inside container |
| DNS through gateway | DNS intercept is active | `getent hosts <mitm-domain>` resolves |
| Gateway CA trusted | Agent trusts the gateway's MITM certificate | CA file exists at `/usr/local/share/ca-certificates/gateway-ca.crt` |
| Traffic interception rules | iptables DNAT is active | `iptables -t nat -L OUTPUT -n` shows `DNAT tcp dpt:443` |
| Default route to gateway | HTTPS traffic actually reaches the gateway | DNAT target contains gateway IP on `:8443` |

## Output

```
Auditing agent-001...

  ✓ Agent can reach external HTTPS
  ✓ Secret isolation
  ✓ DNS through gateway
  ✓ Gateway CA trusted
  ✓ Traffic interception rules
  ✓ Default route to gateway

Auditing agent-002...

  ✓ Agent can reach external HTTPS
  ✓ Secret isolation
  ✓ DNS through gateway
  ✓ Gateway CA trusted
  ✓ Traffic interception rules
  ✓ Default route to gateway

12/12 checks passed
```

## Failure Troubleshooting

| Failed check | Common cause | Fix |
|-------------|-------------|-----|
| Agent container running | Containers not started | `agent-sandbox compose up -d` |
| Agent can reach external HTTPS | Gateway not healthy yet, or CA not installed | Wait for gateway health, check `compose logs` |
| Secret isolation | Secret leaked via plugin env or extra_builds ENV | Remove real credentials from agent container env; use gateway injection |
| DNS through gateway | DNS not intercepted (port 53 not forwarded) | Check entrypoint ran successfully: `compose logs <agent>` |
| Gateway CA trusted | Entrypoint didn't install CA | Check that `/shared/certs/ca.crt` volume is mounted |
| Traffic interception rules | iptables setup failed | Agent needs `NET_ADMIN` capability (set by generated compose) |
| Default route to gateway | DNAT target IP wrong | Regenerate (`agent-sandbox generate`) and restart |

## CI Integration

The audit runs automatically in CI after the examples smoke test:

```yaml
- name: Audit security contract
  run: ./agent-sandbox -C examples/${{ matrix.example }} audit
```

Add a short delay after `compose up` to let entrypoints complete:

```yaml
- name: Wait for agent entrypoint
  run: sleep 5
```
