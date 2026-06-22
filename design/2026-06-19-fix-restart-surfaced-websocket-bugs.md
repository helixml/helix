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

### Fix: resolve the interaction from `acp_thread_id` + DB when `request_id` is absent/unmapped

In `getOrCreateStreamingContext`, when `requestID == ""` **or** the `requestToInteractionMapping` lookup misses, derive the target interaction directly:

1. `helixSessionID` is already resolved upstream in `handleMessageAdded` via `contextMappings[acp_thread_id]`, which itself falls back to `findSessionByZedThreadID` (DB) on a restart miss (`:1116`). So the session is recoverable from the persisted `session.Metadata.ZedThreadID` without any in-memory state.
2. Query the most-recent `state=waiting` interaction for that session+generation (the logic already exists at `:1636-1643`) and bind to it.

The existing most-recent-waiting fallback at `:1636-1643` is the right mechanism; the bug is that on a reused thread after restart it can produce `nil` because the lookup ordering / generation filter or an empty context shortcut skips it. The fix is to make the `acp_thread_id`→session→waiting-interaction path the **primary** resolution when `request_id` is empty, not a best-effort afterthought, and to ensure `handleMessageAdded`'s `targetInteraction == nil` branch (`:1391-1396`) retries this resolution rather than logging-and-dropping.

**Scope guard:** This is the minimal version of forensic-map seam #2. Do NOT delete `requestToInteractionMapping` in this PR — only add the `acp_thread_id`-based resolution as a reliable fallback for the no-`request_id` case. Full removal of the map is a later refactor.

### Why the "use a fresh thread" workaround is unacceptable (from the issue)

Clearing `ZedThreadID` makes Zed create a new thread; Helix's `handleThreadCreated` then spawns a **new orphan session** ("New Conversation") and delivers the reply there, leaving the worker's interaction `waiting`. `restartSessionContainer` deliberately preserves `ZedThreadID`, so it restores the exact thread that triggers the drop. The reused-thread path must work — hence the fix above.

### Test plan (must be live, per CLAUDE.md)
- Live spec-task session with a reused long-lived thread (non-empty `ZedThreadID`).
- Restart the `api` container (`docker compose -f docker-compose.dev.yaml restart api`).
- Send a follow-up prompt → assert the streamed assistant content lands on the waiting interaction and it completes with real content (no "empty response," no orphan "New Conversation" session created).
- Confirm `getOrCreateStreamingContext` logged `has_interaction=true` after the restart.

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

## Sequencing & risk

| Issue | Subsystem | Rebuild needed | Risk | Order |
|-------|-----------|----------------|------|-------|
| #2642 | API (Go) + chat handler | API only (Air hot-reload) | Low — removes a key, fixes a loop bound | 1st (ship first, unblocks chat path) |
| #2643 | API (Go) correlation | API only | Medium — touches hot streaming path; must test live + restart | 2nd |
| #2641 | hydra + sandbox dns-proxy | `build-sandbox` (hydra/DNS wiring runs in sandbox) | Medium — DNS/networking change, test full restart + H-in-H | 3rd (independent) |

None of the three depends on another. #2642 and #2643 are pure Go (API Air hot-reload). #2641 touches hydra DNS wiring (and possibly the sandbox dns-proxy) and needs a sandbox rebuild.

All three must be verified **live against a connected Zed** (per CLAUDE.md: seeded DB rows only exercise the no-connection branch). #2643 specifically must be verified **across an actual `api` restart**, since that is its only trigger.
