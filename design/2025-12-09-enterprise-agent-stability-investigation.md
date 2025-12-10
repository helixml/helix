# Investigation: Enterprise Agent Stability Issues

**Date:** 2025-12-09
**Status:** Root Causes Identified, Fixes Implemented
**Author:** Helix Team

---

## TL;DR - Key Findings

**The real problem:** Two bugs in qwen-code were making file writing APPEAR unreliable:

1. **`[object Object]` error display bug** - Model received useless error messages
   - Every file write failed, but model saw `[object Object]` instead of actual error
   - Model couldn't adapt or understand what was failing
   - **FIXED** in commit a8bc8ce0

2. **Overly aggressive shell security** - Blocked legitimate workarounds
   - Detected backticks/parens in DATA (markdown, Python code) as "command substitution"
   - Forced model into increasingly convoluted workarounds
   - Unnecessary in sandboxed environments (throwaway containers)
   - **FIXED** in commit a8bc8ce0

**Result:** Model eventually succeeded using `echo` commands, but struggled through 15+ failed attempts first.

**Path confusion:** Context shows real paths (`/data/workspaces/...`) but prompt instructs to use symlink (`/home/retro/work/`). Likely contributing to confusion.

**Deployed fixes:**
- ✅ qwen-code fixes (a8bc8ce0) pushed to helixml/qwen-code
- ✅ Dockerfile updated to use fixed version
- ✅ Build system updated to copy from local ~/pm/qwen-code
- **Deploy:** Run `./stack build-sway` to rebuild with fixes

**Other issues documented:** Restart button needed, RevDial reconnection too slow, session history loss on crashes.

---

## Detailed Analysis

This document investigates several stability issues reported in an enterprise deployment where external agents are experiencing:
1. Qwen Code crashes frequently with exit code 1
2. Zed agent with Qwen model mysteriously failing
3. RevDial connection drops killing browser sessions
4. Qwen Code unreliably writing files (falling back to printf/sed)
5. Task list not reliably updating and pushing
6. **IDENTIFIED:** Qwen Code exposes error messages to the model as `[object Object]`
7. **IDENTIFIED:** Overly aggressive shell security blocking legitimate commands
8. **IDENTIFIED:** Model gets into corrupted state (separate issue, needs crash logs)

## Critical Findings from Code Exploration

### [object Object] Error Display Bug (Qwen Code) - CONFIRMED WITH PRODUCTION LOGS

**Concrete Evidence from Production Logs:**

The `write_file` tool fails **EVERY SINGLE TIME** with:
```
"Error checking existing file: [object Object]"
```

**Occurrences in debug logs:**
- Line 1001: `write_file` → `"Error checking existing file: [object Object]"`
- Line 1037: `write_file` → `"Error checking existing file: [object Object]"`
- Line 1187: `write_file` → `"Error checking existing file: [object Object]"`
- Line 1299: `write_file` → `"Error checking existing file: [object Object]"`
- Line 1375: `write_file` → `"Error checking existing file: [object Object]"`
- Line 1449: `write_file` → `"Error checking existing file: [object Object]"`
- Line 1525: `write_file` → `"Error checking existing file: [object Object]"`

**Impact:** The **LLM model** has NO IDEA what the actual error is. Could be:
- Permission denied (EACCES)
- Path doesn't exist (ENOENT)
- Symlink resolution failure
- File system error

All the **model** sees in the `<tool_result>` is useless `[object Object]`.

This isn't just a human debugging problem - **the LLM can't make intelligent decisions** when it doesn't know what failed.

**Model's Workaround Behavior:**
The model is forced to fall back to increasingly desperate workarounds:
1. Try `write_file` again (fails)
2. Try `echo` with heredoc (syntax errors from escaping)
3. Try creating Python script with `write_file` (fails)
4. Try inline Python with `python3 -c` (blocked by command substitution security)
5. Finally settle on simple `echo "content" > file` (works!)

**Root Cause Found:** In `qwen-code/packages/core/src/utils/errors.ts:17-21`:

```typescript
export function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);  // BUG: Returns "[object Object]" for Node.js error objects
}
```

**Node.js fs errors** are NOT instances of Error - they're plain objects with properties like:
```typescript
{
  errno: -13,
  code: 'EACCES',
  syscall: 'open',
  path: '/home/retro/work/file.md'
}
```

Calling `String()` on these returns `"[object Object]"`.

**The Fix:**
```typescript
export function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (error && typeof error === 'object') {
    // Handle Node.js errno errors
    if ('code' in error && 'syscall' in error && 'path' in error) {
      const err = error as { code: string; syscall: string; path: string };
      return `${err.code}: ${err.syscall} failed for ${err.path}`;
    }
    // Handle objects with message property
    if ('message' in error && typeof error.message === 'string') {
      return error.message;
    }
    // Fall back to JSON serialization for other objects
    try {
      return JSON.stringify(error);
    } catch {
      return '[Error object could not be serialized]';
    }
  }
  return String(error);
}
```

This would turn `[object Object]` into useful messages like:
- `EACCES: open failed for /home/retro/work/requirements.md`
- `ENOENT: stat failed for /home/retro/work/design.md`

### ACP Server Exit Handling (Zed Fork)

**How Zed handles ACP server crashes:** In `zed/crates/agent_servers/src/acp.rs:145-162`:

```rust
let wait_task = cx.spawn({
    let sessions = sessions.clone();
    let status_fut = child.status();
    async move |cx| {
        let status = status_fut.await?;

        for session in sessions.borrow().values() {
            session
                .thread
                .update(cx, |thread, cx| {
                    thread.emit_load_error(LoadError::Exited { status }, cx)
                })
                .ok();
        }

        anyhow::Ok(())
    }
});
```

**Problem:** When the ACP server exits, `LoadError::Exited` is emitted, but there's no automatic restart mechanism. The user is left in a broken state.

**stderr logging:** Lines 133-143 show stderr is captured and logged:
```rust
let stderr_task = cx.background_spawn(async move {
    let mut stderr = BufReader::new(stderr);
    let mut line = String::new();
    while let Ok(n) = stderr.read_line(&mut line).await && n > 0 {
        log::warn!("agent stderr: {}", line.trim());
        line.clear();
    }
    Ok(())
});
```

Logs go to Zed's log system at `~/.local/share/zed/logs/`.

---

## Issue 1: Qwen Code Crashes with Exit Code 1

### Symptoms
- Qwen Code agent crashes frequently
- Exit code is 1 (generic error)
- Logs are not visible to diagnose root cause

**NOTE:** The production debug logs show a SUCCESSFUL session (files eventually created and pushed to git). We don't have logs of an actual crash yet. The issues below are still valid for improving crash visibility when they do occur.

### Potential Causes

#### 1.1 No Log Capture for Qwen Process
The Qwen agent is started as an `agent_server` via Zed. Looking at `settings-sync-daemon/main.go:99-108`:

