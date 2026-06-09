# mcp-oauth Login Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/plugins/mcp-oauth/login/{provider}` endpoint so users can trigger OAuth login via curl, with full PKCE support and proper state management between login initiation and callback completion.

**Architecture:** A new login route handler initiates OAuth (DCR + PKCE), stores pending flow state in a shared package-level map, and returns an authorize URL. The existing callback handler is updated to read the PKCE code_verifier from that shared state instead of using deterministic HMAC. Both handlers compile into the same `custom` package inside the gateway binary.

**Tech Stack:** Go, gateway SDK (`core/sdk/gateway`), Go templates (rendered at generate-time)

---

### File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `core/plugins/mcp-oauth/handlers/login.go` | Create | Login route handler: DCR + PKCE + authorize URL |
| `core/plugins/mcp-oauth/handlers/pkce.go` | Create | Shared PKCE state (pending flows map, types, helpers) |
| `core/plugins/mcp-oauth/handlers/callback.go` | Modify | Use PKCE state from shared map instead of HMAC |
| `core/plugins/mcp-oauth/plugin.yaml` | Modify | Add login route declaration |
| `core/plugins/mcp-oauth/middlewares/oauth.go` | Modify | Use PKCE in `mwBuildAuthorizeURL`, add `resource` param |
| `core/plugins/mcp-oauth/handlers/login_test.go` | Create | Tests for login handler |
| `core/plugins/mcp-oauth/handlers/callback_test.go` | Create | Tests for updated callback handler |

---

### Task 1: Create shared PKCE state module

**Files:**
- Create: `core/plugins/mcp-oauth/handlers/pkce.go`

- [ ] **Step 1: Write the PKCE state code**

```go
package custom

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

// PendingOAuthFlow stores state for an in-progress OAuth login.
type PendingOAuthFlow struct {
	Provider     string
	CodeVerifier string
	RedirectURI  string
	CreatedAt    time.Time
}

// pendingFlows maps state parameter → pending flow.
// Shared between login handler (writes) and callback handler (reads + deletes).
var pendingFlows = struct {
	sync.Mutex
	m map[string]*PendingOAuthFlow
}{m: make(map[string]*PendingOAuthFlow)}

// StorePendingFlow stores a new pending OAuth flow keyed by state.
func StorePendingFlow(state string, flow *PendingOAuthFlow) {
	pendingFlows.Lock()
	defer pendingFlows.Unlock()
	pendingFlows.m[state] = flow
}

// ConsumePendingFlow retrieves and removes a pending flow by state.
// Returns nil if not found or expired (>10 minutes).
func ConsumePendingFlow(state string) *PendingOAuthFlow {
	pendingFlows.Lock()
	defer pendingFlows.Unlock()
	flow, ok := pendingFlows.m[state]
	if !ok {
		return nil
	}
	delete(pendingFlows.m, state)
	if time.Since(flow.CreatedAt) > 10*time.Minute {
		return nil
	}
	return flow
}

// GenerateCodeVerifier generates a cryptographically random PKCE code_verifier (43-128 chars).
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeChallengeS256 derives the S256 code_challenge from a code_verifier.
func CodeChallengeS256(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// GenerateState generates a cryptographically random state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd core/plugins/mcp-oauth/handlers && go build ./...`
Expected: May fail because this is a template file (contains `{{ }}` markers in other files). If so, verify syntax only by checking `gofmt` on this file specifically.

Run: `gofmt -e core/plugins/mcp-oauth/handlers/pkce.go`
Expected: Clean output (no syntax errors)

- [ ] **Step 3: Commit**

```bash
git add core/plugins/mcp-oauth/handlers/pkce.go
git commit -m "feat(mcp-oauth): add shared PKCE state module"
```

---

### Task 2: Create login route handler

**Files:**
- Create: `core/plugins/mcp-oauth/handlers/login.go`

- [ ] **Step 1: Write the login handler**

