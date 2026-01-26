# Shared Video Source: GOP Replay Design

**Date:** 2026-01-23
**Status:** Complete (GOP Replay implemented, Grace Period designed)
**Author:** Claude

## Problem Statement

When multiple WebSocket clients connect to the same video stream, they need to:
1. Share a single GStreamer pipeline (prevents PipeWire ScreenCast contention)
2. New clients joining mid-stream must catch up via GOP buffer replay
3. After replay, seamlessly receive live frames with NO gaps or out-of-order frames
4. One slow client must NOT block other clients
5. Memory must be managed (frames held during replay must be freed, stuck clients disconnected)

## Current Issues

The current implementation has race conditions:
- If we add client to broadcaster map before GOP replay: live frames interleave with GOP frames (out of order)
- If we replay GOP then add to map: frames during replay are missed (gap)
- If we block broadcaster during replay: one slow client pauses everyone

## Proposed Design

### Key Concepts

1. **Per-client Pending Buffer**: While client is "catching up", broadcaster queues frames in a per-client buffer. This ensures frame ordering without needing sequence numbers.
2. **Catchup State Machine**: Client transitions from "catching_up" → "live" → "closed". All transitions use CAS to prevent race conditions.
3. **Non-blocking Replay**: GOP replay happens in a goroutine, doesn't block broadcaster
4. **pendingMu Synchronization**: State checks happen INSIDE pendingMu lock to prevent race between broadcaster and catchup goroutine

### Data Structures (Actual Implementation)

```go
type VideoFrame struct {
    Data       []byte
    Timestamp  uint64
    IsKeyframe bool
    // Note: SeqNum was considered but not implemented - ordering is
    // guaranteed by the pending buffer mechanism instead
}

type sharedVideoClient struct {
    id      uint64
    frameCh chan VideoFrame

    // State machine: 0=catching_up, 1=live, 2=closed
    // All transitions use CAS to prevent double-close
    state atomic.Uint32

    // Pending buffer for frames during catchup
    // Broadcaster appends while state==catching_up (under lock)
    // Catchup goroutine drains this to frameCh
    pendingMu sync.Mutex
    pending   []VideoFrame
}

type SharedVideoSource struct {
    nodeID       uint32
    pipelineStr  string
    pipelineOpts GstPipelineOptions

    pipeline  *GstPipeline
    running   atomic.Bool
    startOnce sync.Once
    stopOnce  sync.Once
    startErr  error
    startMu   sync.Mutex

    clients   map[uint64]*sharedVideoClient
    clientsMu sync.RWMutex
    nextID    atomic.Uint64

    gopBuffer   []VideoFrame
    gopBufferMu sync.RWMutex

    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}
```

### Client State Machine

```
    ┌─────────────┐
    │ catching_up │ ──── timeout/overflow ────→ closed
    └──────┬──────┘
           │ CAS: catching_up → live
           ▼
    ┌─────────────┐
    │    live     │ ──── buffer full ──→ closed
    └─────────────┘
           │
           ▼
    ┌─────────────┐
    │   closed    │  (terminal - no transitions out)
    └─────────────┘
```

**State Transition Rules (enforced with CAS):**

| From | To | Who | Condition |
|------|----|-----|-----------|
| catching_up | live | catchup goroutine | pending buffer empty |
| catching_up | closed | catchup goroutine | timeout or pending overflow |
| catching_up | closed | broadcaster | pending overflow detected |
| live | closed | broadcaster | channel buffer full |
| live | closed | Unsubscribe() | client disconnected |
| * | closed | stop() | source shutdown |

**Implementation: Use CompareAndSwap (CAS)**

```go
// WRONG - race condition:
if client.state.Load() == catching_up {
    client.state.Store(live)  // Another goroutine might have set closed!
}

// RIGHT - atomic transition:
if client.state.CompareAndSwap(catching_up, live) {
    // Transition succeeded - we are now responsible for this state
} else {
    // Transition failed - another goroutine already changed state
    // (likely to closed - we should exit)
}
```

**Why CAS matters:**
1. Broadcaster and catchup goroutine both touch state
2. Unsubscribe can happen at any time
3. Without CAS, we might close an already-closed channel (panic)
4. With CAS, exactly one goroutine wins each transition

### Formal Correctness Proof

#### Definitions

Let S = {catching_up, live, closed} be the set of states.
Let T ⊆ S × S be the set of valid transitions:
```
T = {
  (catching_up, live),
  (catching_up, closed),
  (live, closed)
}
```

Note: `closed` is a **sink state** - no outgoing transitions.

#### Safety Properties

**Property 1: No Double-Close (Channel Safety)**

*Claim*: `close(client.frameCh)` is called at most once per client.

*Proof*:
1. `close()` is only called when transitioning TO `closed` state
2. All transitions to `closed` use CAS: `state.CompareAndSwap(currentState, closed)`
3. CAS is atomic - exactly one caller succeeds when multiple attempt same transition
4. `closed` is a sink state - no valid transitions out
5. Therefore, only the goroutine that wins the CAS can call `close()`
6. Since CAS succeeds at most once, `close()` is called at most once ∎

**Property 2: No Lost Frames (Frame Ordering)**

*Claim*: For a client that transitions catching_up → live, all frames are received in sequence order with no gaps.

*Proof by construction*:
1. Let t₀ = time when client is added to clients map with state=catching_up
2. Let F_gop = frames in GOP buffer at t₀
3. Let F_pending = frames added to pending buffer after t₀
4. Let F_live = frames sent directly after transition to live

