# External Agent Testing Status

## ✅ Completed Fixes

### 1. Settings-Sync-Daemon Configuration Parsing
**Problem**: Daemon was only parsing `context_servers` field from API, ignoring all other configuration.

**Fix Applied**:
- Updated `/home/luke/pm/helix/api/cmd/settings-sync-daemon/main.go`
- Modified both `syncFromHelix()` (lines 111-141) and `checkHelixUpdates()` (lines 373-403)
- Now parses ALL fields: `language_models`, `assistant`, `external_sync`, `agent`, `theme`
- **Commit**: `bac3ef1f7` - "Fix settings-sync-daemon to parse all config fields"

### 2. Sway Container Image Rebuilt
**Status**: ✅ Complete

- Rebuilt `helix-sway:latest` with updated settings-sync-daemon binary
- Screenshot server (grim-based) included
- New containers will get the fixed daemon automatically

### 3. Wolf Sessions Cleared
**Status**: ✅ Complete

- Restarted Wolf to clear any old/broken session state
- Ready for fresh external agent sessions

## 🧪 Ready to Test

### Prerequisites
1. Frontend running at `http://localhost:3000`
2. API container running
3. Wolf container running
4. Latest Sway image built (`helix-sway:latest`)

### Test Procedure

#### Step 1: Create External Agent Session
```bash
# Via Frontend (RECOMMENDED - handles auth automatically):
1. Go to http://localhost:3000
2. Navigate to "External Agents" section
3. Click "Start External Agent Session"
4. Enter test message: "Hello! Can you help me test the bidirectional sync?"
```

#### Step 2: Verify Container Created
```bash
# Find the new container
docker ps --filter "name=zed-external" --format "{{.Names}}\t{{.Status}}"

# Save container name for next steps
CONTAINER_NAME=$(docker ps --filter "name=zed-external" --format "{{.Names}}" | head -1)
echo "Testing container: $CONTAINER_NAME"
```

#### Step 3: Check Zed Configuration
```bash
# Inspect settings.json - should contain ALL fields now
docker exec $CONTAINER_NAME cat /root/.config/zed/settings.json | jq '.'

# Expected fields:
# ✅ context_servers
# ✅ language_models
# ✅ assistant
# ✅ external_sync (with WebSocket URL!)
# ✅ agent
# ✅ theme
```

#### Step 4: Verify Processes Running
```bash
# Check Zed process
docker exec $CONTAINER_NAME ps aux | grep zed | grep -v grep

# Check settings-sync-daemon (runs at startup)
docker exec $CONTAINER_NAME ps aux | grep settings-sync-daemon

# Check screenshot server
docker exec $CONTAINER_NAME ps aux | grep screenshot-server
```

#### Step 5: Test Screenshot Server
```bash
# Test from inside container
docker exec $CONTAINER_NAME curl -I http://localhost:9876/screenshot

# Should return HTTP 200 with Content-Type: image/png
```

#### Step 6: Monitor WebSocket Connection
```bash
# Watch API logs for WebSocket activity
docker compose -f docker-compose.dev.yaml logs -f api | grep -i "websocket\|external.*sync"

# Look for:
# - WebSocket connection established
# - Message routing between Zed and Helix
# - "message_completed" events
```

#### Step 7: Test Bidirectional Messaging
```bash
# In Helix frontend:
1. Send a message to the external agent session
2. Watch for Zed's AI response to appear in the conversation
3. Check browser console (F12) for "message_completed" WebSocket events

# In API logs:
docker compose -f docker-compose.dev.yaml logs --tail 50 api | grep "message\|interaction"
```

#### Step 8: Test Screenshot API
```bash
# Get Helix session ID from logs
HELIX_SESSION_ID=$(docker compose -f docker-compose.dev.yaml logs --tail 100 api | \
  grep "helix_session_id" | tail -1 | \
  sed -n 's/.*helix_session_id=\([a-z0-9-]*\).*/\1/p')

echo "Helix Session ID: $HELIX_SESSION_ID"

# Test screenshot endpoint (requires auth - use frontend network tab to get token)
# curl "http://localhost:8080/api/v1/external-agents/$HELIX_SESSION_ID/screenshot" \
#   -H "Authorization: Bearer YOUR_TOKEN_HERE" \
#   -o test-screenshot.png
```

## 🔍 What to Verify

### Critical Success Criteria:

