# Live verification: agent questions (ACP elicitations) in the Helix UI

**Status: IN PROGRESS** — this document is written incrementally as evidence is gathered.
Nothing here is a prediction; every line describes something that was observed.

Branch: `feature/002731-end-to-end-agent` (same commits as helix PR #3009, zed PR #83).
Method: manual drive of the inner dev stack at `http://localhost:8080`, browser automation
plus screenshots, each UI claim cross-checked against `agent_elicitations` rows and API logs.

Scope: **verification only**. No code was changed. Defects, if found, are described here and
reported on the PR — not fixed.

## What is being verified

That a human can actually see and answer a question the agent asks mid-turn. The automated
e2e (incl. Phase 18), the Go handler tests and `tsc`/`vite build` were already green; the
never-proven part was the UI in front of a person.

## Evidence log

| # | Claim | Verdict | Artefact |
|---|---|---|---|
| 1 | Question renders with all options, labels, descriptions, "Other" field | pending | |
| 2 | Answer accepted, turn resumes reflecting the choice | pending | |
| 3 | Answered card survives a page reload | pending | |
| 4 | Normal follow-up still works | pending | |
| 5 | Second question in the same session is uncontaminated | pending | |
| 6 | Skip path settles cleanly and the turn continues | pending | |
| 7 | Interrupt makes the card stop being answerable | pending | |
| 8 | Blocked indicator + auto-wake leaves the interaction alone | pending | |

## 0. Stack bring-up

The startup script (`helix-specs/.helix/startup.sh`) was already running at session start:
`./stack build` → `./stack build-zed release` → `./stack build-sandbox` → `./stack start`.
Zed and zed-agent build stages resolved `CACHED`; the sandbox desktop images were rebuilt.

Bring-up was polled, never blind-slept:

```
docker compose -f docker-compose.dev.yaml ps
curl -s -o /dev/null -w '%{http_code}' http://localhost:8080
```

(Results appended below as they were observed.)

## Blocker found and cleared: the sandbox was running the wrong Zed

First run through the loop **failed**, and the cause was environmental, not the feature.

Observed: a spec task on the Claude Code harness (`claude-opus-5`) did call `AskUserQuestion`
— but Helix rendered it as a **dead tool-call stub**: a collapsible row reading "Which caching
backend should I use for the caching layer in cache-demo-2?" that expands to the same
sentence and nothing else. No options, no descriptions, no "Other" field, no answer control.
The turn sat at "Working for 3m 53s" indefinitely. `agent_elicitations` had **0 rows** and the
API logged **zero** occurrences of `elicit` in ten minutes.

This is precisely the pre-change behaviour the design doc describes.

Root cause: `/home/retro/work/zed` was checked out on **`main`**, and `./stack build-zed`
builds from that working tree. `main` has no elicitation support:

```
# on main
git grep -c elicitation_requested -- crates/external_websocket_sync/src/types.rs   → (no match)
# at the pinned commit
git grep -c elicitation_requested 859325b38f -- crates/…/types.rs                  → 2
# the binary that was actually running
strings zed-build/zed | grep -c elicitation_requested                              → 0
```

`sandbox-versions.txt` pins `ZED_COMMIT=859325b38fa519c28b55157039e7a9fe4990189a`, but that
pin is consumed by **`.drone.yml` only** — CI clones Zed at the pinned commit. The local
`./stack build-zed` path has no such pin; it builds whatever the working tree happens to be.
A local sandbox is therefore not pinned by `sandbox-versions.txt` at all.

Fix applied for this verification (environment only, no code change): checked
`/home/retro/work/zed` out at `859325b38f` and rebuilt. `docker-compose.dev.yaml` bind-mounts
`./zed-build:/helix-dev/zed-build:ro` into the sandbox, so rebuilding the binary is sufficient
— the sandbox image does not need rebuilding.

Evidence: `screenshots/00-wrong-zed-dead-stub.png` (real model tool call, wrong Zed binary).

**Recommendation for the PR (not a change made here):** the local build path silently ignores
the `ZED_COMMIT` pin. Anyone verifying this feature from a dev stack will hit the same dead
stub and may well read it as "the feature is broken". Worth either making `./stack build-zed`
honour `sandbox-versions.txt` or warning when the worktree HEAD ≠ pinned commit.

## Environment gotcha: the sandbox healthcheck can wedge the whole box

Worth recording for anyone else verifying from a dev stack, since it cost ~45 minutes and
looked exactly like "the API is broken".

After the Zed rebuild's I/O storm, the sandbox's inner dockerd wedged. `sandbox-nvidia`'s
healthcheck is:

```yaml
healthcheck:
  test: ["CMD-SHELL", "docker info > /dev/null 2>&1"]
  interval: 30s
  timeout: 5s
```

The 5 s timeout marks the check failed but does **not** kill the hung `docker info`. With the
inner dockerd wedged, one process leaks every 30 s. Observed: **51 `docker info` processes**
stacked in uninterruptible sleep (oldest 45 min), 32 buildx/compose plugin-metadata children,
38 zombies, load average oscillating 30→150, `iowait` pinned near 50 %, CPU pressure
`some avg10=98.8`. The inner Helix API stopped answering on 8080 entirely and stopped logging.

Recovery: `docker restart helix-sandbox-nvidia-1` (it exited 137 and needed
`docker compose up -d sandbox-nvidia` to come back), after which the pile-up went to 0 and
8080 returned 200 immediately. Killing the stacked `docker info` processes alone did **not**
help — they respawn every 30 s until the container is restarted.

Not a bug in this feature, and not something this task changes. Flagging it because the
symptom (API dead, no logs) invites misdiagnosis.

## Provider configuration observed

`/home/retro/work/helix/.env` in the inner stack routes inference through the outer Helix
for both APIs, so a Claude-backed harness is at least configurable:

```
OPENAI_BASE_URL=http://host.docker.internal:8081/v1
ANTHROPIC_BASE_URL=http://host.docker.internal:8081
```

Whether a Claude Code harness agent is actually available to a spec task is recorded below.