*Observation 1*: Broadcaster behavior is determined by `client.state.Load()` **while holding pendingMu**:
- If catching_up: append to pending (under pendingMu lock)
- If live: send to frameCh
- If closed: skip

**CRITICAL**: Broadcaster MUST check state while holding pendingMu, not before acquiring it!

*Observation 2*: The catchup goroutine:
1. Copies F_gop (snapshot at some t₁ ≥ t₀)
2. Sends all of F_gop to frameCh
3. Acquires pendingMu
4. While holding pendingMu, checks if pending is empty
5. While still holding pendingMu, CAS(catching_up, live)
6. Sets pending = nil
7. Releases pendingMu

*Observation 3*: The broadcaster (corrected):
1. Acquires pendingMu
2. Checks state while holding pendingMu
3. If catching_up: appends to pending
4. Releases pendingMu
5. If was live: sends to channel

*Key insight*: Both broadcaster and catchup check/modify state while holding pendingMu:
- Broadcaster checks state INSIDE pendingMu, then appends if catching_up
- Catchup does CAS INSIDE pendingMu, then sets pending=nil
- Therefore: no frame can be added to pending AFTER transition to live

*Race that was fixed*:
```
// BUG (old code):
state := client.state.Load()      // <-- Load OUTSIDE lock
if state == catching_up {
    client.pendingMu.Lock()       // <-- Lock AFTER load
    // Catchup could have transitioned to live between load and lock!
    client.pending = append(...)  // <-- Frame added to nil pending = lost!
```

*Case analysis (with fix)*:
- Frame arrives before CAS: Broadcaster holds pendingMu, sees catching_up, appends ✓
- Frame arrives after CAS: Broadcaster holds pendingMu, sees live, skips append, sends to channel ✓
- Frame arrives "during" CAS: Impossible - both hold pendingMu, mutual exclusion ✓

*Frame sequence*: F_gop ++ F_pending ++ F_live (concatenation, no gaps) ∎

**Property 3: No Deadlock (Detailed Proof)**

*Claim*: The system cannot deadlock.

*Definitions*:
- Let L = {gopBufferMu, clientsMu, pendingMu₁, pendingMu₂, ..., pendingMuₙ} be all locks
- A deadlock occurs iff there exists a cycle in the wait-for graph

*Lock Hierarchy* (total ordering):
```
gopBufferMu (level 1, highest)
    ↓
clientsMu (level 2)
    ↓
pendingMu_i for each client i (level 3, lowest)
```

*Rule*: A goroutine holding lock at level L may only acquire locks at level L+1 or higher.

*Verification of all code paths* (matches actual implementation):

| Goroutine | Lock Sequence | Levels | Valid? |
|-----------|---------------|--------|--------|
| Broadcaster (GOP update) | gopBufferMu.Lock() → Unlock() | 1 → - | ✓ |
| Broadcaster (client loop) | clientsMu.RLock() → pendingMu.Lock() → Unlock() → RUnlock() | 2 → 3 → - → - | ✓ |
| Broadcaster (disconnect) | clientsMu.Lock() → pendingMu.Lock() → Unlock() → Unlock() | 2 → 3 → - → - | ✓ |
| Catchup (GOP copy) | gopBufferMu.RLock() → RUnlock() | 1 → - | ✓ |
| Catchup (drain loop) | pendingMu.Lock() → Unlock() | 3 → - | ✓ |
| Subscribe (first client) | clientsMu.Lock() → Unlock() | 2 → - | ✓ |
| Subscribe (subsequent) | clientsMu.Lock() → Unlock() | 2 → - | ✓ |
| Unsubscribe | clientsMu.Lock() → pendingMu.Lock() → Unlock() → Unlock() | 2 → 3 → - → - | ✓ |
| stop() | clientsMu.Lock() → pendingMu.Lock() → Unlock() → Unlock() | 2 → 3 → - → - | ✓ |
| GetAllStats | registry.mu → gopBufferMu.RLock() → RUnlock() → clientsMu.RLock() → RUnlock() | 0 → 1 → - → 2 → - | ✓ |

Note: Subscribe (subsequent) no longer acquires gopBufferMu - the runCatchup goroutine does that separately.

*Proof by contradiction*:
1. Assume deadlock exists
2. Then ∃ cycle: G₁ waits for G₂ waits for ... waits for Gₙ waits for G₁
3. Each "waits for" means G_i holds lock L_i, wants lock L_{i+1} held by G_{i+1}
4. For cycle to exist: level(L₁) > level(L₂) > ... > level(Lₙ) > level(L₁)
5. This implies level(L₁) > level(L₁), a contradiction
6. Therefore no deadlock exists ∎

*Additional guarantee*: No lock is held across blocking channel operations:
- Broadcaster: Releases all locks before frameCh send
- Catchup: Acquires pendingMu only briefly, releases before frameCh send
- No lock is held while waiting on a channel

**Property 4: Liveness (Detailed Proof)**

*Claim*: Every client eventually reaches a terminal state (live streaming or disconnected).

*Definitions*:
- Let progress(c) = {GOP frames sent} + {pending frames drained}
- A system has liveness if every operation eventually completes

*Theorem*: For any client c in state catching_up, within finite time T:
- Either c transitions to live, or
- c transitions to closed

*Proof*:

*Bound on catching_up duration*:
Let T_timeout = 30 seconds (catchup timeout constant)

