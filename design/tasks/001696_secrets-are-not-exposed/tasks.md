# Implementation Tasks

## Phase 1: API Proxy Setup

- [ ] Create proxy handler for Anthropic API at `/v1/proxy/anthropic/*`
- [ ] Create proxy handler for OpenAI API at `/v1/proxy/openai/*`
- [ ] Add session-based auth injection in proxy handlers
- [ ] Add proxy configuration to `config.go` (upstream URLs, enabled flag)
- [ ] Write unit tests for proxy handlers

## Phase 2: Remove Secrets from Container

- [ ] Update `DesktopAgentAPIEnvVars()` to remove `USER_API_TOKEN`
- [ ] Update `DesktopAgentAPIEnvVars()` to remove `ANTHROPIC_API_KEY`
- [ ] Update `DesktopAgentAPIEnvVars()` to remove `OPENAI_API_KEY`
- [ ] Add `ANTHROPIC_BASE_URL` env var pointing to proxy
- [ ] Add `OPENAI_BASE_URL` env var pointing to proxy
- [ ] Add `X-Helix-Session-ID` header injection for proxy auth

## Phase 3: License Key Protection

- [ ] Create tmpfs mount for `/run/secrets` in container spec
- [ ] Write license key to `/run/secrets/helix_license` file
- [ ] Update nested Helix to read license from file instead of env
- [ ] Remove `LICENSE_KEY` env var from container injection

## Phase 4: macOS App Updates

- [ ] Update `for-mac/settings.go` to match new env var pattern
- [ ] Test macOS desktop app with proxy-based auth
- [ ] Verify Zed IDE integration still works

## Phase 5: Validation

- [ ] Add integration test: verify `env` in container shows no secrets
- [ ] Add integration test: verify AI agent calls work through proxy
- [ ] Add integration test: verify license validation works via file
- [ ] Update documentation for self-hosted deployments
