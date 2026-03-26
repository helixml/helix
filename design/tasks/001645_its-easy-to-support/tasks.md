# Implementation Tasks

## Feature 1: Auto-detect Ollama on Startup

- [x] In `for-mac/vm.go`, add a `probeOllama()` function that does a GET to `http://localhost:11434/v1/models` with a 2s timeout and returns whether Ollama is reachable
- [x] In `waitForReady()`, after the API health check passes, call `probeOllama()` and if true, call the Helix API to upsert a global provider endpoint named "Ollama (local)" with `base_url=http://10.0.2.2:11434/v1`
- [x] Implement the upsert: check `GET /api/v1/provider-endpoints` for an existing entry named "Ollama (local)"; if absent, `POST` it; if present, skip or update base_url
- [x] Use the existing authenticated client (the macOS app already talks to `http://localhost:41080/api/v1`) with the admin/runner token
- [x] Update `createProviderEndpoint` handler to allow runner tokens to create global system-owned endpoints (runner tokens don't have admin flag)

## Feature 2: Auto-rewrite localhost Provider URLs

- [x] In `api/pkg/config/config.go`, add `ProviderLocalhostRewrite string` field backed by env var `PROVIDER_LOCALHOST_REWRITE`
- [x] In `api/pkg/openai/manager/provider_manager.go`, apply localhost rewrite in `initializeClient` before building the OpenAI client
- [x] Apply the rewrite when building the OpenAI client config's `BaseURL`, using the config value
- [x] In `for-mac/vm.go`, `injectDesktopSecret()`, add `PROVIDER_LOCALHOST_REWRITE=10.0.2.2` to the injected env vars map (following the same pattern as `HELIX_EDITION=mac-desktop`)
