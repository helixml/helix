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

## Provider configuration observed

`/home/retro/work/helix/.env` in the inner stack routes inference through the outer Helix
for both APIs, so a Claude-backed harness is at least configurable:

```
OPENAI_BASE_URL=http://host.docker.internal:8081/v1
ANTHROPIC_BASE_URL=http://host.docker.internal:8081
```

Whether a Claude Code harness agent is actually available to a spec task is recorded below.
