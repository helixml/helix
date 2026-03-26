# Add Ollama support for macOS desktop app users

## Summary
Makes Ollama "just work" for the majority of macOS Helix users who already run Ollama locally. Two complementary features: auto-detection on startup (zero config) and automatic localhost URL rewriting (safety net for manual config).

## Changes
- `for-mac/vm.go`: Add `probeOllama()` to check if Ollama is running on host (2s timeout), `upsertOllamaEndpoint()` to register it as a global provider via the API, and call both from `waitForReady()` after sandbox is ready
- `for-mac/vm.go`: Inject `PROVIDER_LOCALHOST_REWRITE=10.0.2.2` into VM env via `injectDesktopSecret()` so user-configured `localhost` URLs work transparently
- `api/pkg/config/config.go`: Add `Providers.LocalhostRewrite` field backed by `PROVIDER_LOCALHOST_REWRITE` env var
- `api/pkg/openai/manager/provider_manager.go`: Apply localhost→rewrite-address substitution in `initializeClient()` before constructing the HTTP client
- `api/pkg/server/provider_handlers.go`: Allow runner tokens to create global system-owned provider endpoints (needed for the auto-detection flow)

## Testing
- Auto-detect: start the macOS app with Ollama running; check that "Ollama (local)" appears in provider endpoints without any user action
- Localhost rewrite: configure a provider at `http://localhost:11434/v1` via the UI; verify requests succeed (rewritten to `http://10.0.2.2:11434/v1` at request time, stored URL unchanged)
- Non-macOS: verify `PROVIDER_LOCALHOST_REWRITE` is not set in k8s/docker-compose; existing provider URLs unaffected