```go
package custom

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/donbader/agent-sandbox/core/sdk/gateway"
)

type loginProviderConfig struct {
	MCPURL           string
	AuthorizeEndpoint string
	TokenEndpoint    string
	ClientID         string
	ClientSecret     string
	Scopes           string
}

var (
	loginProviders map[string]*loginProviderConfig
	loginTokenDir  string
	loginCallbackURL string
)

func init() {
	loginTokenDir = "{{ .options.token_dir }}"
	loginCallbackURL = "{{ .options.callback_url }}"
	providersJSON := `{{ toJSON .options.providers }}`

	loginProviders = make(map[string]*loginProviderConfig)
	var rawProviders map[string]map[string]any
	if err := json.Unmarshal([]byte(providersJSON), &rawProviders); err != nil {
		slog.Error("oauth-login: failed to parse providers", "error", err)
		return
	}

	for name, cfg := range rawProviders {
		p := &loginProviderConfig{}
		if v, ok := cfg["mcp_url"].(string); ok {
			p.MCPURL = v
		}
		if v, ok := cfg["authorize_endpoint"].(string); ok {
			p.AuthorizeEndpoint = v
		}
		if v, ok := cfg["token_endpoint"].(string); ok {
			p.TokenEndpoint = v
		}
		if v, ok := cfg["client_id"].(string); ok {
			p.ClientID = v
		}
		if v, ok := cfg["client_secret"].(string); ok {
			p.ClientSecret = v
		}
		if v, ok := cfg["scopes"].(string); ok {
			p.Scopes = v
		}
		loginProviders[name] = p
	}

	gateway.RegisterRoute(gateway.RouteDef{
		Path:    "{{ .path }}",
		Handler: handleOAuthLogin,
	})
}

func handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	// Extract provider name from path: /plugins/mcp-oauth/login/{provider}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "{{ .path }}"), "/")
	// pathParts[0] is empty (leading /), pathParts[1] is provider name
	var providerName string
	for _, part := range pathParts {
		if part != "" {
			providerName = part
			break
		}
	}

	if providerName == "" {
		// List available providers
		names := make([]string, 0, len(loginProviders))
		for name := range loginProviders {
			names = append(names, name)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"error":     "provider name required",
			"available": names,
			"usage":     "GET {{ .path }}/<provider_name>",
		})
		return
	}

	provider, ok := loginProviders[providerName]
	if !ok {
		names := make([]string, 0, len(loginProviders))
		for name := range loginProviders {
			names = append(names, name)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error":     fmt.Sprintf("unknown provider: %s", providerName),
			"available": names,
		})
		return
	}

	// Ensure we have client credentials (DCR if needed)
	if err := ensureClientRegistration(providerName, provider); err != nil {
		slog.Error("oauth-login: registration failed", "provider", providerName, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("client registration failed: %v", err),
		})
		return
	}

	// Generate PKCE
	codeVerifier, err := GenerateCodeVerifier()
	if err != nil {
		http.Error(w, "failed to generate code verifier", http.StatusInternalServerError)
		return
	}
	codeChallenge := CodeChallengeS256(codeVerifier)

	// Generate state
	state, err := GenerateState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}

	// Store pending flow
	StorePendingFlow(state, &PendingOAuthFlow{
		Provider:     providerName,
		CodeVerifier: codeVerifier,
		RedirectURI:  loginCallbackURL,
		CreatedAt:    time.Now(),
	})

	// Build authorize URL
	params := url.Values{
		"client_id":             {provider.ClientID},
		"response_type":        {"code"},
		"state":                {state},
		"redirect_uri":         {loginCallbackURL},
		"code_challenge":       {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	if provider.MCPURL != "" {
		params.Set("resource", provider.MCPURL)
	}
	if provider.Scopes != "" {
		params.Set("scope", provider.Scopes)
	}

	authorizeURL := provider.AuthorizeEndpoint + "?" + params.Encode()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"authorize_url": authorizeURL,
		"provider":      providerName,
		"instructions":  "Open the authorize_url in your browser to complete login.",
	})
}

// ensureClientRegistration checks for cached credentials or performs DCR.
func ensureClientRegistration(name string, provider *loginProviderConfig) error {
	if provider.ClientID != "" {
		return nil // Already have credentials (static config or previously resolved)
	}

	// Check cached registration file
	regFile := loginTokenDir + "/" + name + ".reg.json"
	if data, err := os.ReadFile(regFile); err == nil {
		var cached struct {
			AuthorizeEndpoint string `json:"authorize_endpoint"`
			TokenEndpoint     string `json:"token_endpoint"`
			ClientID          string `json:"client_id"`
			ClientSecret      string `json:"client_secret"`
		}
		if json.Unmarshal(data, &cached) == nil && cached.ClientID != "" {
			provider.AuthorizeEndpoint = cached.AuthorizeEndpoint
			provider.TokenEndpoint = cached.TokenEndpoint
			provider.ClientID = cached.ClientID
			provider.ClientSecret = cached.ClientSecret
			return nil
		}
	}

	// Perform discovery + DCR
	if provider.MCPURL == "" {
		return fmt.Errorf("no mcp_url configured for provider %s", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	meta, err := gateway.DiscoverOAuthMetadata(ctx, provider.MCPURL)
	if err != nil {
		return fmt.Errorf("metadata discovery: %w", err)
	}

	reg, err := gateway.RegisterOAuthClient(ctx, meta.RegistrationEndpoint, []string{loginCallbackURL}, "agent-sandbox:"+name)
	if err != nil {
		return fmt.Errorf("dynamic registration: %w", err)
	}

	// Update in-memory config
	provider.AuthorizeEndpoint = meta.AuthorizationEndpoint
	provider.TokenEndpoint = meta.TokenEndpoint
	provider.ClientID = reg.ClientID
	provider.ClientSecret = reg.ClientSecret

	// Persist for reuse across restarts
	regData, _ := json.MarshalIndent(map[string]string{
		"authorize_endpoint": meta.AuthorizationEndpoint,
		"token_endpoint":     meta.TokenEndpoint,
		"client_id":          reg.ClientID,
		"client_secret":      reg.ClientSecret,
	}, "", "  ")
	if err := os.MkdirAll(loginTokenDir, 0700); err != nil {
		slog.Warn("oauth-login: failed to create token dir", "error", err)
	}
	if err := os.WriteFile(regFile, regData, 0600); err != nil {
		slog.Warn("oauth-login: failed to cache registration", "error", err)
	}

	return nil
}
```