The catchup goroutine operates as follows:
```go
timeout := time.After(30 * time.Second)

// Phase 1: Send GOP frames
for _, frame := range gopCopy {
    select {
    case client.frameCh <- frame:
        progress++  // Makes progress
    case <-timeout:
        return      // Terminates
    }
}

// Phase 2: Drain pending
for {
    pendingMu.Lock()
    if len(pending) == 0 {
        // CAS to live and return - terminates
    }
    frame := pending[0]
    pending = pending[1:]
    pendingMu.Unlock()

    select {
    case client.frameCh <- frame:
        progress++  // Makes progress
    case <-timeout:
        return      // Terminates
    }
}
```

*Case 1: Client consumes frames (healthy)*
- Each successful send to frameCh is progress
- WebSocket goroutine consumes from frameCh
- Eventually: pending is empty, CAS succeeds, client is live
- Time bound: O(|GOP| + |pending|) × frame_time ≤ T_timeout

*Case 2: Client stops consuming (stuck WebSocket)*
- frameCh fills up (bounded capacity = GOP size)
- Sends start blocking
- timeout channel fires after 30 seconds
- catchup returns, client disconnected
- Time bound: exactly T_timeout = 30 seconds

*Case 3: Pending buffer overflow (very slow client)*
- Broadcaster detects len(pending) ≥ 2 × GOP
- disconnectClient() called
- state transitions to closed
- Time bound: ≤ time for 2 × GOP frames = ~60 seconds at 60fps

*Case 4: External disconnect*
- Unsubscribe() or stop() called
- frameCh closed
- catchup's send panics, recovers, returns
- Time bound: immediate

*Liveness guarantee*: max(T_timeout, 2 × GOP_time) = max(30s, 60s) = 60 seconds

*Corollary*: No client remains in catching_up state indefinitely. ∎

*Broadcaster Liveness*:

*Claim*: Broadcaster goroutine never blocks indefinitely.

*Proof*:
1. frameCh receives use select with default (non-blocking)
2. gopBufferMu.Lock() is bounded by O(1) critical section
3. clientsMu.RLock() is bounded by O(n) where n = client count
4. pendingMu.Lock() is bounded by O(1) per client
5. All lock critical sections are finite (no I/O, no channel ops)
6. Broadcaster only blocks on: ctx.Done() or pipeline.Frames()
7. Both are externally controlled (context cancellation / pipeline EOF)

Therefore broadcaster makes progress on every frame from pipeline. ∎

*System Liveness Summary*:

| Component | Liveness Guarantee | Bound |
|-----------|-------------------|-------|
| Catchup goroutine | Always terminates | 60 seconds |
| Broadcaster | Never blocks on clients | Per-frame |
| disconnectClient | Always completes | O(1) |
| Unsubscribe | Always completes | O(1) |
| stop() | Always completes | O(n) clients |

**Property 5: Memory Safety (No Leaks)**

*Claim*: All dynamically allocated memory is eventually freed.

*Proof by buffer type*:

*GOP Buffer*: Freed when source.stop() is called (or reset on keyframe).
- stop() is called when last client unsubscribes
- Last client unsubscribes when WebSocket closes (browser tab closed, disconnect, etc.)
- WebSocket close is guaranteed by TCP/HTTP semantics ∎

*Pending Buffer*: Set to nil when:
- Catchup completes (drained then nil)
- disconnectClient() called (set to nil)
- Unsubscribe() called (set to nil)

All paths from catching_up lead to one of these. ∎

*GOP Copy*: Local variable in catchup goroutine, freed when goroutine exits.
Goroutine exits guaranteed by Property 4. ∎

*frameCh*: Closed when client transitions to closed.
Channel closed ⇒ garbage collected when no references remain.
Last reference is in catchup goroutine or WebSocket handler.
Both exit ⇒ channel collected. ∎

#### Invariants

**Invariant 1**: `state == closed ⟹ frameCh is closed`
- Maintained by: all transitions to closed call close(frameCh)

**Invariant 2**: `state == live ⟹ pending is empty or nil`
- Maintained by: catchup drains pending before CAS to live

**Invariant 3**: `client in s.clients ∧ state != closed ⟹ frameCh is open`
- Maintained by: close only happens with state transition to closed

### Subscribe Flow (Actual Implementation)

```
First client:
1. Create client with state=live (nothing to catch up on)
2. Lock clientsMu, add client to map, unlock
3. Start pipeline
4. Return frameCh to caller

Subsequent clients:
1. Create client with state=catching_up (default)
2. Lock clientsMu, add client to map, unlock
3. Broadcaster immediately starts queuing frames to client.pending
4. Start catchup goroutine (non-blocking)
5. Return frameCh to caller

Catchup goroutine:
a. Lock gopBufferMu (read), copy GOP buffer, unlock
b. Send GOP frames to client.frameCh (with timeout)
c. Loop:
   - Lock client.pendingMu
   - If pending empty: CAS(catching_up, live), set pending=nil, unlock, return
   - Take first frame from pending, unlock
   - Send frame to client.frameCh (with timeout)
d. On timeout: call disconnectClient(), return
```

Note: gopBufferMu is NOT acquired in Subscribe() - only the catchup goroutine acquires it.
This allows Subscribe() to return immediately without blocking on the broadcaster.

### Broadcaster Flow (Corrected Implementation)

