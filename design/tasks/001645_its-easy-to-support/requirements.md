# Requirements: Easy Ollama Support for Helix macOS App

## Background

The Helix macOS app runs inside a QEMU VM. From the guest VM, the host machine is reachable at `10.0.2.2` (QEMU user networking). Many Mac users already run Ollama locally. Currently they must know to configure `http://10.0.2.2:11434/v1` instead of the intuitive `http://localhost:11434/v1`, and there's no auto-detection.

## User Stories

### Story 1: Auto-detect Ollama (zero config)

**As a** macOS Helix user who already has Ollama running,
**I want** Helix to detect Ollama automatically on startup,
**So that** my Ollama models just appear without any configuration.

**Acceptance Criteria:**
- On app startup, the macOS Go app probes `localhost:11434/v1/models` on the host
- If Ollama responds with a valid model list, a global provider endpoint is automatically created/updated in the VM pointing to `http://10.0.2.2:11434/v1`
- The probe happens before or shortly after the API container is ready
- If Ollama is not running, nothing changes and no error is shown
- If Ollama later becomes unavailable, the endpoint is not actively removed (avoids churn)

### Story 2: Auto-rewrite localhost provider URLs

**As a** macOS Helix user who manually configured Ollama at `http://localhost:11434/v1`,
**I want** that URL to work even though Helix runs in a VM,
**So that** I can use the intuitive localhost URL without knowing about `10.0.2.2`.

**Acceptance Criteria:**
- The macOS app injects `PROVIDER_LOCALHOST_REWRITE=10.0.2.2` into the VM environment (alongside existing injected vars like `HELIX_EDITION=mac-desktop`)
- When the API makes outbound requests to a provider endpoint, if `PROVIDER_LOCALHOST_REWRITE` is set and the base URL contains `localhost` or `127.0.0.1`, it is rewritten to the configured address before the request
- This rewrite happens transparently; the stored URL in the database is unchanged
- No effect on non-macOS deployments (env var is not set in k8s/docker-compose)
- Users who correctly configured `10.0.2.2` are unaffected
