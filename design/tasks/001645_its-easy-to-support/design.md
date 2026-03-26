# Design: Easy Ollama Support for Helix macOS App

## Architecture Overview

Two independent features. Both exploit the QEMU networking fact that `10.0.2.2` (inside VM) == `localhost` (on host).

## Feature 1: Auto-detect Ollama on Startup

### Where to implement

In the macOS Go app (`for-mac/vm.go` or `for-mac/app.go`), after the API container is healthy and before returning from `waitForReady`.

### Flow

```
macOS app boots VM
  → waitForReady: SSH ready, Docker stack up, API health check passes
  → NEW: probe http://localhost:11434/v1/models (host-side, short timeout ~2s)
    → if 200: call API to create/update global provider endpoint
             BaseURL = http://10.0.2.2:11434/v1, Name = "Ollama (local)"
    → if fail: skip silently
  → return ready
```

### API Call

Use the existing provider endpoint API: `POST /api/v1/provider-endpoints` (or `PUT` if exists). The macOS app already has an authenticated HTTP client to the VM API at `http://localhost:41080`. Check for existing endpoint by name `"Ollama (local)"` first to avoid duplicates.

Endpoint struct fields to set:
- `name`: `"Ollama (local)"`
- `base_url`: `http://10.0.2.2:11434/v1`
- `endpoint_type`: `global`
- `owner_type`: `system`
- no API key needed (Ollama has no auth by default)

### Why in the Go app, not the API?

The macOS Go app is the natural place to probe host-side resources (it runs on the host). It already injects other host-aware configuration. The API code should not have macOS-specific probing logic.

---

## Feature 2: Auto-rewrite localhost URLs

### Where to implement

**Two parts:**

1. **Env injection** (`for-mac/vm.go`, `injectDesktopSecret` function):
   Add `PROVIDER_LOCALHOST_REWRITE=10.0.2.2` to the set of variables written to `.env.vm` on the guest. This follows the existing pattern for `HELIX_EDITION`, `GPU_VENDOR`, etc.

2. **Rewrite logic** (`api/pkg/openai/openai_client.go` or the provider manager):
   When constructing the HTTP client or resolving a base URL, check `PROVIDER_LOCALHOST_REWRITE` env var. If set, replace `localhost` and `127.0.0.1` in the base URL with its value.

### Rewrite Implementation Detail

```go
// In a shared helper, called when building provider client config:
func rewriteLocalhostIfNeeded(baseURL string) string {
    rewriteTo := os.Getenv("PROVIDER_LOCALHOST_REWRITE")
    if rewriteTo == "" {
        return baseURL
    }
    baseURL = strings.ReplaceAll(baseURL, "localhost", rewriteTo)
    baseURL = strings.ReplaceAll(baseURL, "127.0.0.1", rewriteTo)
    return baseURL
}
```

Call this when building the OpenAI client config (in `openai_client.go` where `BaseURL` is set), not when storing. Stored URL in DB remains as-entered.

### Why env var, not a config flag?

The env var approach requires zero changes to the user-facing settings UI, affects only the desktop app deployment, and is consistent with how the app already injects runtime configuration.

---

## Key Codebase Patterns (for implementor)

- `for-mac/vm.go`: `injectDesktopSecret()` builds the env map and writes to `.env.vm` on guest via SSH, then restarts API container if changed. Add new vars here.
- `for-mac/vm.go`: `waitForReady()` is the startup sequence. Ollama probe goes at the end, after API health check.
- `api/pkg/openai/openai_client.go`: Creates `openai.ClientConfig` with `BaseURL`. This is where the rewrite should be applied.
- `api/pkg/config/config.go`: Defines all env-var-backed config structs. Add `ProviderLocalhostRewrite` field here.
- Provider endpoints API: `POST /api/v1/provider-endpoints` handled in `api/pkg/server/provider_handlers.go`. The macOS app can use this endpoint directly with the admin token.
- `10.0.2.2` is already used in the codebase for DRM manager → QEMU frame export. Same networking pattern.