```go
return map[string]interface{}{
    "qwen": map[string]interface{}{
        "command": "qwen",
        "args": []string{
            "--experimental-acp",
            "--no-telemetry",
            "--include-directories", "/home/retro/work",
        },
        "env": env,
    },
}
```

**Problem:** When Zed launches agent_servers, stdout/stderr may not be captured to a file. The Qwen process runs as a child of Zed, and its output goes to Zed's log stream which may not be easily accessible.

**Exploration:**
1. Check if Zed logs agent_server output: Look in `~/.local/share/zed/logs/` inside the sandbox
2. Check if Qwen has its own log file: Search for `~/.qwen/` or similar paths
3. Check journalctl in sandbox: `journalctl -u zed` or similar

#### 1.2 OpenAI API Configuration Issues
Qwen Code uses OpenAI-compatible API. Configuration from `settings-sync-daemon/main.go:83-93`:

```go
env := map[string]interface{}{
    "GEMINI_TELEMETRY_ENABLED": "false",
    "OPENAI_BASE_URL":          d.codeAgentConfig.BaseURL,
}
if d.userAPIKey != "" {
    env["OPENAI_API_KEY"] = d.userAPIKey
}
if d.codeAgentConfig.Model != "" {
    env["OPENAI_MODEL"] = d.codeAgentConfig.Model
}
```

**Potential issues:**
- `OPENAI_BASE_URL` might be incorrect or unreachable from sandbox
- API key might be expired or invalid
- Model ID might not match what the backend expects

**Exploration:**
1. Check what `OPENAI_BASE_URL` is set to in the sandbox environment
2. Verify the model ID format matches provider expectations (e.g., `nebius/Qwen/Qwen3-Coder-30B-A3B-Instruct`)
3. Test API connectivity from sandbox: `curl -X POST $OPENAI_BASE_URL/v1/chat/completions`

#### 1.3 Memory/Resource Exhaustion
Exit code 1 could indicate the process was killed due to resource limits.

**Exploration:**
1. Check container resource limits: `docker inspect` the sandbox container
2. Check for OOM kills: `dmesg | grep -i oom` on the host
3. Monitor GPU memory during Qwen operation

#### 1.4 Tool Execution Failures
Qwen Code may crash when trying to execute tools that aren't available or fail.

**Exploration:**
1. Check if required binaries are in PATH
2. Look for permission errors when writing to workspace

### Recommended Logging Improvements

Add explicit log capture for agent_servers:

```bash
# In start-zed-helix.sh or similar, redirect agent output
export QWEN_LOG_FILE="/tmp/qwen-agent.log"
# Wrap the qwen command to capture output
```

---

## Issue 2: Zed Agent with Qwen Model Mysteriously Failing

### Symptoms
- Zed agent was working with Qwen model
- Now it mysteriously fails
- No clear error messages

### Potential Causes

#### 2.1 HelixSessionID Not Set Correctly (Previously Fixed)
Per `design/2025-12-08-qwen-code-agent-investigation.md`, there was a bug where `HelixSessionID` wasn't being set on the `ZedAgent` request, causing settings-sync-daemon to fetch config for the wrong session.

**This was marked as fixed, but may have regressed or the fix may not have been deployed.**

**Verification steps:**
1. Check the deployed version includes the `HelixSessionID` fix
2. Verify settings.json in sandbox has correct `agent_servers.qwen` configuration
3. Check settings-sync-daemon logs: `/tmp/settings-sync.log` in sandbox

#### 2.2 Agent Selection Logic
From `zed_config.go:362-378`, the mapping logic:

```go
func mapHelixToZedProvider(helixProvider, model string) (zedProvider, zedModel string) {
    switch provider {
    case "anthropic":
        return "anthropic", normalizeModelIDForZed(model)
    default:
        // All other providers route through OpenAI
        return "openai", fmt.Sprintf("%s/%s", helixProvider, model)
    }
}
```

**Potential issues:**
- If provider is "openai" but the model needs Qwen-specific handling, it may not route correctly
- The model name format might not match what the Helix router expects

**Exploration:**
1. Log the exact provider and model values being passed to Zed
2. Verify the Helix API's `/v1/chat/completions` endpoint correctly routes `nebius/*` models

#### 2.3 Race Condition in Settings Sync
`start-zed-helix.sh:530-549` waits for settings.json to have `default_model`:

```bash
while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    if [ -f "$HOME/.config/zed/settings.json" ]; then
        if grep -q '"default_model"' "$HOME/.config/zed/settings.json" 2>/dev/null; then
            echo "Zed configuration ready with default_model"
            break
        fi
    fi
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done
```

**Problem:** This checks for `default_model` but Qwen config uses `agent_servers`. The wait might time out and Zed starts without proper Qwen configuration.

**Exploration:**
1. Modify the check to look for `agent_servers.qwen` when using qwen_code runtime
2. Increase the timeout from 30s
3. Add better logging of what was found

---

## Issue 3: RevDial Connection Drops Killing Browser Sessions

### Symptoms
- RevDial connection between sandbox and controlplane keeps getting terminated
- Connection reconnects, but browser session is killed before reconnection
- User loses their streaming connection

### Analysis

#### 3.1 RevDial Client Reconnection Logic
From `revdial/client.go:70-92`:

```go
func (c *Client) runLoop(ctx context.Context) {
    for {
        // ...
        if err := c.runConnection(ctx); err != nil {
            log.Error().Err(err).Msg("RevDial connection error")
            log.Info().Dur("reconnect_in", c.config.ReconnectDelay).Msg("Reconnecting...")

            select {
            case <-time.After(c.config.ReconnectDelay):
                continue
            case <-ctx.Done():
                return
            }
        }
    }
}
```

Default reconnect delay is 5 seconds (`client.go:37-39`).

**The issue:** When the RevDial connection drops, any active HTTP connections proxied through it are terminated immediately. The browser's WebRTC/streaming connection through the API loses its backend.

#### 3.2 Keep-Alive Configuration
From `revdial/client.go:136-152`, ping keepalive runs every 30 seconds:

```go
ticker := time.NewTicker(30 * time.Second)
// ...
err := controlWS.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
if err != nil {
    log.Error().Err(err).Msg("Failed to send WebSocket ping, closing connection")
    controlWS.Close()
    return
}
```

**Potential issues:**
1. 30 second ping interval may be too long for aggressive corporate proxies/firewalls
2. If ping fails, the entire connection is closed and reconnects

#### 3.3 Browser Session Coupling
The browser's Moonlight streaming session goes through the API server, which connects to Wolf via RevDial. When RevDial drops:

1. API loses connection to sandbox
2. Moonlight proxy loses its backend connection
3. Browser receives connection error
4. Browser closes the streaming session

**Problem:** There's no session resumption mechanism. When RevDial reconnects, the browser has already given up.

### Potential Causes of Connection Drops

