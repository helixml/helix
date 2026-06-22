# Fix Design: Restart-Surfaced WebSocket Sync Bugs (#2641, #2642, #2643)

**Date:** 2026-06-19
**Status:** Planning / spec — NOT yet implemented
**Companion:** `design/2026-06-19-request-id-routing-forensic-map.md` (the read-only map this builds on)
**Issues:**
- https://github.com/helixml/helix/issues/2641 — stale `api` IP in `/etc/hosts` → WS reconnect storm
- https://github.com/helixml/helix/issues/2642 — `chat_message` with `role:"user"` silently dropped by Zed
- https://github.com/helixml/helix/issues/2643 — reused-thread assistant response discarded after API restart

All three were surfaced by one event: an `api` container restart on a long-running org deployment. They are independent fixes in three subsystems (infra/hydra, chat ingress, correlation), so they ship as three separate PRs and can land in any order.

---

## #2642 — chat path `role:"user"` drop (+ N-notify history storm)

**Severity:** highest leverage, smallest fix. Two distinct defects in one handler.

### Defect A: `role:"user"` discriminant collision

`NotifyExternalAgentOfNewInteraction` adds `"role":"user"` to the `chat_message` payload (`websocket_external_agent_sync.go:1036`). Zed's sync client unconditionally drops any `chat_message` with `role=="user"` as a UI-sync echo (`crates/external_websocket_sync/src/websocket_sync.rs:421`). The queue path (`sendQueuedPromptToSession`, `sendChatMessageToExternalAgent`) omits `role` entirely and works.

**Fix:** Remove the `"role": "user"` key from the command data built in `NotifyExternalAgentOfNewInteraction` (`websocket_external_agent_sync.go:1034-1038`). This makes the chat path byte-identical in shape to the working queue path.

**Why safe:** The queue path already omits `role` and is not dropped, so no echo-suppression depends on this key being present here. Zed's echo-suppression exists to drop messages Helix broadcasts *back* for multi-client UI sync — those are a different code path. (Confirm during implementation that nothing on the Helix→Zed `chat_message` path requires `role` to be present; grep `role` in the Zed `IncomingChatMessage` deserialization.)

**Alternative considered & rejected:** Change the Zed side to only drop `role:"user"` echoes that match an already-seen message id. More invasive, requires a Zed/sandbox rebuild + `sandbox-versions.txt` bump, and the Helix-side removal fully resolves the bug. Keep Zed unchanged.

### Defect B: full-history re-notification storm

`startChatSessionHandler` notifies every interaction from `newInteractionsStartIndex` to the end (`session_handlers.go:708`). `newInteractionsStartIndex` is computed by scanning backwards for the first interaction whose `GenerationID < session.GenerationID` (`:668-674`). But the message-append path bumps `session.GenerationID++` and **rewrites every existing interaction to the new generation** (`session_handlers.go:1018-1026`). So no interaction is ever "older generation," the scan never breaks, `newInteractionsStartIndex` stays `0`, and **all N interactions are re-notified** on every send (observed ~1381 `Notify` calls → `external agent send channel full`).

**Fix:** Notify only the genuinely new interaction(s) created by this request. The chat handler appends exactly one user interaction per request (`session_handlers.go:1036`). Capture the pre-append length (or, for an existing session, notify only the last interaction `session.Interactions[len-1]`) instead of relying on the broken generation-boundary scan. Recommended: thread the count of appended interactions out of the append step and notify only that suffix.

**Why this is the right fix:** the generation heuristic is structurally defeated by the rewrite at `:1024-1026`; patching the index computation to special-case "all same generation" would just re-encode the same fragility. Notify-the-suffix is unambiguous.

### Test plan
- Live inner-Helix spec-task session (Zed connected). Send a chat-path prompt → assert an ACP turn runs and the interaction completes with real content (not "empty response").
- Long-lived session (≥3 interactions): send one chat prompt → assert exactly one `Notify` call (log count), no "send channel full".
- Regression: queue path (`POST /sessions/{id}/messages`) still works.

---

## #2643 — reused-thread response dropped after API restart

**Severity:** core correlation failure; the fix is the first concrete step of the `acp_thread_id` re-keying named in the forensic map.

### Root cause (confirmed)

