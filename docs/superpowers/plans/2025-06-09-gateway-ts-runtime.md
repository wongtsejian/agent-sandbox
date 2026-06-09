# Gateway TypeScript Runtime Engine — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a TypeScript runtime (goja + esbuild) to the gateway so plugins can define middleware and route handlers in TypeScript instead of compiled Go.

**Architecture:** The gateway embeds goja (pure-Go JS engine) and esbuild (Go-native TS bundler). At startup, it reads a plugin config that declares `.ts` handlers for routes and middleware. Each handler is bundled via esbuild (resolving imports), then loaded into a goja VM with host APIs injected for HTTP, file I/O, crypto, and request manipulation.

**Tech Stack:** Go, goja (JS runtime), esbuild-go (TS→JS bundler), existing gateway SDK

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `core/gateway/internal/jsruntime/runtime.go` | Create | VM pool, script loading, esbuild bundling |
| `core/gateway/internal/jsruntime/runtime_test.go` | Create | Tests for bundling + execution |
| `core/gateway/internal/jsruntime/hostapi.go` | Create | Host API injection (gw.http, gw.fs, gw.crypto, gw.log, gw.secrets) |
| `core/gateway/internal/jsruntime/hostapi_test.go` | Create | Tests for host APIs |
| `core/gateway/internal/jsruntime/request.go` | Create | RequestContext bridge (Go http.Request ↔ JS object) |
| `core/gateway/internal/jsruntime/request_test.go` | Create | Tests for request context |
| `core/gateway/internal/pluginloader/loader.go` | Create | Read plugin config, resolve .ts paths, register middleware/routes via jsruntime |
| `core/gateway/internal/pluginloader/loader_test.go` | Create | Tests for plugin loading |
| `core/gateway/internal/pluginloader/config.go` | Create | Plugin config YAML types |
| `core/gateway/cmd/gateway/main.go` | Modify | Add plugin loader startup after config load |
| `core/gateway/internal/mitm/mitm.go` | Modify | Fix abort handling in applyMiddleware |
| `go.mod` | Modify | Add goja + esbuild dependencies |

---

### Task 1: Add dependencies (goja + esbuild)

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add goja and esbuild dependencies**

```bash
cd /Users/corey/Projects/agent-sandbox
flox activate -- go get github.com/dop251/goja@latest
flox activate -- go get github.com/evanw/esbuild@latest
flox activate -- go mod tidy
```

- [ ] **Step 2: Verify it compiles**

Run: `flox activate -- go build ./...`
Expected: Clean build

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add goja (JS runtime) and esbuild (TS bundler) dependencies"
```

---

### Task 2: Fix abort handling in MITM middleware

**Files:**
- Modify: `core/gateway/internal/mitm/mitm.go`

The existing `applyMiddleware` sets `ctx.AbortStatus` but the caller in `Handle` never checks it. This is a pre-existing bug that must be fixed before TS middleware can abort requests.

- [ ] **Step 1: Write test for abort behavior**

Create `core/gateway/internal/mitm/abort_test.go`:

```go
package mitm

import (
	"net/http"
	"testing"

	"github.com/donbader/agent-sandbox/core/sdk/gateway"
	"github.com/stretchr/testify/assert"
)

func TestApplyMiddleware_Abort(t *testing.T) {
	gateway.ResetForTesting()

	gateway.RegisterMiddleware(gateway.MiddlewareDef{
		Name:    "test-abort",
		Domains: []string{"example.com"},
		Func: func(ctx *gateway.MiddlewareContext) error {
			ctx.Abort(http.StatusUnauthorized, `{"error":"unauthorized"}`)
			ctx.SetAbortHeader("Content-Type", "application/json")
			return nil
		},
	})

	req, _ := http.NewRequest("GET", "https://example.com/api", nil)
	req.Host = "example.com"

	ctx := applyMiddlewareWithContext(req)
	assert.NotNil(t, ctx)
	assert.Equal(t, http.StatusUnauthorized, ctx.AbortStatus)
	assert.Equal(t, `{"error":"unauthorized"}`, ctx.AbortBody)
	assert.Equal(t, "application/json", ctx.AbortHeaders.Get("Content-Type"))
}

func TestApplyMiddleware_NoAbort(t *testing.T) {
	gateway.ResetForTesting()

	gateway.RegisterMiddleware(gateway.MiddlewareDef{
		Name:    "test-passthrough",
		Domains: []string{"example.com"},
		Func: func(ctx *gateway.MiddlewareContext) error {
			ctx.Request.Header.Set("Authorization", "Bearer token")
			return nil
		},
	})

	req, _ := http.NewRequest("GET", "https://example.com/api", nil)
	req.Host = "example.com"

	ctx := applyMiddlewareWithContext(req)
	assert.Nil(t, ctx) // nil means no abort
	assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))
}
```

- [ ] **Step 2: Refactor applyMiddleware to return abort context**

In `core/gateway/internal/mitm/mitm.go`, rename `applyMiddleware` to keep backward compat and add a new function:

```go
// applyMiddlewareWithContext runs middleware and returns the context if aborted, nil otherwise.
func applyMiddlewareWithContext(req *http.Request) *gateway.MiddlewareContext {
	matching := gateway.MatchingMiddleware(req)
	if len(matching) == 0 {
		return nil
	}

	ctx := &gateway.MiddlewareContext{
		Request: req,
		Env:     os.Getenv,
	}

	for _, mw := range matching {
		if err := mw.Func(ctx); err != nil {
			slog.Error("middleware error", "name", mw.Name, "error", err)
			continue
		}
		if ctx.AbortStatus != 0 {
			return ctx
		}
	}
	return nil
}