1. **Corporate proxy/firewall timeouts:** Enterprise networks often have aggressive idle timeouts (30-60 seconds)
2. **Load balancer session timeouts:** If going through a load balancer, it may kill idle connections
3. **TLS certificate issues:** Certificate renegotiation or expiry could cause disconnects
4. **Network MTU issues:** Large packets being fragmented and dropped

### Recommended Explorations

1. **Check connection drop timing:**
   ```bash
   # In sandbox, monitor revdial-client.log
   tail -f /tmp/revdial-client.log | grep -E "(error|reconnect)"
   ```

2. **Increase ping frequency:** Change from 30s to 15s or 10s

3. **Add connection health monitoring on API side:** Log when RevDial connections are lost

4. **Consider session resumption:** Store Moonlight session state and attempt reconnection on browser side

---

## Issue 4: Qwen Code Unreliably Writing Files - ROOT CAUSE IDENTIFIED

### Symptoms
- Qwen Code can't reliably write files using its native file writing tool
- Falls back to using `printf` and `sed` on the command line
- **Production logs show 100% failure rate** for `write_file` tool

### Root Cause: [object Object] Error Masking Actual Error

**The actual issue:** It's NOT that file writing is unreliable - it's that **EVERY file write fails with the same error**, but the error message is completely useless due to the `[object Object]` bug.

**What's really happening:**
1. `write_file` tool calls Node.js fs functions
2. fs operation throws an error object (likely permission or path issue)
3. Error object passes through `getErrorMessage()`
4. `getErrorMessage()` returns `"[object Object]"` instead of actual error
5. Model sees: `"Error checking existing file: [object Object]"`
6. Model has zero information about what went wrong
7. Model tries again (fails identically)
8. Model eventually gives up and uses shell workarounds

**Why shell workarounds work:**
- `echo "content" > file` bypasses Qwen Code's file writing entirely
- Goes straight to bash, which has different permissions/path handling
- Succeeds because the actual file path is accessible from bash

### Likely Actual Root Cause (Hidden by [object Object])

Based on the pattern, the most likely causes are:

#### 4.1 Symlink Resolution in ACP File Writing

From `settings-sync-daemon/main.go:101-104`:

```go
"args": []string{
    "--experimental-acp",
    "--no-telemetry",
    "--include-directories", "/home/retro/work",
},
```

**Hypothesis:** The `--include-directories` only includes `/home/retro/work`, but:
- `/home/retro/work` is a symlink to `$WORKSPACE_DIR`
- Actual path is `/data/workspaces/spec-tasks/spt_01kc1zpahaxwwv8xehb2zmqy28`
- Qwen's ACP `write_file` may not follow symlinks correctly
- Or Qwen checks the "real" path against allowed directories and rejects it

**Why bash `echo > file` works:**
- Bash follows symlinks transparently
- No directory whitelist enforcement

**To Confirm:**
1. Fix `[object Object]` bug first (DONE - commit 8e6862ae)
2. Rebuild sway image with fix
3. See actual error message
4. Likely will be: `EACCES: access failed for /data/workspaces/...` or similar

**Proposed Fix:**
Add both paths to include-directories:
```go
args := []string{
    "--experimental-acp",
    "--no-telemetry",
    "--include-directories", "/home/retro/work",
}
// Also add the real workspace path if it differs
if workspaceDir != "" && workspaceDir != "/home/retro/work" {
    args = append(args, "--include-directories", workspaceDir)
}
```

### Fix Status

**FIXED in qwen-code commit 8e6862ae:**
- Both `packages/core/src/utils/errors.ts` and `packages/cli/src/utils/errors.ts`
- Now properly formats Node.js errno errors
- Pushed to https://github.com/helixml/qwen-code (branch: fix/object-object-error-display)

**INTEGRATED into Helix:**
- Modified `Dockerfile.sway-helix` to copy from local qwen-code build
- Modified `stack` build-sway function to prepare qwen-code-build directory
- Builds from `~/pm/qwen-code` if available, otherwise clones helixml/qwen-code fork
- Smart rebuild detection based on file timestamps

---

## Issue 5: Task List Not Reliably Updating

### Symptoms
- Qwen Code doesn't reliably keep the tasks list up to date
- Tasks are not being pushed to the UI

### Potential Causes

#### 5.1 WebSocket Sync Connection Issues
Task updates go through the WebSocket sync (`ZED_EXTERNAL_SYNC_ENABLED=true`). If the WebSocket connection is unstable, task updates may be lost.

**From wolf_executor.go:238-245:**
```go
"ZED_EXTERNAL_SYNC_ENABLED=true",
fmt.Sprintf("ZED_HELIX_URL=%s", zedHelixURL),
fmt.Sprintf("ZED_HELIX_TOKEN=%s", userAPIToken),
fmt.Sprintf("ZED_HELIX_TLS=%t", zedHelixTLS),
"ZED_HELIX_SKIP_TLS_VERIFY=true",
```

**Exploration:**
1. Check WebSocket connection logs in Zed
2. Verify the external sync is actually enabled in settings.json
3. Monitor WebSocket messages on the API side

#### 5.2 Qwen's Task Management Implementation
Qwen's handling of tasks might be different from Claude Code or other agents. It may not emit task updates in the expected format.

**Exploration:**
1. Compare Qwen's task update format with expected format
2. Check if Qwen sends task updates at all, or only at certain points

#### 5.3 Push Authentication Issues
Task updates require authentication. If the token expires or is invalid, updates will fail silently.

**Exploration:**
1. Check if `USER_API_TOKEN` is valid and not expired
2. Look for 401 errors in WebSocket logs

---

## Recommended Investigation Steps

### Immediate (Today)

1. **Get sandbox logs:**
   ```bash
   docker exec <sandbox> cat /tmp/settings-sync.log
   docker exec <sandbox> cat /tmp/revdial-client.log
   docker exec <sandbox> cat ~/.local/share/zed/logs/*.log
   ```

2. **Verify settings.json is correct:**
   ```bash
   docker exec <sandbox> cat ~/.config/zed/settings.json | jq '.'
   ```

3. **Check environment variables:**
   ```bash
   docker exec <sandbox> env | grep -E "(HELIX|OPENAI|ANTHROPIC)"
   ```

### Short-term

4. **Add Qwen log capture:**
   - Modify agent_server config to redirect output to a log file
   - Or configure Qwen to log to a specific file

5. **Reduce RevDial ping interval:**
   - Change from 30s to 15s to keep connections alive through aggressive proxies

6. **Add symlink resolution to Qwen include-directories:**
   - Include both `/home/retro/work` and the actual workspace path

### Medium-term

7. **Implement Moonlight session resumption:**
   - Store session state on connection drop
   - Attempt reconnection with same session ID

8. **Add comprehensive agent health monitoring:**
   - Monitor agent_server process status
   - Alert on frequent restarts

---

## Related Files

