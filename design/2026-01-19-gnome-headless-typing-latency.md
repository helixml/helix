# GNOME Headless Mode: Terminal Typing Latency Investigation

**Date:** 2026-01-19
**Status:** Investigation
**Priority:** High - UX Critical

## Problem Statement

When streaming a GNOME headless desktop, **typing in a terminal feels laggy** unless there's other high-FPS content (like vkcube) running simultaneously.

**User observation:**
> "When you're running vkcube and it's pushing damage at 60 frames a second, then typing in a terminal next to it is extremely smooth. But when typing into a terminal on an otherwise still screen, the latency feels really high."

## Expected Behavior

Terminal typing should trigger immediate frame delivery:
1. Keypress → terminal renders character → damage event → frame sent → ~16ms latency

## Actual Behavior

On static screen, typing latency can be 50-100ms+, making interaction feel sluggish.

## Root Cause Analysis

### The PASSIVE Frame Clock Architecture

In GNOME headless mode, the frame clock operates in **PASSIVE mode** - completely different from real displays.

**Real display (VARIABLE mode):**
```
VBlank interrupt → frame clock wakes up → checks for damage → paints → sends
No external round-trips required for scheduling
```

**Headless virtual monitor (PASSIVE mode):**
```
Damage → schedule_update → PipeWire trigger → wait for callback → dispatch → paint → send
        └─────────────── PipeWire round-trip adds latency ───────────────┘
```

### Source Code Evidence

**File:** `mutter/src/backends/meta-screen-cast-virtual-stream-src.c`

```c
// Line 264-266: Virtual monitors ALWAYS use PASSIVE frame clock
if (meta_screen_cast_stream_src_is_enabled (src) &&
    !meta_screen_cast_stream_src_is_driving (src))
  make_frame_clock_passive (virtual_src, view);

// Line 209-223: Setting PASSIVE mode removes internal timer
static void
make_frame_clock_passive (MetaScreenCastVirtualStreamSrc *virtual_src,
                          ClutterStageView               *view)
{
  ClutterFrameClock *frame_clock = clutter_stage_view_get_frame_clock (view);
  MetaScreenCastFrameClockDriver *driver;

  driver = g_object_new (META_TYPE_SCREEN_CAST_FRAME_CLOCK_DRIVER, NULL);
  driver->src = src;

  // This removes internal timers - now externally driven
  clutter_frame_clock_set_passive (frame_clock, CLUTTER_FRAME_CLOCK_DRIVER (driver));
}
```

**File:** `mutter/clutter/clutter/clutter-frame-clock.c`

```c
// Line 1208-1209: PASSIVE mode delegates to driver
case CLUTTER_FRAME_CLOCK_MODE_PASSIVE:
  clutter_frame_clock_driver_schedule_update (frame_clock->driver);
```

**File:** `mutter/src/backends/meta-screen-cast-stream-src.c`

```c
// Line 970-982: Driver schedule goes through PipeWire
void meta_screen_cast_stream_src_request_process (MetaScreenCastStreamSrc *src)
{
  if (!priv->pending_process && !pw_stream_is_driving (priv->pipewire_stream))
  {
    pw_stream_trigger_process (priv->pipewire_stream);  // ASYNC!
    priv->pending_process = TRUE;
  }
}
```

### The Full Latency Path

When typing on a static screen:

1. **Keypress** → terminal receives XKB event
2. **Terminal renders** → Wayland `wl_surface.commit`
3. **Mutter compositor** → marks damage on ClutterStageView
4. **ClutterStageView** → calls `clutter_stage_view_schedule_update()`
5. **Frame clock (PASSIVE)** → calls `clutter_frame_clock_driver_schedule_update()`
6. **ScreenCast driver** → calls `meta_screen_cast_stream_src_request_process()`
7. **PipeWire trigger** → `pw_stream_trigger_process()` (ASYNC)
8. **Wait for PipeWire** → main loop iteration, callback scheduling
9. **`on_stream_process` callback** → fires asynchronously
10. **Dispatch** → calls `clutter_frame_clock_dispatch()`
11. **Paint** → stage renders to framebuffer
12. **Record** → `meta_screen_cast_stream_src_record_frame()`
13. **PipeWire queue** → buffer sent to consumer

