# external-services

Connect the agent to external services — Docker containers on shared networks or HTTPS endpoints — with optional header injection for authentication.

## Config

```yaml
features:
  - plugin: external-services
    services:
      - url: docker://rkgw:8765
        network: rkgw-external
        headers:
          x-api-key: ${RKGW_API_KEY}

      - url: docker://redis:6379
        network: my-infra

      - url: https://api.github.com
        headers:
          Authorization: Bearer ${GITHUB_TOKEN}
```

### Service fields

| Field | Required | Description |
|-------|----------|-------------|
| `url` | yes | Service URL. Scheme determines transport: `docker://host:port` for plain HTTP on Docker networks, `https://host` for TLS endpoints. |
| `network` | for `docker://` | External Docker network the service lives on. Gateway joins this network. |
| `headers` | no | Headers to inject on every request. Values must use `${VAR}` references for secrets. |

### URL schemes

| Scheme | Transport | Port default | Gateway behavior |
|--------|-----------|--------------|-----------------|
| `docker://` | Plain HTTP | 80 | HTTP proxy with header injection. Gateway joins the Docker network and intercepts traffic via iptables. |
| `https://` | TLS | 443 | MITM with TLS termination. Gateway generates a per-domain cert signed by the sandbox CA, terminates TLS, injects headers, re-encrypts upstream. |

### Header values

Header values must contain exactly one `${VAR}` reference. The referenced environment variable is read by the gateway at startup and injected into matching requests. The agent never sees the real credential.

```yaml
headers:
  # Simple — value is just the secret
  x-api-key: ${MY_API_KEY}

  # With prefix — "Bearer " is prepended to the secret value
  Authorization: Bearer ${MY_TOKEN}
```

## How it works

```
┌─ Agent Container ─────────────────────────────┐
│  curl http://rkgw:8765/v1/messages            │
│    → iptables PREROUTING redirects to gateway │
└───────────────────────────────────────────────┘
         │ (internal network)
┌────────▼──────────────────────────────────────┐
│  Gateway                                      │
│    1. Detect HTTP (first byte != 0x16)        │
│    2. Parse HTTP request                      │
│    3. Match Host against rewriters            │
│    4. Inject x-api-key header                 │
│    5. Forward to rkgw:8765 (Docker DNS)       │
│                                               │
│  For HTTPS services:                          │
│    1. Detect TLS ClientHello                  │
│    2. Terminate TLS with sandbox CA cert      │
│    3. Parse HTTP, inject headers              │
│    4. Forward to real upstream over TLS       │
└───────────────────────────────────────────────┘
         │ (external Docker network)
┌────────▼──────────────────────────────────────┐
│  rkgw container (pre-existing)                │
│    receives request with x-api-key header     │
└───────────────────────────────────────────────┘
```

## Security

- **No secrets in agent** — credentials exist only in the gateway process
- **Environment variable references** — literal secrets in agent.yaml are rejected; must use `${VAR}`
- **Network isolation** — agent is on internal network only; gateway bridges to external networks
- **Ephemeral certs** — MITM certs are generated per-session, signed by a per-container CA

## Limitations

- **One env var per header** — each header value supports exactly one `${VAR}` reference
- **No per-path rules** — headers are injected on all requests to the host, regardless of path
- **Docker DNS required** — `docker://` services must be resolvable via Docker's embedded DNS (the service must be on the specified network)
