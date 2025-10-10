# Wolf Rejoin Hang - Fix Hypotheses

## The Problem (Confirmed)
- Rejoin to previously-left lobby = 100% hang
- Error: CUDA buffer copy fails on stale buffered frame
- Lobby producer keeps running with no consumers
- interpipesink holds 1 stale frame with freed GPU memory reference

## Hypothesis 1: Stop/Restart Producer When Empty ⭐⭐⭐
**Confidence: 85%**

**Approach:**
When lobby becomes empty (connected_sessions = 0):
- Send StopLobbyEvent to producer pipeline
- Producer shuts down cleanly
- On next join: Recreate producer pipeline from scratch

**Implementation:**
```cpp
// lobbies.cpp after leave
if (lobby.connected_sessions->load()->size() == 0 && !lobby.stop_when_everyone_leaves) {
  logs::log(logs::info, "Lobby empty, stopping producer pipelines to prevent stale buffers");
  ev_bus->fire_event(immer::box<events::StopLobbyEvent>{
    events::StopLobbyEvent{.lobby_id = lobby.id}
  });
  // Store flag: lobby_needs_restart[lobby.id] = true
}

// On rejoin - check if needs restart and recreate producer
```

**Pros:**
- Guaranteed fresh CUDA context
- No stale buffers possible
- Clean solution matching GStreamer lifecycle

**Cons:**
- Producer startup delay on rejoin (~2-3 seconds)
- More complex state tracking
- Wayland display would need recreation

---

## Hypothesis 2: GStreamer FLUSH Events ⭐⭐⭐⭐
**Confidence: 90%**

**Approach:**
Send GStreamer flush events to producer pipeline when lobby empties:
```cpp
// When lobby becomes empty
auto lobby_id_num = std::stoul(lobby.id);
auto flush_start = gst_event_new_flush_start();
auto flush_stop = gst_event_new_flush_stop(true); // reset_time=true

// Send to video producer pipeline
gst_element_send_event(producer_pipeline.get(), flush_start);
gst_element_send_event(producer_pipeline.get(), flush_stop);
```

**What flush does:**
- `flush_start`: Clears ALL buffers in pipeline (including interpipesink!)
- `flush_stop`: Resumes normal operation with clean state
- Standard GStreamer mechanism for exactly this use case

**Implementation Location:**
```cpp
// lobbies.cpp line 60-71 (modified)
if (lobby.connected_sessions->load()->size() == 0) {
  if (!lobby.stop_when_everyone_leaves) {
    // Fire a custom FlushProducerEvent
    ev_bus->fire_event(immer::box<events::FlushProducerEvent>{
      events::FlushProducerEvent{.lobby_id = lobby.id}
    });
  }
}

// In streaming.cpp - add handler
auto flush_handler = event_bus->register_handler<immer::box<events::FlushProducerEvent>>(
    [session_id, pipeline](const immer::box<events::FlushProducerEvent> &ev) {
      if (ev->lobby_id == session_id) {
        logs::log(logs::info, "[GSTREAMER] Flushing producer pipeline: {}", session_id);
        gst_element_send_event(pipeline.get(), gst_event_new_flush_start());
        gst_element_send_event(pipeline.get(), gst_event_new_flush_stop(true));
      }
    });
```

**Pros:**
- Standard GStreamer pattern
- No pipeline recreation needed
- Fast (instant flush)
- Designed for exactly this use case

**Cons:**
- Need to define new event type
- Need to ensure flush doesn't break ongoing streams
- Might affect Wayland capture behavior

---

## Hypothesis 3: Add Queue Element Before interpipesink ⭐⭐⭐⭐⭐
**Confidence: 95%**

**Approach:**
Insert a `queue` element with leaky behavior BEFORE interpipesink:

```cpp
auto pipeline = fmt::format(
    "waylanddisplaysrc ... ! "
    "{buffer_format}, width={width}, height={height}, framerate={fps}/1 ! "
    "{pipeline_fix}"
    "queue leaky=downstream max-size-buffers=1 ! "  // ← ADD THIS
    "interpipesink sync=true async=false name={session_id}_video",
    ...);
```

**How it works:**
- `queue` element DOES have `leaky=downstream` property
- Drops old buffers when queue full
- When no consumers, interpipesink blocks → queue fills → old buffer dropped
- interpipesink only receives fresh frames

**Why high confidence:**
- ✅ `queue` is standard GStreamer element with proven leaky support
- ✅ Pattern used elsewhere in Wolf (audio pipeline has it!)
- ✅ No new event types needed
- ✅ Minimal code change

**Cons:**
- Adds small latency (queue buffering)
- Slightly more complex pipeline

---