- [ ] **Step 2: Verify syntax**

Run: `gofmt -e core/plugins/mcp-oauth/handlers/login.go`
Expected: Clean output (no syntax errors)

- [ ] **Step 3: Commit**

```bash
git add core/plugins/mcp-oauth/handlers/login.go
git commit -m "feat(mcp-oauth): add login route handler with DCR + PKCE"
```

### Task 3: Update callback handler to use PKCE state

**Files:**
- Modify: `core/plugins/mcp-oauth/handlers/callback.go`

The current callback uses deterministic HMAC-based state (`hmac_sig:provider_name`). We need to update it to also check the shared `pendingFlows` map for PKCE flows initiated by the login endpoint, while keeping backward compatibility with the middleware-initiated HMAC flows.

- [ ] **Step 1: Modify `handleOAuthCallback` to check pending flows first**

Replace the state validation logic in `handleOAuthCallback`. The new logic:
1. First check `ConsumePendingFlow(state)` — if found, use it (PKCE flow from login endpoint)
2. If not found, fall back to HMAC validation (middleware-initiated flow)

In `callback.go`, replace the state validation and token exchange section:

```go
func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}
	if state == "" {
		http.Error(w, "missing state parameter", http.StatusBadRequest)
		return
	}

	// Try PKCE flow first (from login endpoint)
	if flow := ConsumePendingFlow(state); flow != nil {
		provider, ok := oauthCallbackProviders[flow.Provider]
		if !ok {
			http.Error(w, "unknown provider in pending flow", http.StatusBadRequest)
			return
		}
		// For PKCE flows, we need to resolve token endpoint if not in static config
		tokenEndpoint := provider.TokenEndpoint
		if tokenEndpoint == "" {
			// Read from registration cache
			regFile := oauthCallbackTokenDir + "/" + flow.Provider + ".reg.json"
			if data, err := os.ReadFile(regFile); err == nil {
				var cached struct {
					TokenEndpoint string `json:"token_endpoint"`
					ClientID      string `json:"client_id"`
					ClientSecret  string `json:"client_secret"`
				}
				if json.Unmarshal(data, &cached) == nil {
					tokenEndpoint = cached.TokenEndpoint
					provider.TokenEndpoint = cached.TokenEndpoint
					provider.ClientID = cached.ClientID
					provider.ClientSecret = cached.ClientSecret
				}
			}
		}
		if tokenEndpoint == "" {
			http.Error(w, "provider token endpoint not configured", http.StatusInternalServerError)
			return
		}

		token, err := exchangeCodeForTokenPKCE(provider, code, flow.RedirectURI, flow.CodeVerifier)
		if err != nil {
			slog.Error("oauth-callback: PKCE token exchange failed", "provider", flow.Provider, "error", err)
			http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tokenFile := oauthCallbackTokenDir + "/" + flow.Provider + ".json"
		if err := writeOAuthToken(tokenFile, token, provider); err != nil {
			slog.Error("oauth-callback: failed to save token", "error", err)
			http.Error(w, "failed to save token", http.StatusInternalServerError)
			return
		}
		gateway.RegisterSecret(token.AccessToken)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html><html><body>
<h1>Authorization successful</h1>
<p>Provider <strong>%s</strong> connected. You can close this tab.</p>
</body></html>`, html.EscapeString(flow.Provider))
		return
	}

	// Fall back to HMAC-based state (middleware-initiated flow)
	parts := strings.SplitN(state, ":", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid state format", http.StatusForbidden)
		return
	}
	sig, providerName := parts[0], parts[1]
	mac := hmac.New(sha256.New, oauthCallbackHMACKey)
	mac.Write([]byte(providerName))
	expectedSig := hex.EncodeToString(mac.Sum(nil))[:16]
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		http.Error(w, "invalid state signature", http.StatusForbidden)
		return
	}
	provider, ok := oauthCallbackProviders[providerName]
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	if provider.TokenEndpoint == "" {
		http.Error(w, "provider not configured", http.StatusInternalServerError)
		return
	}
	redirectURI := "{{ .public_url }}{{ .path }}"
	token, err := exchangeCodeForToken(provider, code, redirectURI)
	if err != nil {
		slog.Error("oauth-callback: token exchange failed", "error", err)
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}
	tokenFile := oauthCallbackTokenDir + "/" + providerName + ".json"
	if err := writeOAuthToken(tokenFile, token, provider); err != nil {
		slog.Error("oauth-callback: failed to save token", "error", err)
		http.Error(w, "failed to save token", http.StatusInternalServerError)
		return
	}
	gateway.RegisterSecret(token.AccessToken)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><body>
