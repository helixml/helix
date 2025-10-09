# External Agent Bidirectional Sync - Final Status

## 🎯 Goal Summary
Achieve bidirectional message sync between Zed (external agent) and Helix sessions:
1. ✅ Zed receives complete configuration including language models
2. ✅ WebSocket connection established
3. ✅ Messages flow Helix → Zed
4. ❌ Messages NOT flowing Zed → Helix (blocked by Zed language model issue)

## ✅ FULLY WORKING Components

### 1. Settings-Sync-Daemon ✅ (Commit: bac3ef1f7)
**Fixed**: Daemon now parses ALL config fields from API, not just context_servers

**Files Modified**:
- `/home/luke/pm/helix/api/cmd/settings-sync-daemon/main.go`
  - Lines 111-141: `syncFromHelix()` - parse language_models, assistant, external_sync, agent, theme
  - Lines 373-403: `checkHelixUpdates()` - same fix for polling

**Verified Working**:
- Daemon fetches config from API every 30 seconds
- Writes complete settings.json to `/home/retro/.config/zed/settings.json`
- All fields present in generated file

### 2. Zed Config Endpoint for External Agents ✅ (Commit: 5499e8451)
**Fixed**: Handle external agent sessions with empty parent_app

**Files Modified**:
- `/home/luke/pm/helix/api/pkg/server/zed_config_handlers.go`
  - Lines 50-65: Create default app config when parent_app is empty
  - Lines 143-147: Use session.Updated for version when app.Updated is zero

**Verified Working**:
- Endpoint returns 200 for external agent sessions
- Generates complete config with all required fields

### 3. Language Models Configuration ✅ (Commit: f674df1ef)
**Fixed**: Add api_url support and startup wait for complete settings

**Files Modified**:
- `/home/luke/pm/helix/api/pkg/external-agent/zed_config.go`
  - Lines 41-44: Added APIURL field to LanguageModelConfig
  - Lines 78: Add session_id parameter to WebSocket URL
  - Lines 88-98: Provide default anthropic/claude-sonnet-4-5 for external agents
  - Lines 101-119: Use api_key from environment (Zed calls Anthropic directly)

- `/home/luke/pm/helix/wolf/sway-config/start-zed-helix.sh`
  - Lines 45-58: Wait for language_models to appear in settings.json before launching Zed

**Verified Working**:
- Startup script waits for complete settings
- settings.json contains api_key

### 4. WebSocket Sync Infrastructure ✅
**Status**: FULLY OPERATIONAL

**Evidence from Logs**:
```
✅ [WEBSOCKET] WebSocket connected! Response status: 101
✅ [WEBSOCKET-IN] Incoming task started, waiting for messages
📥 [WEBSOCKET-IN] Received text: {"type":"chat_message"...}
✅ [WEBSOCKET-IN] Parsed command type: chat_message
✅ [THREAD_SERVICE] Created ACP thread: 4294967571
📤 [THREAD_SERVICE] Sent thread_created: 4294967571
```

**Verified Working**:
- Zed connects to Helix WebSocket on startup
- Receives chat_message events
- Creates ACP threads for conversations
- Sends thread_created events back to Helix

## ❌ BLOCKING ISSUE

### Zed Language Model Configuration Format Mismatch

**Problem**: Zed reports "No language model configured" despite settings.json containing:
```json
{
  "language_models": {
    "anthropic": {
      "api_key": "sk-ant-api03--..."
    }
  },
  "assistant": {
    "version": "2",
    "default_model": {
      "provider": "anthropic",
      "model": "claude-sonnet-4-5"
    }
  }
}
```

**Evidence**:
- Startup script confirms: "✅ Zed configuration ready with language_models"
- Zed attempts to send message: "🔧 [THREAD_SERVICE] Calling thread.send() with message: Test"
- Zed error: "❌ [THREAD_SERVICE] Send task failed: No language model configured"

**Root Cause**: Unknown Zed-specific format requirement

**Possible Issues**:
1. Missing required fields in language_models config (version, available_models, etc.)
2. Wrong provider name format (needs to match exact Zed enum)
3. Missing global language model settings
4. api_key needs to be in different format/location

**Next Steps**:
1. Check Zed source code for LanguageModel settings schema
2. Find working example of Zed settings.json with Anthropic
3. Compare our generated config with working example
4. Add missing required fields

## 🔄 Message Flow (What IS Working)