## Hypothesis 4: Switch interpipesrc to listen-to="" on Leave ⭐⭐
**Confidence: 60%**

**Approach:**
When leaving lobby, disconnect interpipesrc by switching to empty target:

```cpp
// In switch handler, on leave
if (switch_ev->interpipe_src_id == std::to_string(sess_id)) {
  // Switching back to own session = leaving lobby
  auto pipe_name = fmt::format("interpipesrc_{}_video", sess_id);
  if (auto src = gst_bin_get_by_name(...)) {
    g_object_set(src, "listen-to", "", nullptr);  // Disconnect!
    // On rejoin, set it back to lobby ID
  }
}
```

**Theory:**
- Disconnected interpipesrc might force interpipesink to discard buffer
- Reconnecting might establish fresh connection

**Pros:**
- Simple change
- No new events needed

**Cons:**
- Unclear if interpipesink actually discards on disconnect
- May not solve CUDA context issue
- Speculative

---

## Hypothesis 5: Restart JUST interpipesink Element ⭐⭐⭐
**Confidence: 70%**

**Approach:**
When lobby empties, set interpipesink element to NULL state and back to PLAYING:

```cpp
// When connected_sessions = 0
auto sink_name = fmt::format("{}_video", lobby.id);
if (auto sink_el = gst_bin_get_by_name(producer_pipeline, sink_name.c_str())) {
  gst_element_set_state(sink_el, GST_STATE_NULL);  // Stop it
  gst_element_set_state(sink_el, GST_STATE_PLAYING);  // Restart it
  gst_object_unref(sink_el);
}
```

**Theory:**
- Setting element to NULL clears its internal state
- Including buffered frames
- Restarting it fresh prevents stale buffers

**Pros:**
- Doesn't restart entire pipeline
- Fast operation
- Targets exact problematic element

**Cons:**
- Might not be safe while pipeline is PLAYING
- Could cause brief video glitch
- Unclear if NULL state clears interpipe buffers

---

## Hypothesis 6: Don't Auto-Leave Lobbies on Pause ⭐⭐⭐⭐
**Confidence: 80%**

**Approach:**
Lobbies should ONLY leave via explicit LeaveLobbyEvent, not automatic PauseStreamEvent:

```cpp
// lobbies.cpp line 273-276
handlers.push_back(app_state->event_bus->register_handler<immer::box<events::PauseStreamEvent>>(
  [=](const immer::box<events::PauseStreamEvent> &pause_stream_event) {
    // Check if session is in a lobby
    immer::vector<events::Lobby> lobbies = app_state->lobbies->load();
    bool in_lobby = false;
    for (const auto& lobby : lobbies) {
      auto sessions = lobby.connected_sessions->load();
      if (contains(sessions, std::to_string(pause_stream_event->session_id))) {
        in_lobby = true;
        logs::log(logs::info, "Ignoring pause - session in lobby");
        break;
      }
    }

    if (!in_lobby) {
      on_moonlight_session_over(pause_stream_event->session_id);
    }
  }));
```

**Theory:**
- Pause event shouldn't auto-leave lobbies
- Wolf-UI explicitly calls leave API when user clicks leave
- Auto-leave might be causing extra state transitions
- Preventing auto-leave might prevent stale buffer accumulation

**Pros:**
- Cleaner separation of concerns
- Lobbies managed explicitly via API, not implicitly via events
- May fix the underlying confusion

**Cons:**
- Need to verify Wolf-UI does call leave explicitly
- Might break other flows that depend on auto-leave

---

## Hypothesis 7: Use Different interpipe Mode ⭐⭐
**Confidence: 50%**

**Approach:**
Try interpipe `stream-sync` modes:

```cpp
// Current: stream-sync=restart-ts
// Try: stream-sync=compensate-ts
// or: stream-sync=none
```

**Theory:**
- Different sync modes handle disconnected sources differently
- Might auto-discard stale timestamps
- Could prevent buffer corruption

**Pros:**
- Simple config change
- No code restructuring

**Cons:**
- Don't fully understand what each mode does
- May affect A/V sync
- Speculative

---

## My Recommended Next Steps

**Highest Probability of Success:**

1. **Test Hypothesis 3** (queue with leaky) - 95% confidence
   - Proven GStreamer pattern
   - Audio already uses it
   - Simple, clean fix

2. **Test Hypothesis 2** (GStreamer flush events) - 90% confidence
   - Standard mechanism for this
   - Designed for buffer cleanup
   - Requires new event type

3. **Test Hypothesis 6** (don't auto-leave lobbies) - 80% confidence
   - May fix root cause confusion
   - Cleaner architecture
   - Need to verify Wolf-UI behavior

**What do you want to try first?**
