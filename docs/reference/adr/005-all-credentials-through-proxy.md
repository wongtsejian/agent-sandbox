# ADR-005: All Credentials Through Proxy (Including Bot Token)

## Status
Accepted

## Context
Initially, we separated credential management:
- **Channel provider** managed its own platform credentials (Telegram bot token)
- **Gateway proxy** managed third-party API credentials (GitHub PAT, OAuth tokens)

This created two different credential models and meant the channel provider needed direct access to secrets (reading bot token from env).

## Decision
Route ALL credentials through the transparent proxy, including the Telegram bot token. The channel provider uses a dummy token — the proxy intercepts and injects the real one.

## Consequences

**Positive:**
- Unified credential model — one way to handle all secrets
- Channel provider never sees real credentials
- Simpler channel provider implementation (no secret management)
- All credential injection logic lives in one place (the proxy)
- Easier to audit — all secrets flow through one component
- Log output is protected — value-based redaction prevents credentials from leaking into logs even if they appear in unexpected fields (error messages, URL paths, etc.)

**Negative:**
- Proxy must support different injection strategies (not just headers):
  - Header injection (GitHub PAT: `Authorization: token <pat>`)
  - URL path rewrite (Telegram: `/bot<token>/sendMessage`)
  - Bearer token (OAuth: `Authorization: Bearer <token>`)
- Channel provider needs a dummy token to construct valid API URLs
- Slightly more complex proxy (must understand Telegram URL format)

**How it works:**
```
Channel manager calls: POST https://api.telegram.org/bot_dummy/sendMessage
  → Default route sends to gateway container
  → Gateway iptables redirects :443 to proxy :8443
  → Proxy reads SNI: api.telegram.org → matches telegram rewriter
  → Rewriter rewrites URL: /bot_dummy/ → /bot123456:ABC-DEF/
  → Proxy forwards to real Telegram API with real token
```

**Rewriter interface (current):**
The gateway uses typed rewriters that implement request transformation. Each rewriter type handles its own injection strategy:

```go
type Rewriter interface {
    RewriteRequest(req *http.Request) bool
}
```
