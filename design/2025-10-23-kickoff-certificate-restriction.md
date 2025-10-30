# Kickoff Mechanism and Client Certificate Restriction

**Date**: 2025-10-23
**Status**: Active architectural constraint

## The Problem

When Zed instances are launched via the **kickoff mechanism** (session-scoped external agents and SpecTasks), a WebSocket connection is made to moonlight-web to trigger Wolf container startup. This connection establishes a **persistent Moonlight session** with specific client certificates.

## Client Certificate Constraint

**CRITICAL**: Once a session is kicked off, it MUST be resumed using the **same client certificates** that were used during kickoff.

### Why This Matters

The kickoff mechanism creates a Moonlight session with:
```go
"client_unique_id": fmt.Sprintf("helix-agent-%s", sessionID)
```

When the browser connects later with the **same** `client_unique_id`, moonlight-web automatically resumes the existing session instead of creating a new one. This works because:

1. **Same client_unique_id** → moonlight-web recognizes it as resume attempt
2. **Same client certificates** → Moonlight protocol validates authentication
3. Session is **resumed** without re-launching the container

### What Breaks With External Moonlight Clients

If a user tries to connect via an **external Moonlight client** (not the browser):

1. ❌ External client has **different client certificates** than kickoff
2. ❌ Moonlight protocol rejects the connection (certificate mismatch)
3. ❌ Session cannot be resumed
4. ❌ Container may restart or connection fails

## Connection Method Requirements

### ✅ Personal Dev Environments (PDEs)
- **Connection method**: Moonlight pairing (manual pairing with external Moonlight client)
- **Kickoff**: NOT used (no automatic container startup)
- **Certificate handling**: User pairs their Moonlight client manually
- **Resume behavior**: Each connection can use different certificates
- **Why it works**: No kickoff session to resume - fresh connection every time

### ✅ Session-Scoped External Agents
- **Connection method**: WebRTC streaming (browser-based ONLY)
- **Kickoff**: YES (automatic container startup before user connects)
- **Certificate handling**: Browser uses same certificates as kickoff
- **Resume behavior**: Browser automatically resumes kickoff session
- **Why it works**: Browser and kickoff share certificate storage via moonlight-web

### ✅ SpecTask Instances
- **Connection method**: WebRTC streaming (browser-based ONLY)
- **Kickoff**: YES (automatic container startup)
- **Certificate handling**: Same as session-scoped agents
- **Resume behavior**: Same as session-scoped agents
- **Why it works**: Same certificate sharing as session agents

## UI Implementation Requirements

### Personal Dev Environments UI
```typescript
// Show Moonlight pairing option
// Hide WebRTC streaming option (for now - could add later)
<MoonlightPairingFlow pdeID={environment.id} />
```

### External Agent Sessions UI
```typescript
// Show WebRTC streaming ONLY
// Hide Moonlight pairing option
<MoonlightStreamViewer sessionId={session.id} />
```

### SpecTask Instances UI
```typescript
// Show WebRTC streaming ONLY
// Hide Moonlight pairing option
<MoonlightStreamViewer sessionId={specTask.helix_session_id} />
```

## Future Improvements

### Option 1: Make PDEs Support WebRTC Too
- Add kickoff mechanism to PDEs
- Browser connection would work seamlessly
- Still support external Moonlight clients for advanced users

### Option 2: Make Session Agents Support External Moonlight
- Don't use kickoff (wait for manual connection to start container)
- Slower startup, but more flexible
- Trade-off: User waits longer for container to start

### Option 3: Hybrid Approach
- Kickoff creates session with **well-known shared certificates**
- External Moonlight clients configured to use same certificates
- Requires certificate distribution mechanism
- More complex but most flexible

## Technical Details

### Kickoff Session Creation
```go
// From wolf_executor_apps.go:839
authMsg := map[string]interface{}{
    "AuthenticateAndInit": map[string]interface{}{
        "session_id":       fmt.Sprintf("agent-%s-kickoff", sessionID),
        "client_unique_id": fmt.Sprintf("helix-agent-%s", sessionID),  // KEY
        "mode":             "keepalive",
        // ... display settings
    },
}
```

### Browser Resume
```typescript
// Frontend connects with SAME client_unique_id
const sessionID = `agent-${helixSessionID}`;  // Without -kickoff suffix
const clientUniqueID = `helix-agent-${helixSessionID}`;  // SAME as kickoff
```

### Certificate Storage
- moonlight-web stores certificates indexed by `client_unique_id`
- Browser and kickoff share the same `client_unique_id`
- External Moonlight clients generate their own unique IDs
- Mismatched IDs → different certificate → connection rejected

## Conclusion

This restriction is **inherent to the kickoff + certificate architecture**. The UI must enforce these connection method requirements until we implement one of the future improvements above.
