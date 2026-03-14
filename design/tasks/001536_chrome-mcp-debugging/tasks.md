# Implementation Tasks

## Layer 1: Client-side failure tracking (`zed/crates/context_server/src/client.rs`)

- [ ] Add `consecutive_failures: Arc<AtomicU32>` field to `Client` struct
- [ ] Add `health_tx: channel::Sender<()>` / `health_rx` pair to `Client` for notifying when failure threshold is crossed
- [ ] In `request_with()`: on success, reset `consecutive_failures` to 0
- [ ] In `request_with()`: on error (timeout, JSON-RPC error, transport error), increment `consecutive_failures`
- [ ] When `consecutive_failures` crosses threshold (3), send on `health_tx` and log a warning with server ID and error details
- [ ] Expose `pub fn health_notifications(&self)` returning a stream/receiver that `ContextServerStore` can subscribe to

## Layer 2: Store-side health monitoring (`zed/crates/project/src/context_server_store.rs`)

- [ ] Add `restart_backoff: HashMap<ContextServerId, RestartState>` to `ContextServerStore` (tracks attempt count and next retry time)
- [ ] Define `RestartState` struct: `{ attempts: u32, last_restart: Instant }`
- [ ] In `run_server()`: after transitioning to `Running`, spawn a health-watch task that listens on the client's health notification channel
- [ ] Health-watch task: on receiving unhealthy signal, call `update_server_state` to transition to `Error`, then schedule restart
- [ ] Implement `schedule_restart()`: compute delay from backoff (2s, 4s, 8s, 16s, 30s cap), spawn delayed task that calls `stop_server` then `run_server`
- [ ] Reset backoff when server has been `Running` for >60s (timer task that clears the `RestartState` entry)
- [ ] Cap max restart attempts at 5 before giving up (stay in `Error` state, log message)
- [ ] Wire up the existing `Restart` action to call `stop_server` + `run_server` imperatively for a specific server ID

## Layer 3: Orphan cleanup

- [ ] Before spawning a new MCP stdio process in `StdioTransport::new()`, search for existing processes matching the same command with dead parent PIDs
- [ ] Kill any found orphan processes and log their PIDs
- [ ] Keep this best-effort and Linux-only (read `/proc` to find candidates)

## Logging and observability

- [ ] Log at `warn` level when consecutive failure threshold is crossed, including server ID and last error message
- [ ] Log at `info` level when auto-restarting a server, including attempt number and backoff delay
- [ ] Log at `error` level when max restart attempts exhausted

## Testing

- [ ] Unit test: `Client` consecutive failure counter increments on error, resets on success
- [ ] Unit test: failure threshold crossing triggers health notification
- [ ] Unit test: `ContextServerStore` transitions from `Running` to `Error` on health notification
- [ ] Unit test: backoff delay calculation (2, 4, 8, 16, 30, 30, ...)
- [ ] Unit test: backoff resets after sustained `Running` period
- [ ] Integration test: simulate MCP process returning errors → verify auto-restart cycle