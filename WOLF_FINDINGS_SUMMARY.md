# Wolf Lobby Hang - What We Know For Certain

## Testing Session: 2025-10-10

### Confirmed Facts

**✅ Duplicate Guard Fix IS Working:**
```log
10:17:20 WARN | [HANG_DEBUG] Video PauseStreamEvent DUPLICATE IGNORED for session 342532221405053742
```
This proves:
- Binary is updated with our fixes
- Guard logic is executing
- Duplicate events ARE being caught

**✅ Rejoin Pattern 100% Reproducible:**
- Join lobby 1 → Works
- Leave → Works
- Join lobby 2, 3 → Works
- **Rejoin lobby 1 → HANGS** (every time!)

**✅ Error on Rejoin:**
```
ERROR: Failed to map input buffer (CUDA)
ERROR: Failed to copy SYSTEM → CUDA
ERROR: Internal data stream error
```

### What the Duplicate Guard Does

**Code:**
```cpp
auto pause_sent = std::make_shared<bool>(false);

auto pause_handler = event_bus->register_handler<...>(
    [sess_id, pipeline, pause_sent](...) {
      if (*pause_sent) {
        logs::log(logs::warning, "DUPLICATE IGNORED");
        return;  // Don't send another EOS
      }
      *pause_sent = true;
      gst_element_send_event(pipeline.get(), gst_event_new_eos());
    });
```

**What it prevents:**
- Same handler being called multiple times
- Sending duplicate EOS events to same pipeline
- Some of the session counter issues

**What it DOESN'T prevent:**
- Multiple handlers being registered for same session
- Different handlers each firing once
- Stale buffer accumulation when lobby has no consumers

### The Rejoin Problem (Separate Issue)

**Root Cause:**
When lobby has no connected clients:
- Producer pipeline keeps running
- interpipesink buffers 1 frame
- Frame has CUDA memory reference from old session's GPU context

When rejoining:
- interpipesrc connects to interpipesink
- Tries to pull that buffered frame
- Frame's GPU memory is from FREED/OLD CUDA context
- New session's CUDA can't access it
- "Failed to map input buffer" → crash

**Why This Happens with Helix:**
```go
// wolf_executor.go line 258
StopWhenEveryoneLeaves: false  // Keep lobbies alive!
```

Standard Wolf apps would stop the lobby when empty, but Helix lobbies persist.

### Failed Fix Attempts

**Attempt: leaky-type=downstream**
```
interpipesink ... leaky-type=downstream
```

**Error:**
```
WARN: Pipeline parse error: no property "leaky-type" in element "interpipesink"
```

**Why:**
- interpipesink doesn't have that property
- It's a `queue` element property, not interpipesink
- Wrong element for this approach

### Current Status

**Working Fix:**
- ✅ Duplicate pause guard (prevents SOME session corruption)
- ✅ Diagnostic logging (helps debugging)

**Still Broken:**
- ❌ Rejoin hang (stale CUDA buffer references)
- ❌ No known GStreamer property to flush interpipesink

**Wolf Binary:**
- Has duplicate guard active
- Back to original `max-buffers=1` config
- First joins work, rejoins fail

## Potential Solutions Forward

### Solution 1: Report to Upstream (Best Option)

**What to report:**
1. **Duplicate PauseStreamEvent bug**
   - Include our fix (duplicate guard)
   - Show logs with 9 audio vs 2 video events

2. **Rejoin hang with persistent lobbies**
   - Affects `stop_when_everyone_leaves=false` lobbies
   - CUDA buffer corruption from stale frames
   - Request: interpipesink flush mechanism or different pattern

### Solution 2: Workaround in Helix

**Option A: Don't Rejoin**
- Track which lobbies user has visited
- Hide them from lobby list
- Force user to create new sessions
- Not ideal UX

**Option B: Restart Lobby on Empty**
- Override `stop_when_everyone_leaves = true`
- Let Wolf stop empty lobbies
- Helix recreates on next join
- Loses "persistent lobby" benefit

**Option C: Accept 90% Reliability**
- Document rejoin as known issue
- Advise users not to rejoin lobbies
- Fix will come from upstream eventually

### Solution 3: Deeper Investigation Needed

**Questions to answer:**
1. Why does interpipesink buffer stale frames?
2. Can we send a flush event between leave and rejoin?
3. Is there a GStreamer way to reset interpipe connections?
4. Could we restart just the producer pipeline on rejoin?

**Would require:**
- Studying interpipe plugin source code
- Understanding GStreamer buffer lifecycle better
- Testing different interpipe configurations

## My Recommendation

**For now:**
1. ✅ Keep duplicate guard fix (it helps!)
2. ✅ Document rejoin issue clearly
3. ✅ Report both bugs to upstream
4. ⏳ Accept workaround until upstream provides solution

**The duplicate guard alone makes it much more stable** - prevents the -1 session count corruption and multiple EOS issues. The rejoin hang is a separate (though related) problem.

## max-buffers=0 Explanation

**What I briefly tried:**
Changed `max-buffers=1` to `max-buffers=0` in interpipesink

**What max-buffers means in GStreamer:**
- `max-buffers=1`: Hold maximum 1 buffer in queue
- `max-buffers=0`: **UNLIMITED** buffers (NOT "no buffering"!)

**Why I thought it might help:**
- Maybe unlimited queue behaves differently
- Could prevent blocking behavior
- Thought it might auto-flush (wrong assumption!)

**Why it probably WON'T help:**
- `max-buffers=0` means "accumulate UNLIMITED buffers"
- Would make stale buffer problem WORSE, not better!
- All those buffers would still reference freed CUDA memory
- On rejoin: Would try to consume HUNDREDS of stale frames

**Why I reverted immediately:**
- Realized unlimited buffers is opposite of what we want
- Need FEWER buffers, not more
- Want buffers DISCARDED, not accumulated
- Switched to leaky-type attempt (which also failed)

**Verdict:**
Bad idea - would accumulate even more stale buffers. The real need is a way to **flush/clear** the interpipesink when disconnected, not unlimited accumulation.
