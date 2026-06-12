# Requirements: Verify Qwen-Code Upgrade Landed and Diagnose Per-Tool Permission Prompts

## Context

Spec task `001804` ("we haven't updated qwen-code in quite some time") was meant
to merge upstream `QwenLM/qwen-code` v0.14.4 into the Helix fork (previously
pinned at v0.4.1 + 25 custom commits). The user suspects two problems:

1. The PRs from task 001804 may not have actually landed in the `qwen-code`
   and `helix` repos — work might be sitting on feature branches.
2. Even if the upgrade landed, **qwen-code agents still appear to ask for
   per-tool permission on every file edit**, which makes them unusable as
   autonomous spec-task workers. Task 001804 never explicitly addressed
   approval mode, so the issue is likely still open.

## Investigation findings (already gathered during planning)

These are facts established by reading the repos, not speculation:

- `qwen-code` branch `feature/001804-we-havent-updated-qwen` contains 3
  commits on top of `14ebe78ca` (the previous fork tip): the upstream
  v0.14.4 merge (`420b2013a`), a "Complete implementation" commit
  (`f01cdc413`), and telemetry disabling (`ca5f3a28c`).
- **None of those commits are on `qwen-code` `main`.** `git log main`
  in `/home/retro/work/qwen-code` still has `14ebe78ca` as the tip.
- `helix-specs/design/tasks/001804_*` includes a `pull_request_qwen-code.md`
  describing the qwen-code PR, but no helix PR description.
- `helix` PR `#2238` (commit `a532195d1`) was opened from a branch named
  `feature/001804-we-havent-updated-qwen` AND was merged into helix main —
  but the diff contains only `api/pkg/openai/openai_client.go` (+123) and
  `api/pkg/openai/reasoning_field_mapper_test.go` (+365). That is an
  OpenAI reasoning-field mapping fix, **not** a qwen-code commit bump.
  Someone reused the branch name for a different change.
- `/home/retro/work/helix/sandbox-versions.txt` still pins
  `QWEN_COMMIT=14ebe78ca83328323bbaa8cc714d8f3b4a6fce46` — the pre-merge
  commit. CI therefore still builds the old qwen-code.
- The fork's `packages/cli/src/acp-integration/session/Session.ts:317-339`
  shows qwen-code accepts an ACP `session/set_mode` request mapping
  `default`/`auto-edit`/`yolo` → `ApprovalMode`. Tools requiring approval
  call `client.requestPermission()` (line 492). If Zed never tells the
  agent to enter `auto-edit`/`yolo`, every edit will round-trip a
  permission request to Zed.
- Helix's `zed_config.go:213` sets `AlwaysAllowToolActions: true`, which
  `zed_config_handlers.go:239` translates to `tool_permissions.default =
  "allow"`. That governs **Zed's own** tool-use prompts — it does not
  automatically forward to a child ACP agent like qwen-code, which has
  its own approval state machine.

## User Stories

1. **As a maintainer**, I want a clear yes/no on whether task 001804
   actually shipped, so we know whether to re-open the PRs or treat it as
   merged.

2. **As a spec-task operator**, I want qwen-code agents to run in an
   auto-approve mode by default in Helix-launched sandboxes, so they
   don't block on every file edit waiting for a permission click that
   no one will give.

3. **As a developer**, I want to confirm the fix in the inner Helix by
   running a qwen-code spec task against a real model (GLM-5.1 if
   available via the outer LLM proxy) and watching it edit files
   without intervention.

## Acceptance Criteria

### Investigation (must complete before any code changes)

- [ ] Confirmed (with `git log` evidence) whether `qwen-code` PR for
      branch `feature/001804-we-havent-updated-qwen` was opened, and its
      current state (open / merged / closed-unmerged).
- [ ] Confirmed `helix` PR #2238 contents do not bump `QWEN_COMMIT`.
- [ ] Confirmed current `sandbox-versions.txt` `QWEN_COMMIT` value and
      mapped it to a specific qwen-code commit (pre- or post-merge).
- [ ] Determined whether GLM-5.1 is reachable through the outer LLM
      proxy. If not, list which GLM/Qwen-class models *are* reachable
      and pick one for the test.

### Reproduction

- [ ] In the inner Helix, started a spec task using a qwen-code agent
      bound to the chosen model.
- [ ] Observed (and documented with screenshots / log excerpts) whether
      the agent asks for per-tool permission on file edits, or runs them
      autonomously.

### Remediation (only if reproduction confirms the bug)

- [ ] Identified where the auto-approve mode should be set:
      (a) Helix `zed_config.go` injects a hint to Zed,
      (b) Zed's ACP client auto-calls `session/set_mode = "yolo"` when
          the user has `tool_permissions.default = "allow"`, or
      (c) qwen-code's CLI accepts a `--approval-mode yolo` startup flag
          and Helix passes it via the agent's launch command.
- [ ] Implemented and tested the chosen fix end-to-end in the inner
      Helix.
- [ ] Bumped `sandbox-versions.txt` `QWEN_COMMIT` to the merged commit
      (whatever finally lands, including any new approval-mode patch).

### Closeout

- [ ] If task 001804's PRs are still unmerged, either land them or write
      a short note in the design doc explaining why they were abandoned.
- [ ] If the permission issue's root cause is on the Zed side (option b
      above), open a separate spec task for the Zed-side fix and link it.
