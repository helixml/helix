# Investigation: Qwen Code Agent Not Being Used

**Date:** 2025-12-08
**Status:** Fixed
**Author:** Investigation notes

## Problem Statement

After deploying the latest Zed changes intended to support the "qwen code agent", users report that:
1. Zed says it's talking to "zed agent" even when qwen_code is configured in Helix frontend
2. After local deployment, no responses are received from Zed prompts at all

## Background: Two Code Agent Runtimes

Helix supports two code agent runtimes:

| Runtime | Agent Server | How It Works |
|---------|-------------|--------------|
| `zed_agent` | Zed's built-in NativeAgent | Reads env vars (ANTHROPIC_API_KEY, etc.) from container |
| `qwen_code` | Custom agent via `agent_servers` config | Runs `qwen` command with OPENAI_* env vars |

## Investigation Findings

### 1. Thread Creation Flow (External WebSocket Sync)

When Helix sends a chat_message via WebSocket, the flow is:

```
Helix API → WebSocket → thread_service.rs → ExternalAgent → AgentServer → ACP Thread
```

**Critical finding in `thread_service.rs` (lines 269-279):**

```rust
let agent = match request.agent_name.as_deref() {
    Some("zed-agent") | None => ExternalAgent::NativeAgent,
    Some(name) => ExternalAgent::Custom {
        name: gpui::SharedString::from(name.to_string()),
        command: project::agent_server_store::AgentServerCommand {
            path: std::path::PathBuf::new(),  // EMPTY!
            args: vec![],
            env: None,
        },
    },
};
```

The `command` field is empty because external sync doesn't have access to the command - it relies on the agent being registered in Zed's settings.

### 2. How CustomAgentServer Finds the Agent

**In `custom.rs` (lines 78-94):**

```rust
let agent = store
    .get_external_agent(&ExternalAgentServerName(name.clone()))
    .with_context(|| {
        format!("Custom agent server `{}` is not registered", name)
    })?;
```

`CustomAgentServer` looks up the agent by name from `agent_server_store`. This store is populated from Zed's `settings.json` → `agent_servers` section.

### 3. Settings Sync Chain

```
Helix API (/api/v1/sessions/{id}/zed-config)
    ↓
settings-sync-daemon (reads config, writes settings.json)
    ↓
Zed's settings.json (agent_servers, language_models, etc.)
    ↓
agent_server_store (populated from settings)
```

### 4. Settings-Sync-Daemon Logic

**In `settings-sync-daemon/main.go` (lines 65-115):**

```go
func (d *SettingsDaemon) generateAgentServerConfig() map[string]interface{} {
    switch d.codeAgentConfig.Runtime {
    case "qwen_code":
        // Returns qwen agent_servers config
        return map[string]interface{}{
            "qwen": map[string]interface{}{
                "command": "qwen",
                "args": []string{"--experimental-acp", "--no-telemetry"},
                "env": env,
            },
        }
    default: // "zed_agent" or empty
        // Returns nil - no agent_servers needed
        return nil
    }
}
```

**Key insight:** For `zed_agent` runtime, NO `agent_servers` config is written. This is correct because NativeAgent doesn't use agent_servers.

### 5. How agent_name is Determined

**In `zed_config_handlers.go` (getAgentNameForSession):**

```go
func (s *HelixAPIServer) getAgentNameForSession(ctx context.Context, session *types.Session) (string, *CodeAgentConfig, error) {
    // 1. Get spec task if session has one
    // 2. Get app from spec task
    // 3. Get assistant config from app
    // 4. Build CodeAgentConfig based on assistant's CodeAgentRuntime
}

func buildCodeAgentConfigFromAssistant(assistant *types.AssistantConfig, ...) {
    switch runtime {
    case types.CodeAgentRuntimeQwenCode:
        agentName = "qwen"
    default:
        agentName = "zed-agent"
    }
}
```

### 6. UI vs External Sync Comparison

**UI Thread Creation (agent_panel.rs lines 1551-1594):**
```rust
AgentType::Custom { name, command } => self.external_thread(
    Some(crate::ExternalAgent::Custom { name, command }),  // REAL command!
    None, None, window, cx,
)
```

The UI has the real command because it reads from settings. External sync passes empty command but relies on settings lookup.

## Root Cause: HelixSessionID Not Being Set on ZedAgent Request

**The core bug:** When starting/resurrecting spec task agents via `spec_task_orchestrator_handlers.go`, the `HelixSessionID` field is NOT being set on the `ZedAgent` request.

### The Problem Chain

1. **Spec task planning session is created correctly** with `SpecTaskID` in metadata (verified in `spec_driven_task_service.go:231-236`)

2. **When resurrecting the agent**, the `agentReq` is missing `HelixSessionID`:
   ```go
   // spec_task_orchestrator_handlers.go:448-460
   agentReq := &types.ZedAgent{
       SessionID:           externalAgent.ID,
       // ... other fields ...
       SpecTaskID:          task.ID,
       // BUG: Missing HelixSessionID! Should be: HelixSessionID: task.PlanningSessionID,
   }
   ```

