# Design: Escape Cancels the Current Turn in the Spec Task Chat

## 1. Where the pieces live (discovery notes)

The "Spectas details page" chat box is a four-layer stack — worth writing down, it is not
obvious from the file names:

| Layer | File | Role |
|---|---|---|
| Page | `frontend/src/pages/SpecTaskDetailPage.tsx` | Route `/projects/:id/tasks/:taskId`. Has a **window-level** `keydown` listener: Escape closes the slide-in "New Task" panel (line ~70). |
| Content | `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | 3k-line tab shell; renders `<AgentChat>` in two places (~2480, ~2786). |
| Chat | `frontend/src/components/session/AgentChat.tsx` | Owns `handleCancel` → `v1SessionsCancelCreate(sessionId)`, `isCancelling` state, and `isAgentBusy` (derived from the newest interaction being `Waiting`, polled every 3s). |
| Composer | `frontend/src/components/common/RobustPromptInput.tsx` | The chat box itself. `handleKeyDown` (line ~1022) is bound by **both** composer variants — the plain `textarea` (~1490) and `SandboxPromptEditor` (~1476). The red Stop button (~1681) is `isAgentBusy && onCancel`. |

Backend cancel path:

```
POST /api/v1/sessions/{id}/cancel        server.go:1060
  → cancelSessionTurn                    session_handlers.go:2409
    → cancelActiveTurn                   prompt_history_handlers.go:547
        ├─ turn not yet dispatched  → mark interaction Interrupted in store, return "cancelled"
        └─ turn live in Zed         → sendCancelToExternalAgent (cancel_current_turn, 3s ack wait)
                                        → handleTurnCancelled marks Interrupted, then acks
```

## 2. Frontend change — Escape in the composer

Add one branch to `handleKeyDown` in `RobustPromptInput.tsx`, placed **after** the existing
`composerTrigger` block (so the completion popup keeps first claim on Escape) and **before**
the `Enter` branch:

```ts
if (e.key === 'Escape') {
  if (!isAgentBusy || !onCancel || isCancelling) return   // let it bubble to the page handler
  e.preventDefault()                                       // marks the native event handled
  void onCancel()
  return
}
```

Design decisions:

- **Component-level, not window-level.** The requirement is "when the chat box is focused",
  and a window listener would fight `SpecTaskDetailPage`'s panel-close handler and the
  desktop viewer's key forwarding.
- **`preventDefault()` as the "consumed" signal, plus a `defaultPrevented` guard in
  `SpecTaskDetailPage`.** React attaches listeners at the root container, so the native event
  still reaches the page's `window` listener afterwards. Without a guard, one Escape would
  cancel the turn *and* close the New Task panel. This repo already uses exactly this
  idiom — `HelixOrgSideDrawer.tsx:41` (`if (event.key !== 'Escape' || event.defaultPrevented) return`).
  We follow it rather than inventing a new mechanism.
- **Same guards as the button.** Reusing `isAgentBusy` / `isCancelling` / `onCancel` means
  Escape and the click share one code path and one duplicate-suppression rule; key repeat
  from a held Escape is a no-op after the first press.
- **No new props.** Every consumer that can cancel already passes `onCancel`, so the shortcut
  lights up wherever cancelling is possible and stays inert everywhere else.
- **Tooltip** becomes `Stop generation (Esc)`; `aria-label` untouched so the existing tests
  (`RobustPromptInput.test.tsx` → "active-turn controls") keep passing.

Rejected alternatives:
- *Handle Escape in `AgentChat`* — the composer swallows key events at the editor level and
  `AgentChat` has no focus context; it would have to sniff `document.activeElement`.
- *`stopPropagation()` instead of `preventDefault()`* — React synthetic `stopPropagation` does
  not stop the native event from reaching a `window` listener, so it would not fix the
  double-handling.

## 3. Backend change — drain the queue after a cancel

**Problem (verified in code, not assumed).** The normal completion path fans out to the queue:
`handleMessageCompleted` ends with `go apiServer.processPromptQueue(...)`
(`websocket_external_agent_sync.go:3362`). The **cancel** path does not: neither
`cancelActiveTurn` nor `handleTurnCancelled` nudges the queue. `processPendingPromptsForIdleSessions`
only runs from `POST /prompt-history/sync` (`prompt_history_handlers.go:114`), and the
frontend's queue poll (`usePromptHistory.ts:407`) is a read-only `list` — it does not nudge.
Net effect today: cancel a turn with messages queued and they sit there until some unrelated
event nudges the session.

**Fix.** In `cancelSessionTurn` (`session_handlers.go`), after `cancelActiveTurn` returns
successfully with a non-`noop` status, nudge the session queue:

```go
status, err := s.cancelActiveTurn(ctx, sessionID)
if err != nil { ... }
if status != "noop" {
    s.nudgeSessionQueue(sessionID)   // detached goroutine; dispatches oldest pending prompt
}
return map[string]string{"status": status}, nil
```

Why here and why this call:

- **One place covers every caller** — Escape, the Stop button, and `helix` CLI / API clients
  all funnel through this handler. Putting it in `cancelActiveTurn` would also catch internal
  callers, but those are already part of interrupt flows that dispatch a prompt themselves
  (`processInterruptPrompt`), which would double up. The HTTP handler is the user-initiated
  boundary.
- **`nudgeSessionQueue` is the sanctioned entry point** (`prompt_history_handlers.go:465`). It
  runs `processPendingPromptsForSession` on a detached context, which re-reads the newest
  interaction, honours the per-session drain lock (`lockPromptDrain`), the
  thread-establishment barrier, and interrupt-vs-queue selection. We are not adding a second
  dispatch mechanism — reusing this keeps the out-of-order-dispatch guarantees documented in
  `design/2026-06-23-queue-drain-out-of-order-dispatch.md`.
- **Ordering is safe.** `handleTurnCancelled` deliberately persists the `Interrupted` state
  *before* signalling the ack channel ("Acknowledge the HTTP caller only after the interaction
  state transition is durable"). So by the time we nudge, the busy-check sees a non-`Waiting`
  latest interaction and dispatches instead of deferring. The pre-dispatch branch writes the
  state inline before returning, so it is safe too.
- **No-op safety.** If nothing is queued, `processPendingPromptsForSession` returns early.

## 4. Testing

- **Unit (frontend)** — `RobustPromptInput.test.tsx`, existing `active-turn controls` block:
  Escape fires `onCancel` when busy; does nothing when idle; does nothing when `isCancelling`;
  first Escape with the completion popup open closes the popup instead of cancelling.
- **Unit (backend)** — `session_handlers` / prompt-queue suite (gomock + testify suite, per
  `CLAUDE.md`): cancel with a pending queue-mode prompt dispatches it; cancel returning `noop`
  does not.
- **End-to-end in the inner Helix (required — this is a UI + lifecycle change).** Per
  `CLAUDE.md`, a live Zed is mandatory for lifecycle changes: register at
  `http://localhost:8080` (`test@helix.ml` / `helixtest`), create a spec task (a bare chat
  session will not connect Zed), send a long-running prompt, then:
  1. focus the chat box, press Escape → turn stops, Stop button disappears;
  2. queue two messages while busy, press Escape → the first queued message starts;
  3. press Escape with the agent idle and the New Task panel open → panel closes (no
     regression);
  4. **test the next operation after the cancel** — send a new message and confirm it runs.

