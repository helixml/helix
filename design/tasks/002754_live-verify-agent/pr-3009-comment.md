## Live verification: **NOT VERIFIED** — one real blocker found and fixed, then the sandbox gave out

Full write-up: `design/2026-08-12-agent-questions-live-verification.md` (pushed to `feature/002731-end-to-end-agent`). Screenshots in `helix-specs@helix-specs:design/tasks/002754_live-verify-agent/screenshots/`.

Reporting what I observed, not what should happen. **I did not manage to see a question rendered as a real card, answer it, and watch the turn resume.** I also did not disprove it — three attempts never got far enough to exercise the feature. No elicitation was injected; there are no simulated screenshots here.

### Confirmed

- A **Claude Code** harness agent exists in the inner Helix (`claude-opus-5` and friends, routed via the outer Helix's Anthropic endpoint). Not a blocker.
- The model **does** call `AskUserQuestion` on request — first try.
- A spec task provisions a repo and gets a real `zed_thread_id`.

### The blocker worth acting on: `sandbox-versions.txt` doesn't pin local builds

Run 1 reproduced the exact pre-change behaviour — a **dead tool-call stub**: a row reading "Which caching backend should I use…" that expands to the same sentence and nothing else. No options, no descriptions, no "Other", no Skip. Turn stuck at "Working for 3m 53s". `agent_elicitations` had **0 rows**; the API logged **0** `elicit` lines in ten minutes. (`00-wrong-zed-dead-stub.png`)

Cause: `/home/retro/work/zed` was on **`main`**, and `./stack build-zed` builds from the worktree.

```
main            : git grep -c elicitation_requested -- crates/external_websocket_sync/src/types.rs → no match
859325b38f      : same grep → 2
running binary  : grep -c elicitation_requested zed-build/zed → 0
```

`sandbox-versions.txt` pins `ZED_COMMIT=859325b38f`, but that pin is consumed by **`.drone.yml` only**. The local build path ignores it entirely. Rebuilding at the pinned commit produced a binary that does contain the elicitation wire format. Note `docker-compose.dev.yaml` bind-mounts `./zed-build:ro`, so rebuilding just the binary is enough — no sandbox image rebuild.

**Recommendation (no change made):** make `./stack build-zed` honour `sandbox-versions.txt`, or warn when the Zed worktree HEAD ≠ pinned commit. Anyone verifying this feature locally will otherwise hit the dead stub and reasonably conclude the feature is broken.

### Why verification stopped

After the rebuild, three runs never got a `zed_thread_id`. Zed and `claude-agent-acp` were both running each time; the transport was flapping:

```
websocket: close 1006 (abnormal closure): unexpected EOF
[CONNECT] Session loaded for reconnect … zed_thread_id=        ← still empty
RevDial WebSocket ping failed … i/o timeout / broken pipe
```

Run 2 ended `error`: *"Agent never connected after auto-wake cold-start retries (no WebSocket — see #2397)"*.

Underneath that, an environment trap: `sandbox-nvidia`'s healthcheck is `docker info` every 30 s with a 5 s timeout that **doesn't kill the hung process**. Once the inner dockerd wedges, one leaks every 30 s — I measured **51** stacked in uninterruptible sleep, load oscillating 30→150, iowait ~50 %, and the inner API dead on 8080 with no logs. Recovery is `docker restart helix-sandbox-nvidia-1` (exits 137, then needs `compose up -d`); killing the processes alone doesn't help. It recurred on a ~25 min cycle, each time triggered by a desktop cold-start.

### Also hit on the way in (unrelated to this PR)

Project creation fails with *"default new project agent provider and model are not configured in Admin > System Settings"* (`app_handlers.go:653`) on a fresh stack, and there's no `/admin/settings` route — I set it via `PUT /api/v1/system/settings`.

Nothing merged, nothing refactored, no new PRs.