<h1>Authorization successful</h1>
<p>Provider <strong>%s</strong> connected. You can close this tab.</p>
</body></html>`, html.EscapeString(providerName))
}
```

- [ ] **Step 2: Add `exchangeCodeForTokenPKCE` function**

Add this function to `callback.go` (similar to `exchangeCodeForToken` but includes `code_verifier`):

```go
func exchangeCodeForTokenPKCE(provider oauthCallbackConfig, code, redirectURI, codeVerifier string) (*oauthTokenExchangeResponse, error) {
	u, err := url.Parse(provider.TokenEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid token_endpoint: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("token_endpoint must use https, got %q", u.Scheme)
	}
	params := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {provider.ClientID},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}
	if provider.ClientSecret != "" {
		params.Set("client_secret", provider.ClientSecret)
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: cbSSRFSafe()}
	resp, err := client.Post(provider.TokenEndpoint, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var tr oauthTokenExchangeResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &tr, nil
}
```

- [ ] **Step 3: Also store `token_endpoint` and `client_id` in the token file**

Update `writeOAuthToken` to include fields needed for token refresh:

```go
func writeOAuthToken(path string, token *oauthTokenExchangeResponse, provider oauthCallbackConfig) error {
	expiresIn := token.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}
	stored := map[string]any{
		"access_token":   token.AccessToken,
		"expires_at":     time.Now().Unix() + expiresIn,
		"token_endpoint": provider.TokenEndpoint,
		"client_id":      provider.ClientID,
	}
	if token.RefreshToken != "" {
		stored["refresh_token"] = token.RefreshToken
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Verify syntax**

Run: `gofmt -e core/plugins/mcp-oauth/handlers/callback.go`
Expected: Clean output

- [ ] **Step 5: Commit**

```bash
git add core/plugins/mcp-oauth/handlers/callback.go
git commit -m "feat(mcp-oauth): update callback to support PKCE flows from login endpoint"
```

### Task 4: Update plugin.yaml to declare login route

**Files:**
- Modify: `core/plugins/mcp-oauth/plugin.yaml`

- [ ] **Step 1: Add the login route to plugin.yaml**

Add a second route entry for the login handler:

```yaml
name: mcp-oauth
options:
  providers:
    type: object
    required: true
    description: "Map of provider name to MCP config (each needs at least mcp_url)"
  token_dir:
    type: string
    required: false
    default: "/data/oauth-tokens"
    description: "Directory for OAuth token files"

