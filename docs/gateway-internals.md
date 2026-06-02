# Gateway Internals

The gateway is a transparent egress proxy that runs as a separate container alongside the agent. All outbound TCP from the agent routes through it via default route manipulation. Its job is to intercept specific TLS connections for credential injection while passing everything else through untouched.

## Architecture

```
Agent outbound TCP → Gateway TCP listener (:443)
                          │
                          ├─ Read TLS ClientHello
                          ├─ Extract SNI (server name)
                          │
                          ├─ SNI matches MITM domain?
                          │    YES → mitm.Handler (terminate TLS, rewrite HTTP, forward)
                          │    NO  → passthrough (pipe bytes directly to upstream)
                          │
Agent DNS queries  → Gateway DNS server (:53)
                          └─ Forward to 8.8.8.8
```

## Module Layout

```
gateway/
├── cmd/gateway/main.go            ← entrypoint, wiring
└── internal/
    ├── proxy/
    │   ├── config.go              ← Config structs, YAML loading
    │   ├── proxy.go               ← TCP listener, SNI routing, passthrough
    │   ├── sni.go                 ← TLS ClientHello SNI extraction
    │   └── forward.go             ← Generic TCP port forwarder
    ├── mitm/
    │   ├── mitm.go                ← MITM handler (TLS termination + HTTP pipeline)
    │   ├── cert.go                ← CA loading, on-demand cert generation, CertCache
    │   ├── telegram.go            ← TelegramRewriter (URL path token swap)
    │   └── auth_header.go         ← AuthHeaderRewriter (header injection)
    ├── redact/
    │   └── handler.go             ← slog handler that masks secrets in logs
    └── dns/
        └── dns.go                 ← UDP DNS forwarder
```

## Key Interfaces

### RequestHandler (proxy package)

The central routing contract. The proxy iterates registered handlers on every connection:

```go
type RequestHandler interface {
    Matches(host string) bool
    Handle(clientConn net.Conn, initialData []byte, serverName string)
}
```

### Rewriter (mitm package)

Modifies HTTP requests in-place before they're forwarded upstream:

```go
type Rewriter interface {
    RewriteRequest(req *http.Request) bool
}
```

Built-in implementations:
- `TelegramRewriter` — swaps dummy bot token in URL path with real `TELEGRAM_BOT_TOKEN`
- `AuthHeaderRewriter` — injects an auth header (supports `${value}` and `${base64_basic}` templates)

## Connection Flow

1. Agent makes outbound TCP connection (e.g., `curl https://github.com`)
2. Default route sends packet to gateway container
3. Gateway's TCP listener accepts the connection
4. Reads first 4096 bytes — expects a TLS ClientHello
5. Calls `extractSNI()` to parse the server name from the ClientHello
6. Iterates registered `RequestHandler`s, calling `Matches(serverName)`
7. **Match found** → delegates to handler (currently only `mitm.Handler`)
8. **No match** → passthrough: dials `serverName:443`, replays the ClientHello bytes, then pipes bidirectionally (`io.Copy` both directions with half-close)

The passthrough path preserves end-to-end TLS — the gateway never sees plaintext for non-MITM'd hosts.

## DNS Server

The gateway runs a UDP DNS forwarder on port 53. The agent's `/etc/resolv.conf` points to the gateway IP.

- All queries forwarded to `8.8.8.8:53`
- Prevents DNS-based proxy bypass (agent can't resolve names through an alternative path)
- Simple packet relay — no caching, no filtering

## MITM Pipeline

When a connection matches a MITM domain, `mitm.Handler` takes over:

1. **Certificate generation** — `CertCache.GetOrCreate(domain, caCert)` generates an ECDSA P-256 leaf cert signed by the sandbox CA. Cached per domain (thread-safe, double-checked locking).
2. **TLS handshake** — wraps the client connection in a `prefixConn` (replays the already-read ClientHello bytes), then performs a `tls.Server` handshake using the generated cert.
3. **HTTP request loop** (keep-alive aware):
   - `http.ReadRequest` from the decrypted stream
   - Apply each `Rewriter` in order (token swap, header injection, etc.)
   - Forward the modified request to real upstream via fresh TLS connection
   - Write upstream response back to agent over the MITM'd connection
   - Repeat until `Connection: close` or EOF

The agent's TLS client trusts the sandbox CA (installed during Docker build via `update-ca-certificates`), so the MITM is transparent.

## Log Redaction

Gateway logs are protected with two layers via the `redact.Handler` (wraps `slog`):

1. **Key-based** — attributes named `token`, `authorization`, `api_key`, etc. are always redacted
2. **Value-based** — all string attribute values are scanned for known secret substrings (collected from rewriter env vars at startup) and replaced with `[REDACTED]`

This prevents credentials from leaking into container logs even if a handler inadvertently logs request details.

## Adding a New Rewriter

1. Create a new file in `gateway/internal/mitm/` (e.g., `my_rewriter.go`)
2. Implement the `Rewriter` interface:

```go
type MyRewriter struct {
    domains []string
    secret  string
}

func (r *MyRewriter) RewriteRequest(req *http.Request) bool {
    // Check if this request is for one of our domains
    // Modify req in-place (add headers, rewrite URL, etc.)
    // Return true if modified, false otherwise
    return true
}
```

3. Add a case in `buildRewriters()` in `cmd/gateway/main.go` to instantiate it from config
4. Declare the MITM domains in your plugin's `feature.yaml` under `gateway.hosts`

The gateway binary is recompiled during Docker build, so new rewriters are picked up automatically when you `agent-sandbox compose up --build`.