| File | Purpose |
|------|---------|
| `api/cmd/settings-sync-daemon/main.go` | Syncs Helix config to Zed settings.json |
| `api/pkg/external-agent/zed_config.go` | Generates Zed config from Helix app config |
| `api/pkg/revdial/client.go` | RevDial client for sandbox ↔ controlplane |
| `wolf/sway-config/startup-app.sh` | Sandbox startup, creates workspace symlinks |
| `wolf/sway-config/start-zed-helix.sh` | Launches Zed with proper configuration |
| `api/pkg/external-agent/wolf_executor.go` | Creates sandbox containers with env vars |

---

---

## Proposed Features

### Feature 1: Agent Restart Button

**Priority:** HIGH - Users get stuck when Qwen Code crashes or enters corrupted state

**Problem:**
- When Qwen Code crashes, the session is left in an unusable state
- When the model gets into a weird state (spewing malformed XML), there's no way to reset
- Currently requires creating a new session entirely

**Consideration: Session History Loss**
- ACP doesn't support listing or resuming sessions
- Restarting Qwen Code means losing chat history in the Qwen Code side
- This is why Zed Agent is preferred - it should be resumable through our sync mechanism

**Implementation Options:**

1. **Kill and restart agent_server process:**
   - Zed would need UI to trigger `AgentConnection` recreation
   - Would need to emit event that frontend can display restart button for

2. **Create new ACP session without killing process:**
   - If the process is alive but stuck, create a new session
   - Preserves the process, starts fresh conversation

3. **Full sandbox restart (heavyweight):**
   - Kill the Wolf container and restart
   - Most reliable but loses all state

**Recommended Approach:** Start with option 1 - add a restart button that kills the agent_server process and allows Zed to reconnect. The `LoadError::Exited` event already exists; we need UI to display a restart button when this occurs.

### Feature 2: Live ACP Log Tailing in Kitty Window

**Priority:** MEDIUM - Essential for debugging, helps understand failures

**Problem:**
- Qwen Code logs are hidden deep in Zed's log directory
- No easy way to watch what Qwen Code is doing in real-time
- Makes debugging in production extremely difficult

**Implementation:**

In `wolf/sway-config/start-zed-helix.sh`, open a Kitty terminal that tails the ACP logs:

```bash
# Before starting Zed, start a log monitoring terminal
if [ "$SHOW_ACP_DEBUG_LOGS" = "true" ]; then
    kitty --class acp-log-monitor \
        --title "ACP Agent Logs" \
        -e tail -f ~/.local/share/zed/logs/*.log | grep -E "(agent|acp|qwen)" &
fi
```

**Alternative:** Use a dedicated log file for agent_servers:
```bash
# In Qwen Code startup, redirect to a known location
export QWEN_DEBUG_LOG="/tmp/qwen-code-debug.log"
# Then tail this in Kitty
```

### Feature 3: RevDial Fast Reconnection + UI Feedback

**Priority:** HIGH - Connection drops are frustrating and lose user context

**Current State:**
- RevDial ping interval: 30 seconds (too long for corporate proxies)
- Reconnect delay: 5 seconds (too long, user perceives failure)
- Browser immediately closes stream on disconnect
- Fullscreen window closes on reconnect

**Proposed Changes:**

1. **Reduce ping interval** (`api/pkg/revdial/client.go`):
   ```go
   // Change from 30s to 15s (or configurable)
   ticker := time.NewTicker(15 * time.Second)
   ```

2. **Reduce reconnect delay** (`api/pkg/revdial/client.go`):
   ```go
   // Change from 5s to 1s for fast reconnect
   ReconnectDelay: 1 * time.Second,
   ```

3. **Add reconnection UI in browser** (frontend):
   - On WebSocket disconnect, show overlay "Reconnecting..."
   - Don't close the stream immediately - wait and retry
   - Keep fullscreen mode active during reconnect

4. **Session resumption** (medium-term):
   - Store Moonlight session ID on connection
   - On reconnect, attempt to resume with same session
   - Wolf needs to keep session alive briefly during reconnect window

**Implementation Files:**
- `api/pkg/revdial/client.go` - ping and reconnect timing
- `frontend/src/components/streaming/` - UI overlay for reconnecting
- `api/pkg/server/moonlight_proxy.go` - session resumption logic

### Feature 4: ACP Session Persistence (Zed-Side Infrastructure)

**Priority:** HIGH - Required for session history and resume functionality

**Status:** Zed-side implementation COMPLETE

**What was implemented (2025-12-09):**

1. **Vendored ACP runtime crate** (`crates/acp_runtime/`):
   - Forked from `agent-client-protocol` v0.9.0
   - Added `list_sessions` method to `Agent` trait
   - Added `ClientSideConnection::list_sessions` implementation
   - Added RPC decoder and handler for `session/list`

2. **AcpConnection.list_sessions** (`crates/agent_servers/src/acp.rs`):
   - Checks `session_capabilities.list` capability
   - Calls `session/list` RPC method on ACP agent
   - Returns `Vec<SessionInfo>` for history UI

3. **AgentConnection trait** (`crates/acp_thread/src/connection.rs`):
   - Already had `list_sessions()` method (default returns empty)
   - Now properly implemented for ACP agents

**Current limitation: Qwen Code doesn't support session/list or session/load yet**

Investigation of `qwen-code/packages/cli/src/zed-integration/`:
- `zedIntegration.ts:150` sets `loadSession: false` in capabilities
- No `session/list` handler exists
- Sessions are not persisted between Qwen Code restarts

**For session history to work with Qwen Code:**
1. Implement session persistence in Qwen Code (store to disk/database)
2. Add `session/list` handler returning saved sessions
3. Add `session/load` handler to restore session context
4. Set `loadSession: true` and `sessionCapabilities.list: {}` in capabilities

**Alternative: Use Zed NativeAgent instead**
- NativeAgent sessions sync to Helix backend
- Crash recovery handled by Zed + Helix
- Model routing through Helix LLM router works

**Investigation Steps for NativeAgent:**
1. Check if `HelixSessionID` fix is deployed
2. Verify Qwen model routes correctly through Helix router
3. Test Zed Agent with Qwen model end-to-end

### Feature 5: Default to Screenshot Mode in Moonlight Viewer

**Status:** ✅ COMPLETED (2025-12-10)

**Priority:** HIGH - 60fps streaming is unreliable for customers, screenshots work better

**Implementation:**
- Changed `qualityMode` default from `'high'` to `'low'` in MoonlightStreamViewer.tsx:108
- Screenshot mode uses adaptive JPEG quality polling (targets 2 FPS minimum)
- WebSocket connection remains active for keyboard/mouse input
- Video channel is automatically paused when in screenshot mode (saves bandwidth)
- User can toggle to 60fps video mode via the speed icon in toolbar

**Files Modified:**
- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Line 108

---

## Appendix: Qwen Code vs Zed Agent

