# Requirements: Chrome MCP Process Lifecycle Debugging

## Problem Statement

When the Chrome browser process that the Chrome DevTools MCP server connects to dies, the MCP server node processes become orphaned zombies. They remain running but return "server shut down" to all tool calls. Zed has no mechanism to detect this state or recover from it — the `ContextServerStore` still considers the server `Running` even though the underlying transport is dead.

## User Stories

1. **As an agent user**, when Chrome crashes or is killed, I want Zed to detect the dead MCP server and automatically restart it so my workflow isn't interrupted.

2. **As a developer**, when I see MCP tools returning errors, I want clear status indication in Zed showing the server is dead, not just generic "server shut down" messages.

3. **As a developer**, I want a manual way to restart a specific MCP server without editing settings or restarting Zed.

## Root Cause Analysis

The failure chain:

1. Chrome process dies (PID 9848 becomes zombie — parent `npm exec` wrapper didn't reap it).
2. The Chrome DevTools MCP server (node process) loses its Puppeteer CDP connection to Chrome.
3. The MCP node process does **not** exit — it stays alive but all tool calls fail.
4. Zed's `StdioTransport` stdin/stdout pipes to the MCP node process are still open (process is alive), so Zed never detects a problem.
5. `ContextServerStore` still has the server in `Running` state.
6. `Client::request_with()` sends requests, they either time out or the MCP server responds with errors.
7. The `response_handlers` are never `.take()`'d because `handle_input`/`handle_output` loops haven't terminated.

Key insight: **The MCP process didn't die — Chrome did.** The MCP server is a middleman that lost its downstream connection but kept its upstream (stdio) connection alive. This is fundamentally different from the process-crash case.

## Acceptance Criteria

### Must Have

- [ ] When an MCP server's tool calls fail repeatedly, Zed transitions the server state from `Running` to `Error`
- [ ] An MCP server in `Error` state is automatically restarted (with backoff to avoid restart loops)
- [ ] Orphaned MCP node processes from dead Zed parent PIDs are cleaned up on restart
- [ ] The `Restart` action on a context server works: kills the old process, spawns a new one

### Should Have

- [ ] MCP server status (Running/Error/Stopped) is visible in the agent panel or status bar
- [ ] Log messages when an MCP server enters error state, including the last error seen

### Won't Do (Out of Scope)

- Fixing the Chrome DevTools MCP server itself to handle Chrome disconnects gracefully (upstream issue)
- Monitoring Chrome process health separately from MCP
- Multi-instance MCP deduplication (two Zed sessions spawning two MCP instances is by design)