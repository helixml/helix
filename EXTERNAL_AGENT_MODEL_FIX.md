# External Agent Model Parameter Fix

## Problem Description

Users were encountering a "400 Bad Request: you must provide a model parameter" error when trying to create new sessions with external agents (specifically Zed editor instances) from the `/new` page in Helix.

The issue occurred because when `agent_type: "zed_external"` was specified, the frontend was not setting the `model` parameter correctly, causing the backend to reject the request.

## Root Cause

1. **Frontend Issue**: The React frontend in `CreateContent.tsx` was not automatically setting the `model` parameter when creating external agent sessions
2. **Backend Issue**: The Go backend's `ProcessModelName` function in `models.go` didn't have a case to handle the `external_agent` model identifier
3. **Timing Issue**: The session handler didn't properly wait for external agents to be ready before processing requests

## Solution Overview

The fix involved three key changes:

### 1. Backend Model Processing (`helix/api/pkg/model/models.go`)

Added support for the `external_agent` model identifier:

```go
case "external_agent":
    // External agent requests should use this identifier
    // The actual model is configured within the external agent (Zed, etc.)
    return "external_agent", nil
```

### 2. Frontend Auto-Configuration (`helix/frontend/src/components/create/CreateContent.tsx`)

Added logic to automatically set the model parameter for external agents:

```typescript
// For external agents, override model to use external_agent identifier
if (inputs.sessionConfig.agentType === 'zed_external') {
  useModel = 'external_agent'
}
```

### 3. Backend Wait Logic (`helix/api/pkg/server/session_handlers.go`)

Added `waitForExternalAgentReady()` function with:
- 30-second timeout for agent connection
- Proper error handling if agent doesn't connect
- Removed simulation fallback logic

## Architecture Flow

```
User creates session → Frontend sets model="external_agent" → Backend accepts model → 
Backend spawns Zed instance → Backend waits for WebSocket connection → 
Messages route to Zed → Responses stream back via SSE
```

## Implementation Details

### Model Parameter Handling
- **For External Agents**: `model` is set to `"external_agent"`
- **Purpose**: This is an identifier, not an actual LLM model name
- **Actual Model**: Configured within the external agent (Zed instance)

### Session Request Structure
```json
{
  "type": "text",
  "model": "external_agent",
  "agent_type": "zed_external",
  "messages": [...],
  "external_agent_config": {
    "workspace_dir": "/path/to/workspace"
  }
}
```

### WebSocket Communication
- External agents connect via WebSocket to `/api/v1/sessions/{session_id}/websocket`
- Backend waits up to 30 seconds for connection establishment
- Commands sent as JSON messages through WebSocket
- Responses streamed back to frontend via Server-Sent Events (SSE)

## Testing

### Unit Tests
Added tests in:
- `helix/api/pkg/model/models_test.go` - Tests `ProcessModelName` with `external_agent`
- `helix/api/pkg/server/session_handlers_test.go` - Tests external agent session creation

### Integration Test
Added `TestExternalAgentModelParameter` in `helix/integration-test/api/integration_test.go`:
- Verifies session creation doesn't fail with model parameter error
- Tests end-to-end external agent configuration

### Running Tests
```bash
# Unit tests
cd helix/api
go test ./pkg/model -v -run TestProcessModelName
go test ./pkg/server -v -run TestExternalAgentSessionSuite

# Integration test (requires test server)
cd helix/integration-test/api
START_HELIX_TEST_SERVER=true go test -v -run TestExternalAgentModelParameter
```

## Expected Behavior After Fix

1. **User visits `/new` page** with external agent app
2. **Frontend automatically sets** `model: "external_agent"` when `agentType: "zed_external"`
3. **Backend accepts** the model name and creates session
4. **Backend spawns** Zed instance and waits for WebSocket connection
5. **Once connected**, user messages get routed to real Zed instance
6. **Responses stream back** through SSE to the frontend

## Error Scenarios

### Before Fix
```
400 Bad Request: you must provide a model parameter
```

### After Fix
Possible errors (these are expected and acceptable):
```
External agent not available
External agent not ready: timeout waiting for external agent to be ready after 30s
No external agent connection for session {session_id}
```

## Files Modified

| File | Purpose | Changes |
|------|---------|---------|
| `helix/api/pkg/model/models.go` | Model processing | Added `external_agent` case |
| `helix/frontend/src/components/create/CreateContent.tsx` | Session creation | Auto-set model for external agents |
| `helix/api/pkg/server/session_handlers.go` | Session handling | Added wait logic, removed simulation |
| `helix/api/pkg/server/websocket_external_agent_sync.go` | WebSocket communication | Fixed linting issue |

## Verification Steps

1. **Create External Agent Session**: Visit `/new` page and create session with `agent_type: "zed_external"`
2. **Check Model Parameter**: Verify `model: "external_agent"` is automatically set
3. **Verify No Error**: Should not receive "you must provide a model parameter" error
4. **Test Full Flow**: If external agent is available, test that messages are properly routed

## Notes

- The `external_agent` model identifier is **not** an actual LLM model
- External agents (like Zed) manage their own model configuration internally
- This fix resolves the session creation barrier but doesn't guarantee external agent availability
- External agents must still be properly configured and running to handle requests

## Future Improvements

- Add better error messages for different external agent failure scenarios
- Implement health checks for external agent availability
- Add configuration validation for external agent parameters
- Consider adding external agent discovery mechanisms