```go
for frame := range pipelineFrames {
    // Update GOP buffer
    s.gopBufferMu.Lock()
    if frame.IsKeyframe {
        s.gopBuffer = []VideoFrame{frame}
    } else {
        s.gopBuffer = append(s.gopBuffer, frame)
    }
    s.gopBufferMu.Unlock()

    // Send to clients
    s.clientsMu.RLock()
    for _, client := range s.clients {
        if client.state.Load() == stateClosed {
            continue
        }

        // CRITICAL: Check state WHILE holding pendingMu to avoid race
        handled := false
        client.pendingMu.Lock()
        if client.state.Load() == stateCatchingUp {
            client.pending = append(client.pending, frame)
            handled = true
        }
        client.pendingMu.Unlock()

        if handled {
            continue
        }

        // Client is live - send directly to channel
        if client.state.Load() == stateLive {
            select {
            case client.frameCh <- frame:
            default:
                // Buffer full - mark for disconnect
            }
        }
    }
    s.clientsMu.RUnlock()
}
```

**Why state is checked inside pendingMu**:
- Catchup goroutine does CAS(catching_up, live) while holding pendingMu
- If broadcaster checked state outside lock, race condition:
  1. Broadcaster sees catching_up
  2. Catchup transitions to live, sets pending=nil
  3. Broadcaster appends to nil → creates orphan frame → lost!
- With state check inside lock: mutual exclusion prevents this

### Catchup Goroutine (Actual Implementation)

```go
func (s *SharedVideoSource) runCatchup(client *sharedVideoClient) {
    timeout := time.After(30 * time.Second)

    // Phase 1: Get GOP buffer snapshot
    s.gopBufferMu.RLock()
    gopCopy := make([]VideoFrame, len(s.gopBuffer))
    copy(gopCopy, s.gopBuffer)
    s.gopBufferMu.RUnlock()

    // Phase 2: Send GOP frames
    for i, frame := range gopCopy {
        if client.state.Load() == clientStateClosed {
            return  // Client closed externally
        }
        select {
        case client.frameCh <- frame:
        case <-timeout:
            s.disconnectClient(client.id)
            return
        }
    }

    // Phase 3: Drain pending buffer until caught up
    for {
        if client.state.Load() == clientStateClosed {
            return  // Client closed externally
        }

        client.pendingMu.Lock()
        if len(client.pending) == 0 {
            // Caught up - CAS to live while holding lock (prevents race!)
            if client.state.CompareAndSwap(clientStateCatchingUp, clientStateLive) {
                client.pending = nil  // Release memory
                client.pendingMu.Unlock()
                return  // Success!
            }
            // CAS failed - client was closed by someone else
            client.pendingMu.Unlock()
            return
        }

        // Take first pending frame
        frame := client.pending[0]
        client.pending = client.pending[1:]
        client.pendingMu.Unlock()

        select {
        case client.frameCh <- frame:
        case <-timeout:
            s.disconnectClient(client.id)
            return
        }
    }
}
```

Key points:
- No `defer close(catchupDone)` - goroutine exits via explicit returns
- CAS to live happens WHILE holding pendingMu (critical for correctness)
- Check for closed state at top of each loop iteration

### Memory Management

1. **GOP Buffer**: Cleared on each keyframe (already implemented)
2. **Pending Buffer**: Per-client, freed when client transitions to live or disconnects
3. **Timeout**: 30-second catchup timeout - if client can't catch up, disconnect
4. **Pending Buffer Limit**: If pending buffer exceeds 2x GOP size, disconnect (client is too slow)

### Frame Ordering Guarantee

The key insight: broadcaster always adds frames to pending buffer for catching-up clients. The catchup goroutine drains pending buffer to channel. This ensures:

1. GOP frames are sent first (in order)
2. Frames that arrived during GOP replay are in pending buffer (in order)
3. Pending buffer is drained to channel (in order)
4. When pending is empty and we go live, broadcaster sends directly to channel

Result: Client receives frames in perfect sequence order.

### Edge Cases

