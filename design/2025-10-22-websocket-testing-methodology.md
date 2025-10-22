# WebSocket Sync Testing Methodology - 2025-10-22

## Objective
Verify that WebSocket bidirectional sync works end-to-end between Helix API and Zed agent running in Wolf container.

## Test Environment
- **Helix API**: http://localhost:8080
- **API Key**: hl-M-hfmHiCZQsdnX2iz7xBnjNbmNyXzrtLsr8ZrTJXBXc=
- **App ID**: app_01k63mw4p0ezkgpt1hsp3reag4
- **Docker Compose**: docker-compose.dev.yaml

## Test Methodology

### Phase 1: Baseline Test (feature/external-thread-sync-backup)
Test known working branch to verify test methodology.

**Steps:**
1. Checkout feature/external-thread-sync-backup
2. Build Zed: `./stack build-zed`
3. Rebuild Sway image: `./stack build-sway`
4. Create new external agent session via API
5. Send test message
6. Poll for response (30 seconds max)
7. Verify logs show WebSocket activity

**Expected Results:**
- ✅ WebSocket connects successfully
- ✅ Message delivered to Zed
- ✅ AI response streams back
- ✅ Response appears in session via API

### Phase 2: Test Fixed Branch (feature/external-thread-sync)
Apply fixes and verify they work.

**Steps:**
1. Checkout feature/external-thread-sync
2. Apply stashed fixes
3. Build Zed
4. Rebuild Sway image
5. Run same test as Phase 1
6. Compare results

**Success Criteria:**
- Results match Phase 1 exactly
- No regressions introduced

### Phase 3: Automated E2E Test
Create reusable test script for future validation.

## Test Script Structure

```bash
#!/bin/bash
# test-websocket-sync.sh

API_URL="http://localhost:8080"
API_KEY="hl-M-hfmHiCZQsdnX2iz7xBnjNbmNyXzrtLsr8ZrTJXBXc="
APP_ID="app_01k63mw4p0ezkgpt1hsp3reag4"

# 1. Create session
SESSION_ID=$(curl -s -X POST "$API_URL/api/v1/sessions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"session_id\": \"\", \"mode\": \"inference\", \"type\": \"text\", \"model_name\": \"\", \"helix_internal\": {\"app_id\": \"$APP_ID\"}}" \
  | jq -r '.id')

echo "Created session: $SESSION_ID"

# 2. Wait for container to start
sleep 10

# 3. Send test message
curl -s -X POST "$API_URL/api/v1/sessions/$SESSION_ID/chat" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"message": "Write a hello world function in Python"}'

# 4. Poll for response (30 seconds)
for i in {1..30}; do
  RESPONSE=$(curl -s "$API_URL/api/v1/sessions/$SESSION_ID" \
    -H "Authorization: Bearer $API_KEY" | jq -r '.interactions[0].responses[0].message')

  if [ "$RESPONSE" != "null" ] && [ -n "$RESPONSE" ]; then
    echo "✅ Got response after ${i} seconds!"
    echo "$RESPONSE"
    exit 0
  fi

  echo "⏳ Waiting for response... ($i/30)"
  sleep 1
done

echo "❌ No response after 30 seconds"
exit 1
```

## Verification Points

### Container Level
- **Binary MD5**: Verify Zed binary in container matches build
- **Process Running**: `docker exec <container> ps aux | grep zed`
- **Logs**: Check for WebSocket initialization messages

### API Level
- **Session Created**: Response has valid session ID
- **Container Started**: Docker shows new zed-external container
- **WebSocket Connected**: API logs show connection from Zed

### Zed Level
- **Panel Initialized**: Logs show AgentPanel::new()
- **WebSocket Setup**: Logs show "Setting up WebSocket integration"
- **Thread Created**: Logs show thread creation for request
- **Message Sent**: Logs show message sent to AI
- **Response Streamed**: Logs show message_added/message_completed events

### Frontend Level (Browser)
- **WebSocket Messages**: Network tab shows WS messages
- **message_completed Events**: Console shows events with correct request_id
- **UI Updates**: Session shows response text

## Progress Tracking

### Test Run 1: feature/external-thread-sync-backup (Baseline)
- **Time**: 2025-10-22 08:53
- **Branch**: feature/external-thread-sync-backup (3186d1d864)
- **Zed Build**: In progress...
- **Sway Image**: Pending
- **Test**: Pending
- **Result**: TBD

### Test Run 2: feature/external-thread-sync (Fixed)
- **Time**: TBD
- **Branch**: feature/external-thread-sync (with stashed fixes)
- **Zed Build**: Pending
- **Sway Image**: Pending
- **Test**: Pending
- **Result**: TBD

## Known Issues to Watch For
- Container using old image (check image ID vs latest)
- WebSocket setup not running (check logs for initialization)
- Thread creation failures (check thread_service.rs logs)
- Type mismatches between agent/agent2 HistoryStore
- AgentPanel not added to workspace (blocks WebSocket setup)
