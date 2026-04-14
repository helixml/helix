# Requirements: Merge Upstream qwen-code

## Context

Our fork of `QwenLM/qwen-code` (adapted from `gemini-cli`) is pinned at **v0.4.1** with 25 custom commits on top. Upstream is now at **v0.14.4** — a gap of ~10 major versions. The ACP protocol has evolved significantly upstream (SDK `@agentclientprotocol/sdk` now at `^0.14.1`), and many of our custom changes may now be redundant.

## User Stories

1. **As a maintainer**, I want to merge upstream v0.14.4 into our fork so we get bug fixes, new features, and ACP protocol improvements without maintaining a stale fork.

2. **As a maintainer**, I want a clear inventory of our 25 custom commits — which are still needed vs. superseded by upstream — so we don't carry dead code forward.

3. **As a developer using the fork**, I want the merge to preserve any container/sandbox-specific changes we still need (bind-mount path normalization, security check disabling, `QWEN_DATA_DIR`→`QWEN_RUNTIME_DIR` migration).

## Acceptance Criteria

- [ ] Upstream `QwenLM/qwen-code` `main` (v0.14.4) is merged into our fork's `main` branch
- [ ] All merge conflicts are resolved
- [ ] Each of our 25 custom commits is categorized as: **keep**, **drop** (upstream supersedes), or **adapt** (partial overlap, needs rework)
- [ ] Fork-specific `schema.ts` (ACP Zod schema) is reconciled with upstream's ACP structure
- [ ] `QWEN_DATA_DIR` env var is migrated to upstream's `QWEN_RUNTIME_DIR` pattern (or kept alongside with a note)
- [ ] Sandbox security disabling still works in our containerized environment
- [ ] `npm install` and `npm run build` succeed after merge
- [ ] ACP integration tests pass (`integration-tests/acp-integration.test.ts`)