contributes:
  gateway:
    services:
{{- range $name, $cfg := .plugin.options.providers }}
      - url: "{{ index $cfg "mcp_url" }}"
{{- end }}
    volumes:
      - "oauth-tokens:{{ .plugin.options.token_dir }}"
    middlewares:
      - custom: "./middlewares/oauth.go"
    routes:
      - path: "/callback"
        handler: "./handlers/callback.go"
      - path: "/login"
        handler: "./handlers/login.go"
```

- [ ] **Step 2: Verify YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('core/plugins/mcp-oauth/plugin.yaml'))" 2>&1 || echo "YAML parse error"`

Note: This will fail because of Go template syntax `{{`. That's expected — the CLI's template engine handles this. Just visually confirm the YAML structure is correct.

- [ ] **Step 3: Commit**

```bash
git add core/plugins/mcp-oauth/plugin.yaml
git commit -m "feat(mcp-oauth): declare login route in plugin.yaml"
```

### Task 5: Update middleware to add PKCE and resource param

**Files:**
- Modify: `core/plugins/mcp-oauth/middlewares/oauth.go`

The middleware's `mwBuildAuthorizeURL` currently generates authorize URLs without PKCE (`code_challenge`) and without the `resource` parameter. Since the middleware-initiated flow uses HMAC state (not stored PKCE), we have two options:
1. Add PKCE to middleware flow too (requires storing verifier somewhere)
2. Leave middleware flow without PKCE (some providers may reject this)

For now, we add the `resource` parameter to the middleware authorize URL. PKCE in the middleware flow is lower priority because the recommended path is `curl /login/{provider}` which always uses PKCE. The middleware 401 is a fallback hint.

- [ ] **Step 1: Add `resource` parameter to `mwBuildAuthorizeURL`**

In `middlewares/oauth.go`, update the `mwBuildAuthorizeURL` function:

```go
func mwBuildAuthorizeURL(provider oauthProviderInfo, providerName, callbackURL string) string {
	mac := hmac.New(sha256.New, oauthHMACKey)
	mac.Write([]byte(providerName))
	sig := hex.EncodeToString(mac.Sum(nil))[:16]
	state := sig + ":" + providerName

	params := url.Values{
		"client_id":     {provider.ClientID},
		"response_type": {"code"},
		"state":         {state},
		"redirect_uri":  {callbackURL},
	}
	if provider.Scopes != "" {
		params.Set("scope", provider.Scopes)
	}
	if provider.MCPURL != "" {
		params.Set("resource", provider.MCPURL)
	}
	return provider.AuthorizeEndpoint + "?" + params.Encode()
}
```

- [ ] **Step 2: Add `MCPURL` field to `oauthProviderInfo` struct**

In `middlewares/oauth.go`, update the struct:

```go
type oauthProviderInfo struct {
	AuthorizeEndpoint string
	TokenEndpoint     string
	ClientID          string
	ClientSecret      string
	Scopes            string
	MCPURL            string
	Dynamic           bool
}
```

And populate it in the `init()` function where providers are parsed:

```go
for name, cfg := range rawProviders {
	p := oauthProviderInfo{}
	if v, ok := cfg["mcp_url"].(string); ok {
		p.MCPURL = v
	}
	if v, ok := cfg["authorize_endpoint"].(string); ok {
		p.AuthorizeEndpoint = v
	}
	// ... rest of existing fields ...
}
```

- [ ] **Step 3: Update the middleware 401 response to suggest using the login endpoint**

In the middleware's abort message, include a hint about the login endpoint:

```go
ctx.Abort(http.StatusUnauthorized, fmt.Sprintf(
	`{"error":"oauth_required","provider":%q,"authorize_url":%q,"hint":"For PKCE login, use: curl http://<gateway>/plugins/mcp-oauth/login/%s"}`,
	providerName, authorizeURL, providerName))
```

- [ ] **Step 4: Verify syntax**

Run: `gofmt -e core/plugins/mcp-oauth/middlewares/oauth.go`
Expected: Clean output

- [ ] **Step 5: Commit**

```bash
git add core/plugins/mcp-oauth/middlewares/oauth.go
git commit -m "feat(mcp-oauth): add resource param to authorize URL, hint login endpoint"
```

### Task 6: Update README and verify end-to-end

**Files:**
- Modify: `core/plugins/mcp-oauth/README.md`

- [ ] **Step 1: Update README with login endpoint documentation**

Add a "Login Flow" section to the README showing the curl-based workflow:

```markdown
## Login Flow (Recommended)

The login endpoint handles the full OAuth lifecycle including PKCE and Dynamic Client Registration.

### Quick Start

```bash
# 1. Start your agent-sandbox environment
agent-sandbox -C examples/local-coding compose up --build

# 2. Initiate login for a provider
curl http://localhost:8080/plugins/mcp-oauth/login/notion

# Response:
# {"authorize_url":"https://mcp.notion.com/authorize?...","provider":"notion","instructions":"Open the authorize_url in your browser to complete login."}

# 3. Open the authorize_url in your browser and complete authorization
#    The browser will redirect back to the gateway and show "Authorization successful"

# 4. Done — the agent can now use the MCP provider transparently
```

### How It Works

1. `GET /plugins/mcp-oauth/login/{provider}` — Gateway performs Dynamic Client Registration (if needed), generates PKCE challenge, and returns an authorize URL
2. User opens the URL in their browser and authorizes
3. Provider redirects to `http://localhost:8080/plugins/mcp-oauth/callback` with an authorization code
4. Gateway exchanges the code (with PKCE code_verifier) for tokens and stores them
5. All subsequent agent requests to the provider domain get `Authorization: Bearer <token>` injected automatically

### Listing Providers

```bash
curl http://localhost:8080/plugins/mcp-oauth/login/
# {"available":["notion"],"error":"provider name required","usage":"GET /plugins/mcp-oauth/login/<provider_name>"}
```
```

- [ ] **Step 2: Commit**

```bash
git add core/plugins/mcp-oauth/README.md
git commit -m "docs(mcp-oauth): add login endpoint usage to README"
```

- [ ] **Step 3: Verify the generate step still works**

Run: `flox activate -- go build ./cmd/agent-sandbox/`
Expected: Builds successfully

Run: `flox activate -- go test ./...`
Expected: All existing tests pass

- [ ] **Step 4: Manual end-to-end verification**

```bash
# Generate build artifacts for local-coding example
agent-sandbox -C examples/local-coding generate

# Verify login.go was copied into the build output
ls examples/local-coding/.build/gateway-src/core/gateway/middlewares/custom/
# Should contain: login.go, callback.go, oauth.go, pkce.go (all template-rendered)

# Verify the login route path in the rendered login.go
grep "RegisterRoute" examples/local-coding/.build/gateway-src/core/gateway/middlewares/custom/login.go
# Should show the resolved path (e.g., /plugins/mcp-oauth/login)

# Build and start
agent-sandbox -C examples/local-coding compose up --build

# Test login endpoint
curl http://localhost:8080/plugins/mcp-oauth/login/notion
# Should return JSON with authorize_url
```

- [ ] **Step 5: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix(mcp-oauth): address integration issues from e2e test"
```