// applyMiddleware runs all matching middleware against the request.
// Returns true if any middleware was applied.
func applyMiddleware(req *http.Request) bool {
	ctx := applyMiddlewareWithContext(req)
	return ctx != nil || len(gateway.MatchingMiddleware(req)) > 0
}
```

- [ ] **Step 3: Update Handle to check abort**

In the `Handle` method, replace:
```go
rewritten := applyMiddleware(req)
slog.Debug("request", "host", serverName, "method", req.Method, "path", originalPath, "rewritten", rewritten)

// Forward to real server
resp, err := h.forwardRequest(req, serverName)
```

With:
```go
abortCtx := applyMiddlewareWithContext(req)
rewritten := abortCtx != nil || len(gateway.MatchingMiddleware(req)) > 0
slog.Debug("request", "host", serverName, "method", req.Method, "path", originalPath, "rewritten", rewritten)

// If middleware aborted, return the abort response instead of forwarding
if abortCtx != nil && abortCtx.AbortStatus != 0 {
	abortResp := &http.Response{
		StatusCode: abortCtx.AbortStatus,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     abortCtx.AbortHeaders,
		Body:       io.NopCloser(strings.NewReader(abortCtx.AbortBody)),
	}
	if abortResp.Header == nil {
		abortResp.Header = make(http.Header)
	}
	_ = abortResp.Write(tlsConn)
	continue
}

// Forward to real server
resp, err := h.forwardRequest(req, serverName)
```

- [ ] **Step 4: Run tests**

Run: `flox activate -- go test ./core/gateway/internal/mitm/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/gateway/internal/mitm/
git commit -m "fix(gateway): wire abort handling in MITM middleware"
```

### Task 3: Create JS runtime core (esbuild bundling + goja execution)

**Files:**
- Create: `core/gateway/internal/jsruntime/runtime.go`
- Create: `core/gateway/internal/jsruntime/runtime_test.go`

- [ ] **Step 1: Write test for TS bundling and execution**

Create `core/gateway/internal/jsruntime/runtime_test.go`:

```go
package jsruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundle_SimpleTS(t *testing.T) {
	dir := t.TempDir()

	// Write a simple TS file
	err := os.WriteFile(filepath.Join(dir, "handler.ts"), []byte(`
		export default function(ctx: any): string {
			return "hello from ts";
		}
	`), 0644)
	require.NoError(t, err)

	js, err := Bundle(filepath.Join(dir, "handler.ts"))
	require.NoError(t, err)
	assert.Contains(t, js, "hello from ts")
	assert.NotContains(t, js, "ctx: any") // types stripped
}

func TestBundle_WithImports(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "helper.ts"), []byte(`
		export function greet(name: string): string {
			return "hello " + name;
		}
	`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "handler.ts"), []byte(`
		import { greet } from "./helper";
		export default function(): string {
			return greet("world");
		}
	`), 0644)
	require.NoError(t, err)

	js, err := Bundle(filepath.Join(dir, "handler.ts"))
	require.NoError(t, err)
	assert.Contains(t, js, "hello")
	assert.Contains(t, js, "world")
}

func TestBundle_SyntaxError(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "bad.ts"), []byte(`
		export default function( {{{ invalid
	`), 0644)
	require.NoError(t, err)

	_, err = Bundle(filepath.Join(dir, "bad.ts"))
	assert.Error(t, err)
}

