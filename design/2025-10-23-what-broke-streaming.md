# What Broke Streaming - Root Cause Analysis

## Summary

Streaming broke sometime between **Oct 22 21:24** and **Oct 23 10:00**. Systematic rollback revealed the issue was in commits between `a7fb1cc19` and `8f559f0dd`.

## Working State (Confirmed)

**Commit**: `a7fb1cc19` (Oct 22 21:24)
**Result**: ✅ **STREAMING WORKS**

Components at this commit:
- helix frontend: Proxy-based auth, 10 Mbps bitrate, H264 only
- moonlight-web: NO cert persistence (e0f1de6)
- wolf: Clean state (eb78bcc)

## Broken State

**Commits**: `8f559f0dd` and later (Oct 22 21:46 onwards)
**Result**: ❌ **BLACK SCREEN**

## Suspects (Changes Between Working and Broken)

### Suspect #1: Codec Detection Changes (MOST LIKELY)

**Commits**:
- `e3dee9622` (Oct 22 21:29): Enable H265/HEVC codec
- `62e2c79b6` (Oct 22 21:30): Use browser codec auto-detection
- `133ccee50` (Oct 22 21:36): Fix async await for getSupportedVideoFormats()

**Why suspicious**:
- Changed codec negotiation flow
- Even though logs showed H264 only, something in the negotiation broke
- Timing matches: last working at 21:24, first codec change at 21:29

**Evidence**:
- Rollback to BEFORE codec changes (`a7fb1cc19`) = streaming works
- Rollback to AFTER codec changes (`8f559f0dd`) = black screen

### Suspect #2: SSE Parsing Changes

**Commits**:
- `95a18e836` (Oct 22 21:38): Fix SSE double data: prefix
- `e3ea88422` (Oct 22 21:39): Revert frontend SSE workaround

**Why less likely**:
- SSE is for chat messages, not streaming
- But could affect WebSocket message processing

### Suspect #3: Port Range Changes

**Commits**:
- `359b7e99d` (Oct 22 21:46): Expand moonlight-web UDP ports to 201
- `8f559f0dd` (Oct 22 21:50): Update config to match

**Why less likely**:
- Just configuration changes
- Should not break existing functionality

### Suspect #4: Certificate Persistence (Moonlight-Web)

**Commit**: `38d287f` (Oct 22 21:18)
**Status**: REMOVED in rollback (moonlight-web at e0f1de6)

**Why suspected**:
- User's original suspicion
- Timing: Added at 21:18, problems appeared later
- May have broken Wolf `/launch` call flow
- But working state still has helix at 21:24, AFTER cert persistence

**Conclusion**: Probably not the primary cause, but could be contributing factor

## What We Learned

### The Actual Problem (Not What I Thought)

**What I assumed**:
- Apps mode missing runner start
- Need to call `/api/v1/runners/start`

**Reality**:
- Kickoff SHOULD trigger Wolf `/launch` via Moonlight protocol
- Kickoff creates sessions, pairs, spawns streamer
- But somewhere the `/launch` HTTP call stopped happening
- No `/launch` = no Wolf StreamSession = no video pipeline

**My StartRunner hack**:
- Bypassed the real problem
- Didn't pass client certs
- Would break RESUME
- Was completely wrong approach

### The Rollback Revealed

By rolling back in stages:
1. First to `8f559f0dd` - STILL BROKEN
2. Then to `a7fb1cc19` - WORKS

This narrows the problem to one of:
- Codec detection changes
- SSE parsing changes
- Port range changes

**Most likely**: Codec detection, since that's the most invasive change.

## Next Steps

### Test These Scenarios

1. ✅ **Basic streaming works** (confirmed)
2. ⬜ **Disconnect/reconnect** - Does RESUME work without cert persistence?
3. ⬜ **Page refresh** - Does it break? (Expected: yes, without cert persistence)
4. ⬜ **Multiple concurrent agents** - Do they interfere?

### Investigate on Feature Branches

**Certificate persistence** (`feature/cert-persistence-2025-10-22`):
- Understand why it might have broken `/launch` calls
- Design proper fix that preserves cert reuse AND `/launch` flow

**Codec detection** (`feature/decrypt-debug-2025-10-23` has these):
- Review the codec negotiation changes
- Understand what actually broke
- Test incremental re-addition

### Production Readiness

**Current state is DEMO-READY**:
- ✅ Streaming works
- ✅ Containers start properly via kickoff
- ✅ Proxy auth working
- ⚠️ Bitrate at 10 Mbps (looks bad) - increasing to 30 Mbps now
- ⚠️ RESUME may not work without cert persistence (needs testing)

## Configuration

**Current Settings**:
- Resolution: 1920x1080@60fps
- Bitrate: 10 Mbps → **changing to 30 Mbps**
- Codec: H264 only (hardcoded, no auto-detection)
- Auth: Proxy-based (JWT → shared secret swap)

**Architecture**:
- WOLF_MODE=apps (apps-based external agents)
- Kickoff approach (temporary session to start container)
- Frontend connects via `/moonlight` proxy
- NO certificate persistence (fresh certs each time)
