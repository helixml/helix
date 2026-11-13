# Frontend test suite: speed and CI reliability (2026-11-12)

## Failure audit: last 250 Drone builds

90 of the 250 builds ran `build-frontend`, and 50 of those failed. Every failing log was read and classified:

| Class | Builds | Runner | What happened |
|---|---|---|---|
| Mass timeouts | 28 | all node03 | 49–139 test files fail at the 5s default timeout in a single run. The runner was starved of CPU, so the branch itself was not broken. |
| A few tests fail | 19 | 18 node03, 1 node04 | Some tests time out at 5s and some time-sensitive assertions fail (listed below). |
| Worker crash | 2 | node04 | Every test passed, then `Error: Worker exited unexpectedly` (tinypool fork exit). Most likely the OOM killer on the shared runner; not reproduced locally. |
| Real breakage | 1 | node03 | 5335: `error TS2339` on `codex/logout-confirm`. |

So 49 of the 50 failures were flaky or infrastructure-related, and 1 was a real break. Of the 56 builds that ran `build-frontend` on node03, only 9 passed; on luke-node04, 26 of 29 passed. Commit 827180075 failed `build-frontend` 4 times and passed it twice. node03 being degraded is an infrastructure problem (see the drone-runners memory note). The test-side causes below made the suite fail whenever a runner was slow.

### Root causes fixed

- **RobustPromptInput, grace-window test:** the entry was placed 150ms inside the 2s window on the real clock, so any render slower than 150ms already showed the panel. The test now uses fake timers.
- **RobustPromptInput, queue-pump tests:** they slept 800ms, or waited up to 2s, for the pump's 300/500ms timers. They now use fake timers.
- **SessionPromptQueue:** the test asserted `listPrompts` was called exactly 2 times, but the hook also polls every 2s, so a slow run made a third call. The test now asserts the refreshed list contents.
- **ProjectChatGroup:** the test waited for a MUI tooltip's exit animation (real timers) to finish within waitFor's 1s budget.
- **findBy\*/waitFor on CI:** vitest's CI test timeout had been raised to 30s (#3541), but Testing Library's async budget was still 1s. It is now 10s on CI only.
- **Real HTTP from jsdom:** SpecTaskExecutionControls and CodeAgentExecutionControls sent `GET /api/v1/provider-endpoints` to `localhost:3000`. setupTests now fails any test that makes an unmocked XHR or fetch request, and never touches the network.

## Speed

The vitest summary showed `collect` and `environment` dominating; test bodies were a small share. Measured as CPU time (user+sys) of `vitest run`, because wall time on the shared dev host was noisy. Each change was compared against the commit before it in alternating runs, with zero new failures:

| Change | Suite CPU |
|---|---|
| Import `@mui/icons-material` icons by path (that barrel costs ~2.2s per import in Node; one file had an unused `import * as Icons`) | −16% |
| Import `@mui/material` components by path (that barrel costs ~0.5–1s per import) | −11% |
| happy-dom by default; 12 files that need jsdom's fidelity opt in with `// @vitest-environment jsdom` | −11% CPU, −16% wall |

A source-scanning test (`src/utils/muiImports.test.ts`) fails if a barrel value import comes back.

Rejected after measuring:

- `isolate: false`: about half the files fail because `vi.mock` leaks between files.
- `pool: 'threads'`: no CPU win and slower wall time.
- `deps.optimizer.web`: −7% CPU, but it breaks React CJS interop and stops deep-path `vi.mock`s from intercepting (25 files fail).
- Disabling `react-transition-group` transitions globally: MUI Modal stops restoring `aria-hidden`, so behaviour diverges from the real app.

## Sharding

`--shard=1/2` and `--shard=2/2` cover all files (124 + 124) and pass under `CI=1`. Each shard inherits `maxWorkers: '50%'`, so two shards on one host use the same CPU as one unsharded run. Sharding helps only if the shards land on separate runners or the per-shard worker cap is raised.
