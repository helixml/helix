# Design: Chrome MCP Process Lifecycle Detection and Recovery

## Architecture Overview

The fix spans two layers of the Zed MCP stack:

1. **`context_server::Client`** — Detect when the transport is dead (repeated request failures)
2. **`ContextServerStore`** — React to dead server detection, transition state, and auto-restart

### Current Flow (Broken)

```
Chrome dies → MCP node process loses CDP → MCP stays alive (stdio open)
→ Zed sends requests via stdio → MCP returns errors or timeouts
→ ContextServerStore still says "Running" → User stuck
```

### Proposed Flow (Fixed)

```
Chrome dies → MCP node process loses CDP → MCP stays alive (stdio open)
→ Zed sends requests via stdio → MCP returns errors
→ Client tracks consecutive failures → Threshold exceeded
→ Client emits/notifies "server unhealthy"
→ ContextServerStore transitions to Error state
→ ContextServerStore kills process, waits (backoff), restarts
→ New MCP process launches fresh Chrome connection
```

## Key Findings from Code Investigation

### `context_server::Client` (`zed/crates/context_server/src/client.rs`)

- `request_with()` returns `"server shut down"` when `response_handlers` is `None` (L366-370). This happens when `handle_output` finishes and the deferred cleanup runs.
- But in our scenario, `response_handlers` is still `Some` — the MCP process is alive, stdio is open. The MCP server *responds* with JSON-RPC errors or the requests time out.
- There is **no health-checking or failure-counting logic** anywhere in the client.

### `StdioTransport` (`zed/crates/context_server/src/transport/stdio_transport.rs`)

- Uses `kill_on_drop(true)` — the child process is killed when `StdioTransport` is dropped.
- The `Drop` impl on `StdioTransport` calls `self.server.kill()`.
- This means stopping/dropping the `ContextServer` *will* clean up the node process. The problem is nothing triggers the stop.

### `ContextServerStore` (`zed/crates/project/src/context_server_store.rs`)

- Has `Running`, `Starting`, `Stopped`, `Error` states. The `Error` state exists but is only set on startup failure.
- `maintain_servers()` runs when settings change but does **not** poll server health.
- `ServerStatusChangedEvent` is emitted and subscribed to by the agent, but only for state transitions that already happen.
- The `Restart` action exists but is settings-triggered only — there's no "restart this specific server" imperative API.

### Chrome DevTools MCP Server (`browser.js`)

- Uses Puppeteer's `puppeteer.connect()` to attach to a running Chrome via CDP WebSocket.
- The module-level `browser` variable is checked with `browser?.connected` in `ensureBrowserConnected()`.
- When Chrome dies, `browser.connected` becomes `false`, but the MCP server doesn't exit — it just fails subsequent tool calls.
- Each tool call goes through `ensureBrowserConnected()` which *would* try to reconnect, but the reconnect fails if there's no Chrome to connect to, and the error propagates back as a JSON-RPC error response.

## Design Decisions

### Decision 1: Detect failure via consecutive request errors, not process monitoring

**Why:** The MCP process is alive. Chrome is dead. We can't monitor Chrome (it's the MCP server's concern). What we *can* observe is that tool calls are failing. After N consecutive failures, the server is unhealthy.

**Approach:** Add a failure counter to `Client`. When consecutive failures exceed a threshold (e.g., 3), emit a notification or set a flag that `ContextServerStore` can observe.

### Decision 2: Use `ContextServerStore` subscription to drive restart, not the Client

**Why:** The `Client` is a transport-level concern. Restart policy (backoff, max retries) belongs in the store layer.

**Approach:** `ContextServerStore` subscribes to a health signal from the server. On unhealthy signal, it transitions to `Error` state, kills the process, and schedules a restart with exponential backoff.

### Decision 3: Exponential backoff on restart with a cap

**Why:** If Chrome keeps crashing (OOM, GPU driver, etc.), restarting the MCP server in a tight loop wastes resources and spams logs.

**Approach:** Backoff sequence: 2s, 4s, 8s, 16s, 30s (capped). Reset backoff on successful `Running` state lasting >60s.

### Decision 4: Reuse existing `Restart` action for manual restart

**Why:** The `Restart` action already exists in `context_server_store.rs` but only triggers on settings changes. Wire it up as an imperative "restart this server now" command.

## Implementation Approach

### Layer 1: Client-side failure tracking (`context_server::Client`)

Add to `Client`:
- `consecutive_failures: Arc<AtomicU32>` — incremented on request error, reset on success.
- When threshold (3) is exceeded, set a flag or notify via existing notification mechanism.

In `request_with()`:
- On success: reset counter to 0.
- On error (timeout, JSON-RPC error, transport error): increment counter.
- When counter crosses threshold: log warning, notify subscribers.

### Layer 2: Store-side health monitoring (`ContextServerStore`)

- After `run_server` transitions to `Running`, start a lightweight health-watch task.
- The health-watch observes the client's failure state (either by polling the counter or subscribing to a notification).
- On detecting unhealthy state:
  1. Log the error with context (server ID, last error message, failure count).
  2. Transition to `Error` state (this emits `ServerStatusChangedEvent`).
  3. Schedule restart with backoff.
- Track restart attempts per server to implement backoff.

### Layer 3: Orphan cleanup (defense in depth)

On Zed startup or when starting a new MCP server:
- Before spawning, check if a process matching the same command is already running with a dead parent PID.
- Kill orphans. This handles the case where Zed itself crashes and leaves MCP processes behind.
- `kill_on_drop(true)` already handles graceful shutdown; this covers ungraceful ones.

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| False positives: transient errors trigger unnecessary restart | Threshold of 3 consecutive failures; single error doesn't trigger restart |
| Restart storm: Chrome keeps dying | Exponential backoff capped at 30s; max restart attempts before giving up and staying in `Error` |
| Race between auto-restart and manual restart | Use the same `stop_server` → `run_server` path for both; store-level mutex via entity update |
| MCP server exits cleanly but Zed doesn't notice | Already handled: `handle_input` stream ends → `response_handlers.take()` → "server shut down" errors. This path works today for process death. |