| Feature | Zed Agent | Qwen Code |
|---------|-----------|-----------|
| Runtime | Built-in NativeAgent | Custom agent_server |
| API | Anthropic (via Helix proxy) | OpenAI-compatible |
| Config | `language_models.anthropic` | `agent_servers.qwen` |
| Log Location | Zed logs | `~/.qwen/tmp/<project>/chats/` |
| Tool Protocol | ACP (native) | ACP (v0.4.1) |
| Session Resumable | Yes (via Helix sync) | ✅ Yes (via SessionService) |
| Restart Recovery | Sync recovers history | ✅ Auto-resumes from `.zed/acp-session-*.json` |

---

## Next Steps (Prioritized)

1. **Fix `[object Object]` error display** - Quick win, submit PR to qwen-code
2. **Add restart button** - Unblocks stuck users
3. **Reduce RevDial ping/reconnect timing** - Reduces connection drops
4. **Add reconnection UI overlay** - Better UX during transient failures
5. **Investigate Zed Agent + Qwen model failure** - Enable more robust path
6. **Add ACP log tailing** - Improves debuggability

---

## Testing Recommendations

Since we don't have access to the customer's environment:

1. **Reproduce locally with Nebius Qwen:**
   - Configure a session to use Nebius Qwen model
   - Run through common workflows
   - Trigger error conditions (network drops, API failures)

2. **Simulate enterprise network conditions:**
   - Use tc/netem to add latency and packet loss
   - Configure aggressive proxy timeouts
   - Test RevDial reconnection behavior

3. **Stress test Qwen Code:**
   - Run multiple concurrent file operations
   - Test with large files
   - Monitor for memory leaks and crashes

---

## Executive Summary

### What We Found

1. **[object Object] Bug (CRITICAL - FIXED)** - The #1 blocker
   - Every `write_file` call failed with hidden error message
   - Model couldn't see what was failing, blindly retried
   - Fixed in qwen-code commit 8e6862ae
   - Dockerfile updated to use fixed version
   - **Impact:** Will immediately reveal the ACTUAL root cause of file writing failures