func TestExecHandler_ReturnsValue(t *testing.T) {
	js := `(function() { return "test_result"; })()`

	vm := NewVM()
	val, err := vm.RunString(js)
	require.NoError(t, err)
	assert.Equal(t, "test_result", val.Export())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `flox activate -- go test ./core/gateway/internal/jsruntime/ -v`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Implement runtime.go**

Create `core/gateway/internal/jsruntime/runtime.go`:

```go
// Package jsruntime provides TypeScript bundling and JavaScript execution for gateway plugins.
package jsruntime

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

// Bundle takes a TypeScript entry point, resolves all imports, and returns bundled JavaScript.
func Bundle(entryPoint string) (string, error) {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{entryPoint},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatIIFE,
		Platform:    api.PlatformNeutral,
		Target:      api.ES2020,
		GlobalName:  "__handler",
		Loader: map[string]api.Loader{
			".ts": api.LoaderTS,
		},
	})

	if len(result.Errors) > 0 {
		msg := result.Errors[0].Text
		if result.Errors[0].Location != nil {
			msg = fmt.Sprintf("%s:%d: %s", result.Errors[0].Location.File, result.Errors[0].Location.Line, msg)
		}
		return "", fmt.Errorf("esbuild: %s", msg)
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("esbuild: no output")
	}

	return string(result.OutputFiles[0].Contents), nil
}

// VM wraps a goja runtime with helper methods.
type VM struct {
	runtime *goja.Runtime
}

// NewVM creates a new JavaScript VM.
func NewVM() *VM {
	rt := goja.New()
	rt.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	return &VM{runtime: rt}
}

// RunString executes JavaScript code and returns the result.
func (vm *VM) RunString(code string) (goja.Value, error) {
	return vm.runtime.RunString(code)
}

// Runtime returns the underlying goja runtime for advanced use.
func (vm *VM) Runtime() *goja.Runtime {
	return vm.runtime
}

// Set sets a global variable in the VM.
func (vm *VM) Set(name string, value any) error {
	return vm.runtime.Set(name, value)
}
```

- [ ] **Step 4: Run tests**

Run: `flox activate -- go test ./core/gateway/internal/jsruntime/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/gateway/internal/jsruntime/
git commit -m "feat(gateway): add jsruntime package with esbuild bundling and goja execution"
```

### Task 4: Request context bridge (Go ↔ JS)

**Files:**
- Create: `core/gateway/internal/jsruntime/request.go`
- Create: `core/gateway/internal/jsruntime/request_test.go`

- [ ] **Step 1: Write test for request context**

Create `core/gateway/internal/jsruntime/request_test.go`:

```go
package jsruntime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestContext_ReadHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com/api?foo=bar", nil)
	req.Host = "example.com"
	req.Header.Set("X-Custom", "value")

	vm := NewVM()
	ctx := NewRequestContext(req, nil)
	require.NoError(t, vm.Set("ctx", ctx.ToJSObject(vm)))

	val, err := vm.RunString(`ctx.request.headers["X-Custom"]`)
	require.NoError(t, err)
	assert.Equal(t, "value", val.Export())

	val, err = vm.RunString(`ctx.request.method`)
	require.NoError(t, err)
	assert.Equal(t, "GET", val.Export())

	val, err = vm.RunString(`ctx.request.url`)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/api?foo=bar", val.Export())

	val, err = vm.RunString(`ctx.request.host`)
	require.NoError(t, err)
	assert.Equal(t, "example.com", val.Export())
}

func TestRequestContext_ModifyHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com/api", nil)
	req.Host = "example.com"

	vm := NewVM()
	ctx := NewRequestContext(req, nil)
	require.NoError(t, vm.Set("ctx", ctx.ToJSObject(vm)))

	_, err := vm.RunString(`ctx.request.setHeader("Authorization", "Bearer token123")`)
	require.NoError(t, err)

	assert.Equal(t, "Bearer token123", req.Header.Get("Authorization"))
}

func TestRequestContext_Abort(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com/api", nil)
	req.Host = "example.com"

	vm := NewVM()
	ctx := NewRequestContext(req, nil)
	require.NoError(t, vm.Set("ctx", ctx.ToJSObject(vm)))

	_, err := vm.RunString(`ctx.abort(401, '{"error":"unauthorized"}')`)
	require.NoError(t, err)

	assert.Equal(t, 401, ctx.AbortStatus)
	assert.Equal(t, `{"error":"unauthorized"}`, ctx.AbortBody)
}

func TestRequestContext_RouteHandler(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://localhost:8080/plugins/mcp-oauth/login/notion", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	vm := NewVM()
	ctx := NewRequestContext(req, w)
	require.NoError(t, vm.Set("ctx", ctx.ToJSObject(vm)))

	_, err := vm.RunString(`
		ctx.response.status(200);
		ctx.response.header("Content-Type", "application/json");
		ctx.response.body('{"ok":true}');
	`)
	require.NoError(t, err)

	assert.Equal(t, 200, ctx.ResponseStatus)
	assert.Equal(t, "application/json", ctx.ResponseHeaders.Get("Content-Type"))
	assert.Equal(t, `{"ok":true}`, ctx.ResponseBody)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `flox activate -- go test ./core/gateway/internal/jsruntime/ -v -run TestRequestContext`
Expected: FAIL (types not defined)

- [ ] **Step 3: Implement request.go**

Create `core/gateway/internal/jsruntime/request.go`:

```go
package jsruntime

import (
	"net/http"

	"github.com/dop251/goja"
)

// RequestContext bridges a Go HTTP request/response to the JS world.
type RequestContext struct {
	Request  *http.Request
	Writer   http.ResponseWriter // nil for middleware (proxy), non-nil for route handlers

	// Abort fields (middleware mode)
	AbortStatus  int
	AbortBody    string
	AbortHeaders http.Header

	// Response fields (route handler mode)
	ResponseStatus  int
	ResponseHeaders http.Header
	ResponseBody    string
}

// NewRequestContext creates a new request context for JS handlers.
func NewRequestContext(req *http.Request, w http.ResponseWriter) *RequestContext {
	return &RequestContext{
		Request:         req,
		Writer:          w,
		AbortHeaders:    make(http.Header),
		ResponseHeaders: make(http.Header),
	}
}

// ToJSObject converts the context into a JS-accessible object.
func (rc *RequestContext) ToJSObject(vm *VM) map[string]any {
	headers := make(map[string]string)
	for k, v := range rc.Request.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	query := make(map[string]string)
	for k, v := range rc.Request.URL.Query() {
		if len(v) > 0 {
			query[k] = v[0]
		}
	}

	requestObj := map[string]any{
		"method":  rc.Request.Method,
		"url":     rc.Request.URL.String(),
		"host":    rc.Request.Host,
		"path":    rc.Request.URL.Path,
		"query":   query,
		"headers": headers,
		"setHeader": func(call goja.FunctionCall) goja.Value {
			key := call.Argument(0).String()
			val := call.Argument(1).String()
			rc.Request.Header.Set(key, val)
			return goja.Undefined()
		},
	}

	responseObj := map[string]any{
		"status": func(call goja.FunctionCall) goja.Value {
			rc.ResponseStatus = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		},
		"header": func(call goja.FunctionCall) goja.Value {
			key := call.Argument(0).String()
			val := call.Argument(1).String()
			rc.ResponseHeaders.Set(key, val)
			return goja.Undefined()
		},
		"body": func(call goja.FunctionCall) goja.Value {
			rc.ResponseBody = call.Argument(0).String()
			return goja.Undefined()
		},
	}

	return map[string]any{
		"request":  requestObj,
		"response": responseObj,
		"abort": func(call goja.FunctionCall) goja.Value {
			rc.AbortStatus = int(call.Argument(0).ToInteger())
			if len(call.Arguments) > 1 {
				rc.AbortBody = call.Argument(1).String()
			}
			return goja.Undefined()
		},
		"env": func(call goja.FunctionCall) goja.Value {
			// Env is injected by the host when creating the context
			return goja.Undefined()
		},
	}
}
```

- [ ] **Step 4: Run tests**

Run: `flox activate -- go test ./core/gateway/internal/jsruntime/ -v -run TestRequestContext`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/gateway/internal/jsruntime/request.go core/gateway/internal/jsruntime/request_test.go
git commit -m "feat(gateway): add request context bridge for JS handlers"
```

### Task 5: Host APIs (HTTP, file I/O, crypto, logging, secrets)

**Files:**
- Create: `core/gateway/internal/jsruntime/hostapi.go`
- Create: `core/gateway/internal/jsruntime/hostapi_test.go`

- [ ] **Step 1: Write tests for host APIs**

Create `core/gateway/internal/jsruntime/hostapi_test.go`:

```go
package jsruntime

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostAPI_Crypto_SHA256(t *testing.T) {
	vm := NewVM()
	InjectHostAPIs(vm, &HostAPIConfig{DataDir: t.TempDir()})

	val, err := vm.RunString(`gw.crypto.sha256("hello")`)
	require.NoError(t, err)
	// SHA256 of "hello" = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", val.Export())
}

func TestHostAPI_Crypto_RandomBytes(t *testing.T) {
	vm := NewVM()
	InjectHostAPIs(vm, &HostAPIConfig{DataDir: t.TempDir()})

	val, err := vm.RunString(`gw.crypto.randomBytes(16).length`)
	require.NoError(t, err)
	assert.Equal(t, int64(16), val.ToInteger())
}

func TestHostAPI_Crypto_Base64URL(t *testing.T) {
	vm := NewVM()
	InjectHostAPIs(vm, &HostAPIConfig{DataDir: t.TempDir()})

	val, err := vm.RunString(`gw.crypto.base64url.encode("hello world")`)
	require.NoError(t, err)
	assert.Equal(t, "aGVsbG8gd29ybGQ", val.Export())

	val, err = vm.RunString(`gw.crypto.base64url.decode("aGVsbG8gd29ybGQ")`)
	require.NoError(t, err)
	assert.Equal(t, "hello world", val.Export())
}

func TestHostAPI_FS_ReadWrite(t *testing.T) {
	dir := t.TempDir()
	vm := NewVM()
	InjectHostAPIs(vm, &HostAPIConfig{DataDir: dir})

	// Write a file
	_, err := vm.RunString(`gw.fs.write("test.json", '{"key":"value"}')`)
	require.NoError(t, err)

	// Verify on disk
	data, err := os.ReadFile(filepath.Join(dir, "test.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, string(data))

	// Read it back from JS
	val, err := vm.RunString(`gw.fs.read("test.json")`)
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, val.Export())
}

func TestHostAPI_FS_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	vm := NewVM()
	InjectHostAPIs(vm, &HostAPIConfig{DataDir: dir})

	_, err := vm.RunString(`gw.fs.read("../../etc/passwd")`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestHostAPI_HTTP_Fetch(t *testing.T) {
	// Start a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	vm := NewVM()
	InjectHostAPIs(vm, &HostAPIConfig{DataDir: t.TempDir(), AllowPrivateIPs: true})

	val, err := vm.RunString(`
		var resp = gw.http.fetch("` + ts.URL + `", {method: "GET"});
		resp.body;
	`)
	require.NoError(t, err)
	assert.Equal(t, `{"status":"ok"}`, val.Export())
}

func TestHostAPI_Secrets(t *testing.T) {
	vm := NewVM()
	cfg := &HostAPIConfig{DataDir: t.TempDir()}
	InjectHostAPIs(vm, cfg)

	_, err := vm.RunString(`gw.secrets.register("super-secret-token")`)
	require.NoError(t, err)
	assert.Contains(t, cfg.RegisteredSecrets, "super-secret-token")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `flox activate -- go test ./core/gateway/internal/jsruntime/ -v -run TestHostAPI`
Expected: FAIL (InjectHostAPIs not defined)

- [ ] **Step 3: Implement hostapi.go**

Create `core/gateway/internal/jsruntime/hostapi.go`:

```go
package jsruntime

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// HostAPIConfig configures the host APIs injected into JS VMs.
type HostAPIConfig struct {
	DataDir          string   // Base directory for file I/O (plugin-scoped)
	AllowPrivateIPs  bool     // Allow HTTP fetch to private IPs (testing only)
	RegisteredSecrets []string // Secrets registered by the plugin (collected by host)
}

// InjectHostAPIs injects the `gw` global object into a VM with all host APIs.
func InjectHostAPIs(vm *VM, cfg *HostAPIConfig) {
	rt := vm.Runtime()

	cryptoObj := rt.NewObject()
	_ = cryptoObj.Set("sha256", func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).String()
		h := sha256.Sum256([]byte(data))
		return rt.ToValue(hex.EncodeToString(h[:]))
	})
	_ = cryptoObj.Set("hmac", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		data := call.Argument(1).String()
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(data))
		return rt.ToValue(hex.EncodeToString(mac.Sum(nil)))
	})
	_ = cryptoObj.Set("randomBytes", func(call goja.FunctionCall) goja.Value {
		n := int(call.Argument(0).ToInteger())
		b := make([]byte, n)
		_, _ = rand.Read(b)
		return rt.ToValue(b)
	})

	base64urlObj := rt.NewObject()
	_ = base64urlObj.Set("encode", func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).String()
		return rt.ToValue(base64.RawURLEncoding.EncodeToString([]byte(data)))
	})
	_ = base64urlObj.Set("decode", func(call goja.FunctionCall) goja.Value {
		encoded := call.Argument(0).String()
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			panic(rt.NewGoError(fmt.Errorf("base64url decode: %w", err)))
		}
		return rt.ToValue(string(decoded))
	})
	_ = cryptoObj.Set("base64url", base64urlObj)

	// File I/O (scoped to DataDir)
	fsObj := rt.NewObject()
	_ = fsObj.Set("read", func(call goja.FunctionCall) goja.Value {
		relPath := call.Argument(0).String()
		absPath, err := safeJoin(cfg.DataDir, relPath)
		if err != nil {
			panic(rt.NewGoError(err))
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			panic(rt.NewGoError(err))
		}
		return rt.ToValue(string(data))
	})
	_ = fsObj.Set("write", func(call goja.FunctionCall) goja.Value {
		relPath := call.Argument(0).String()
		content := call.Argument(1).String()
		absPath, err := safeJoin(cfg.DataDir, relPath)
		if err != nil {
			panic(rt.NewGoError(err))
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
			panic(rt.NewGoError(err))
		}
		if err := os.WriteFile(absPath, []byte(content), 0600); err != nil {
			panic(rt.NewGoError(err))
		}
		return goja.Undefined()
	})

	// HTTP client (synchronous, SSRF-safe by default)
	httpObj := rt.NewObject()
	_ = httpObj.Set("fetch", func(call goja.FunctionCall) goja.Value {
		urlStr := call.Argument(0).String()
		opts := map[string]string{"method": "GET"}
		if len(call.Arguments) > 1 {
			optsVal := call.Argument(1).Export()
			if m, ok := optsVal.(map[string]any); ok {
				for k, v := range m {
					opts[k] = fmt.Sprintf("%v", v)
				}
			}
		}

		req, err := http.NewRequest(opts["method"], urlStr, nil)
		if err != nil {
			panic(rt.NewGoError(err))
		}
		if h, ok := opts["headers"]; ok {
			// Simple header parsing: "Key: Value\nKey2: Value2"
			for _, line := range strings.Split(h, "\n") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				}
			}
		}
		if body, ok := opts["body"]; ok {
			req.Body = io.NopCloser(strings.NewReader(body))
			req.ContentLength = int64(len(body))
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			panic(rt.NewGoError(err))
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			panic(rt.NewGoError(err))
		}

		respHeaders := make(map[string]string)
		for k, v := range resp.Header {
			if len(v) > 0 {
				respHeaders[k] = v[0]
			}
		}

		return rt.ToValue(map[string]any{
			"status":  resp.StatusCode,
			"headers": respHeaders,
			"body":    string(body),
		})
	})

	// Secrets
	secretsObj := rt.NewObject()
	_ = secretsObj.Set("register", func(call goja.FunctionCall) goja.Value {
		secret := call.Argument(0).String()
		if secret != "" {
			cfg.RegisteredSecrets = append(cfg.RegisteredSecrets, secret)
		}
		return goja.Undefined()
	})

	// Logging
	logObj := rt.NewObject()
	_ = logObj.Set("info", func(call goja.FunctionCall) goja.Value {
		msg := call.Argument(0).String()
		slog.Info("plugin: "+msg)
		return goja.Undefined()
	})
	_ = logObj.Set("error", func(call goja.FunctionCall) goja.Value {
		msg := call.Argument(0).String()
		slog.Error("plugin: "+msg)
		return goja.Undefined()
	})
	_ = logObj.Set("debug", func(call goja.FunctionCall) goja.Value {
		msg := call.Argument(0).String()
		slog.Debug("plugin: "+msg)
		return goja.Undefined()
	})

	// Assemble gw object
	gwObj := rt.NewObject()
	_ = gwObj.Set("crypto", cryptoObj)
	_ = gwObj.Set("fs", fsObj)
	_ = gwObj.Set("http", httpObj)
	_ = gwObj.Set("secrets", secretsObj)
	_ = gwObj.Set("log", logObj)
	_ = vm.Set("gw", gwObj)
}

