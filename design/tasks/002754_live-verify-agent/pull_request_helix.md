# Add live-verification report for agent questions (ACP elicitations)

## Summary

Documentation only — no product code changed. This records a live attempt to verify, with a
real user in the Helix UI, that an agent question asked via `AskUserQuestion` reaches a human,
can be answered, and resumes the turn.

**Verdict: NOT VERIFIED.** The user-facing loop was neither proven to work nor proven broken.
One real blocker was found and fixed along the way; after that the sandbox could not hold an
agent WebSocket open long enough to reach a question. Reporting this honestly rather than
claiming a pass — a false green here would be worse than no result.

No elicitation was ever injected, so there are no simulated screenshots in this report.

## Changes

- `design/2026-08-12-agent-questions-live-verification.md` — full write-up: what was
  confirmed, what was not, the blocker and its root cause, the environment failure that
  stopped the run, and a shorter path for whoever picks it up next.

## What was confirmed

- A **Claude Code** harness agent exists in the inner Helix (`claude-opus-5` via the outer
  Helix's Anthropic endpoint) — not a blocker.
- The model **does** call `AskUserQuestion` when asked to, first try.
- A spec task provisions a repo and gets a real `zed_thread_id`.

## The blocker worth acting on

Run 1 reproduced the exact pre-change behaviour — a **dead tool-call stub**: a row reading
"Which caching backend should I use…" that expands to the same sentence and nothing else. No
options, no descriptions, no "Other", no Skip. `agent_elicitations` had 0 rows and the API
logged 0 `elicit` lines in ten minutes.

Cause: `/home/retro/work/zed` was on `main`, and `./stack build-zed` builds from the worktree.
`sandbox-versions.txt` pins `ZED_COMMIT`, but that pin is consumed by **`.drone.yml` only** —
the local build path ignores it entirely.

```
main           : git grep -c elicitation_requested -- crates/external_websocket_sync/src/types.rs → no match
859325b38f     : same grep → 2
running binary : grep -c elicitation_requested zed-build/zed → 0
```

**Recommendation (no change made here):** make `./stack build-zed` honour
`sandbox-versions.txt`, or warn when the Zed worktree HEAD differs from the pinned commit.
Anyone verifying this feature from a dev stack will otherwise hit the same dead stub and
reasonably conclude the feature is broken.

## Why verification stopped

After rebuilding Zed at the pinned commit, four further runs never got a `zed_thread_id`. Zed
and `claude-agent-acp` were live each time; the sync WebSocket flapped every ~2 minutes
(`close 1006 (abnormal closure)`, RevDial `i/o timeout`). One run ended `error`: *"Agent never
connected after auto-wake cold-start retries (no WebSocket — see #2397)"*.

Underneath that, an environment trap documented in the report: `sandbox-nvidia`'s healthcheck
is `docker info` every 30 s with a 5 s timeout that does not kill the hung process. Once the
inner dockerd wedges, one leaks every 30 s — 51 were measured stacked in uninterruptible
sleep, with the inner API dead on 8080 and no logs. It recurred five times on a ~25 minute
cycle.

## Screenshots

![Real AskUserQuestion rendered as a dead stub, wrong Zed binary](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002754_live-verify-agent/screenshots/00-wrong-zed-dead-stub.png)
![ACP error after the Zed rebuild](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002754_live-verify-agent/screenshots/01-acp-error-after-zed-rebuild.png)
![Third task: ACP error, desktop paused](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002754_live-verify-agent/screenshots/02-third-task-acp-error-desktop-paused.png)
![Run 5: desktop restarting on the fixed Zed](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002754_live-verify-agent/screenshots/03-run5-desktop-restarting-fixed-zed.png)
