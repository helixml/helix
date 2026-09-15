# Requirements: Remove PR Review Coordinator Bootstrap Overhead

## Background

The PR Review Coordinator (`b-pr-coordinator` in the `helix` org, project `prj_01m145sbbc3jyvpcb5ngh6ezvq`) wakes on GitHub PR events and dispatches one skip-planning review SpecTask per (repository, PR, head SHA). Today each dispatch pays avoidable bootstrap overhead:

- The coordinator calls `read_events` to rediscover the event that is already rendered in its activation trigger.
- The coordinator calls `update_spectask` after `create_spectask` just to append a FINISH-STEP instruction.
- The coordinator and the dispatched review task both use the `gh` CLI, which must be installed in the sandbox and hides useful HTTP details.
- Review tasks launch on the full `ubuntu-desktop` runtime although OpenCode review work is terminal-only.

## Isolation model (unchanged)

- One independent skip-planning SpecTask + sandbox per (repository, PR, head SHA). Never reuse a session, sandbox, or conversation across PRs or review tasks.
- `skip_planning: true` and the formal GitHub review behavior (verdict posted as a PR review) are preserved.

## User stories

1. **As the coordinator bot**, when activated by a PR event, I process the event already present in my activation trigger (`subject`, `body`, `extra` payload) so I do not spend a round-trip on `read_events`.
2. **As the coordinator bot**, I embed the review task's completion instruction (FINISH-STEP) in the `create_spectask` description using the `$HELIX_SPEC_TASK_ID` environment variable, so I never need a post-create `update_spectask` call.
3. **As the coordinator bot**, I dispatch each review task with `sandbox_runtime: "headless-ubuntu"` because the review agent is terminal-only (OpenCode, curl, jq, git, helix CLI — no streamed desktop).
4. **As the coordinator bot and as a review task**, I authenticate to GitHub only via `list_secrets` → `get_secret` → `curl` + `jq` against `https://api.github.com`, and I never install or invoke `gh`.
5. **As a review task**, I POST my verdict as a formal review on `POST /repos/helixml/helix/pulls/<N>/reviews` with `commit_id` set to the intended head SHA, using the minted GitHub credential without printing or persisting it.
6. **As an operator**, I still get the same guards: no self-loop (events from the reviewer login are dropped), no duplicate review for a (PR, head SHA) already reviewed or already in flight, and the concurrency cap stays.

Out of scope: event filtering (handled separately as a manual update to the PR Review Coordinator topic/processor).

## Acceptance criteria

- A PR event still creates exactly one isolated review SpecTask for its target PR and head SHA.
- The created task uses `headless-ubuntu`, OpenCode, and the configured project model.
- Neither coordinator nor review-task instructions install or invoke `gh`.
- REST review submission is attached to the intended commit SHA (`commit_id` == reviewed head SHA) and uses the minted GitHub credential without printing or persisting it.
- The coordinator does not call `read_events` or `update_spectask` for the normal dispatch path.
- Existing self-loop and duplicate (PR, head SHA) guards remain intact.
- Validated end to end with a test PR; task URL and review URL recorded.

## Open Questions

- Should the coordinator bot's **own** sandbox runtime also be patched to `headless-ubuntu` (same rationale: its work is also terminal-only)? The requested scope covers only the dispatched review tasks; default is to leave the coordinator on `ubuntu-desktop`.
- Should `read_events` / `update_spectask` / `list_trigger_events` also be **removed from the bot's tool grants**, or only banned in the prompt? Default: prompt-only (acceptance criteria speak of calls, not grants; prompt-only is the smaller diff).
- For the end-to-end test: use this task's own implementation PR as the trigger, or open a separate trivial throwaway PR? Default: whichever fires first cleanly; record which was used.