// safeJoin joins base and rel, rejecting path traversal.
func safeJoin(base, rel string) (string, error) {
	abs := filepath.Join(base, rel)
	cleaned := filepath.Clean(abs)
	if !strings.HasPrefix(cleaned, filepath.Clean(base)+string(filepath.Separator)) && cleaned != filepath.Clean(base) {
		return "", fmt.Errorf("path traversal: %q escapes base %q", rel, base)
	}
	return cleaned, nil
}
```

- [ ] **Step 4: Run tests**

Run: `flox activate -- go test ./core/gateway/internal/jsruntime/ -v -run TestHostAPI`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/gateway/internal/jsruntime/hostapi.go core/gateway/internal/jsruntime/hostapi_test.go
git commit -m "feat(gateway): add host APIs for JS plugins (crypto, fs, http, secrets, log)"
```

### Task 6: Plugin loader (config parsing + handler registration)

**Files:**
- Create: `core/gateway/internal/pluginloader/config.go`
- Create: `core/gateway/internal/pluginloader/loader.go`
- Create: `core/gateway/internal/pluginloader/loader_test.go`

- [ ] **Step 1: Write config types**

Create `core/gateway/internal/pluginloader/config.go`:

```go
// Package pluginloader reads plugin configuration and registers TS handlers with the gateway.
package pluginloader

// PluginConfig represents a single plugin's configuration as resolved at generate-time.
type PluginConfig struct {
	Name    string            `yaml:"name"`
	Dir     string            `yaml:"dir"`     // Absolute path to plugin directory
	Options map[string]any    `yaml:"options"` // Resolved plugin options
	Gateway GatewayContrib    `yaml:"gateway"`
}

// GatewayContrib describes what a plugin contributes to the gateway.
type GatewayContrib struct {
	Middlewares []MiddlewareEntry `yaml:"middlewares"`
	Routes      []RouteEntry      `yaml:"routes"`
}

// MiddlewareEntry declares a TS middleware handler scoped to domains.
type MiddlewareEntry struct {
	Script  string   `yaml:"script"`  // Relative path to .ts file
	Domains []string `yaml:"domains"` // Domain scope (empty = all)
}

// RouteEntry declares a TS route handler at a path.
type RouteEntry struct {
	Path    string `yaml:"path"`    // Route path (namespaced at load time)
	Handler string `yaml:"handler"` // Relative path to .ts file
}

// PluginsConfig is the top-level config for all plugins loaded by the gateway.
type PluginsConfig struct {
	Plugins []PluginConfig `yaml:"plugins"`
}
```

