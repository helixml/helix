# Investigation: Enterprise Agent Stability Issues

**Date:** 2025-12-09
**Status:** Investigation in Progress
**Author:** Helix Team

## Summary

This document investigates several stability issues reported in an enterprise deployment where external agents are experiencing:
1. Qwen Code crashes frequently with exit code 1
2. Zed agent with Qwen model mysteriously failing
3. RevDial connection drops killing browser sessions
4. Qwen Code unreliably writing files (falling back to printf/sed)
5. Task list not reliably updating and pushing
6. **NEW:** Qwen Code exposes error messages to the model as `[object Object]`
7. **NEW:** Model gets into corrupted state spewing malformed XML-like function calling syntax

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

### Feature 4: Prefer Zed Agent over Qwen Code

**Priority:** MEDIUM - Zed Agent is more resumable

**Rationale:**
- Zed Agent (NativeAgent) sessions sync to Helix backend
- If Zed crashes, we can resume the conversation
- Qwen Code (ACP agent_server) doesn't support session listing/resumption
- If Qwen Code crashes, conversation history is lost

**Current Status:**
- Zed Agent with Qwen model was working but "mysteriously failing"
- Need to investigate why before recommending as default

**Investigation Steps:**
1. Check if `HelixSessionID` fix is deployed
2. Verify Qwen model routes correctly through Helix router
3. Test Zed Agent with Qwen model end-to-end

---

## Appendix: Qwen Code vs Zed Agent

| Feature | Zed Agent | Qwen Code |
|---------|-----------|-----------|
| Runtime | Built-in NativeAgent | Custom agent_server |
| API | Anthropic (via Helix proxy) | OpenAI-compatible |
| Config | `language_models.anthropic` | `agent_servers.qwen` |
| Log Location | Zed logs | Unknown (needs investigation) |
| Tool Protocol | ACP (native) | ACP (experimental) |
| Session Resumable | Yes (via Helix sync) | No (ACP limitation) |
| Restart Recovery | Sync recovers history | History lost |

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

1. **[object Object] Bug (CRITICAL FIX IMPLEMENTED)** - The #1 reason file writing appeared "unreliable"
   - Every `write_file` call failed with hidden error
   - Fixed in qwen-code commit 8e6862ae
   - Dockerfile updated to use fixed version
   - **Impact:** Will immediately reveal the ACTUAL root cause of file writing failures

2. **No Restart Mechanism** - Users get stuck when agent crashes/corrupts
   - Zed has no UI to restart crashed ACP agents
   - Need restart button that recreates AgentConnection
   - Medium implementation effort

3. **RevDial Reconnection Too Slow** - Connection drops kill sessions
   - 30s ping interval too long for corporate proxies
   - 5s reconnect delay feels like failure to users
   - Browser closes stream instead of waiting
   - Quick fixes: reduce timings, add reconnection UI overlay

4. **Session History Loss** - Qwen Code crashes lose all context
   - ACP doesn't support session resumption
   - Zed Agent is more resumable (syncs to backend)
   - Should investigate why Zed Agent + Qwen model failed

5. **Task List Updates** - Likely consequence of file writing issues
   - If agent can't write files reliably, it can't update tasks
   - May resolve once [object Object] bug is fixed

### Critical Path Forward

**IMMEDIATE (Deploy Today):**
1. ✅ Fix [object Object] error display (DONE - 8e6862ae)
2. ✅ Push fix to helixml/qwen-code fork (DONE - branch: fix/object-object-error-display)
3. ✅ Update Dockerfile.sway-helix to copy from local (DONE)
4. ✅ Update stack build-sway to prepare qwen-code-build (DONE)
5. ✅ Add qwen-code-build/ to .gitignore (DONE)
6. Rebuild sway image: `./stack build-sway` (builds qwen-code locally, then Docker image)
7. Test with customer's workload
8. Monitor logs for ACTUAL error messages

**WILL REVEAL:**
- The real reason file writes fail
- Whether it's symlink, permissions, or path issues
- Allows us to implement targeted fix

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

### Why This Matters

The [object Object] bug has been **masking the real problems** this entire time. Every debugging session was blind because we couldn't see what was actually failing.

With the fix deployed:
- **The model will receive actual error messages in tool results**
- Model can make intelligent decisions based on real errors (EACCES vs ENOENT vs etc.)
- Model can adapt its strategy instead of blindly retrying
- We (humans) can also diagnose symlink vs permission vs path issues from logs
- Reduces model's reliance on shell command workarounds

The other issues (restart button, RevDial reconnection) are important for UX but are separate from the file writing root cause.
