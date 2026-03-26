# Implementation Tasks

## Feature 1: Auto-detect Ollama on Startup

- [ ] In `for-mac/vm.go`, add a `probeOllama()` function that does a GET to `http://localhost:11434/v1/models` with a 2s timeout and returns whether Ollama is reachable
- [ ] In `waitForReady()`, after the API health check passes, call `probeOllama()` and if true, call the Helix API to upsert a global provider endpoint named "Ollama (local)" with `base_url=http://10.0.2.2:11434/v1`
- [ ] Implement the upsert: check `GET /api/v1/provider-endpoints` for an existing entry named "Ollama (local)"; if absent, `POST` it; if present, skip or update base_url
- [ ] Use the existing authenticated client (the macOS app already talks to `http://localhost:41080/api/v1`) with the admin/runner token

## Feature 2: Auto-rewrite localhost Provider URLs

- [ ] In `api/pkg/config/config.go`, add `ProviderLocalhostRewrite string` field backed by env var `PROVIDER_LOCALHOST_REWRITE`
- [ ] In `api/pkg/openai/openai_client.go`, add a `rewriteLocalhostIfNeeded(baseURL, rewriteTo string) string` helper that replaces `localhost` and `127.0.0.1` with `rewriteTo`
- [ ] Apply the rewrite when building the OpenAI client config's `BaseURL`, using the config value
- [ ] In `for-mac/vm.go`, `injectDesktopSecret()`, add `PROVIDER_LOCALHOST_REWRITE=10.0.2.2` to the injected env vars map (following the same pattern as `HELIX_EDITION=mac-desktop`)
