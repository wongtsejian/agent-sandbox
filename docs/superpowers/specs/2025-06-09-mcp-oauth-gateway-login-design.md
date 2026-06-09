# mcp-oauth Gateway Login Flow

## Problem

When an AI coding agent (e.g., Codex) runs inside a Docker container and needs to authenticate with an MCP provider (e.g., Notion), its native OAuth flow fails. The agent starts a callback server on `127.0.0.1:<random-port>` inside the container, but the user's browser is on the host — the redirect URL is unreachable.

## Solution

The gateway owns the entire OAuth lifecycle. Users trigger login via a simple HTTP request to the gateway's published port on the host. The gateway performs DCR, PKCE, and token exchange. The agent never does its own OAuth — it talks to MCP endpoints through the transparent proxy, and the gateway injects auth headers.

## Architecture

```
Host browser                          Gateway (localhost:8080)              MCP Provider
    │                                        │                                      │
    │  curl .../login/notion                 │                                      │
    │──────────────────────────────────────▶│                                      │
    │                                        │── DCR (if no client_id yet) ────────▶│
    │                                        │◀─ client_id + client_secret ─────────│
    │                                        │                                      │
    │                                        │── generate PKCE code_verifier        │
    │                                        │── store {state → verifier, provider} │
    │  ◀── {authorize_url} ─────────────────│                                      │
    │                                        │                                      │
    │── open authorize_url in browser ──────────────────────────────────────────────▶│
    │◀───────────────────── redirect to localhost:8080/plugins/mcp-oauth/callback ──│
    │                                        │                                      │
    │── GET /callback?code=X&state=Y ──────▶│                                      │
    │                                        │── POST token endpoint (code+verifier)▶│
    │                                        │◀─ access_token + refresh_token ──────│
    │                                        │── store token on disk                │
    │  ◀── "Login successful" ──────────────│                                      │
```

```
Agent container                       Gateway                              MCP Provider
    │                                        │                                      │
    │── request to mcp.notion.com/mcp ─────▶│                                      │
    │  (no auth header)                      │── lookup token for mcp.notion.com    │
    │                                        │── inject Authorization: Bearer <tok> │
    │                                        │── forward request ──────────────────▶│
    │                                        │◀─ response ─────────────────────────│
    │◀─ response ───────────────────────────│                                      │
```

## Gateway Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/plugins/mcp-oauth/login/{provider}` | GET | Initiate login — does DCR if needed, returns authorize URL |
| `/plugins/mcp-oauth/callback` | GET | OAuth callback — exchanges code for token, stores it |

## Login Endpoint Behavior

`GET /plugins/mcp-oauth/login/{provider}`

1. Look up `{provider}` in the configured providers map (from plugin options)
2. If no client registration exists for this provider:
   - Discover the authorization server metadata via RFC 8414 (`.well-known/oauth-authorization-server` on the MCP URL's origin)
   - Perform Dynamic Client Registration (DCR) at the registration endpoint
   - Persist `client_id` and `client_secret` to disk
3. Generate a PKCE `code_verifier` (random 43-128 chars) and derive `code_challenge` (S256)
4. Generate a unique `state` parameter
5. Store pending flow: `{state → code_verifier, provider_name}`
6. Construct authorize URL with:
   - `response_type=code`
   - `client_id` from registration
   - `code_challenge` + `code_challenge_method=S256`
   - `redirect_uri=http://<public_url>/plugins/mcp-oauth/callback`
   - `state`
   - `resource=<mcp_url>` (per MCP OAuth spec)
7. Return JSON: `{"authorize_url": "..."}`

## Callback Endpoint Behavior

`GET /plugins/mcp-oauth/callback?code=X&state=Y`

1. Look up `state` in pending flows map
2. If not found → 400 error (expired or invalid)
3. Retrieve `code_verifier` and `provider_name`
4. POST to provider's token endpoint with:
   - `grant_type=authorization_code`
   - `code`
   - `code_verifier`
   - `redirect_uri` (same as in authorize request)
   - `client_id` + `client_secret`
5. Receive `access_token`, `refresh_token`, `expires_in`
6. Write token to disk: `<token_dir>/<provider>.json`
7. Delete pending flow entry
8. Return HTML: "Login successful. You can close this tab."

## Token Injection Middleware

For each outgoing request from the agent:

1. Match request destination against configured provider MCP URLs
2. If match found and token file exists:
   - Check expiry — if expired, attempt refresh using `refresh_token`
   - If refresh succeeds, update token file
   - If refresh fails, return 401 with message: "Token expired, re-run login"
   - Inject `Authorization: Bearer <access_token>` header
3. Forward request to destination

## State & Persistence

| Data | Storage | Lifetime |
|------|---------|----------|
| Client registrations (client_id, client_secret) | Disk: `<token_dir>/<provider>_client.json` | Permanent (reusable across logins) |
| Pending auth flows (state → verifier, provider) | In-memory map | Short-lived; cleared on callback or gateway restart |
| Tokens (access_token, refresh_token, expires_at) | Disk: `<token_dir>/<provider>.json` | Until expiry or re-login |

All disk state lives under `token_dir` (default `/data/oauth-tokens`), which is a Docker volume persisted across container restarts.

## Configuration

In `agent.yaml`:

```yaml
plugins:
  mcp-oauth:
    providers:
      notion:
        mcp_url: "https://mcp.notion.com/mcp"
```

The gateway derives authorization server metadata from the `mcp_url` using RFC 8414 well-known discovery.

The `public_url` (from `gateway.public_url` in agent.yaml) determines the `redirect_uri` base. For local development this is `http://localhost:8080`.

## User Experience

```bash
# Containers already running
$ curl http://localhost:8080/plugins/mcp-oauth/login/notion
{"authorize_url": "https://mcp.notion.com/authorize?response_type=code&client_id=...&redirect_uri=http://localhost:8080/plugins/mcp-oauth/callback&state=...&code_challenge=...&code_challenge_method=S256&resource=https://mcp.notion.com/mcp"}

# Open the authorize_url in browser
# Authorize on Notion
# Browser redirects to localhost:8080/plugins/mcp-oauth/callback
# Gateway handles token exchange
# Browser shows "Login successful"

# Agent can now use Notion MCP transparently — no config change, no restart needed
```

## Edge Cases

- **Token expiry** — Middleware refreshes automatically using refresh_token
- **Refresh token expired** — Middleware returns 401; user re-runs curl login
- **Multiple providers** — Each gets its own DCR registration + token, keyed by provider name
- **Concurrent logins** — State map keyed by unique `state` param; no conflicts
- **Gateway restart** — Client registrations and tokens persist on disk. Pending auth flows (in-memory) are lost; user re-runs curl if mid-flow
- **Unknown provider in login URL** — Return 404 with list of configured providers
- **DCR failure** — Return error with upstream message; user checks provider config
- **Agent requests before login** — No token file exists; middleware returns 401 to the agent with a message indicating the provider needs login (e.g., "Run: curl http://localhost:8080/plugins/mcp-oauth/login/notion")

## Out of Scope

- Plugin CLI infrastructure (`agent-sandbox plugin <name> <cmd>`)
- Pre-compiled plugin binaries
- stdout URL rewriting / pty wrappers
- Port forwarding from host to agent container
- Agent-side OAuth flows (agent never does its own auth)
