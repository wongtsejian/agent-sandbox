# Security

## Egress Model

**Allow all by default.** MITM only for hosts where credential injection is needed. Everything else passes through with end-to-end TLS preserved.

Rationale: Dev agents need `npm install`, `pip install`, `curl` arbitrary URLs. Default deny creates too much friction.

## Transparent Proxy

```bash
# TCP: redirect to gateway (kernel enforced, agent cannot bypass)
iptables -t nat -A OUTPUT -p tcp -m owner ! --uid-owner gateway -j REDIRECT --to-port 8443
# UDP: only DNS allowed (to gateway's resolver), rest dropped
iptables -A OUTPUT -p udp --dport 53 -m owner ! --uid-owner gateway -j REDIRECT --to-port 8053
iptables -A OUTPUT -p udp -m owner ! --uid-owner gateway -j DROP
```

## Credential Flow

```
Agent → api.github.com:443
  → iptables redirects to gateway:8443
  → Gateway reads SNI: "api.github.com"
  → Matches github plugin rule → MITM mode
  → Terminates TLS (sandbox CA), reads HTTP request
  → Strips dummy token, injects real PAT
  → Opens TLS to real api.github.com, forwards
  → Agent receives response (thinks it talked directly)
```

Agent never sees real credentials. Bridge gets dummy tokens. Real creds exist only in gateway memory.

## Docker Access

When `docker: true`, the docker plugin contributes a DinD sidecar. The gateway itself handles Docker API validation — no separate proxy container needed.

1. Agent runs `docker run ...` → connects to `dind:2375`
2. Gateway intercepts (iptables, like all TCP)
3. Gateway's `DockerHandler` validates the request (block privileged, host binds)
4. Injects gateway redirect into spawned container config
5. Forwards to real DinD

Docker API is HTTP (not HTTPS), so no MITM/TLS needed — plain HTTP inspection.

Spawned containers:
- Forced onto internal network
- iptables injected to redirect egress → agent's gateway (0.0.0.0:8443)
- Cannot spawn further containers (no DOCKER_HOST env)

## Hardening

### Attack Surface

| Vector | Mitigation |
|--------|------------|
| Agent reads bridge env vars via /proc | Bridge only has dummy tokens. Real creds in gateway (different user, hidepid=2). |
| Agent kills gateway | Gateway runs as `gateway` user. Agent cannot signal it. |
| Agent modifies iptables | NET_ADMIN dropped after entrypoint. Agent has no capabilities. |
| Agent modifies gateway config | Config owned by root, mode 0444. |
| Agent ptraces bridge | hidepid=2 + different UIDs + no-new-privileges. |
| DNS tunneling | DNS redirected to gateway's resolver. No raw UDP allowed. |
| DinD direct access | DinD uses TLS client cert auth. Cert in gateway-owned path. |
| Resource exhaustion | mem_limit, cpus, pids_limit per container. |

### Container Security

```yaml
services:
  agent:
    cap_drop: [ALL]
    cap_add: [NET_ADMIN]        # entrypoint only, dropped after
    security_opt: [no-new-privileges:true]
    read_only: true
    tmpfs: [/tmp, /run]
    mem_limit: 4g
    cpus: 2
    pids_limit: 256
```

### File Permissions

```
/etc/gateway/config.yaml    root:root       0444
/etc/gateway/ca.key         gateway:gateway 0400
/usr/local/bin/gateway      root:root       0555
/home/agent/                agent:agent     0750 (writable, volume)
```

### Failure Modes

| Failure | Behavior |
|---------|----------|
| Gateway crashes | All TCP fails (safe default). Bridge detects, restarts or exits. |
| Bridge crashes | Agent dies (child). Docker restart policy recovers. |
| DinD crashes | Docker commands fail. Agent retries. |
