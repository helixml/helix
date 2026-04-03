# Requirements: Secrets Not Exposed to Human Desktop

## Problem Statement

The human desktop (container environment where users interact with AI agents) currently receives sensitive credentials via environment variables. These secrets are visible to any process running in the container, potentially exposing them through:
- Process inspection (`ps aux`, `/proc/<pid>/environ`)
- Container logs
- MCP server processes
- User-initiated commands that dump env vars

## User Stories

### US1: Secrets Invisible to Desktop Processes
**As a** security-conscious user
**I want** my API keys and tokens to not be visible in the desktop container
**So that** malicious or careless processes cannot steal my credentials

**Acceptance Criteria:**
- [ ] `USER_API_TOKEN` not visible in container environment
- [ ] `ANTHROPIC_API_KEY` not visible in container environment
- [ ] `OPENAI_API_KEY` not visible in container environment
- [ ] `ZED_HELIX_TOKEN` not visible in container environment
- [ ] Running `env` or inspecting `/proc/*/environ` shows no secrets

### US2: AI Agents Still Function
**As a** developer using Helix
**I want** AI agents to continue working after secret hiding
**So that** security improvements don't break functionality

**Acceptance Criteria:**
- [ ] Claude/Anthropic API calls still work from desktop
- [ ] OpenAI-compatible calls still work
- [ ] Zed IDE integration still functions
- [ ] MCP servers can still make authenticated requests

### US3: License Key Protection
**As a** Helix operator
**I want** the license key hidden from nested Helix containers
**So that** license keys cannot be extracted by users

**Acceptance Criteria:**
- [ ] `LICENSE_KEY` not visible in container environment
- [ ] Nested Helix instances still validate licenses

## Out of Scope

- OAuth provider ClientSecret masking in API responses (separate task)
- Desktop auto-login token URL exposure (separate task)
- Encryption key fallback hardening (separate task)

## Technical Constraints

- Desktop containers run user code that cannot be trusted
- MCP servers need to make authenticated API calls
- Helix API proxy handles auth for most requests
- Solution must work for both macOS app and cloud deployments