**The latency is in steps 7-9**: The PipeWire round-trip to trigger a dispatch.

### Why vkcube Masks the Problem

With vkcube running at 60 FPS:
- Frame clock is constantly cycling through dispatch
- Damage is checked and painted on every cycle
- Terminal typing piggybacks on the existing 16ms frame rhythm
- No waiting for PipeWire round-trip because dispatch is already scheduled

Without vkcube:
- Frame clock is dormant (PASSIVE with no scheduled updates)
- Terminal typing must wake up the frame clock via PipeWire
- Each keypress pays the PipeWire round-trip cost

## Quantifying the Latency

### PipeWire Round-Trip Components

| Component | Typical Time |
|-----------|--------------|
| `pw_stream_trigger_process()` call | ~1µs |
| PipeWire thread wakeup | 1-10ms |
| Main loop iteration | 1-5ms |
| Callback scheduling | ~1ms |
| **Total Round-Trip** | **5-20ms per frame** |

### Compounded Latency on Static Screen

On a static screen with 10 FPS keepalive:
- Base typing latency: 16ms (one frame)
- + PipeWire round-trip: 5-20ms
- + Potential keepalive interference: 0-100ms
- **Total perceived latency: 20-136ms**

This explains why typing feels "laggy" on a still screen.

## Potential Solutions

### Solution 1: Reduce Keepalive Interval (Quick Win)

**Current:** `keepalive-time=500` (10 FPS minimum)
**Proposed:** `keepalive-time=33` (30 FPS minimum)

**Pros:**
- No code changes to Mutter
- Frame clock stays active, reducing wake-up latency

**Cons:**
- Wastes bandwidth on truly static screens
- Doesn't fix the fundamental architecture issue

### Solution 2: Synthetic Damage Injection

Keep frame clock constantly active by injecting minimal synthetic damage:

```c
// In meta_screen_cast_virtual_stream_src_enable():
g_timeout_add (16, inject_synthetic_damage, virtual_src);

static gboolean inject_synthetic_damage (gpointer user_data)
{
  MtkRectangle damage = { 0, 0, 1, 1 };  // 1x1 pixel
  clutter_actor_queue_redraw_with_clip (CLUTTER_ACTOR (stage), &damage);
  return G_SOURCE_CONTINUE;
}
```

**Pros:**
- Frame clock runs at consistent 60 FPS
- Damage-based delivery still works (1x1 pixel is minimal overhead)
- Terminal typing has no wake-up latency

**Cons:**
- Requires patching Mutter or implementing in our streaming code
- Minimal but non-zero GPU overhead

### Solution 3: Make Virtual Monitors Use VARIABLE Mode

Modify Mutter to use VARIABLE frame clock mode for virtual monitors with a software timer instead of PASSIVE mode.

```c
// Instead of:
make_frame_clock_passive (virtual_src, view);

// Use:
clutter_frame_clock_set_mode (frame_clock, CLUTTER_FRAME_CLOCK_MODE_VARIABLE);
clutter_frame_clock_set_refresh_rate (frame_clock, refresh_rate);
```

**Pros:**
- Frame clock runs on internal timer like real displays
- Most architecturally correct solution
- No PipeWire round-trip for scheduling

**Cons:**
- Requires upstream Mutter patch
- May have unintended consequences for other ScreenCast users

### Solution 4: Client-Side Frame Pacing (Workaround)

In our streaming client, implement predictive frame requests:

```go
// Speculatively request frames even when display is static
// to keep Mutter's frame clock warm
if timeSinceLastFrame > 16*time.Millisecond {
    requestNewFrame()
}
```

**Pros:**
- No Mutter changes required
- Can tune aggressiveness

**Cons:**
- Works against damage-based design
- Increases PipeWire traffic

## Recommended Approach

### Phase 1: Immediate Mitigation
- Reduce keepalive to 33ms (30 FPS) - trades bandwidth for latency
- Test if typing feels responsive

### Phase 2: Proper Fix
- Implement synthetic damage injection on the desktop side
- Small timeout (16ms) that queues 1x1 pixel damage
- Keeps frame clock active without significant overhead

