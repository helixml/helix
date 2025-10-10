# Ready for Wolf Hang Testing

## Current Status

✅ **Streaming Works**: moonlight-web + Helix integration complete
✅ **Diagnostic Logging**: Added to Wolf streaming code
🔄 **Wolf Rebuilding**: With hang detection logging (in progress)
✅ **Monitoring Script**: Ready to detect and capture hangs

## When Wolf Rebuild Completes

You'll see:
```
✅ Wolf container rebuilt and restarted successfully
```

Then you're ready to test!

## Testing Instructions

### Option 1: Automated Monitoring (Recommended)

**Terminal 1** - Run the monitor (detects hangs automatically):
```bash
cd /home/luke/pm/helix
./monitor-wolf-hang.sh
```

**Your Browser** - Test normally:
1. `http://node01.lukemarsden.net:8081` or `:8080`
2. Stream to Wolf-UI
3. Join different lobbies 10+ times
4. Monitor will alert when hang occurs

When hang detected:
- Logs automatically saved to `/tmp/wolf-hang-<timestamp>.log`
- Wolf automatically restarted
- You can continue testing

### Option 2: Manual Observation

If you prefer to watch logs yourself:
```bash
# Watch for hang pattern
docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep HANG_DEBUG
```

Look for:
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session X switching to Y
[HANG_DEBUG] Video PauseStreamEvent for session X, pipeline state: NULL
<-- This pattern indicates race condition
```

## What the Diagnostic Logs Capture

### Normal Lobby Switch (Working):
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session 123 switching to 456, pipeline state: PLAYING
[HANG_DEBUG] Switching interpipesrc listen-to: interpipesrc_123_video → 456_video
[HANG_DEBUG] Unrefing interpipesrc element
[HANG_DEBUG] Switch complete for session 123
```

### Bug Pattern (Hang):

**Pattern A - Race with Pause**:
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session 123 switching to 456, pipeline state: PLAYING
[HANG_DEBUG] Video PauseStreamEvent for session 123, pipeline state: PLAYING → NULL
[HANG_DEBUG] Switching interpipesrc listen-to: interpipesrc_123_video → 456_video
[HANG_DEBUG] Unrefing interpipesrc element
<-- Crash: trying to unref while pipeline is being destroyed
```

**Pattern B - Missing Completion**:
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session 123 switching to 456, pipeline state: PLAYING
[HANG_DEBUG] Switching interpipesrc listen-to: interpipesrc_123_video → 456_video
[HANG_DEBUG] Unrefing interpipesrc element
<-- No "Switch complete" = crashed during unref
```

**Pattern C - Multiple Simultaneous**:
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session 123 switching to 456, pipeline state: PLAYING
[HANG_DEBUG] Video SwitchStreamProducerEvents: session 123 switching to 789, pipeline state: PLAYING
<-- Two switches firing concurrently = race condition
```

## What Happens Next

1. **You test** → Lobby switching 10-20 times
2. **Monitor detects hang** → Saves logs automatically
3. **I analyze logs** → Identify exact pattern
4. **Implement fix** → Based on diagnostic data
5. **Test fix** → Verify hang doesn't occur
6. **Commit fix** → To wolf-ui branch

## Files Ready

- `monitor-wolf-hang.sh` - Auto-detect and recover from hangs
- `wolf-lobby-stress-test.sh` - Automated switching (needs session IDs)
- `WOLF_CRASH_INVESTIGATION.md` - Full investigation plan
- `NEXT_STEPS_WOLF_DEBUG.md` - Detailed debugging guide

## Quick Commands

```bash
# Check if Wolf rebuild done
docker images | grep wolf.*helix-fixed

# Start monitoring
./monitor-wolf-hang.sh

# Check Wolf logs for new diagnostic messages
docker compose -f docker-compose.dev.yaml logs wolf --since 5m | grep HANG_DEBUG

# If Wolf hangs, recover manually
docker rm -f helix-wolf-1
docker compose -f docker-compose.dev.yaml up -d wolf
```

I'm ready to analyze the diagnostic data as soon as we reproduce the hang!