## 5. Implementation notes (filled in during implementation)

Files changed in `helix`:

| File | Change |
|---|---|
| `frontend/src/components/common/RobustPromptInput.tsx` | Escape branch in `handleKeyDown` (after `composerTrigger`, before `Enter`); `isAgentBusy`/`isCancelling`/`onCancel` added to the dep array; Stop tooltip → `Stop generation (Esc)`. |
| `frontend/src/pages/SpecTaskDetailPage.tsx` | `if (e.defaultPrevented) return` at the top of the window `keydown` handler. |
| `frontend/src/pages/SpecTasksPage.tsx` | Same guard on its Escape handler — **not in the original plan**, see below. |
| `api/pkg/server/session_handlers.go` | `nudgeSessionQueue(sessionID)` in `cancelSessionTurn` when status != `noop`. |
| `frontend/src/components/common/RobustPromptInput.test.tsx` | 3 tests + a `createEscapeEvent()` helper. |
| `frontend/src/components/common/RobustPromptInput.sandbox.test.tsx` | 1 test: completion popup consumes the first Escape. |
| `api/pkg/server/prompt_history_handlers_test.go` | 2 tests on `cancelSessionTurn` (drain / no drain). |

Discovered during implementation:

- **`SpecTasksPage` needed the same guard.** It renders `TabsView` (line ~1269), which
  contains an `AgentChat` composer, *and* it has its own window-level Escape handler for the
  create/chat panels. Without the guard, Escape in the workspace-tab chat would cancel the
  turn and close the panel. Its other handler (Enter) already filters on
  `target.tagName === "TEXTAREA"`, so it needed no change.
- **Testing `preventDefault` needs a real native event.** `fireEvent.keyDown(el, {...})` gives
  no handle on the dispatched event, so the tests build
  `new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })` and pass
  it to `fireEvent(el, event)`, then assert `event.defaultPrevented`. That is the property the
  page-level guards actually key off, so it is worth asserting directly.
- **The popup-precedence test lives in `RobustPromptInput.sandbox.test.tsx`, not the main test
  file.** The main file mocks `usePromptHistory` with a constant `draft: ''`, so no amount of
  typing can open the completion popup (`composerTrigger` is derived from `draft` + cursor +
  focus). The sandbox test file mocks the hook with real `useState`, which is why the trigger
  can fire there.
- **Backend test needed one shared `*types.Interaction` pointer.** `cancelActiveTurn` flips it
  to `Interrupted` and the drain then re-reads it via `ListInteractions`; returning the same
  pointer from both expectations reproduces production ordering (state durable before ack) and
  lets the drain see an idle session. The assertion is that `GetNextPendingPrompt` is reached
  (signalled over a channel), which is the moment a queued prompt gets claimed.
- **Go tests need CGo/gcc** (tree-sitter), per `CLAUDE.md`:
  `sudo apt-get install -y gcc libc6-dev` then `CGO_ENABLED=1 go test ...`. The
  `frontend/node_modules` directory was also absent in this sandbox — `yarn install` first.

## 6. Gotchas for whoever implements this

- `SpecTaskDetailPage`'s Escape listener is on `window`, not the React tree — you must add the
  `defaultPrevented` guard there or you get double handling.
- `isAgentBusy` comes from a **3-second poll** of the latest interaction, so it can lag reality
  by up to 3s; Escape has the same staleness window as the Stop button. Cancelling when the
  backend has already finished returns `noop` and shows the informational snackbar. Acceptable,
  and not something to "fix" in this task.
- `sendCancelToExternalAgent` waits only **3s** for Zed's ack and then errors; the snackbar path
  is shared, so do not add a second error toast on the Escape path.
- Both composer variants share `handleKeyDown` — one edit covers both. Do not add a second
  handler to `SandboxPromptEditor`.
- Do not touch `DesktopStreamViewer`'s double-Escape modifier reset (line ~4121); it is a
  different focus target and a different feature.