`message_added` events from Zed carry only `acp_thread_id`, **no `request_id`**. `getOrCreateStreamingContext` (`websocket_external_agent_sync.go:1491`) resolves the target interaction via `requestToInteractionMapping[requestID]`; with `requestID==""` it falls back to "most recent waiting interaction" (`:1637`). After an API restart, the in-memory maps and streaming context are gone, and a **reused** thread emits no `thread_created` handshake to re-bind. The assistant `message_added` resolves to `sctx.interaction == nil` → `No interaction found to update` (`:1396`); content is dropped. `message_completed` then reloads `response_length=0` and marks the interaction error (`:2697`).

But note: the issue text says the empty-response path "Matched interaction by request_id mapping." After a restart the map is empty, so the more general failure is the `message_added` stream landing on a nil interaction because the streaming context's most-recent-waiting fallback didn't fire (or fired against the wrong/empty set). The fix removes dependence on the in-memory map.

### Fix: re-key routing on the persisted `acp_thread_id` (full re-key, this PR)

> **Scope decision (per Luke's review):** the full `acp_thread_id` re-key — previously deferred to a "later" task — is folded into this pass. It is the structural fix for the common cause behind all three bugs, and it is **evidence-based, not speculative**: the forensic map's restart-survival matrix shows `acp_thread_id`/`ZedThreadID` is the *only* DB-persisted piece of correlation state, while all five maps + `streamingContexts` are in-memory-only and destroyed on restart. Routing on the persisted key is what makes the system survive an `api` restart. A minimal "additive fallback" would leave the fragile request_id-primary path in place — two coexisting correlation mechanisms — which is harder to maintain and still leaks bugs through paths the fallback doesn't cover. So we commit to the re-key now.

Make `acp_thread_id` the **primary routing key**, derived behind a single chokepoint:

1. **Chokepoint.** `getOrCreateStreamingContext` (`websocket_external_agent_sync.go:1491`) is the function nearly all routing already flows through (forensic-map seam #1). Make it resolve target interaction as: `acp_thread_id` → session (via `contextMappings`, which already falls back to `findSessionByZedThreadID` / persisted `session.Metadata.ZedThreadID` on a restart miss, `:1116`) → most-recent `state=waiting` interaction for that session+generation (logic already exists at `:1636-1643`). No dependence on any in-memory map for the primary path.
2. **`request_id` demoted to an in-thread disambiguator, not the key.** ACP runs one turn at a time per thread, so `acp_thread_id` + most-recent-waiting is sufficient to route. Keep `request_id` only where it genuinely refines (matching a specific in-flight turn when present) and for **duplicate-completion dedup** — do not rip out the dedup/sentinel logic blindly; replace it with an interaction-state check (a `completed` interaction rejects a second completion) so correctness no longer depends on the consumed-sentinel (`""`) in `requestToInteractionMapping`.
3. **Retire the request_id-primary lookups** at the call sites the forensic map named: `getOrCreateStreamingContext` (`:1491`), `handleMessageCompleted` Step 1 (`:2570-2598`), and the writes in `sendQueuedPromptToSession` (`:3254-3264`). `handleMessageAdded`'s `targetInteraction == nil` branch (`:1391-1396`) resolves via the chokepoint instead of logging-and-dropping. Verify the auto-wake re-send path (`auto_wake_stuck_interactions.go:603-607`) routes through the same chokepoint.
4. **Keep the chokepoint swappable.** Derive turn-state (running / done / waiting) behind this one function so that when ACP v2's `state_update` notification lands we swap the *source* of transitions rather than rewrite — the design intent from the rewrite-strategy doc.

This subsumes the former "Later: full re-key" task; that section is removed.

**Honest cost (not a robustness doubt — an execution one):** the re-key touches the ~130 critical sections that read those maps. The robustness gain is certain; the risk is regression in concurrent-turn and dedup edge cases. That is what the expanded test matrix below must cover. If, during implementation, an inbound event that needs routing turns out to lack `acp_thread_id` (the map answers this — confirm during the change), fall back to its existing request_id binding for *that event only* and log it loudly rather than silently dropping.

### Why the "use a fresh thread" workaround is unacceptable (from the issue)

Clearing `ZedThreadID` makes Zed create a new thread; Helix's `handleThreadCreated` then spawns a **new orphan session** ("New Conversation") and delivers the reply there, leaving the worker's interaction `waiting`. `restartSessionContainer` deliberately preserves `ZedThreadID`, so it restores the exact thread that triggers the drop. The reused-thread path must work — hence the fix above.

### Test plan (must be live, per CLAUDE.md; expanded for the re-key)
- **Restart survival (the bug):** live spec-task session with a reused long-lived thread (non-empty `ZedThreadID`); restart the `api` container (`docker compose -f docker-compose.dev.yaml restart api`); send a follow-up → assert streamed content lands on the waiting interaction and completes with real content (no "empty response," no orphan "New Conversation"). Confirm `getOrCreateStreamingContext` logged `has_interaction=true` after restart.
- **Happy path unchanged:** fresh thread (with `thread_created`) still routes correctly.
- **Dedup:** a duplicate `message_completed` for an already-`completed` interaction is rejected by the state check, not by the consumed-sentinel — assert no double-write, no error re-queue.
- **Concurrent/sequential turns on one thread:** send turn B before turn A's completion is fully processed → assert each completion lands on its own interaction (this is the case `request_id`-as-disambiguator must still cover).
- **Queue path + auto-wake:** both route through the chokepoint and resolve the right interaction without the request_id maps.
- **Regression:** run the Go unit suite (`TestWebSocketSyncSuite`) and the Zed↔Helix E2E (`run_docker_e2e.sh`, both agents) — the 9-phase E2E exercises new-thread, follow-up, non-visible-thread, interrupt, and rapid 3-turn cancel, which are exactly the correlation edges the re-key touches.

---

## #2641 — stale `api` IP pinned in desktop `/etc/hosts`

**Severity:** infra; separate subsystem (`hydra/devcontainer.go` + compose). Self-contained.

### Root cause (confirmed)

`buildExtraHosts()` (`api/pkg/hydra/devcontainer.go:1100`) resolves `api` to a concrete IP at container-creation time and bakes `api:<ip>` / `outer-api:<ip>` into the desktop via `hostConfig.ExtraHosts` (`:877`). The entry is immutable for the container's lifetime. When `api` is recreated on a new IP, surviving desktops dial a dead address and Zed reconnects forever. Compounding: `./stack stop` doesn't stop `sandbox-nvidia` (gated behind the `code-nvidia` profile, `docker-compose.dev.yaml:216`), so desktops survive a full stop/start while `api` gets a new IP — guaranteeing a stale pin. The recovery path `autoStartDevContainerForSession` is a no-op when the container already exists, so nothing self-heals.

### Why we pin an IP at all (Luke's question — answered from the code)

The desktop runs inside the sandbox's **inner dockerd** on a `bridge` network (`devcontainer.go:842`); it has no route to the outer compose network where `api` lives. So `buildExtraHosts()` resolves `api` once (`net.LookupHost("api")`, `:1104`) and writes `api:<ip>` / `outer-api:<ip>` into the desktop's `/etc/hosts`. Two needs are bundled into that one line:

1. **Bridge inner→outer:** give the desktop *some* way to resolve the outer `api` name. Legitimate.
2. **Defeat Helix-in-Helix DNS shadowing:** when Helix runs inside a Helix desktop, the nested inner compose creates its *own* `api` service that would shadow the real outer one; `outer-api` is the escape-hatch name. Also legitimate.

Neither need requires **freezing the IP**. That is the actual defect: it snapshots a value that Docker reassigns on every `api` recreation, and because **`/etc/hosts` takes precedence over DNS**, the frozen entry also *shadows* the live resolution path that would otherwise self-correct.

This is also why it never bites production: there `HELIX_API_URL` is a real FQDN resolved by normal DNS (`:796-816`), so nothing is pinned. The bug is specific to the compose/self-hosted topology where `api` is a Docker service name with a volatile IP.

### Recommended fix (primary): resolve `api` dynamically, stop freezing the IP

Luke is right that the static-IP idea is backwards — it doubles down on the snapshot. The sandbox already has the machinery to resolve `api` *live*: it runs a **dns-proxy** (`sandbox/dns-proxy/main.go`, `sandbox/05-start-dns-proxy.sh`) bound to the inner bridge gateway that forwards queries to the outer Docker embedded DNS, which always returns `api`'s current IP. The fix is to route the desktop's resolution of `api`/`outer-api` through that proxy and **remove the frozen `ExtraHosts` pin** (`devcontainer.go:877`, `1100-1126`):

- Point the desktop container's resolver at the sandbox dns-proxy (per-container `--dns <SANDBOX_GATEWAY>` via `HostConfig.DNS`, or the inner dockerd's `daemon.json` `dns`), and have the proxy answer `api`/`outer-api` by live-resolving the real outer `api` (the proxy already forwards to the outer embedded DNS).
- Then an `api` restart → new IP → the desktop re-resolves on its next reconnect attempt and recovers on its own. No stale pin, no static-IP compose change, no `/etc/hosts` rewrite.
- `outer-api` stays the shadow-proof name for the nested H-in-H case, but resolved dynamically through the proxy instead of pinned.

**First implementation step (must confirm):** trace whether desktop containers *already* point their resolver at the dns-proxy gateway. There is no `HostConfig.DNS` set in `devcontainer.go` today, and `daemon.json` (`04-start-dockerd.sh:66-93`) sets no default `dns`, so the wiring likely needs to be added. If desktops already use the proxy, the fix collapses to simply deleting the `api`/`outer-api` lines from `buildExtraHosts()` and letting DNS win.

### Defense-in-depth (optional, bounded): self-heal a desktop that still can't connect

If dynamic resolution can't recover a given desktop (e.g. a cached/wedged Zed connection), add a bounded self-heal: when a session has a live desktop *container* but no WS past a threshold (the auto-wake cold-start path already detects "no WS" — `auto_wake_stuck_interactions.go:425-435`), **recreate** the desktop container instead of the current no-op re-kick. Bound it with the existing `AutoWakeCount` cap so a genuinely-down `api` doesn't churn recreation forever. With the dynamic-DNS fix in place this should rarely fire — it is a backstop, not the primary mechanism.

### Note on `./stack stop`

The issue notes `./stack stop` leaves `sandbox-nvidia` (and its desktops) running while `api` is recreated, which is what makes a full restart reliably trigger the stale pin. With dynamic resolution this is no longer a correctness problem (survivors re-resolve), so changing `stop` is unnecessary — leave it unless there's an independent reason to tear desktops down.

### Test plan
- Start stack, get a connected desktop (prompts complete).
- `./stack stop && ./stack start` (or recreate only `api` so it lands on a new IP).
- Assert the surviving desktop re-resolves `api` to the new IP and reconnects within one Zed retry interval, and a queued prompt is delivered — **without** recreating the desktop container.
- Verify `/etc/hosts` in the desktop no longer contains a pinned `api`/`outer-api` IP (or that DNS resolution wins over it).
- H-in-H regression: in a nested Helix-in-Helix desktop, confirm `outer-api` still resolves to the *real* outer api and not the nested compose's `api`.

---

## Scope: one PR (per Luke's review)

All three fixes **plus** the full `acp_thread_id` re-key land in a **single PR**. Build it in this internal order so each layer is independently verifiable before the next stacks on top:

| Layer | Subsystem | Rebuild | Risk | Build order within the PR |
|-------|-----------|---------|------|---------------------------|
| #2642 — `role` drop + notify storm | API (Go) + chat handler | API only (Air hot-reload) | Low — removes a key, fixes a loop bound | 1st (smallest, unblocks chat path) |
| #2643 + full re-key | API (Go) correlation | API only | **High** — touches the ~130 critical sections; the substantive change | 2nd (the core) |
| #2641 — dynamic DNS | hydra + sandbox dns-proxy | `build-sandbox` | Medium — DNS/networking; test full restart + H-in-H | 3rd (independent subsystem) |

The three are functionally independent, so they can be reviewed as distinct commits within the one PR. **One honest note for the reviewer:** #2641 is a different subsystem (sandbox DNS, needs `build-sandbox`) from the Go correlation work; keeping it as its own commit means it can be reverted or split out without unpicking the re-key if CI/topology surprises arise. The re-key (#2643) is the high-risk centre of the PR — its expanded test matrix above (restart survival, dedup, concurrent turns, full E2E both agents) is the gate for the whole PR.

All layers must be verified **live against a connected Zed** (per CLAUDE.md: seeded DB rows only exercise the no-connection branch). The re-key must additionally be verified **across an actual `api` restart** and against the **Zed↔Helix E2E** suite, since that is where the correlation edges live.
