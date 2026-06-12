# Implementation Tasks: Verify Qwen-Code Upgrade Landed and Diagnose Per-Tool Permission Prompts

## Phase 1 — Audit

- [ ] `git fetch --all` in `/home/retro/work/qwen-code` and confirm `main` tip is still `14ebe78ca`
- [ ] List commits on `origin/feature/001804-we-havent-updated-qwen` not on `main` (expect 3: upstream merge, completion, telemetry-off)
- [ ] Check whether a qwen-code PR exists for that branch in the internal git server; record its state (open / closed / never opened)
- [ ] Confirm helix PR #2238 (`a532195d1`) ships only the OpenAI reasoning-field mapper, not a `QWEN_COMMIT` bump
- [ ] Confirm `sandbox-versions.txt` still pins `QWEN_COMMIT=14ebe78ca…`
- [ ] Query the outer LLM proxy from inside the inner Helix sandbox to list available models and check for `glm-5.1`
- [ ] If GLM-5.1 absent, ask user to wire it up OR pick the largest available GLM/Qwen-coder model as a fallback

## Phase 2 — Reproduce permission-prompt bug

- [ ] Register and onboard in inner Helix (`test@helix.ml` / `helixtest`, org `testorg`)
- [ ] Create a `qwen_code` agent in `testproj` pointed at the chosen model
- [ ] Start a spec task that requires at least one file edit
- [ ] Stream the Zed session and tail API + sandbox logs grepping for `requestPermission|approval`
- [ ] Capture a screenshot of the Zed pane (with or without permission prompt) into `screenshots/`
- [ ] Write a 1-paragraph reproduction summary into `design.md` under a new "Reproduction Result" heading

## Phase 3 — Fix (only if Phase 2 reproduces)

- [ ] Locate where Helix builds the qwen-code launch command (`grep -rn qwen-code api/`)
- [ ] Check qwen-code CLI source for a startup `--approval-mode` flag (`grep -rn "approval-mode\|approvalMode" packages/cli/src/`)
- [ ] Pick Option A / B / C per design.md and document choice with rationale
- [ ] Implement the chosen fix
- [ ] Rebuild affected components per CLAUDE.md's "When to Rebuild What" table
- [ ] Re-run the Phase 2 reproduction; confirm the agent now edits files autonomously
- [ ] Capture a second screenshot showing the fix

## Phase 4 — Land 001804 properly

- [ ] If qwen-code feature branch is still mergeable, open / re-open its PR; otherwise rebase
- [ ] Once qwen-code PR is merged (or before, per CLAUDE.md's strict ordering rule), open a Helix PR that bumps `sandbox-versions.txt` `QWEN_COMMIT` to the new tip
- [ ] Merge Helix PR after qwen-code PR
- [ ] Verify CI (Drone) builds the new sandbox image with the merged qwen-code

## Closeout

- [ ] If the permission fix needs a Zed-side change (Option B), open a separate spec task and link it from `design.md`
- [ ] Note in `design.md` why helix PR #2238 shipped unrelated content under the 001804 branch name (process bug worth flagging to user)