### Phase 3: Upstream Discussion
- File GNOME GitLab issue discussing PASSIVE mode latency for interactive sessions
- Propose VARIABLE mode option for remote desktop use cases

## Implementation Files

### For Keepalive Change
- `api/pkg/desktop/ws_stream.go` - Change `keepalive-time=500` to `keepalive-time=33`

### For Synthetic Damage
- `desktop/helix-ubuntu/` startup scripts - Add synthetic damage daemon
- Or: Patch `gst-pipewire-zerocopy` to periodically trigger damage

## Testing Procedure

```bash
# 1. Start session
/tmp/helix spectask start --project $HELIX_PROJECT -n "latency test"

# 2. Connect to stream
/tmp/helix spectask benchmark $SESSION_ID --duration 30

# 3. Open terminal and type rapidly
# Observe: Is each character visible within 1-2 frames (~32ms)?

# 4. Compare with vkcube running
vkcube &
# Now type in terminal - should feel smoother
```

## CRITICAL BUG: DmaBuf Buffer Reuse Race Condition

**Symptom:** Frames appear out of order - e.g., maximizing a window shows maximized → flash back to un-maximized → maximized again. Worse at 4K than 1080p.

**Root Cause:** In `pipewire_stream.rs`, we return the PipeWire buffer BEFORE the CUDA copy completes.

```rust
// Line 844: Frame extracted, sent to channel
let _ = frame_tx_process.try_send(frame);

// Line 859: Buffer returned IMMEDIATELY - race condition!
unsafe { stream.queue_raw_buffer(pw_buffer) };
```

**The Race:**
1. Mutter renders Frame A → GPU buffer X
2. PipeWire delivers buffer with FD pointing to GPU buffer X
3. We dup the FD (our FD still points to GPU buffer X)
4. We return buffer to PipeWire via `queue_raw_buffer`
5. **Mutter reuses GPU buffer X for Frame B** (race!)
6. GStreamer thread imports our FD → reads Frame B instead of Frame A!

**Why worse at 4K:** CUDA copy takes longer, larger race window.

**Fix Options:**

1. **Quick Fix:** Do CUDA copy in PipeWire callback before returning buffer
   - Blocks PipeWire thread during copy
   - Simple but adds latency to capture

2. **Proper Fix:** Hold buffer until CUDA copy completes
   - Pass `pw_buffer` reference through channel
   - Return buffer via callback after copy
   - Requires architectural changes

### IMPLEMENTED FIX (2026-01-19)

Chose Option 1: Do CUDA copy in PipeWire callback before returning buffer.

**Changes:**
- `pipewire_stream.rs`: Added `CudaResources` struct passed to PipeWire thread
- `pipewire_stream.rs`: Added `process_dmabuf_to_cuda()` function called in process callback
- `pipewire_stream.rs`: Added `FrameData::CudaBuffer` variant for pre-processed frames
- `pipewire_stream.rs`: `queue_raw_buffer` now called AFTER CUDA copy completes
- `pipewiresrc/imp.rs`: Updated to pass `CudaResources` to `PipeWireStream::connect()`
- `pipewiresrc/imp.rs`: Added handler for `FrameData::CudaBuffer` in `create()`
- Removed fallback code: if CUDA processing fails, frame is dropped (no corruption)

**Key insight:** The CUDA copy must complete synchronously in the PipeWire thread
BEFORE calling `queue_raw_buffer`. Dup'd file descriptors point to the same GPU
memory, so returning the buffer allows Mutter to overwrite the data we're reading.

## Related Design Documents

- `design/2026-01-11-mutter-damage-based-frame-pacing.md` - Frame rate investigation
- `design/2026-01-06-pipewire-keepalive-mechanism.md` - Keepalive implementation

## References

- Mutter source: `~/pm/mutter/src/backends/meta-screen-cast-virtual-stream-src.c`
- Clutter frame clock: `~/pm/mutter/clutter/clutter/clutter-frame-clock.c`
- [GitLab GNOME Mutter](https://gitlab.gnome.org/GNOME/mutter)