2. **Overly Aggressive Shell Security (CRITICAL - EASY FIX)** - The #2 blocker
   - Blocks legitimate commands containing backticks/parens in DATA
   - Forces model into convoluted workarounds
   - Unnecessary in sandboxed environments (can't cause damage)
   - **Fix:** Disable check when `QWEN_SANDBOXED=true` or just comment out in our fork
   - **Impact:** Unblocks Python scripts, heredocs, markdown backticks in file content

3. **No Restart Mechanism** - Users get stuck when agent crashes/corrupts
   - Zed has no UI to restart crashed ACP agents
   - Need restart button that recreates AgentConnection
   - Medium implementation effort

4. **RevDial Reconnection Too Slow** - Connection drops kill sessions
   - 30s ping interval too long for corporate proxies
   - 5s reconnect delay feels like failure to users
   - Browser closes stream instead of waiting
   - Quick fixes: reduce timings, add reconnection UI overlay

5. **Session History Loss** - Qwen Code crashes lose all context
   - ACP doesn't support session resumption
   - Zed Agent is more resumable (syncs to backend)
   - Should investigate why Zed Agent + Qwen model failed

6. **Task List Updates** - Likely consequence of #1 and #2
   - If agent can't write files reliably, it can't update tasks
   - May resolve once [object Object] and shell security are fixed

7. **Model Ignores todo_write Tool** - Workflow issue (low priority)
   - Prompt emphasizes using todos, model doesn't
   - May be distracted by constant failures
   - Test after fixing #1 and #2

8. **Confusing Path Context** - Minor UX issue
   - Context shows `/data/workspaces/...` but prompt says use `/home/retro/work/`
   - Model handles it correctly but it's confusing
   - Low priority

### Critical Path Forward

**IMMEDIATE (Deploy Today):**
1. ✅ Fix [object Object] error display (DONE - 8e6862ae)
2. ✅ Push fix to helixml/qwen-code fork (DONE - branch: fix/object-object-error-display)
3. ✅ Update Dockerfile.sway-helix to copy from local (DONE)
4. ✅ Update stack build-sway to prepare qwen-code-build (DONE)
5. ✅ Add qwen-code-build/ to .gitignore (DONE)
6. ✅ Commit all changes to main helix repo (DONE - 360903150)
7. Rebuild sway image: `./stack build-sway` (builds qwen-code locally, then Docker image)
8. Test with customer's workload - will reveal ACTUAL file write error
9. Monitor logs for real error messages

**WILL REVEAL:**
- The real reason file writes fail
- Whether it's symlink, permissions, or path issues
- Allows us to implement targeted fix

**OPTION A: Deploy [object Object] Fix Alone**
- See what the REAL file write error is
- Then decide on shell security based on what we learn
- More cautious approach

**OPTION B: Deploy Both Fixes Together (RECOMMENDED)**
1. [object Object] fix (already done)
2. Disable shell security check:
   ```bash
   cd ~/pm/qwen-code
   # Comment out line 328 in packages/core/src/utils/shell-utils.ts
   # Comment out line 248 in packages/core/src/utils/shellReadOnlyChecker.ts
   git add -A && git commit -m "fix: Disable command substitution security in sandboxed environments"
   ```
3. Rebuild qwen-code and sway image
4. Deploy together

**Why Option B:**
- Both fixes are LOW RISK (we're sandboxed)
- Shell security is actively blocking model workarounds
- Faster to iterate if both issues are resolved
- If anything breaks, we can revert both

**SHORT-TERM (This Week):**
1. Reduce RevDial ping interval: 30s → 15s
2. Reduce RevDial reconnect delay: 5s → 1s
3. Add frontend reconnection overlay (don't immediately close stream)
4. Add restart button UI when ACP agent crashes

**MEDIUM-TERM (Next Sprint):**
1. Investigate Zed Agent + Qwen model failure
2. Implement proper session resumption for Moonlight
3. Add ACP log tailing in Kitty window
4. Submit [object Object] fix as PR to upstream qwen-code

---

## Issue 6: Overly Aggressive Shell Security Blocking Legitimate Commands

### Symptoms from Logs

The model tries to use shell commands as a workaround for `write_file` failures, but gets blocked:

**Line 1411:**
```
Command substitution using $(), `` ` ``, <(), or >() is not allowed for security reasons
```

**The command that was blocked:**
```bash
echo -e "...`config_helper.py`..." > design.md
```

### Root Cause

The shell security check is **too naive** - it's doing a text search for backticks and parentheses in the ENTIRE command string, including data/content.

**False positives:**
- Markdown backticks in file content: `` `config_helper.py` ``
- Parentheses in text: `"Using (uv) for dependency management"`
- Backticks in Python code: `` `template string` ``

**These are NOT command substitution** - they're just character data inside quotes. But the security check blocks them anyway.

### Impact

The model's workarounds (echo, printf, Python scripts) are unnecessarily blocked, forcing it to use even MORE convoluted approaches.

**Cascade effect:**
1. `write_file` fails with `[object Object]` (we fixed this)
2. Model tries `echo "content with \`backticks\`" > file`
3. Security check blocks it (false positive)
4. Model tries Python script with `write_file` (fails - same [object Object])
5. Model tries `python3 -c "...with (parens)..."`
6. Security check blocks it (false positive on parens)
7. Model finally tries simple echo without markdown formatting (works!)

### The Real Fix: Remove the Security Check Entirely

**Why this security exists:**
- Inherited from base Qwen Code / Gemini CLI
- Designed for running agents OUTSIDE sandboxes on user's real machine
- Prevents models from running arbitrary commands via substitution

**Why this is wrong for Helix:**
- Models run INSIDE throwaway sandboxes
- Sandbox gets destroyed/recreated if it breaks
- No persistent damage possible
- Security check actively hurts model performance

**The correct approach:**
```typescript
// In qwen-code/packages/core/src/utils/shell-utils.ts

// Add environment variable to disable security checks in sandboxed environments
export function detectCommandSubstitution(command: string): boolean {
  // In sandboxed environments, allow everything
  if (process.env.QWEN_SANDBOXED === 'true') {
    return false; // Never block
  }

  // Original security logic for non-sandboxed environments
  // ...
}
```

**In Helix settings-sync-daemon, set:**
```go
env["QWEN_SANDBOXED"] = "true"
```

**Impact:**
- Model can use Python scripts with `python3 -c "..."`
- Model can use heredocs properly
- Model can include markdown backticks in file content
- Removes entire class of false-positive failures
- Simplifies model's life significantly

### Recommended Fix: Disable in Sandboxed Environments

**Simplest approach for our fork:**

In `qwen-code/packages/core/src/utils/shell-utils.ts:328`:

```typescript
// OLD:
if (detectCommandSubstitution(command)) {
  return {
    allAllowed: false,
    // ...
  };
}

// NEW: Just skip the check in sandboxed environments
if (process.env.QWEN_SANDBOXED !== 'true' && detectCommandSubstitution(command)) {
  return {
    allAllowed: false,
    // ...
  };
}
```

Then in `settings-sync-daemon/main.go`, add to the qwen env:
```go
env["QWEN_SANDBOXED"] = "true"
```

**Even simpler:** Just comment out the check entirely in our fork. We ONLY run sandboxed.

---

## Issue 7: Model Doesn't Use todo_write Despite Prompting

### Observation from Logs

The task requires creating 3 files - a clear multi-step task. The prompt heavily emphasizes:

```
Use the 'todo_write' tool proactively for complex, multi-step tasks to track progress
```

**But the model NEVER uses todo_write in the entire conversation.**

### Potential Causes

1. **Prompt overload** - The system prompt is ~6000 words. Model may be skipping sections.
2. **Distracted by failures** - Constant `[object Object]` errors may prevent normal workflow
3. **Task doesn't seem "complex enough"** - Model may not consider 3 files complex

### Impact

Without todo tracking:
- User has no visibility into model progress
- Model may forget steps
- Harder to debug what the model is trying to do

### Recommendation

**Test with [object Object] fix first** - The constant failures may be disrupting normal behavior. Once file writing works reliably, model may naturally use todos.

If still not using todos after fix:
- Simplify/shorten the system prompt
- Make todo_write instruction more prominent
- Or accept that simpler tasks don't need todos

---

## Issue 8: Confusing Path Context vs Prompt Instructions

### Contradiction in Initial Context

**Context message says:**
```
I'm currently working in the following directories:
  - /data/workspaces/spec-tasks/spt_01kc1zpahaxwwv8xehb2zmqy28/DLK_Ingestion.HelixPoC.BnpPivots
  - /data/workspaces/spec-tasks/spt_01kc1zpahaxwwv8xehb2zmqy28
```

**Prompt says:**
```
ALL work happens in /home/retro/work/. No other paths.
```

### Why This is Confusing

The model sees two conflicting pieces of information:
- Real workspace paths under `/data/workspaces/`
- Instruction to only use `/home/retro/work/`

This might contribute to path-related confusion, though the model does seem to use `/home/retro/work/` correctly.

### Recommendation

**Context should match instructions:**
- Either show `/home/retro/work/` in the context (hide real paths)
- Or update prompt to acknowledge the symlink: "/home/retro/work (symlink to /data/workspaces/...)"

---

### Why This Matters

The [object Object] bug has been **masking the real problems** this entire time. Every debugging session was blind because we couldn't see what was actually failing.

With the fix deployed:
- **The model will receive actual error messages in tool results**
- Model can make intelligent decisions based on real errors (EACCES vs ENOENT vs etc.)
- Model can adapt its strategy instead of blindly retrying
- We (humans) can also diagnose symlink vs permission vs path issues from logs
- Reduces model's reliance on shell command workarounds

The other issues (restart button, RevDial reconnection, shell security) are important for UX but are separate from the file writing root cause.

---

## Issue 9: Session Resumption After Agent/Zed Restart - SOLUTION FOUND

### Background

GitHub issue zed-industries/zed#80 documents that Claude Code/Qwen Code threads don't persist across Zed restarts. Users lose their entire conversation history when:
- Qwen Code crashes (exit code 1)
- Zed restarts
- Container restarts

The issue has been open since October 2024 with slow progress from Zed team.

### MAJOR DISCOVERY: Qwen Code ALREADY Has Session Persistence!

**Investigation on 2025-12-09 found that the solution already exists but isn't connected:**

#### 1. Qwen Code Persists Sessions as JSONL Files

Sessions are stored at `~/.qwen/tmp/<project_hash>/chats/<sessionId>.jsonl`

From `qwen-code/packages/core/src/services/sessionService.ts`:
```typescript
export class SessionService {
  // Lists all sessions for a project with pagination
  async listSessions(options: ListSessionsOptions = {}): Promise<ListSessionsResult>

  // Loads a specific session for resumption
  async loadSession(sessionId: string): Promise<ResumedSessionData | undefined>

  // Convenience method to load most recent session
  async loadLastSession(): Promise<ResumedSessionData | undefined>
}
```

Each JSONL file contains:
- All chat messages (user + assistant)
- Tool calls and results
- Usage metadata (tokens)
- Compression checkpoints for long conversations

#### 2. ACP Protocol Already Supports Session Resumption

From `qwen-code/packages/cli/src/acp-integration/acp.ts`:
```typescript
case schema.AGENT_METHODS.session_load: {
  const validatedParams = schema.loadSessionRequestSchema.parse(params);
  return agent.loadSession(validatedParams);
}
case schema.AGENT_METHODS.session_list: {
  const validatedParams = schema.listSessionsRequestSchema.parse(params);
  return agent.listSessions(validatedParams);
}
```

The ACP protocol defines:
- `session/list` - Returns paginated list of sessions with metadata
- `session/load` - Loads a specific session by ID

#### 3. Qwen Code Advertises This Capability

From `qwen-code/packages/cli/src/acp-integration/acpAgent.ts:113`:
```typescript
agentCapabilities: {
  loadSession: true,  // Advertises session resumption support
  promptCapabilities: {
    image: true,
    audio: true,
    embeddedContext: true,
  },
}
```

#### 4. The Implementation Exists in Qwen Code

From `acpAgent.ts:204-258`:
```typescript
async loadSession(params: acp.LoadSessionRequest): Promise<acp.LoadSessionResponse> {
  const sessionService = new SessionService(params.cwd);
  const exists = await sessionService.sessionExists(params.sessionId);
  if (!exists) {
    throw acp.RequestError.invalidParams(`Session not found for id: ${params.sessionId}`);
  }

  const config = await this.newSessionConfig(params.cwd, params.mcpServers, params.sessionId);
  await this.ensureAuthenticated(config);

  const sessionData = config.getResumedSessionData();
  await this.createAndStoreSession(config, sessionData.conversation);

  return null;
}

async listSessions(params: acp.ListSessionsRequest): Promise<acp.ListSessionsResponse> {
  const sessionService = new SessionService(params.cwd);
  const result = await sessionService.listSessions({
    cursor: params.cursor,
    size: params.size,
  });

  return {
    items: result.items.map((item) => ({
      sessionId: item.sessionId,
      cwd: item.cwd,
      startTime: item.startTime,
      mtime: item.mtime,
      prompt: item.prompt,
      gitBranch: item.gitBranch,
      filePath: item.filePath,
      messageCount: item.messageCount,
    })),
    nextCursor: result.nextCursor,
    hasMore: result.hasMore,
  };
}
```

### The Missing Link: Zed Doesn't Call These Methods

From `zed/crates/agent_servers/src/acp.rs`, the `AcpConnection` struct:
- Creates new sessions via `new_session`
- Sends prompts via `session/prompt`
- Handles cancellation via `session/cancel`
- **But NEVER calls `session/list` or `session/load`**

Neither our fork nor upstream Zed implements session resumption for ACP agents.

### Implementation Plan

**To enable session resumption in Zed:**

1. **Add `session_list` call to AcpConnection** (Rust)
   - Call when ACP agent initializes
   - Check if agent advertises `loadSession: true` in capabilities
   - If so, call `session/list` with `cwd` parameter

2. **Add UI for session selection** (Rust/GPUI)
   - Show list of previous sessions with:
     - First prompt (truncated)
     - Start time
     - Message count
     - Git branch (if available)
   - Allow user to resume or start new

3. **Add `session_load` call** (Rust)
   - When user selects a session, call `session/load`
   - Replay history into Zed's thread UI

### Schema Reference

```typescript
// ListSessionsRequest
{
  cwd: string;        // Working directory (required)
  cursor?: number;    // Pagination cursor (mtime of last item)
  size?: number;      // Page size (default: 20)
}

// ListSessionsResponse
{
  items: SessionListItem[];
  nextCursor?: number;
  hasMore: boolean;
}

// SessionListItem
{
  sessionId: string;
  cwd: string;
  startTime: string;    // ISO 8601
  mtime: number;        // File modification time
  prompt: string;       // First user prompt (truncated)
  gitBranch?: string;
  filePath: string;
  messageCount: number;
}

// LoadSessionRequest
{
  sessionId: string;
  cwd: string;
  mcpServers: McpServer[];
}
```

### Why Zed Team Is Slow

From the GitHub issue discussion:

1. **They're waiting on Claude Code SDK** - "Claude Code SDK currently still has some limitations around this"
2. **They're designing the protocol** - Working on agentclientprotocol.com/rfds/session-list
3. **Generic solution needed** - They want something that works for all ACP agents

**But for Helix**, we don't need the generic solution. Qwen Code already implements it. We just need to call it.

### Impact

This is a **HIGH VALUE, MEDIUM EFFORT** fix:
- Eliminates conversation loss on crashes
- Makes Qwen Code feel as persistent as native Zed Agent
- Requires Rust/GPUI work in our Zed fork
- ~200-400 lines of code to add session list/load UI

### Files to Modify

| File | Change |
|------|--------|
| `zed/crates/agent_servers/src/acp.rs` | Add `session_list` and `session_load` RPC calls |
| `zed/crates/agent/src/lib.rs` | Add session list UI component |
| `zed/crates/agent/src/agent_panel.rs` | Add "Resume Session" button/dropdown |

### Implementation Status (2025-12-09)

**COMPLETED:**
- Added `supports_session_load()` to `AgentConnection` trait
- Added `get_last_session_id()` to `AgentConnection` trait
- Added `load_thread()` method to `AgentConnection` trait
- Implemented all three in `AcpConnection` for ACP agents
- Added session ID persistence to `.zed/acp-session-<agent-name>.json`
- Code compiles successfully

**REMAINING:**
- Add UI to offer session resumption on Zed startup
- Test end-to-end with Qwen Code

---

## Feature 5: Restart Button - CRITICAL Thread ID Mapping Requirement

### Context

When implementing the restart button for crashed agents, there's a **CRITICAL** requirement related to the Zed-threads to Helix sessions mapping.

### The Problem

Helix maintains a mapping between:
- Zed thread IDs (in the agent panel)
- Helix session IDs (in the control plane)

This mapping enables:
1. **External message delivery**: Users can send messages to the agent via Helix UI (session details panel, spec task text box)
2. **WebSocket sync**: Messages sent from Helix get routed to the correct Zed thread via the WebSocket connection

### The Requirement

**When restarting an agent session, we MUST ensure:**

1. The new Zed thread ID is communicated back to Helix
2. OR the restart preserves/reuses the same thread ID
3. OR Helix's mapping is updated to point to the new thread ID

**If this mapping gets out of sync:**
- Messages sent from Helix won't reach the agent
- The text box at the bottom of active sessions won't work
- The spec task UI message input won't work

### Implementation Approach Options

1. **Option A: Preserve Thread ID on Restart**
   - When loading a previous session, reuse the same session ID
   - This naturally maintains the mapping
   - Requires Qwen Code to accept the same session ID

2. **Option B: Update Helix Mapping**
   - After restart, send an RPC to Helix with the new thread ID
   - Helix updates its session→thread mapping
   - Requires API endpoint for mapping updates

3. **Option C: Use Helix Session ID as Thread ID**
   - Always use the Helix session ID as the Zed thread ID
   - Eliminates mapping issues entirely
   - May require changes to how threads are identified in Zed

### Related Code

| File | Purpose |
|------|---------|
| `api/pkg/server/external_agent_websocket.go` | WebSocket handler that routes messages by session ID |
| `api/pkg/server/external_agent_handlers.go` | Session management and thread ID tracking |
| `zed/crates/agent/src/external_sync.rs` | External sync WebSocket client |

---

## PRIORITY IMPLEMENTATION PLAN (2025-12-09)

### 🚨 P-1: Complete ACP Session Persistence (CRITICAL FIRST)

**Status:** ✅ FULLY IMPLEMENTED - Qwen Code v0.4.1 has native session persistence

**Updated (2025-12-10):** Discovered that upstream Qwen Code v0.4.1 already implements full session persistence. Our fork has been rebased to include this.

**Qwen Code Session Persistence (packages/core/src/services/sessionService.ts):**
- `listSessions()` - Lists sessions with pagination, ordered by mtime
- `loadSession(sessionId)` - Loads a session by ID
- `loadLastSession()` - Convenience method for most recent session
- Sessions stored as JSONL files at `~/.qwen/tmp/<project_id>/chats/`
- Full conversation reconstruction from tree-structured records
- Chat compression checkpoint support

**Qwen Code ACP Integration (packages/cli/src/acp-integration/acpAgent.ts):**
- `loadSession: true` in agentCapabilities
- `session/list` handler - Calls SessionService.listSessions()
- `session/load` handler - Calls SessionService.loadSession()

**What's IMPLEMENTED (2025-12-09):**

1. **Low-level ACP methods** (in `zed/crates/agent_servers/src/acp.rs`):
   - `supports_session_load()` - checks if agent advertises `load_session` capability
   - `get_last_session_id(cwd)` - reads saved session ID from `.zed/acp-session-<name>.json`
   - `load_thread()` - full implementation to load a session via ACP protocol
   - `save_session_id()` - saves session ID when new sessions are created
   - `list_sessions()` - calls session/list on ACP agent

2. **Auto-resume on Zed restart** (in `zed/crates/agent_ui/src/acp/thread_view.rs:714-743`):
   ```rust
   } else if connection.supports_session_load() {
       // ACP agent (e.g., Qwen Code): Check for saved session to resume
       if let Some(session_id) = connection.get_last_session_id(&root_dir) {
           log::info!("🔄 [ACP SESSION] Found saved session, attempting to resume");
           cx.update(|_, cx| {
               connection.clone().load_thread(session_id, project.clone(), &root_dir, cx)
           })
       } else {
           // No saved session, create new
           connection.clone().new_thread(project.clone(), &root_dir, cx)
       }
   }
   ```

**How it works:**
- When Zed opens and creates an ACP thread view
- `initial_state()` checks if agent supports session loading
- If yes, reads last session ID from `.zed/acp-session-<agent>.json`
- If found, calls `load_thread()` to resume instead of `new_thread()`
- User automatically continues where they left off!

**What's REMAINING (Future Enhancement):**

**History UI Integration** - Showing ACP sessions in the existing thread history list
- NativeAgent threads are stored in Zed's SQLite database
- ACP sessions are stored on the agent side (Qwen stores in `~/.qwen/tmp/...`)
- To list ACP sessions in history UI, would need to:
  1. Call `session/list` ACP method on the agent
  2. Store session metadata in Zed's database or cache
  3. When user clicks, call `load_thread()` to fetch from agent
- This is a nice-to-have but NOT required for core "resume on restart" functionality

**Why History UI is lower priority:**
- Primary use case (auto-resume on restart) is now working
- ACP agents can only have ONE active session at a time anyway
- Most users just want "pick up where I left off", not "browse all old sessions"

**Files Modified:**
- `zed/crates/agent_ui/src/acp/thread_view.rs` - Added auto-resume logic in `initial_state()`
- `zed/crates/agent_servers/src/acp.rs` - Already had session persistence methods

---

### P0: Restart Button in Helix UI (COMPLETED)

**Location:** SpecTaskDetailDialog.tsx (in spec task details page, NOT in streaming UI)

**Requirements:**
- Button in the title bar area of the spec task dialog
- "Are you sure?" confirmation dialog before restarting
- Must wire across Helix API -> Zed -> Qwen Code
- Must fix thread-to-session mappings after restart

**Implementation Steps:**

1. **Frontend: Add Restart Button**
   - Add restart button to SpecTaskDetailDialog.tsx header
   - Add confirmation dialog with warning about session restart
   - Call new API endpoint `/api/v1/sessions/:id/restart`

2. **API: Restart Endpoint**
   - Add handler for `POST /api/v1/sessions/:id/restart`
   - Stop existing sandbox container
   - Start new container with same configuration
   - Update ZedThread -> HelixSession mapping

3. **Thread-to-Session Mapping Fix**
   - Current mappings are in-memory in `ZedToHelixSessionService`
   - On restart: new Zed thread ID gets created
   - Must update mapping: `createOrUpdateZedThreadMapping()`
   - Consider using Helix session ID as thread ID (Option C)

### P1: Live ACP Log Tailing in Kitty

**Requirements:**
- Open a Kitty terminal that tails ACP/Qwen Code logs
- Show startup message "Tailing ACP logs..."
- Display Qwen Code crash errors prominently
- Do NOT close the kitty window

**Implementation:**
```bash
# In wolf/sway-config/start-zed-helix.sh
kitty --class acp-log-viewer \
      --title "ACP Agent Logs" \
      -e bash -c 'echo "Tailing ACP logs..."; tail -f ~/.local/share/zed/logs/*.log 2>/dev/null | grep --line-buffered -E "(agent|acp|qwen|error|Error|ERROR)"' &

# Keep window open on exit
kitty_pid=$!
# Don't wait for it - let it run independently
```

### P2: RevDial Reconnection Fixes

**Status:** ✅ COMPLETED (2025-12-10)

**Backend Fixes (ALREADY DONE):**
- Ping interval: Already at 15s (`api/pkg/revdial/client.go:137`)
- Reconnect delay: Already at 1s (`api/pkg/revdial/client.go:38`)

**Frontend Fixes (COMPLETED):**
- ✅ Keep MoonlightStreamViewer mounted during transient state changes
- ✅ Show "Reconnecting..." overlay instead of unmounting
- ✅ Fullscreen no longer exits on Wolf state polling hiccups

**Implementation Details (ExternalAgentDesktopViewer.tsx):**
- Added `hasEverBeenRunning` state to track if stream was ever active
- Once running, keep MoonlightStreamViewer mounted even if Wolf state changes
- Show semi-transparent overlay with spinner during reconnection
- Prevents fullscreen exit by keeping the DOM structure stable

### P3: Crash Error Display

**Requirements:**
- If Qwen Code exits with error code 1, display the error
- Error should be visible in kitty window (live log tail will catch it)
- Consider also displaying in Helix UI

### Implementation Order

0. ~~**🚨 CRITICAL FIRST:** Complete ACP session persistence UI in Zed agent panel~~ ✅ COMPLETED
1. ~~**First:** Add restart button to SpecTaskDetailDialog~~ ✅ COMPLETED (uses stop+resume APIs)
2. ~~**Fourth:** Fix RevDial reconnection timing~~ ✅ COMPLETED (already optimal)
3. ~~**Fifth:** Add reconnection spinner to MoonlightStreamViewer~~ ✅ COMPLETED
4. **Second:** Wire up restart across Helix, Zed, Qwen Code (fix thread-to-session mappings)
5. **Third:** Add kitty log tailing to start-zed-helix.sh

---