3. **Without `HelixSessionID`**, wolf_executor defaults to using `agent.SessionID` (the external agent ID):
   ```go
   // wolf_executor.go:573-576
   helixSessionID := agent.SessionID  // Uses external agent ID!
   if agent.HelixSessionID != "" {
       helixSessionID = agent.HelixSessionID
   }
   ```

4. **`HELIX_SESSION_ID` env var is set to the WRONG value** (external agent ID instead of planning session ID)

5. **settings-sync-daemon fetches config for wrong session:**
   ```go
   // settings-sync-daemon/main.go:124,185
   sessionID := os.Getenv("HELIX_SESSION_ID")  // Wrong session ID!
   url := fmt.Sprintf("%s/api/v1/sessions/%s/zed-config", d.apiURL, d.sessionID)
   ```

6. **`getZedConfig` can't find SpecTaskID** because it's looking at the wrong session (which doesn't exist or has no SpecTaskID)

7. **Result:** `CodeAgentConfig` is nil, no `agent_servers.qwen` is written

### The Fix

Add `HelixSessionID` when creating the ZedAgent request in `spec_task_orchestrator_handlers.go`:

```go
agentReq := &types.ZedAgent{
    SessionID:           externalAgent.ID,
    HelixSessionID:      task.PlanningSessionID,  // ADD THIS LINE
    UserID:              task.CreatedBy,
    // ... rest of fields ...
}
```

**File that needs fixing:**
- `api/pkg/server/spec_task_orchestrator_handlers.go:448-460` - Add `HelixSessionID: task.PlanningSessionID`

### Why This Fixes Everything

With `HelixSessionID` set correctly:
1. `HELIX_SESSION_ID` env var → `task.PlanningSessionID`
2. settings-sync-daemon fetches config for the correct session
3. That session has `SpecTaskID` in metadata (set by `spec_driven_task_service.go`)
4. `getZedConfig` finds the spec task
5. `buildCodeAgentConfig` returns correct runtime config
6. settings-sync-daemon writes `agent_servers.qwen` to settings.json
7. Qwen agent appears in Zed's agent list

## Issue Analysis (RESOLVED)

### Issue 1: Qwen agent doesn't show in list - FIXED

**Root cause:** `HelixSessionID` was not being set on the ZedAgent request when resurrecting spec task agents, causing settings-sync-daemon to fetch config for the wrong session (external agent ID instead of planning session).

**Fix:** Added `HelixSessionID: task.PlanningSessionID` to the agentReq in `spec_task_orchestrator_handlers.go:450`.

### Issue 2: "Zed says talking to zed agent" - FIXED

**Root cause:** Same as Issue 1 - without the correct session ID, the API couldn't look up the code agent runtime from the spec task's app configuration.

**Fix:** Same fix as Issue 1.

## Verification Steps

1. Check settings.json in sandbox:
   ```bash
   docker exec <sandbox> cat /home/retro/.config/zed/settings.json
   ```

2. Verify language_models has api_url:
   ```json
   {
     "language_models": {
       "anthropic": {
         "api_url": "https://your-helix-url/v1"
       }
     }
   }
   ```

3. Check env vars in sandbox:
   ```bash
   docker exec <sandbox> env | grep -E "(ANTHROPIC|OPENAI)"
   ```

4. Check settings-sync-daemon logs:
   ```bash
   docker exec <sandbox> journalctl -u settings-sync-daemon
   ```

## Recommendations

### For qwen_code runtime:
1. Ensure app's assistant config has `code_agent_runtime: qwen_code`
2. Verify `agent_servers.qwen` is written to settings.json
3. Verify qwen command is available in sandbox PATH

### For zed_agent runtime:
1. Verify `language_models.anthropic.api_url` points to Helix proxy
2. Verify ANTHROPIC_API_KEY is set in container environment
3. Verify settings-sync-daemon is running and syncing

### Code fixes needed:
1. **Trace missing SpecTaskID** - WebSocket session creation may not be linking to spec tasks correctly
2. **Add logging** - Add debug logs to trace agent_name resolution chain

## Related Files

| File | Purpose |
|------|---------|
| `zed/crates/external_websocket_sync/src/thread_service.rs` | Creates threads from WebSocket messages |
| `zed/crates/external_websocket_sync/src/types.rs` | ExternalAgent enum and server() method |
| `zed/crates/agent_servers/src/custom.rs` | CustomAgentServer implementation |
| `helix/api/pkg/server/zed_config_handlers.go` | API endpoint for Zed config |
| `helix/api/cmd/settings-sync-daemon/main.go` | Syncs config to Zed settings.json |
| `helix/api/pkg/external-agent/wolf_executor.go` | Sets env vars for sandbox |

## Next Steps

1. [ ] Add debug endpoint to inspect current session's agent configuration
2. [ ] Add logs to trace agent_name from spec task → app → assistant config
3. [ ] Verify language_models.anthropic.api_url is being set correctly
4. [ ] Test end-to-end with explicit qwen_code configuration
