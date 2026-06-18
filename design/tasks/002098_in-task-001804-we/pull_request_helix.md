# Auto-approve tool calls in qwen-code ACP sessions

## Summary

Qwen Code agents launched from Helix spec tasks were stalling on every file edit because Helix injected no `default_mode` for the qwen entry in Zed's `agent_servers` settings. Without it, qwen-code's `Session.setMode` left the session in `ApprovalMode.DEFAULT` and routed each tool call through `session/request_permission` to Zed — which nobody clicks in a headless sandbox. The Claude Code branch already injected `default_mode: "bypassPermissions"` (line 220); qwen was the odd one out.

## The fix

One line in `api/cmd/settings-sync-daemon/main.go`: add `"default_mode": "yolo"` to the qwen agent_servers map, alongside `claude_code`'s existing `bypassPermissions` injection.

## Chain that this fix unblocks (verified by code reading)

1. Helix daemon writes `agent_servers.qwen.default_mode = "yolo"` into `~/.config/zed/settings.json` at session start.
2. Zed reads it via `agent_settings.default_mode()` (`agent_servers/src/acp.rs:1685`) and calls `SetSessionModeRequest(session_id, "yolo")` after `new_session` succeeds.
3. qwen-code's `Session.setMode` (`packages/cli/src/acp-integration/session/Session.ts:327-339`) maps `"yolo"` → `ApprovalMode.YOLO` on the live config.
4. qwen-code's `CoreToolScheduler` skips the `awaiting_approval` state entirely when the mode is `YOLO` (`packages/core/src/core/coreToolScheduler.test.ts:1102-1163` pins this behavior upstream).

## Tests

- Added `TestQwenCodeAgentServerHasYoloDefaultMode` (`main_test.go`) — pinning test that fails if the `default_mode` field is removed or changed away from `yolo`. Keeps qwen and claude_code's auto-approve patterns in step.
- Existing tests still pass.

## Verification

- Unit test: PASS
- `./stack build-ubuntu` produced new image `helix-ubuntu:a4dfd0` (prev `5314cc`); `strings /usr/local/bin/settings-sync-daemon` confirms `default_mode` + `yolo` are present in the binary.
- Live end-to-end qwen spec-task session was attempted but blocked by an unrelated, pre-existing bug in the inner Helix's `AdvancedModelPicker` (`enabled: !isLoadingOrg` never goes true under `/onboarding`, so `useListProviders({loadModels: true})` never fires). Tracked as follow-up; not in scope for this PR.

## Also in this PR: model-routing fixes that unblock internal Helix validation

Validating a qwen_code agent end-to-end in the Helix-in-Helix dev environment surfaced two genuine routing bugs (latent in any single Helix too, not just the nested setup). Fixed here so internal validation works:

1. **Model cache format mismatch.** `refreshProviderModels` writes the wrapped `cachedModels{Models,FetchedAt}` payload, but `findProviderWithModel` (chat-completion routing) and `getCachedModels` (aggregate `/v1/models`) read the entry as a bare `[]OpenAIModel`. The decode always failed, so chat routing could only resolve a provider via a static `Models` list (never the dynamically-fetched cache), and `/v1/models` returned empty for env-baked global providers. Both readers now decode via `loadCachedModels`.

2. **No live resolution on a cold cache.** `findProviderWithModel` only reads the cache, which is warmed lazily by `/v1/models` and the model picker. An unprefixed model id — e.g. one a downstream Helix forwards after stripping its provider prefix, or any bare id requested before the cache is warm — fell through to the default `helix` provider and 500'd with "model X is not configured in the default provider". Added `resolveModelProviderLive`: a last-resort resolver that warms each accessible provider's model list (via the shared stale-while-revalidate `getProviderModels` path) and resolves the owning provider. Only runs when both the cache lookup and prefix parsing fail, so prefixed / cache-warm requests are unaffected.

Tests: `TestGetCachedModels_DecodesWrappedPayload`, `TestFindProviderWithModel_ResolvesFromWrappedCache`, and the existing `FindProviderWithModelSuite` updated to seed the real wrapped cache format. Verified live against a running Helix with a fake upstream provider: a bare, cold-cache model now routes and completes instead of 500ing.

Note on the nested-Helix validation topology: the failing hop is the *outer* Helix receiving a prefix-stripped bare model. Inner and outer run the same build, so once this PR is deployed both hops get the fix and qwen agents can run end-to-end against the inner Helix.

## Related context

- Task 002098 also audited the status of task 001804 ("Merge upstream QwenLM/qwen-code v0.14.4"). Finding: the qwen-code feature branch was never merged to qwen-code `main`, and `sandbox-versions.txt` still pins `QWEN_COMMIT=14ebe78ca…` (pre-merge). Helix PR https://github.com/helixml/helix/pull/2238 reused the same branch name but shipped unrelated OpenAI reasoning-field mapper changes. Re-landing 001804 is tracked separately in the design doc.
- Sanity check on upstream future: `QwenLM/qwen-code` is at v0.17.1 (today), 25.2k stars, 434 contributors, very active. Google sunsetting `gemini-cli` June 18, 2026 does not affect our qwen-code dependency.
