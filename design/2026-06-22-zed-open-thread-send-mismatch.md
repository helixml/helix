# Zed `open_thread` ↔ `chat_message` thread mismatch on reconnect

**Date:** 2026-06-22
**Area:** `api/pkg/server/websocket_external_agent_sync.go`
**Symptom:** Within a single Helix session — without the user switching session in
the UI — the Zed thread that Zed *opened* (foregrounded/streamed) differed from
the thread Helix was *sending messages to*. User messages appeared to vanish into
a thread that wasn't on screen.

## How it was found

A spec task (`spt_01kvfq6m8a07gywpj6jmadyrb8`, "Configurable Model Selection for
Claude Subscription Mode") had **5 distinct Zed threads** across 5 `Spec Generation`
Helix sessions. The session-proliferation itself was expected (repeated
backlog → re-plan cycles each clear `planning_session_id` and mint a fresh
session + thread — see `spec_driven_task_handlers.go:1015-1040`). The real defect
was the *intra-session* mismatch described below.

## Root cause

`open_thread` and every `chat_message` send path disagreed about which Zed thread
a given Helix session maps to.

- **`chat_message`** (all paths: the reconnect-queued send in `pickupWaitingInteraction`,
  `sendChatMessageToExternalAgent`, and the first-message sends) used the session's
  own thread: `session.Metadata.ZedThreadID`.
- **`open_thread`** (sent on WS reconnect in `handleExternalAgentConnection`) used a
  *spec-task-global* override, `findLatestZedThreadForSpecTask(specTaskID)`, which
  scans **all** work sessions of the spec task and returns whichever thread has the
  largest `session.Updated` — independent of which session the connection belongs to.

`findLatestZedThreadForSpecTask` was the **only** caller of that helper; every other
thread reference in the file used `session.Metadata.ZedThreadID`.

### Why it produces "opened ≠ sent-to" with no UI action

On an ordinary WS reconnect of the desktop bound to session **S** (thread `T_S`):

1. `open_thread` runs the global query → returns `T_latest`, the thread of whichever
   *other* session was most recently touched → **Zed foregrounds `T_latest`**.
2. `pickupWaitingInteraction`, called moments later on the same connection, sends
   `S`'s waiting interaction to `T_S`.

Zed shows/streams `T_latest`; Helix streams the user's messages into `T_S`. The
mismatch is produced entirely server-side by the reconnect handler — no session
switch in the UI is required. Reconnects happen on API restart, network blips, and
the agent_ready cycle.

It is also self-perturbing: each send calls `TouchSession(S)`, bumping `S.Updated`,
so the session that wins the global "latest" query flips around as different
sessions get touched — the thread `open_thread` picks can change connect-to-connect
independently of which session is actually being driven.

### The override never worked anyway

The override (commit `92293ca98`, "use latest thread on reconnect") was intended to
follow zed-agent **manual compaction**, where the agent spins up a fresh thread
mid-run and reconnect should land on it. But the immediately-following send always
used the session's *own* `ZedThreadID`, so the override could never make the send
land on the compacted thread — it could only foreground a thread nobody was sending
to. It strictly caused the mismatch and never delivered the compaction-follow it was
written for.

## Fix

Remove the spec-task-global override in the `open_thread` reconnect path; reopen the
connection's own `helixSession.Metadata.ZedThreadID` — identical to every send path.
`open_thread` and `chat_message` are then guaranteed to address the same thread, so
multiple sessions per task become harmless (each session ↔ exactly one thread, both
opened and sent-to). The now-dead `findLatestZedThreadForSpecTask` is deleted.

Compaction-created threads are already their own Helix sessions
(`handleUserCreatedThread`). The correct way to "follow" them is to reconnect the
desktop *under that session*, not to retarget a different session's `open_thread` —
that belongs to the broader ACP-v2 / WS-sync rewrite
(`design/2026-06-19-acp-v2-and-websocket-sync-rewrite-strategy.md`) and is out of
scope here.

## Testing

- `CGO_ENABLED=0 go build ./pkg/server/` — clean.
- `go test -run TestWebSocketSyncSuite ./pkg/server/` — see PR notes.
- Live reconnect test against a connected Zed (spec task) is the meaningful check
  for this path; see PR description for status.
