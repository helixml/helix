# Wolf Rejoin Hang - Deep Root Cause Analysis

## Stop and Think Differently

**We've been attacking the PRODUCER side (lobby pipeline).**
**But the error occurs on the CONSUMER side (client pipeline)!**

```
ERROR: Internal data stream error
Location: interpipesrc_342532221405053742_video
```

This is the **consumer's** interpipesrc, NOT the producer's interpipesink!

## The Real Question

**Why does the CONSUMER fail when reconnecting to a producer it was connected to before?**

### Theory 1: Consumer Pipeline Not Fully Recreated
**What if the consumer pipeline is REUSED instead of recreated on rejoin?**

Flow analysis:
1. Join lobby 1 → `start_streaming_video()` creates consumer pipeline
2. Leave lobby 1 → `PauseStreamEvent` → sends EOS
3. EOS should destroy the consumer pipeline
4. Rejoin lobby 1 → Should create NEW consumer pipeline

**But what if it doesn't?**

Check: Does `gst_element_send_event(pipeline.get(), gst_event_new_eos())` actually DESTROY the pipeline, or just pause it?

Looking at code: The pipeline is wrapped in `run_pipeline()` which probably handles cleanup. But what if the event handlers persist?

**Key insight**: The handlers are registered in the pipeline callback and return a list of handlers. If those handlers persist after EOS, they might be preventing proper cleanup!

### Theory 2: interpipesrc Caps Negotiation Fails on Reconnect
**What if interpipesrc can't re-negotiate caps with interpipesink?**

First connect:
- interpipesrc connects to interpipesink
- Negotiates caps (video format, resolution, etc.)
- Works fine

On rejoin:
- interpipesrc reconnects to SAME interpipesink
- interpipesink still has buffered frame with OLD caps
- New session has DIFFERENT caps (different resolution, bitrate, etc.)
- Caps mismatch → "Internal data stream error"

**Evidence**: Your native client uses 3024×1890, lobbies use 2560×1600!

### Theory 3: CUDA Context Isolation
**The consumer pipeline (client) has its OWN CUDA context separate from producers.**

Producer lobby pipelines:
- Each has waylanddisplaysrc → CUDA upload → interpipesink
- Each creates its own CUDA context for its resolution

Consumer pipeline (your Moonlight client):
- Has interpipesrc → CUDA download → encoder
- Creates CUDA context for CLIENT resolution (3024×1890)

**On first join**: Consumer CUDA context matches or adapts
**On rejoin**: Maybe consumer's CUDA context is stale/destroyed?

### Theory 4: The Queue Breaking Everything Tells Us Something
**Why did adding queue break even FIRST join?**

The queue must interfere with interpipe's connection negotiation!

interpipe works via:
- Direct element-to-element connection
- Caps negotiation between sink and src
- Buffer handoff protocol

Adding queue in between:
- Breaks direct connection
- Queue might not forward caps properly
- interpipesrc can't negotiate with interpipesink through the queue

**This means**: We CAN'T modify the producer pipeline structure at all!

## Outside-the-Box Solutions

### Solution A: Force Consumer Pipeline Recreation on Rejoin ⭐⭐⭐⭐⭐
**Stop trying to fix the producer - fix the consumer!**

When Wolf-UI rejoins a lobby it previously left:
1. **Detect** it's a rejoin (track visited lobbies)
2. **Force recreation** of consumer pipeline with new session ID
3. **Clean slate** - no stale CUDA, no stale caps

Implementation:
```cpp
// Track which lobbies each session has visited
static std::map<uint64_t, std::set<std::string>> visited_lobbies;

// On SwitchStreamProducerEvents
if (visited_lobbies[sess_id].contains(switch_ev->interpipe_src_id)) {
  // Rejoin detected!
  logs::log(logs::warning, "Rejoin detected, forcing consumer pipeline recreation");

  // Send EOS to current pipeline
  gst_element_send_event(pipeline.get(), gst_event_new_eos());

  // Wait for pipeline to fully stop
  gst_element_set_state(pipeline.get(), GST_STATE_NULL);

  // Recreate consumer pipeline fresh
  // (This requires refactoring start_streaming_video to be callable multiple times)
}

visited_lobbies[sess_id].insert(switch_ev->interpipe_src_id);
```

### Solution B: Flush Consumer interpipesrc on Switch
**Send flush events to the CONSUMER, not producer!**

```cpp
// In switch handler, BEFORE changing listen-to
auto pipe_name = fmt::format("interpipesrc_{}_video", sess_id);
if (auto src = gst_bin_get_by_name(GST_BIN(pipeline.get()), pipe_name.c_str())) {
  // Flush the interpipesrc to clear any buffered/cached state
  gst_element_send_event(src, gst_event_new_flush_start());
  gst_element_send_event(src, gst_event_new_flush_stop(true));

  // NOW switch
  g_object_set(src, "listen-to", video_interpipe.c_str(), nullptr);
}
```

This clears the CONSUMER's buffers/state before reconnecting!

### Solution C: Different interpipesrc for Each Connection
**Don't reuse the same interpipesrc - create new one for each lobby!**

Current: 1 consumer pipeline with 1 interpipesrc that switches between lobbies
Proposed: Create separate interpipesrc for each potential lobby

This is more complex but avoids the reconnection issue entirely.

### Solution D: Restart Lobby Producers When Empty (Nuclear Option)
**When lobby becomes empty, stop its producer completely:**

```cpp
// lobbies.cpp - when connected_sessions becomes 0
if (!lobby.stop_when_everyone_leaves && lobby.connected_sessions->load()->size() == 0) {
  logs::log(logs::info, "Lobby empty - stopping producer to prevent stale state");
  ev_bus->fire_event(immer::box<events::StopLobbyEvent>{
    events::StopLobbyEvent{.lobby_id = lobby.id}
  });

  // Mark lobby as "needs restart"
  lobby.producer_stopped = true;
}

// On join - check if needs restart and recreate producer
```

Downside: ~5 second delay to restart producer on rejoin

### Solution E: Make Helix Use stop_when_everyone_leaves=true
**Simplest Helix-side fix:**

```go
// wolf_executor.go line 258
StopWhenEveryoneLeaves: true,  // Changed from false
```

Pros:
- Empty lobbies auto-stop
- Fresh start on rejoin
- No stale buffers possible

Cons:
- Lose "persistent lobby" benefit
- Agent container stops when empty
- Restart delay on rejoin

## My REAL Recommendation

**Try Solution B first** (flush consumer interpipesrc) - 90% confidence

Why:
1. Targets the actual failing component (consumer)
2. Standard GStreamer flush mechanism
3. Minimal code change
4. No pipeline structure modification (unlike queue)
5. Doesn't break producer/consumer isolation

This is the one fix we HAVEN'T tried yet that actually makes sense!

Should I implement Solution B?