- [ ] **Step 2: Write loader tests**

Create `core/gateway/internal/pluginloader/loader_test.go`:

```go
package pluginloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donbader/agent-sandbox/core/sdk/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPlugins_Middleware(t *testing.T) {
	gateway.ResetForTesting()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "test-plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "src"), 0755))

	// Write a simple middleware that injects a header
	err := os.WriteFile(filepath.Join(pluginDir, "src", "auth.ts"), []byte(`
		export default function(ctx: any, options: any) {
			ctx.request.setHeader("X-Injected", "from-ts-plugin");
		}
	`), 0644)
	require.NoError(t, err)

	cfg := &PluginsConfig{
		Plugins: []PluginConfig{
			{
				Name:    "test-plugin",
				Dir:     pluginDir,
				Options: map[string]any{"token": "secret"},
				Gateway: GatewayContrib{
					Middlewares: []MiddlewareEntry{
						{Script: "./src/auth.ts", Domains: []string{"api.example.com"}},
					},
				},
			},
		},
	}

	err = LoadPlugins(cfg)
	require.NoError(t, err)

	// Verify middleware was registered
	all := gateway.All()
	require.Len(t, all, 1)
	assert.Equal(t, "ts:test-plugin:auth.ts", all[0].Name)
	assert.Equal(t, []string{"api.example.com"}, all[0].Domains)
}

func TestLoadPlugins_Route(t *testing.T) {
	gateway.ResetForTesting()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "test-plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "src"), 0755))

	err := os.WriteFile(filepath.Join(pluginDir, "src", "hello.ts"), []byte(`
		export default function(ctx: any, options: any) {
			ctx.response.status(200);
			ctx.response.header("Content-Type", "text/plain");
			ctx.response.body("hello from plugin");
		}
	`), 0644)
	require.NoError(t, err)

	cfg := &PluginsConfig{
		Plugins: []PluginConfig{
			{
				Name: "test-plugin",
				Dir:  pluginDir,
				Gateway: GatewayContrib{
					Routes: []RouteEntry{
						{Path: "/hello", Handler: "./src/hello.ts"},
					},
				},
			},
		},
	}

	err = LoadPlugins(cfg)
	require.NoError(t, err)

	// Verify route was registered
	handler := gateway.MatchRoute("/plugins/test-plugin/hello")
	assert.NotNil(t, handler)
}

func TestLoadPlugins_BadScript(t *testing.T) {
	gateway.ResetForTesting()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "bad-plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "src"), 0755))

	err := os.WriteFile(filepath.Join(pluginDir, "src", "broken.ts"), []byte(`
		export default function( {{{ invalid syntax
	`), 0644)
	require.NoError(t, err)

	cfg := &PluginsConfig{
		Plugins: []PluginConfig{
			{
				Name: "bad-plugin",
				Dir:  pluginDir,
				Gateway: GatewayContrib{
					Middlewares: []MiddlewareEntry{
						{Script: "./src/broken.ts", Domains: []string{"example.com"}},
					},
				},
			},
		},
	}

	err = LoadPlugins(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "esbuild")
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `flox activate -- go test ./core/gateway/internal/pluginloader/ -v`
Expected: FAIL (LoadPlugins not defined)

- [ ] **Step 4: Implement loader.go**

Create `core/gateway/internal/pluginloader/loader.go`:

```go
package pluginloader

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/donbader/agent-sandbox/core/gateway/internal/jsruntime"
	"github.com/donbader/agent-sandbox/core/sdk/gateway"
	"gopkg.in/yaml.v3"
)

// LoadPluginsFromFile reads a plugins config YAML file and loads all plugins.
func LoadPluginsFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No plugins config = no TS plugins to load
		}
		return fmt.Errorf("read plugins config: %w", err)
	}

	var cfg PluginsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse plugins config: %w", err)
	}

	return LoadPlugins(&cfg)
}

