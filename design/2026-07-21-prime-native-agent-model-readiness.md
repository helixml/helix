# Prime NativeAgent model readiness

Date: 2026-07-21
Status: implemented and verified on Prime; approved for PR

## Incident

Resurrecting the `exploding-kittens` Chief of Staff loaded a stale Zed ACP
thread that no longer existed. Recovery took 60 seconds because Prime's
deployed Zed binary predated the already-merged `thread_load_error` patch, even
though Helix pinned the corrected Zed commit.

Rebuilding the pinned Zed on Prime restored immediate missing-thread error
reporting. That deployment exposed a separate model-readiness bug.

## Root cause

`AgentSettings` configured an exact Anthropic model, but NativeAgent
`new_session` ran before the language model registry and NativeAgent's private
model cache contained that model. The existing selector therefore persisted
the environment fallback `openai` and `gpt-5-mini`. Current Zed then sent
`gpt-5-mini` through `/v1/responses`, which Helix does not serve and returned
404.

## Fix

NativeAgent thread creation now performs one atomic UI update that checks the
exact provider and model from `AgentSettings` against both
`LanguageModelRegistry` and NativeAgent's private model cache. It constructs
the session in that same update only when all three agree.

If the exact configured model is not ready, thread creation retries for up to
15 seconds. A timeout returns a request-correlated `ChatResponseError` and does
not create or persist a session. The existing model selector and fallback are
unchanged. Custom agents do not use this readiness gate and are unchanged.

## Verification

- Two focused tests passed on Prime.
- `./stack build-zed release`, `./stack build-ubuntu`, and
  `./stack build-sandbox` passed on Prime.
- The deployed desktop image is `9a47ac`; the sandbox artifact starts with
  `c58e55`.
- Fresh org task `spt_01ky4fkhdtxhbge4fjd52gtb48`, session
  `ses_01ky4fkhhegnbrx0bwxj4yqa7y`, and ACP thread
  `965840c7-0154-49f6-aaf1-7d9078595aa0` used the exact `anthropic` provider
  and `claude-opus-4-6` model.
- Initial request `req_01ky4fkhhfehbb0k0dvpsr231t` returned
  `INITIAL_READY`. The immediately following interaction
  `int_01ky4fpk41sq3vp5qbssz9v2f7` returned `FOLLOWUP_READY`.
- Logs and persisted state showed no fallback model, settings rewrite, or
  thread error.
- NOT tested: forcing the 15-second timeout in a live shared Prime session.
  Mutating shared Prime provider readiness would be unsafe. The deterministic
  timeout test passed.

## Related work

The `stop-auto-wake-prompt-replay` change is complementary. Deploy its Helix
change before deploying this Zed change so Helix stops replaying a prompt after
a terminal thread-creation error.

That Helix change has a medium-risk stale-snapshot full-save race at
`auto_wake_stuck_interactions.go:354`: an interaction loaded as Waiting can be
completed concurrently, then overwritten back to an error by the full save.
Replace it with a conditional update guarded by `state = waiting`.

The implementation and this note are approved for PR.