### Helix → Zed ✅
1. User sends message to Helix session
2. API creates interaction
3. `notifyExternalAgentsOfNewInteraction()` called
4. WebSocket sends `chat_message` event to Zed
5. Zed receives and processes message
6. Zed creates/updates ACP thread

### Zed → Helix ❌ (Blocked)
1. Zed attempts to send message to LLM
2. **FAILS**: "No language model configured"
3. No AI response generated
4. No `message_completed` event sent back
5. Helix interaction stays in "waiting" state

## 📊 Test Sessions Created

| Agent Session ID | Helix Session ID | Status | Notes |
|-----------------|------------------|---------|-------|
| test-1760002579 | ses_01k7458724534h80vw8q6vj5jn | Config working, no LLM | Old api_url config |
| screenshot-test-1760003244 | ses_01k745wgmmt1gzbfh34cy9cayj | Config working, no LLM | WebSocket connected, 3 messages sent |
| final-1760004155 | ses_01k746ra6xvbjt3j6w8eysfjjd | Config working, no LLM | api_url version |
| SUCCESS-1760004401 | ses_01k746zszmggfw5mbvbnz0avxv | Config working, no LLM | Wait fix applied |
| APIKEY-1760004790 | ses_01k747bpnez21zf796trq0xa1g | Config working, no LLM | api_key version (current) |

**All sessions show same pattern**:
- ✅ WebSocket connects
- ✅ Messages reach Zed
- ✅ Threads created
- ❌ "No language model configured" blocks AI responses

## 🐛 Screenshot Issue (Separate from Sync)

**Status**: Compositor protocol limitation

**Problem**: Sway compositor doesn't support wlr-screencopy-unstable-v1

**Evidence**:
```bash
$ docker exec $CONTAINER grim /tmp/test.png
compositor doesn't support wlr-screencopy-unstable-v1
```

**Tried**:
- ✅ wayland-1 socket
- ✅ wayland-2 socket
- Both fail with same error

**Root Cause**: Games-on-Whales Sway build doesn't include screencopy protocol

**Solution Needed**: Rebuild Sway with wlr-screencopy support OR use alternative screenshot method

## 📝 Architecture Clarifications (From User Feedback)

### WebSocket URLs
- **Current**: Include session_id parameter
- **Should Be**: No session_id - support 1 Zed instance handling multiple sessions via thread mapping
- **Reason**: External agents reuse Zed instances across Helix sessions

### Config Sync
- **Current**: 30-second polling
- **Should Be**: Event-driven (file watch or HTTP callback)

### Parent App Linking
- **Current**: External agents have empty parent_app
- **Should Be**: Link to originating Helix app/agent for context

### API Endpoints
- **WebSocket Sync**: ws://api:8080/api/v1/external-agents/sync (for bidirectional messaging)
- **Anthropic API**: Zed calls anthropic.com directly with api_key (NOT through Helix proxy)

## 🔍 Debugging Commands

### Check Settings in Container
```bash
CONTAINER=$(docker ps --filter "name=zed-external" --format "{{.Names}}" | head -1)
docker exec "$CONTAINER" cat /home/retro/.config/zed/settings.json | jq '.'
```

### Check Zed Logs
```bash
docker logs "$CONTAINER" 2>&1 | grep -i "language\|model\|error" | tail -30
```

### Check WebSocket Connection
```bash
docker compose -f docker-compose.dev.yaml logs --tail 100 api | grep -i "websocket.*connect"
```

### Check Message Flow
```bash
docker compose -f docker-compose.dev.yaml logs --tail 200 api | grep -E "chat_message|thread_created|message_completed"
```

### Test Config Endpoint
```bash
API_KEY="hl-80B8fQwxPScxobApjVvA-ag8N7_m6X48ss0qPu3Dvig="
HELIX_SID="ses_01k747bpnez21zf796trq0xa1g"
curl -s "http://localhost:8080/api/v1/sessions/$HELIX_SID/zed-config" \
  -H "Authorization: Bearer $API_KEY" | jq '.'
```

## 🎯 Critical Next Step

**MUST**: Find correct Zed language_models configuration format

**Potential Sources**:
1. Zed GitHub repository settings schema
2. Zed documentation for language model configuration
3. Working Zed installation settings.json example
4. Zed source code for settings validation

**Current Blocking Error**: `❌ [THREAD_SERVICE] Send task failed: No language model configured`

This is the ONLY remaining blocker for full bidirectional sync.