// LoadPlugins loads all plugins from the given config.
func LoadPlugins(cfg *PluginsConfig) error {
	for _, plugin := range cfg.Plugins {
		if err := loadPlugin(plugin); err != nil {
			return fmt.Errorf("plugin %q: %w", plugin.Name, err)
		}
		slog.Info("loaded plugin", "name", plugin.Name,
			"middlewares", len(plugin.Gateway.Middlewares),
			"routes", len(plugin.Gateway.Routes))
	}
	return nil
}

func loadPlugin(plugin PluginConfig) error {
	dataDir := "/data/plugins/" + plugin.Name
	if d, ok := plugin.Options["token_dir"].(string); ok {
		dataDir = d
	}

	// Load middleware handlers
	for _, mw := range plugin.Gateway.Middlewares {
		entryPoint := filepath.Join(plugin.Dir, mw.Script)
		bundled, err := jsruntime.Bundle(entryPoint)
		if err != nil {
			return fmt.Errorf("bundle middleware %s: %w", mw.Script, err)
		}

		domains := mw.Domains
		scriptName := filepath.Base(mw.Script)
		mwName := fmt.Sprintf("ts:%s:%s", plugin.Name, scriptName)
		opts := plugin.Options

		gateway.RegisterMiddleware(gateway.MiddlewareDef{
			Name:    mwName,
			Domains: domains,
			Func: func(ctx *gateway.MiddlewareContext) error {
				return execMiddleware(bundled, ctx, opts, dataDir)
			},
		})
	}

	// Load route handlers
	for _, route := range plugin.Gateway.Routes {
		entryPoint := filepath.Join(plugin.Dir, route.Handler)
		bundled, err := jsruntime.Bundle(entryPoint)
		if err != nil {
			return fmt.Errorf("bundle route handler %s: %w", route.Handler, err)
		}

		namespacedPath := "/plugins/" + plugin.Name + normalizePath(route.Path)
		opts := plugin.Options

		gateway.RegisterRoute(gateway.RouteDef{
			Path: namespacedPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				execRouteHandler(bundled, w, r, opts, dataDir)
			},
		})
	}

	return nil
}

