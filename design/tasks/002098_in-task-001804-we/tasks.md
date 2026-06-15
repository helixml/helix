# Implementation Tasks: Verify Qwen-Code Upgrade Landed and Diagnose Per-Tool Permission Prompts

## Phase 1 — Audit

- [x] `git fetch --all` in `/home/retro/work/qwen-code` and confirm `main` tip is still `14ebe78ca`
- [x] List commits on `origin/feature/001804-we-havent-updated-qwen` not on `main` (confirmed: 3 commits — upstream merge, completion, telemetry-off)
- [~] Check whether a qwen-code PR exists for that branch in the internal git server; record its state (open / closed / never opened) — no working API endpoint found; need user input or UI check
- [x] Confirm helix PR #2238 (`a532195d1`) ships only the OpenAI reasoning-field mapper, not a `QWEN_COMMIT` bump
- [x] Confirm `sandbox-versions.txt` still pins `QWEN_COMMIT=14ebe78ca…`
- [x] Query the outer LLM proxy from inside the inner Helix sandbox to list available models and check for `glm-5.1` (found via nebius and togetherai)
- [x] If GLM-5.1 absent, ask user to wire it up OR pick the largest available GLM/Qwen-coder model as a fallback (chosen: `nebius / zai-org/GLM-5.1`)

## Phase 2 — Diagnose permission-prompt bug (code-level)

- [x] Inspect Zed's ACP client integration — found `acp.rs:1685` auto-sends `SetSessionModeRequest` when agent settings include `default_mode`
- [x] Inspect qwen-code's `Session.setMode` — confirmed `"yolo"` → `ApprovalMode.YOLO` mapping at `Session.ts:327-339`
- [x] Inspect Helix `settings-sync-daemon` — found `claude_code` already injects `default_mode: "bypassPermissions"` (line 220) but `qwen_code` does NOT. **This is the bug.**
- [~] Live qwen spec-task session in inner Helix — blocked by independent pre-existing picker cache-freshness bug (see design.md "Verification Status")

## Phase 3 — Fix

- [x] Add `default_mode: "yolo"` to `qwen_code` branch in `api/cmd/settings-sync-daemon/main.go`
- [x] Add pinning unit test `TestQwenCodeAgentServerHasYoloDefaultMode` in `main_test.go`
- [x] Run unit tests — PASS
- [x] Rebuild `helix-ubuntu` image — new tag `a4dfd0`
- [x] Verify the rebuilt image's daemon binary contains `default_mode`, `yolo`, and `bypassPermissions` strings
- [ ] **OPTIONAL FOLLOW-UP**: Fix the inner Helix model picker cache bug so live qwen e2e can be run (separate spec task)

## Phase 4 — Land 001804 properly

- [ ] If qwen-code feature branch is still mergeable, open / re-open its PR; otherwise rebase
- [ ] Once qwen-code PR is merged (or before, per CLAUDE.md's strict ordering rule), open a Helix PR that bumps `sandbox-versions.txt` `QWEN_COMMIT` to the new tip
- [ ] Merge Helix PR after qwen-code PR
- [ ] Verify CI (Drone) builds the new sandbox image with the merged qwen-code

## Closeout

- [ ] If the permission fix needs a Zed-side change (Option B), open a separate spec task and link it from `design.md`
- [ ] Note in `design.md` why helix PR #2238 shipped unrelated content under the 001804 branch name (process bug worth flagging to user)
