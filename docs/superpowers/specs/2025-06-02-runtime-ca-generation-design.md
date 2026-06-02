# Runtime CA Generation Design

> Eliminate `.build/certs/` by generating the CA keypair at gateway startup and sharing only the public cert via a Docker volume.

## Context

Currently, `agent-sandbox generate` creates a self-signed CA keypair (`ca.crt` + `ca.key`) and writes them to `.build/certs/`. These files are then `COPY`'d into container images at Docker build time. This works but leaves sensitive key material on the host filesystem.

This design moves CA generation to runtime, removes all cert files from the host, and uses a shared Docker volume to distribute the public cert to the agent container.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| CA lifetime | Fresh every `docker compose up` | Ephemeral by default; minimal trust window |
| Key isolation | Single shared volume, only `ca.crt` written | Private key stays internal to gateway container |
| Agent readiness | `depends_on` healthcheck + entrypoint poll loop | Belt-and-suspenders; healthcheck for orchestration, poll for race conditions |
| Conditional behavior | Skip entirely when no MITM domains | No unnecessary volume/startup overhead |

## Architecture

```
docker compose up
│
├── Gateway container starts
│   ├── Generate ECDSA P-256 CA keypair in-memory
│   ├── Write ca.key → /etc/gateway/private/ca.key (internal only)
│   ├── Write ca.crt → /shared/certs/ca.crt (shared volume)
│   ├── Load keypair into MITM cert cache
│   └── Healthcheck passes (file-exists on /shared/certs/ca.crt)
│
└── Agent container starts (after gateway healthy)
    ├── Entrypoint polls for /usr/local/share/ca-certificates/ca.crt
    ├── Runs update-ca-certificates
    ├── Sets NODE_EXTRA_CA_CERTS
    └── Starts agent process
```

## Component Changes

### 1. Remove: `internal/generate/certs.go`

- Delete `GenerateCA()` function and its call site in the generator
- Delete `.build/certs/` directory creation
- Remove `certs_test.go`
- The generator no longer touches certificates at all

### 2. Modify: Gateway startup

The gateway gains a CA initialization step that runs before accepting connections:

```go
// gateway/internal/ca/generate.go (new)
func GenerateAndStore(sharedCertPath, privateKeyPath string) (*tls.Certificate, error) {
    // 1. Generate ECDSA P-256 keypair
    // 2. Create self-signed CA cert (O=agent-sandbox, CN=agent-sandbox CA, 24h validity)
    // 3. Write PEM-encoded cert to sharedCertPath (0644)
    // 4. Write PEM-encoded key to privateKeyPath (0600)
    // 5. Return parsed tls.Certificate for MITM use
}
```

Notes:
- Validity reduced from 10 years to 24 hours (ephemeral containers don't need long-lived CAs)
- The function is called during gateway `main()` before the proxy listener starts
- The existing `mitm.LoadCA()` is replaced by directly using the returned `*tls.Certificate`

### 3. Modify: Dockerfile templates

**Gateway Dockerfile** — Remove:
```dockerfile
COPY certs/ca.crt /etc/gateway/ca.crt
COPY certs/ca.key /etc/gateway/ca.key
```

Add:
```dockerfile
RUN mkdir -p /shared/certs /etc/gateway/private && \
    chmod 700 /etc/gateway/private
```

**Agent Dockerfile** — Remove:
```dockerfile
COPY certs/ca.crt /usr/local/share/ca-certificates/sandbox-ca.crt
RUN update-ca-certificates
ENV NODE_EXTRA_CA_CERTS=/usr/local/share/ca-certificates/sandbox-ca.crt
```

The agent Dockerfile no longer mentions certs at all. Trust is established at runtime via the entrypoint.

### 4. Modify: Compose template

Add a named volume and wire it up (only when MITM domains are configured):

```yaml
volumes:
  shared-certs:

services:
  gateway:
    volumes:
      - shared-certs:/shared/certs
    healthcheck:
      test: ["CMD", "test", "-f", "/shared/certs/ca.crt"]
      interval: 1s
      timeout: 1s
      retries: 10
      start_period: 2s

  agent:
    volumes:
      - shared-certs:/usr/local/share/ca-certificates:ro
    depends_on:
      gateway:
        condition: service_healthy
```

### 5. Modify: Agent entrypoint template

Add cert-waiting logic before the agent process starts:

```bash
#!/bin/sh
# --- CA cert trust (only present when MITM domains configured) ---
echo "Waiting for sandbox CA certificate..."
timeout=30
elapsed=0
while [ ! -f /usr/local/share/ca-certificates/ca.crt ]; do
  sleep 0.1
  elapsed=$((elapsed + 1))
  if [ "$elapsed" -ge "$((timeout * 10))" ]; then
    echo "ERROR: CA certificate not available after ${timeout}s" >&2
    exit 1
  fi
done
update-ca-certificates 2>/dev/null
export NODE_EXTRA_CA_CERTS=/usr/local/share/ca-certificates/ca.crt
# --- End CA cert trust ---

exec "$@"
```

This block is only templated into the entrypoint when at least one feature plugin declares MITM domains.

### 6. Modify: Gateway config template

Update `gateway-config.yaml` template:

```yaml
# Before:
ca_cert: /etc/gateway/ca.crt
ca_key: /etc/gateway/ca.key

# After:
ca_cert: /shared/certs/ca.crt
ca_key: /etc/gateway/private/ca.key
```

## Conditional Behavior

When no feature plugin declares MITM domains:
- Gateway skips CA generation entirely
- No `shared-certs` volume in compose output
- No cert-waiting logic in agent entrypoint
- No healthcheck on gateway (or a simpler one unrelated to certs)

This preserves the current behavior: MITM infrastructure only exists when needed.

## Security Properties

| Property | How it's achieved |
|----------|-------------------|
| Private key never on host | Generated inside container at runtime |
| Private key isolated from agent | Written to gateway-internal path, not shared volume |
| Agent can't sign certs | Only receives `ca.crt` (public), volume is read-only |
| Short-lived CA | 24h validity, regenerated every `docker compose up` |
| No persistent secrets | Named volume cleared by `docker compose down -v` |

## What Gets Deleted

- `internal/generate/certs.go`
- `internal/generate/certs_test.go`
- Any reference to `.build/certs/` in generator code
- `COPY certs/` lines from Dockerfile templates
- `ca_cert`/`ca_key` static paths from gateway config template

## Testing

- **Unit test**: Gateway CA generation produces valid cert + key, cert is a CA, key matches cert
- **Integration test**: `docker compose up` succeeds, agent can `curl https://` a MITM'd domain without TLS errors
- **Negative test**: Agent container cannot read `/etc/gateway/private/ca.key` (different container, path doesn't exist)
- **Conditional test**: When no MITM domains configured, no volume, no cert logic, compose still works
