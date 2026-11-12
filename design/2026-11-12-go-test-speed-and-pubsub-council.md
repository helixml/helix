# Go CI test speed, flakes, and the pubsub backend council

## Where unit-test time went (Drone, before)

- Most packages are `(cached)` on a typical build; ~13 packages never cache.
  Their tests open testdata files, and Go's test cache keys opened files on
  mtime + size. Drone's fresh clone gives every file a new mtime, so these
  packages rerun on every build (server, crawler, git, janitor, tools, ...).
  Fix belongs in `.drone.yml` (restore per-file commit mtimes after clone);
  do NOT touch all files to one fixed time — a same-size edit would then
  serve a stale cached result.
- `api/pkg/server` (always reruns): 32–50s, of which 20s was two tests sleeping
  out the real 9s switch-agent budget and 8s was blind 2s negative-assertion
  windows.
- Cold builds: `hydra` (31s proxy test + 10s log-tailer rescan), `pubsub`
  (sleeps), `openai` (1s retry backoff), `connman` (5s grace sweeps), and a
  fixed ~4s per test in every package using `NewInMemoryNats`: it probed a free
  port and assumed port+1 was free; losing that race cost
  `ReadyForConnections`' full 4s before a retry.
- `go test`'s vet subset is the only vet the API gets (the golangci-lint step
  for `api/` is commented out in `.drone.yml`), so it is not redundant.

## Failures in the last 250 builds

| Build | Test | Verdict |
|---|---|---|
| 5393 | server `TestReadinessWaitWakesStoppedExploratorySession` | Flaky: gomock verified before the auto-start goroutine's `UpdateSession`. Fixed. |
| 5515 | pubsub `TestQueueMultipleSubs` (99 vs 100) | Flaky: worker incremented after replying, on another goroutine. Fixed. |
| 5423, 5484 | integration `TestAgentTestSuite` | Environment: real OpenAI call returned 401 "Incorrect API key" — the CI secret. |
| 5410 | `oauth_test` | Real: that branch's SSRF guard blocks the test's loopback mock server. |

Also fixed: openai `TestWaitForTokensDoesNotOverAdmitUnderConcurrency` failed
9/40 under CPU load (asserted gaps between admissions; late wakeups bunch them).

## Backend council — `api/pkg/pubsub`

**Reference:** `api/pkg/org/infrastructure/wakebus` + `agentdelivery` at
`e8c62bd66`, frozen. **Panel:** GPT-6 (codex), Claude Opus 5.5, GLM-5.3 Flash
(node06). Qwen node06 ("model not found") and Grok ("usage limit") failed and
were replaced by GLM.

| Round | Commit | Reviewer | Scope | Reference | Gap |
|---|---|---|---|---|---|
| r0 | e8c62bd66 | GPT-6 | 4 | 9 | −5 |
| r0 | e8c62bd66 | Opus | 4 | 7 | −3 |
| r0 | e8c62bd66 | GLM | 4 | 8 | −4 |
| r1 | 6b9637830 | GPT-6 | 6 | 8 | −2 |
| r1 | 6b9637830 | Opus | 5 | 7 | −2 |
| r1 | 6b9637830 | GLM | 6 | 8 | −2 |

Tests category, r0 → r1: GPT-6 4 → 7, Opus 3 → 7 (reference 6), GLM 4 → 7.

### Fixed
- r0 (all three): `StreamConsume` returned a no-op `Unsubscribe`; now a real
  subscription, loop exits on terminal iterator errors —
  `TestStreamConsumeUnsubscribeStopsDelivery`.
- r0 (all three): unbuffered reply channels blocked a delivery goroutine on a
  late reply; buffered + non-blocking.
- r0 (all three): `Close` leaked the embedded server, monitor goroutine and
  temp dir.
- r0: racy counters / `require` in goroutines / 16 sleeps; tests rewritten.
- r0: overwritten `Reconnecting` handler; `Debug`/`Trace` always on.
- r0 (GLM): no-consumer diagnostic always inspected ZED_AGENTS —
  `TestStreamRequestReportsMissingConsumers`.
- r1 (GPT-6, GLM, Opus): `NewNats` leaked the connection on stream/consumer
  creation failure; status handlers ran under the read lock.
- r1 (GLM, Opus): dead `Nats.stream`, `pubsub.Config`, four uncalled
  `GetExternalAgent*` helpers, emoji publish logs.
- r1 (Opus): in-memory NATS websocket listened unauthenticated on 0.0.0.0.

### Tests deleted
- `Auth_Token` + `setupAuthTestNats`: set no token; replaced by
  `TestEmbeddedServerRequiresToken`.
- `TestStreamAfterDelay`: merged into `TestStreamConsumeRedeliversNak`, which
  now consumes only after all requests are in the stream.

### Rejected
- r1 GPT-6 "tests still use fixed sleeps": the scope's tests have none; the
  sleeps cited are in the reference's `wakebus_test.go`.
- r1 GLM "late reply blocks later replies" test: a mutant reverting the
  buffered channel still passed — each reply inbox has its own delivery
  goroutine, so the effect is a leaked goroutine, not blocked replies.
- r1 GPT-6 "honour ctx in `Subscribe`": callers unsubscribe explicitly; no
  consumer for the extra mechanism.

### Deferred to the owner (production behaviour or contract)
- `gcJetStream` deletes the ZED_AGENTS stream and consumers on every startup
  (a second replica on shared NATS wipes the first's queue); this also makes
  the startup consumer-reuse branch dead.
- `StreamConsume` consumers are ephemeral (server reaps after 5s inactive) and
  `Messages()` swallows missing heartbeats, so a reaped consumer leaves the
  loop waiting forever (Opus r1 F2, its biggest risk).
- `NewNats` silently falls back to random ports when configured ones are busy.
- One `n.consumer` slot shared across streams.
- Durable handler errors neither ack nor nak (redelivery waits `AckWait`).
- `zed_protocol.go` publishes to streams nothing creates.