func execMiddleware(bundledJS string, ctx *gateway.MiddlewareContext, opts map[string]any, dataDir string) error {
	vm := jsruntime.NewVM()
	hostCfg := &jsruntime.HostAPIConfig{DataDir: dataDir}
	jsruntime.InjectHostAPIs(vm, hostCfg)

	reqCtx := jsruntime.NewRequestContext(ctx.Request, nil)
	if err := vm.Set("ctx", reqCtx.ToJSObject(vm)); err != nil {
		return fmt.Errorf("set ctx: %w", err)
	}
	if err := vm.Set("options", opts); err != nil {
		return fmt.Errorf("set options: %w", err)
	}

	// Execute: __handler contains {default: function(ctx, options){...}}
	_, err := vm.RunString(bundledJS + "\n__handler.default(ctx, options);")
	if err != nil {
		return fmt.Errorf("exec middleware: %w", err)
	}

	// Propagate abort back to gateway context
	if reqCtx.AbortStatus != 0 {
		ctx.AbortStatus = reqCtx.AbortStatus
		ctx.AbortBody = reqCtx.AbortBody
		ctx.AbortHeaders = reqCtx.AbortHeaders
	}

	// Register any secrets the plugin declared
	for _, s := range hostCfg.RegisteredSecrets {
		gateway.RegisterSecret(s)
	}

	return nil
}

