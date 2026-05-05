# Design: Restore Immediate Loading State When Waking a Paused Desktop by Chat

## Why the original fix degraded

The April fix made the spinner appear within ~3 s, by ensuring `useGetSession` polls every 3 s regardless of WebSocket connectivity. Several things have happened since that change the picture:

1. **Query keys are no longer shared.** `07e3a313b` (added BEFORE the polling fix, but the polling fix's commit message still describes the pre‑split world) changed `useGetSession` to suffix the key with `'full'` or `'skip'`:
   ```ts
   queryKey: [...GET_SESSION_QUERY_KEY(sessionId), skipInteractions ? 'skip' : 'full'],
   ```
   - `useSandboxState` calls without `skipInteractions` → key `["session", id, "full"]`
   - `EmbeddedSessionView` passes `skipInteractions: true` → key `["session", id, "skip"]`
   These are now two independent React Query entries, each polling every 3 s. Polling still works, but the WebSocket "fast path" no longer reaches either of them.

2. **WebSocket `session_update` writes to a non‑existent cache slot.** `frontend/src/contexts/streaming.tsx:270, 301` calls
   ```ts
   queryClient.getQueryData(GET_SESSION_QUERY_KEY(currentSessionId))   // ["session", id]
   queryClient.setQueryData(GET_SESSION_QUERY_KEY(currentSessionId), ...) // ["session", id]
   ```
   `setQueryData` requires an **exact** key match (unlike `invalidateQueries`, which prefix-matches). So WS-delivered session updates never land in the queries that `useSandboxState` and `EmbeddedSessionView` actually read. The "preserve config" guard at line 297-309 is irrelevant for the same reason.

3. **The chat send path is purely backend-mediated.** `RobustPromptInput.handleSend` (`frontend/src/components/common/RobustPromptInput.tsx:655`) only calls `saveToHistory` + `syncEntryImmediately` — it never invokes the `onSend` prop wired up in `SpecTaskDetailContent.tsx:1943-1950`. No frontend hook flips any state to indicate "we're trying to wake a session". Spinner-vs-paused decisions are 100% driven by polled `external_agent_status`.

The resulting timeline today, when the user chats to a paused session:

| t (approx) | what happens |
| --- | --- |
| 0 ms | user clicks Send, prompt added to local history (queue shows it) |
| 50–300 ms | POST `/v1/projects/.../prompt-history` returns |
| 50–500 ms | backend dispatches; `sendCommandToExternalAgent` finds no WS; goroutine `autoStartDevContainerForSession` fires |
| 200 ms – 2 s | goroutine finishes loading session/spec_task/project, calls `StartDesktop` |
| ~2 s | `setExternalAgentStatus(ctx, sessionID, "starting")` writes to DB |
| 0–3 s after that | next React-Query poll cycle fires |
| +50 ms | UI re-renders → spinner appears |

So spinner appears anywhere from ~2.5 s to ~5 s after the click. Long enough to feel like nothing is happening, especially on local-feeling chat inputs.

## Proposed fix

Add an **optimistic local state flip** on chat send, plus tighten the WebSocket cache writes so they actually do something.

### Change 1 — Optimistic "starting" on chat send (primary fix)

In the chat-send path used inside spec-task detail, when the cached session shows the desktop is paused/absent, **synchronously write the cached session metadata to `external_agent_status: "starting"`** for both query keys before returning from the send handler.

Where to hook it:
- `RobustPromptInput.handleSend` (`frontend/src/components/common/RobustPromptInput.tsx:655`) — already has access to `sessionId` and `apiClient`. Add an optional callback prop `onWillSend?(): void` invoked synchronously inside `handleSend` after the queue persist but before `syncEntryImmediately`. (Avoids an unrelated refactor of the dead `onSend` prop.)
- The caller in `SpecTaskDetailContent.tsx:1937-1955` and the equivalent in `ExternalAgentDesktopViewer.tsx:283-298` passes `onWillSend={optimisticallyMarkStarting}`.

The optimistic helper writes both query slots (`'full'` and `'skip'`) using `queryClient.setQueryData`:

```ts
const optimisticallyMarkStarting = useCallback(() => {
  for (const variant of ['full', 'skip'] as const) {
    queryClient.setQueryData(
      [...GET_SESSION_QUERY_KEY(sessionId), variant],
      (old: { data?: TypesSession } | undefined) => {
        if (!old?.data) return old;
        const cfg = old.data.config ?? {};
        if (cfg.external_agent_status === 'running' || cfg.external_agent_status === 'starting') return old;
        return {
          data: {
            ...old.data,
            config: { ...cfg, external_agent_status: 'starting', status_message: cfg.status_message || 'Starting Desktop...' },
          },
        };
      }
    );
  }
}, [queryClient, sessionId]);
```

Behaviour:
- If the user is on a running session, the early `return old` keeps state untouched.
- If the user is on a paused session, both query slots flip to `"starting"` synchronously → next render shows spinner.
- The next poll (≤3 s later) brings the authoritative backend value, harmlessly overwriting either to `"starting"` (boot in flight) or to `"absent"` (something failed) or `"running"` (already up).

### Change 2 — Fix the WebSocket cache key so `session_update` actually lands

In `frontend/src/contexts/streaming.tsx`:

- Line 270 (`getQueryData`): read both `["session", id, "full"]` and `["session", id, "skip"]`, pick whichever has data. (For the stale-update interaction-count check, prefer `'full'` because that's the one carrying interactions.)
- Line 301 (`setQueryData`): write to **both** key variants. For `'skip'`, omit `interactions` from the writeback to match what that consumer expects.

This is correctness independent of Change 1 — a long-standing latent bug that the existing comment at line 297-309 documents but no longer affects, because the writes target an empty slot. Worth fixing while we're in there.

### Change 3 — Belt-and-braces: nudge poll on send

Inside `optimisticallyMarkStarting`, also call

```ts
queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) }); // prefix match
```

This is what `ExternalAgentDesktopViewer.handleSendMessage` already does (line 293-295) but `SpecTaskDetailContent`'s send path does not. With the optimistic state already showing the spinner, the invalidation is just an early refresh trigger to confirm the backend transition faster than the next 3 s tick.

## Files to Modify

| File | Change |
| --- | --- |
| `frontend/src/components/common/RobustPromptInput.tsx` | Add optional `onWillSend` prop; invoke it inside `handleSend` immediately after `saveToHistory` |
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Define `optimisticallyMarkStarting` near `handleSendMessage`; pass `onWillSend={optimisticallyMarkStarting}` to `RobustPromptInput` (line 1938 region and the second mount near line 2742 if applicable) |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` | Same: define `optimisticallyMarkStarting` and pass to `RobustPromptInput` (the inline send path uses `handleSendMessage`; either reuse it or add `onWillSend`) |
| `frontend/src/contexts/streaming.tsx` | Update `getQueryData` / `setQueryData` calls in the `session_update` handler to use both `['full']` and `['skip']` key suffixes |

No backend changes; the existing auto-start path is correct and the API still returns the right `external_agent_status` once the boot starts.

## Key Decisions

1. **Optimistic UI rather than synchronous backend signal.** The backend chain (HTTP POST → goroutine → `StartDesktop` → DB write) is intrinsically several hundred milliseconds. Trying to make it synchronous would couple HTTP latency to UI responsiveness for no real win. Optimistic local state is the standard React Query idiom and recovers cleanly because polling is the source of truth.

2. **Add a separate `onWillSend` callback rather than reviving the unused `onSend` prop.** The existing `onSend` is dead code (kept because of a queue-driven refactor). Reviving it would invite future readers to reintroduce the double-send bug. A clearly named "this fires before queueing, do optimistic UI here" prop is harder to misuse.

3. **Write both query variants instead of consolidating to a single key.** Consolidating `'full'` / `'skip'` would be a more invasive refactor (it would re-introduce the 50 MB-response problem that `07e3a313b` fixed). Touching both slots in two places is a lighter, more local change.

4. **Don't modify the backend.** The backend is doing the right thing; the regression is entirely a frontend latency / cache‑routing issue.

## Risks & Mitigations

- **Risk:** optimistic flip masks a real failure (e.g. backend rejects the prompt). **Mitigation:** the existing 2-min "Desktop may have failed to start" timeout (`ExternalAgentDesktopViewer.tsx:170-179`) still fires; spinner can't lock the UI indefinitely.
- **Risk:** flipping `external_agent_status` in the cache races with an in-flight poll that returns the old `"absent"` value. **Mitigation:** `staleTime: 2000` on `useGetSession` (`sessionService.ts:65`) means a fresh fetch is unlikely to immediately refetch; even if it does, the next 3 s tick will reconcile correctly.
- **Risk:** `setQueryData` for the `'skip'` variant could accidentally overwrite a poll response that was about to land. **Mitigation:** the helper uses the function form `(old) => ...` which atomically composes on top of whatever's there; we only mutate `config.external_agent_status` and `config.status_message`, leaving everything else (including a freshly polled `interactions`) untouched.

## Validation Plan

1. Helix‑in‑Helix at `http://localhost:8080`. Register `test@helix.ml` / `helixtest` if no user exists, complete onboarding.
2. Create a spec task that runs to "ready", let the desktop go to paused (or stop the desktop manually).
3. Open the task detail page. Confirm "Desktop Paused / Start Desktop" UI.
4. Type any message in the chat input and submit.
5. **Expected:** within ~500 ms the right pane flips to the **Starting Desktop...** spinner.
6. After ~30–90 s the desktop comes up; spinner disappears, stream renders.
7. Repeat with the chat panel collapsed (icon‑only mode) to confirm no React errors from missing context.