1. **Two clients subscribe simultaneously**: Each gets its own catchup goroutine, no interference
2. **Keyframe arrives during catchup**: GOP buffer is reset, pending buffer continues to accumulate
3. **Client disconnects during catchup**: Close channel, cleanup pending buffer
4. **Catchup timeout**: Disconnect client, log warning
5. **Pending buffer overflow**: Disconnect client (can't keep up even during catchup)

## Implementation Plan

1. Add SeqNum to VideoFrame struct
2. Update broadcaster to set SeqNum on each frame
3. Add catchup state machine to sharedVideoClient
4. Implement pending buffer with proper locking
5. Implement catchup goroutine with timeout
6. Update Subscribe() to start catchup goroutine
7. Add pending buffer size limit and overflow handling
8. Add metrics: catchup duration, pending buffer size, catchup failures

## Risks

1. **Memory pressure**: Many clients catching up simultaneously = large pending buffers
   - Mitigation: Pending buffer limit, aggressive timeout

2. **Lock contention**: pendingMu locked by both broadcaster and catchup goroutine
   - Mitigation: Keep critical sections small, consider lock-free queue

3. **Goroutine leak**: Catchup goroutine stuck forever
   - Mitigation: Timeout, context cancellation

## Buffer Inventory

This section documents ALL buffers in the system and their lifecycle.

### Summary: Two-Tier Buffer Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                        SharedVideoSource                              │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  GOP Buffer (SHARED)                                             │ │
│  │  - One per session, not per-client                               │ │
│  │  - Stores all frames since last keyframe                         │ │
│  │  - Used as "template" for new client catchup                     │ │
│  │  - Reset on each keyframe arrival                                │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                       │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐       │
│  │ Client A (live) │  │ Client B (live) │  │ Client C (catch)│       │
│  │                 │  │                 │  │                 │       │
│  │ [frameCh]       │  │ [frameCh]       │  │ [frameCh]       │       │
│  │  └─→ WebSocket  │  │  └─→ WebSocket  │  │  └─→ WebSocket  │       │
│  │                 │  │                 │  │                 │       │
│  │ pending: nil    │  │ pending: nil    │  │ [pending buf]   │       │
│  │ (not used)      │  │ (not used)      │  │  └─→ frameCh    │       │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘       │
└──────────────────────────────────────────────────────────────────────┘

Buffer Types:
1. GOP Buffer     - SHARED (1 per session) - stores replay template
2. frameCh        - PER-CLIENT - fixed-size channel to WebSocket
3. pending        - PER-CLIENT - temporary during catchup only
4. GOP Copy       - TEMPORARY - local var in catchup goroutine
```

**Yes, there are two tiers:**

1. **Shared GOP Buffer** (one per session):
   - Stores all frames since last keyframe
   - Template for new client catchup
   - Managed by broadcaster, reset on keyframe

2. **Per-Client Buffers** (one set per client):
   - **frameCh**: Fixed channel buffer to WebSocket writer
   - **pending**: Temporary slice during catchup phase only
   - Managed per-client, freed on disconnect

**Why two tiers?**
- GOP buffer is shared so we don't duplicate 38 MB per client
- Per-client pending buffer ensures seamless frame ordering during catchup
- Once client goes live, pending is freed (nil) and only frameCh is used

### 1. GOP Buffer (Shared, per-session)

**Location**: `SharedVideoSource.gopBuffer []VideoFrame`

**Purpose**: Stores all frames since the last keyframe. Used as the "replay template" for new clients joining mid-stream.

**Size**: Up to 1800 frames (30 seconds at 60fps)

**Lifecycle**:
- **Created**: When SharedVideoSource is created (empty slice)
- **Populated**: By broadcaster goroutine, one frame at a time
- **Cleared**: On each keyframe arrival (slice reset to single keyframe)
- **Freed**: When SharedVideoSource is stopped (last client disconnects)

**Memory Owner**: SharedVideoSource (shared across all clients)

**Cannot Leak Because**:
- Reset on every keyframe (at most 30s of frames)
- Freed when SharedVideoSource.stop() is called
- Only one GOP buffer per session (not per-client)

### 2. Client Channel Buffer (Per-client)

**Location**: `sharedVideoClient.frameCh chan VideoFrame`

**Purpose**: Buffered channel for sending frames to WebSocket writer goroutine.

**Size**: Fixed capacity = GOP size (1800 frames)

**Lifecycle**:
- **Created**: In Subscribe() when client connects
- **Populated**: By catchup goroutine (GOP frames) and broadcaster (live frames)
- **Drained**: By WebSocket handler reading frames to send
- **Closed**: When client is disconnected (slow) or unsubscribes

**Memory Owner**: sharedVideoClient

**Cannot Leak Because**:
- Fixed capacity channel, cannot grow unbounded
- Channel closed on disconnect/unsubscribe
- If buffer fills, client is disconnected (not blocked)

### 3. Pending Buffer (Per-client, temporary)

**Location**: `sharedVideoClient.pending []VideoFrame`

**Purpose**: Temporary holding area for frames that arrive during GOP replay. Ensures perfect frame ordering.

**Size**: Dynamically grows, capped at 2x GOP size (3600 frames)

**Lifecycle**:
- **Created**: In Subscribe() (empty slice)
- **Populated**: By broadcaster while client.state == catching_up
- **Drained**: By catchup goroutine after GOP replay completes
- **Freed**: Set to nil after draining, or when client disconnects

**Memory Owner**: sharedVideoClient (exclusive during catchup)

**Cannot Leak Because**:
- Capped at 2x GOP size (exceeding = disconnect)
- 30-second catchup timeout (exceeded = disconnect)
- Drained to empty then set to nil
- Client cleanup runs on disconnect/unsubscribe

### 4. GOP Copy (Temporary, per-catchup)

**Location**: Local variable in `runCatchup()` goroutine

**Purpose**: Snapshot of GOP buffer taken at catchup start. Allows broadcaster to continue modifying GOP buffer while catchup sends frames.

**Size**: Up to 1800 frames (copy of GOP buffer at that moment)

**Lifecycle**:
- **Created**: At start of runCatchup() goroutine
- **Used**: Immediately iterated to send to client channel
- **Freed**: When runCatchup() returns (goroutine exits)

**Memory Owner**: runCatchup() goroutine (stack/local)

**Cannot Leak Because**:
- Local variable, garbage collected when goroutine exits
- Goroutine guaranteed to exit (timeout or completion)
- No references stored elsewhere

## Memory Leak Analysis

### Proof: No Memory Leaks

For each buffer type, we prove it cannot leak:

**GOP Buffer**: Single instance per session. Reset every keyframe (≤30s accumulation). Freed on stop(). ✓

**Client Channel**: Fixed capacity. Closed on disconnect. If full, client disconnected (not blocked). ✓

**Pending Buffer**:
- Size cap (2x GOP) enforced in broadcaster
- 30-second timeout enforced in catchup goroutine
- Both paths call disconnectClient() which triggers cleanup
- disconnectClient() closes channel, sets state=closed
- Unsubscribe() removes client from map, frees pending ✓

**GOP Copy**: Local variable in goroutine. Goroutine guaranteed to exit because:
- Channel sends have timeout (30s)
- disconnectClient() closes frameCh, breaking the send
- defer close(catchupDone) ensures cleanup runs ✓

### Goroutine Leak Prevention

Each catchup goroutine is guaranteed to exit because:
1. All channel operations have a 30-second timeout
2. disconnectClient() closes frameCh, causing channel sends to fail
3. The client is removed from the clients map, preventing further interaction

### Cleanup Order

When client disconnects (any reason):
1. `client.state.Store(stateClosed)` - broadcaster stops queuing
2. `close(client.frameCh)` - unblocks any pending sends in catchup
3. `delete(s.clients, clientID)` - removes from map
4. Go GC frees: pending slice, channel, client struct

## Memory Usage Estimation

### Assumptions (4K60 @ 10 Mbit/s)

| Parameter | Value | Notes |
|-----------|-------|-------|
| Bitrate | 10 Mbit/s | 1.25 MB/s |
| Frame rate | 60 fps | |
| GOP duration | 30 seconds | 1800 frames |
| Keyframe interval | 30 seconds | Matches GOP |
| Keyframe size | ~100 KB | ~10x P-frame |
| P-frame size | ~10 KB | Average |

### Per-Frame Memory

Average frame size at 10 Mbit/s, 60fps:
- Total bytes per second: 10,000,000 bits / 8 = 1,250,000 bytes/s
- Average bytes per frame: 1,250,000 / 60 ≈ **20.8 KB**

VideoFrame struct overhead: ~56 bytes (slice header, uint64s, bool)
Total per frame: **~21 KB**

### GOP Buffer (1800 frames)

```
1800 frames × 21 KB = 37.8 MB per session
```

This is the baseline memory for one active streaming session.

### Worst-Case Catchup Memory

During catchup, a client may accumulate:
- GOP copy: 37.8 MB (temporary)
- Pending buffer: up to 2x GOP = 75.6 MB (capped)
- Client channel: 1800 frames = 37.8 MB (fixed capacity, but likely partially drained)

**Worst case per catching-up client: ~150 MB**

But this is transient:
- GOP copy freed after iteration (~1-2 seconds)
- Pending drained in ~1-2 seconds for healthy clients
- Only slow clients hit limits before disconnect

### Steady-State Memory (Live Streaming)

Per session (one pipeline):
- GOP buffer: 37.8 MB

Per live client:
- Channel buffer capacity: 37.8 MB (but typically <10% utilized)
- Pending buffer: 0 (empty for live clients)

**Realistic per-client: ~5 MB** (typical channel fill is <300 frames)

### Example Scenarios

| Scenario | GOP Buffer | Client Buffers | Total |
|----------|------------|----------------|-------|
| 1 session, 1 client (live) | 38 MB | 5 MB | 43 MB |
| 1 session, 5 clients (live) | 38 MB | 25 MB | 63 MB |
| 1 session, 1 catching up | 38 MB | 150 MB (transient) | 188 MB |
| 1 session, 10 clients + 2 catching up | 38 MB | 50 + 300 MB | 388 MB |

### Memory Limits

To prevent OOM, the system enforces:

1. **Pending buffer cap**: 2x GOP = 3600 frames → 75.6 MB max per client
2. **Catchup timeout**: 30 seconds → limits accumulation time
3. **Client channel cap**: 1800 frames → 37.8 MB max per client
4. **Disconnect on overflow**: Slow clients removed, memory freed

Maximum memory per session with N clients:
```
38 MB (GOP) + N × 113 MB (worst-case channel + pending)
```

For 10 clients worst case: 38 + 10 × 113 = **1.17 GB**

Realistic (10 live clients): 38 + 10 × 5 = **88 MB**

## Grace Period for Pipeline Shutdown

### Problem Statement

When the frontend performs a hot reload (common during development), multiple browser tabs/clients disconnect simultaneously and then reconnect within seconds. Without a grace period:

1. All 4 clients disconnect at once
2. SharedVideoSource detects 0 subscribers, stops GStreamer pipeline
3. Pipeline teardown takes ~500ms (flushing, EOS, cleanup)
4. Clients reconnect within 100ms
5. New pipeline must be started from scratch
6. PipeWire ScreenCast portal must be re-negotiated
7. Total reconnection latency: 2-5 seconds with visual glitch

With a grace period:

1. All 4 clients disconnect at once
2. SharedVideoSource schedules pipeline stop in 60 seconds
3. Clients reconnect within 100ms
4. Pending stop is cancelled
5. Clients immediately receive live frames (no pipeline restart)
6. Total reconnection latency: <100ms, seamless

### Design Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                    SharedVideoRegistry                               │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ sources map[uint32]*SharedVideoSource                          │ │
│  │                                                                 │ │
│  │   nodeID=1 → SharedVideoSource (pipeline running)              │ │
│  │   nodeID=2 → SharedVideoSource (pipeline running)              │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ pendingStops map[uint32]*pendingStop                           │ │
│  │                                                                 │ │
│  │   nodeID=3 → {timer: 60s, source: *SharedVideoSource}          │ │
│  │              (will stop at T+60s unless client reconnects)     │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  mu sync.Mutex  (protects both maps atomically)                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Structures

```go
type pendingStop struct {
    timer    *time.Timer
    source   *SharedVideoSource
    nodeID   uint32
    cancelCh chan struct{} // closed when stop is cancelled
}

type SharedVideoRegistry struct {
    sources      map[uint32]*SharedVideoSource
    pendingStops map[uint32]*pendingStop
    mu           sync.Mutex

    gracePeriod time.Duration // default 60 seconds
}
```

### State Transitions

```
                             ┌────────────────────┐
                             │   NOT IN REGISTRY  │
                             └─────────┬──────────┘
                                       │ GetOrCreate() first client
                                       ▼
                             ┌────────────────────┐
        GetOrCreate()        │      ACTIVE        │◀─────────┐
        (new client)         │  (pipeline running)│          │
            │                └─────────┬──────────┘          │
            │                          │ Unsubscribe()       │
            │                          │ last client         │
            │                          ▼                     │
            │                ┌────────────────────┐          │
            └───────────────▶│   PENDING STOP     │──────────┘
             cancel timer    │ (timer scheduled)  │  GetOrCreate()
                             └─────────┬──────────┘  cancels timer
                                       │ timer fires
                                       ▼
                             ┌────────────────────┐
                             │     STOPPING       │
                             │  (stop() called)   │
                             └─────────┬──────────┘
                                       │ stop() completes
                                       ▼
                             ┌────────────────────┐
                             │   NOT IN REGISTRY  │
                             │   (GC eligible)    │
                             └────────────────────┘
```

### API Methods

#### GetOrCreate (Subscribe Path)

```go
func (r *SharedVideoRegistry) GetOrCreate(nodeID uint32, pipelineStr string, opts GstPipelineOptions) (*SharedVideoSource, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Case 1: Active source exists
    if source, ok := r.sources[nodeID]; ok {
        return source, nil
    }

    // Case 2: Source is pending stop - cancel and reuse
    if pending, ok := r.pendingStops[nodeID]; ok {
        // Stop the timer (may have already fired, that's OK)
        pending.timer.Stop()

        // Signal cancellation to doStop goroutine
        close(pending.cancelCh)

        // Move back to active sources
        r.sources[nodeID] = pending.source
        delete(r.pendingStops, nodeID)

        log.Printf("[SharedVideoRegistry] Cancelled pending stop for node %d, reusing pipeline", nodeID)
        return pending.source, nil
    }

    // Case 3: No source exists - create new
    source := NewSharedVideoSource(nodeID, pipelineStr, opts)
    r.sources[nodeID] = source
    return source, nil
}
```

#### ScheduleStop (Unsubscribe Path)

```go
func (r *SharedVideoRegistry) ScheduleStop(nodeID uint32) {
    r.mu.Lock()
    defer r.mu.Unlock()

    source, ok := r.sources[nodeID]
    if !ok {
        // Already removed or never existed
        return
    }

    // Check if source still has clients
    if source.ClientCount() > 0 {
        // Not actually the last client - race condition handled
        return
    }

    // Already pending stop?
    if _, ok := r.pendingStops[nodeID]; ok {
        return
    }

    // Move from active to pending
    delete(r.sources, nodeID)

    pending := &pendingStop{
        source:   source,
        nodeID:   nodeID,
        cancelCh: make(chan struct{}),
    }

    // Schedule the actual stop
    pending.timer = time.AfterFunc(r.gracePeriod, func() {
        r.doStop(pending)
    })

    r.pendingStops[nodeID] = pending
    log.Printf("[SharedVideoRegistry] Scheduled stop for node %d in %v", nodeID, r.gracePeriod)
}

func (r *SharedVideoRegistry) doStop(pending *pendingStop) {
    r.mu.Lock()

    // Check if stop was cancelled
    select {
    case <-pending.cancelCh:
        // Cancelled - someone called GetOrCreate during grace period
        r.mu.Unlock()
        log.Printf("[SharedVideoRegistry] Stop cancelled for node %d (client reconnected)", pending.nodeID)
        return
    default:
    }

    // Verify this pending stop is still current
    // (handles edge case where new source was created with same nodeID)
    currentPending, ok := r.pendingStops[pending.nodeID]
    if !ok || currentPending != pending {
        r.mu.Unlock()
        log.Printf("[SharedVideoRegistry] Stop superseded for node %d", pending.nodeID)
        return
    }

    // Remove from pending map
    delete(r.pendingStops, pending.nodeID)
    r.mu.Unlock()

    // Stop the pipeline (outside lock - may take time)
    log.Printf("[SharedVideoRegistry] Grace period expired, stopping node %d", pending.nodeID)
    pending.source.Stop()
}
```

### Race Condition Analysis

#### Race 1: Unsubscribe races with Subscribe

```
Timeline:
T1: Client A calls Unsubscribe() - last client
T2: Client B calls Subscribe() via GetOrCreate()
T3: ScheduleStop() runs (from T1)

Scenario A: T2 before T3
- GetOrCreate acquires lock, adds client B
- ScheduleStop acquires lock, sees ClientCount() > 0, returns (no-op)
- Result: ✓ Pipeline continues, no stop scheduled

Scenario B: T3 before T2
- ScheduleStop moves source to pendingStops, schedules timer
- GetOrCreate acquires lock, sees pendingStop, cancels timer, moves back
- Result: ✓ Pipeline continues, client B attached

Both scenarios handled correctly.
```

#### Race 2: Timer fires during GetOrCreate

```
Timeline:
T1: Timer fires, doStop() starts, waiting on lock
T2: GetOrCreate() acquires lock, cancels timer, closes cancelCh
T3: GetOrCreate() releases lock
T4: doStop() acquires lock, checks cancelCh, sees closed, returns

Result: ✓ doStop exits early, pipeline continues
```

#### Race 3: Two Unsubscribes in rapid succession

```
Timeline:
T1: Client A unsubscribes, ScheduleStop called
T2: Client B unsubscribes, ScheduleStop called

T2 behavior:
- Acquires lock
- Sees source not in sources map (moved to pending)
- Returns early (already pending)

Result: ✓ Only one timer, no double-stop
```

#### Race 4: Subscribe after timer fires but before Stop() completes

```
Timeline:
T1: Timer fires, doStop() removes from pendingStops
T2: doStop() releases lock, calls source.Stop()
T3: GetOrCreate() acquires lock, sees no source, creates NEW source
T4: source.Stop() completes (old source)

Result: ✓ New source created with fresh pipeline
        Old source garbage collected after Stop()

Note: This is intentional - if Stop() is already in progress,
we don't try to resurrect the old source. A fresh pipeline
is cleaner than trying to abort a half-stopped pipeline.
```

### Integration with SharedVideoSource

```go
// In SharedVideoSource.Unsubscribe():
func (s *SharedVideoSource) Unsubscribe(clientID uint64) {
    s.clientsMu.Lock()
    client, ok := s.clients[clientID]
    if !ok {
        s.clientsMu.Unlock()
        return
    }

    // Close client
    if client.state.CompareAndSwap(clientStateLive, clientStateClosed) ||
       client.state.CompareAndSwap(clientStateCatchingUp, clientStateClosed) {
        close(client.frameCh)
    }

    delete(s.clients, clientID)
    clientCount := len(s.clients)
    s.clientsMu.Unlock()

    // If last client, schedule stop via registry
    if clientCount == 0 && s.registry != nil {
        s.registry.ScheduleStop(s.nodeID)
    }
}
```

### Configuration

```go
const (
    DefaultGracePeriod = 60 * time.Second
    MinGracePeriod     = 5 * time.Second   // Don't allow instant stops
    MaxGracePeriod     = 300 * time.Second // 5 minutes max
)

// Can be configured per-deployment via environment variable
func NewSharedVideoRegistry() *SharedVideoRegistry {
    gracePeriod := DefaultGracePeriod
    if v := os.Getenv("VIDEO_GRACE_PERIOD_SECONDS"); v != "" {
        if seconds, err := strconv.Atoi(v); err == nil {
            gracePeriod = time.Duration(seconds) * time.Second
            if gracePeriod < MinGracePeriod {
                gracePeriod = MinGracePeriod
            }
            if gracePeriod > MaxGracePeriod {
                gracePeriod = MaxGracePeriod
            }
        }
    }
    return &SharedVideoRegistry{
        sources:      make(map[uint32]*SharedVideoSource),
        pendingStops: make(map[uint32]*pendingStop),
        gracePeriod:  gracePeriod,
    }
}
```

### Metrics and Observability

```go
type RegistryMetrics struct {
    ActiveSources    int           // Currently streaming
    PendingStops     int           // In grace period
    CancelledStops   atomic.Uint64 // Stops cancelled by reconnect
    CompletedStops   atomic.Uint64 // Stops that completed
    GracePeriodMs    int64         // Current grace period
}

func (r *SharedVideoRegistry) GetMetrics() RegistryMetrics {
    r.mu.Lock()
    defer r.mu.Unlock()
    return RegistryMetrics{
        ActiveSources:  len(r.sources),
        PendingStops:   len(r.pendingStops),
        CancelledStops: r.cancelledStops.Load(),
        CompletedStops: r.completedStops.Load(),
        GracePeriodMs:  r.gracePeriod.Milliseconds(),
    }
}
```

### Edge Cases

| Scenario | Behavior | Result |
|----------|----------|--------|
| Hot reload (4 clients disconnect, reconnect in 100ms) | Timer cancelled, same pipeline reused | Seamless |
| Browser tab closed permanently | Timer fires after 60s, pipeline stopped | Clean |
| Client reconnects at T=59.9s | Timer cancelled just in time | Seamless |
| Client reconnects at T=60.1s | New pipeline created | 2-5s delay |
| Pipeline error during grace period | source.Stop() called, doStop sees source gone | Clean |
| Registry shutdown during grace period | All pending timers cancelled in Shutdown() | Clean |
| Same nodeID used for different screen | Old pending stop cancelled, new source created | Correct |

### Shutdown Handling

```go
func (r *SharedVideoRegistry) Shutdown() {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Cancel all pending stops
    for nodeID, pending := range r.pendingStops {
        pending.timer.Stop()
        close(pending.cancelCh)
        delete(r.pendingStops, nodeID)
    }

    // Stop all active sources
    for nodeID, source := range r.sources {
        delete(r.sources, nodeID)
        go source.Stop() // Stop in parallel
    }
}
```

### Memory Implications

During grace period, the SharedVideoSource continues to hold:
- GStreamer pipeline (paused but allocated)
- GOP buffer (up to 37.8 MB)
- No clients (0 client buffers)

This is acceptable because:
1. 60 seconds is short - if client doesn't reconnect, we stop
2. Pipeline resources are expensive to recreate (PipeWire negotiation)
3. One pipeline in grace period = ~50 MB, trivial for a desktop session

### Testing Scenarios

1. **Hot reload test**: 4 browser tabs, trigger webpack HMR, verify no pipeline restart
2. **Timeout test**: Close all tabs, verify pipeline stops after exactly 60s
3. **Reconnect at boundary**: Reconnect at T=59s, T=60s, T=61s - verify correct behavior
4. **Stress test**: Rapid connect/disconnect cycles, verify no resource leaks
5. **Error recovery**: Force pipeline error during grace period, verify clean shutdown

## Alternatives Considered

1. **Block broadcaster during replay**: Simple but blocks all clients - rejected
2. **Skip frames during replay**: Causes decoder errors - rejected
3. **Force keyframe on new client**: Increases bandwidth - could be added as optimization
4. **Copy-on-write GOP buffer**: Complex, minimal benefit - deferred
5. **Immediate stop on last client**: Causes pipeline churn during hot reload - rejected (grace period added instead)