1. **✅ Complete Configuration in settings.json**
   - All 6 fields present (not just context_servers)
   - `external_sync.websocket_sync.external_url` contains correct WebSocket URL
   - `language_models.anthropic.api_url` points to Helix API

2. **✅ Zed Receives Messages from Helix**
   - User messages appear in Zed's ACP thread
   - Zed creates assistant thread automatically
   - No "missing configuration" errors in Zed logs

3. **✅ Helix Receives Responses from Zed**
   - AI responses appear in Helix frontend conversation
   - `message_completed` WebSocket events fire
   - Interaction state updates to "completed"

4. **✅ Screenshot Server Working**
   - Server responds with HTTP 200
   - Returns valid PNG image data
   - Helix API can fetch screenshots successfully

## 🐛 Troubleshooting

### Issue: settings.json only has context_servers
**Cause**: Using old Sway image or settings-sync-daemon didn't run

**Fix**:
```bash
# Verify container is using latest image
docker inspect $CONTAINER_NAME | jq '.[0].Config.Image'
# Should show: helix-sway:latest

# Check if daemon ran
docker logs $CONTAINER_NAME 2>&1 | grep "settings-sync-daemon"

# Manually trigger sync (if daemon not running)
docker exec $CONTAINER_NAME /usr/local/bin/settings-sync-daemon \
  -api-url "http://api:8080" \
  -session-id "$HELIX_SESSION_ID" \
  -poll-interval 30
```

### Issue: Screenshot server returns 500
**Cause**: Sway compositor doesn't support wlr-screencopy protocol OR grim not finding Wayland display

**Fix**:
```bash
# Check Wayland display
docker exec $CONTAINER_NAME env | grep WAYLAND_DISPLAY
# Should be: wayland-1

# Check if grim works
docker exec $CONTAINER_NAME grim /tmp/test.png
# If fails: compositor issue or Wayland socket missing
```

### Issue: No WebSocket connection
**Cause**: Zed not connecting to Helix OR WebSocket URL incorrect in settings

**Check**:
```bash
# Verify WebSocket URL in settings
docker exec $CONTAINER_NAME jq '.external_sync.websocket_sync.external_url' \
  /root/.config/zed/settings.json

# Should contain correct session_id parameter
# Format: ws://api:8080/api/v1/external-agents/sync?session_id=<agent_session_id>

# Check Zed logs for connection attempts
docker exec $CONTAINER_NAME cat /root/.local/state/zed/logs/* 2>/dev/null | \
  grep -i "websocket\|external.*sync"
```

### Issue: Messages not appearing in Helix
**Cause**: Session mapping broken OR `message_completed` handler not working

**Check**:
```bash
# Verify session mappings in API logs
docker compose -f docker-compose.dev.yaml logs api | \
  grep "session.*mapping\|agent_session_id.*helix_session_id"

# Check message_completed handler
docker compose -f docker-compose.dev.yaml logs api | \
  grep "message_completed\|handleMessageCompleted"
```

## 📊 Expected Log Patterns

### Successful External Agent Creation:
```
INF Creating external agent via API endpoint session_id=<id>
INF Created Helix session for external agent helix_session_id=<id>
INF Stored user mapping for external agent session
INF Starting Wolf session for external agent
```

### Successful WebSocket Connection:
```
INF External agent WebSocket connected session_id=<id>
INF Registered external agent sync connection
```

### Successful Message Flow:
```
INF Sending message to external agent via WebSocket
INF Received message_completed from Zed session_id=<id>
INF Updated interaction with Zed response interaction_id=<id>
```

### Successful Screenshot:
```
INF Requesting screenshot from external agent container
INF Successfully retrieved screenshot from container
```

## 📝 Next Steps After Successful Test

1. **Verify bidirectional sync works end-to-end**
2. **Confirm screenshots display correctly in frontend**
3. **Test session persistence (disconnect/reconnect)**
4. **Test multiple concurrent external agent sessions**

## 🔗 Related Files

- Settings daemon: `/home/luke/pm/helix/api/cmd/settings-sync-daemon/main.go`
- Zed config generation: `/home/luke/pm/helix/api/pkg/external-agent/zed_config.go`
- API handlers: `/home/luke/pm/helix/api/pkg/server/external_agent_handlers.go`
- Screenshot handler: `/home/luke/pm/helix/api/pkg/server/external_agent_handlers.go:611-705`
- Sway image: `Dockerfile.sway-helix`