func execRouteHandler(bundledJS string, w http.ResponseWriter, r *http.Request, opts map[string]any, dataDir string) {
	vm := jsruntime.NewVM()
	hostCfg := &jsruntime.HostAPIConfig{DataDir: dataDir}
	jsruntime.InjectHostAPIs(vm, hostCfg)

	reqCtx := jsruntime.NewRequestContext(r, w)
	if err := vm.Set("ctx", reqCtx.ToJSObject(vm)); err != nil {
		slog.Error("plugin route: set ctx", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := vm.Set("options", opts); err != nil {
		slog.Error("plugin route: set options", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	_, err := vm.RunString(bundledJS + "\n__handler.default(ctx, options);")
	if err != nil {
		slog.Error("plugin route handler error", "error", err)
		http.Error(w, "plugin error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Write response from context
	for k, vals := range reqCtx.ResponseHeaders {
		for _, v := range vals {
			w.Header().Set(k, v)
		}
	}
	if reqCtx.ResponseStatus > 0 {
		w.WriteHeader(reqCtx.ResponseStatus)
	}
	if reqCtx.ResponseBody != "" {
		_, _ = w.Write([]byte(reqCtx.ResponseBody))
	}

	// Register any secrets
	for _, s := range hostCfg.RegisteredSecrets {
		gateway.RegisterSecret(s)
	}
}

func normalizePath(path string) string {
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}
	return path
}
```

- [ ] **Step 5: Run tests**

Run: `flox activate -- go test ./core/gateway/internal/pluginloader/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add core/gateway/internal/pluginloader/
git commit -m "feat(gateway): add plugin loader for TypeScript middleware and routes"
```

### Task 7: Integrate plugin loader into gateway main.go

**Files:**
- Modify: `core/gateway/cmd/gateway/main.go`

- [ ] **Step 1: Add plugin loader import and startup call**

In `core/gateway/cmd/gateway/main.go`, add the import:

```go
"github.com/donbader/agent-sandbox/core/gateway/internal/pluginloader"
```

After the config is loaded and logger is set up (after `slog.SetDefault(logger)`), add:

```go
// Load TypeScript plugins if plugins config exists
pluginsConfigPath := "/etc/gateway/plugins.yaml"
if p := os.Getenv("GATEWAY_PLUGINS_CONFIG"); p != "" {
	pluginsConfigPath = p
}
if err := pluginloader.LoadPluginsFromFile(pluginsConfigPath); err != nil {
	slog.Error("load plugins", "error", err)
	os.Exit(1)
}
// Re-collect secrets after plugins have loaded
secrets = gateway.Secrets()
```

Note: This must come BEFORE the MITM handler setup since plugins register middleware that the MITM handler needs to find via `gateway.MatchingMiddleware`.

- [ ] **Step 2: Update secrets collection**

The current code collects secrets once at startup (from Go init() middleware). After loading TS plugins, secrets need to be re-collected. The logger's redact handler needs all secrets upfront, so move the logger setup AFTER plugin loading:

Reorder the startup sequence in `main.go`:
1. Load proxy config
2. Set up a preliminary logger (for plugin load errors)
3. Load TS plugins
4. Collect all secrets (from both Go init() and TS plugins)
5. Set up final logger with redaction

```go
func main() {
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	if os.Getenv("LOG_LEVEL") == "debug" {
		level.Set(slog.LevelDebug)
	}

	// Preliminary logger (no redaction yet)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	configPath := "/etc/gateway/config.yaml"
	if p := os.Getenv("GATEWAY_CONFIG"); p != "" {
		configPath = p
	}

	cfg, err := proxy.LoadConfig(configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	// Load TypeScript plugins
	pluginsConfigPath := "/etc/gateway/plugins.yaml"
	if p := os.Getenv("GATEWAY_PLUGINS_CONFIG"); p != "" {
		pluginsConfigPath = p
	}
	if err := pluginloader.LoadPluginsFromFile(pluginsConfigPath); err != nil {
		slog.Error("load plugins", "error", err)
		os.Exit(1)
	}

	// Now collect all secrets (from Go init() middleware + TS plugins)
	secrets := gateway.Secrets()

	// Final logger with redaction
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "token" || a.Key == "authorization" || a.Key == "api_key" {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	})
	logger := slog.New(redact.NewHandler(jsonHandler, secrets))
	slog.SetDefault(logger)

	// ... rest of startup (DNS, proxy, MITM, health) unchanged ...
}
```

- [ ] **Step 3: Verify build**

Run: `flox activate -- go build ./core/gateway/cmd/gateway/`
Expected: Clean build

- [ ] **Step 4: Run all gateway tests**

Run: `flox activate -- go test ./core/gateway/... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add core/gateway/cmd/gateway/main.go
git commit -m "feat(gateway): integrate TypeScript plugin loader at startup"
```

---

### Task 8: Add --core flag to CLI

**Files:**
- Modify: `cmd/agent-sandbox/main.go`
- Modify: `internal/generate/v1/generator.go` (or wherever core path resolution lives)

- [ ] **Step 1: Find where core path is resolved**

Look at how `agent-sandbox generate` currently resolves `core_version: latest` to a cached directory path. This is in `internal/release/` — the function that returns the core path.

- [ ] **Step 2: Add --core flag to the generate command**

In `cmd/agent-sandbox/main.go`, find `generateV1Cmd` and add a `--core` flag:

```go
func generateV1Cmd(dir *string) *cobra.Command {
	var corePath string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate build artifacts from agent.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			// ... existing logic ...
			// If --core is set, use it directly instead of fetching from releases
			if corePath != "" {
				// Resolve to absolute path
				abs, err := filepath.Abs(corePath)
				if err != nil {
					return fmt.Errorf("resolve --core path: %w", err)
				}
				// Pass abs as the core directory to the generator
				// (replaces the release.Fetch call)
			}
			// ... rest of generate logic ...
		},
	}
	cmd.Flags().StringVar(&corePath, "core", "", "Path to local core directory (skips release download)")
	return cmd
}
```

- [ ] **Step 3: Wire the flag into generator initialization**

The generator constructor (likely `NewGeneratorWithCore` or similar) takes a `coreDir` parameter. When `--core` is set, pass the flag value directly instead of calling `release.Fetch`:

```go
var coreDir string
if corePath != "" {
	coreDir = corePath
} else {
	// Existing logic: fetch from GitHub Releases
	coreDir, err = release.Fetch(cfg.CoreVersion)
	if err != nil {
		return fmt.Errorf("fetch core: %w", err)
	}
}

gen, err := v1.NewGeneratorWithCore(*dir, coreDir)
```

- [ ] **Step 4: Verify it works with local core**

Run: `flox activate -- go build -o ./agent-sandbox-dev ./cmd/agent-sandbox/`
Run: `./agent-sandbox-dev -C examples/local-coding generate --core=./core`
Expected: Generates successfully using local core directory

- [ ] **Step 5: Run tests**

Run: `flox activate -- go test ./cmd/agent-sandbox/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/agent-sandbox/main.go internal/generate/v1/generator.go
git commit -m "feat(cli): add --core flag to generate command for local development"
```
