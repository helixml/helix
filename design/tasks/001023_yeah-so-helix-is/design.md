# Design: Incremental Streaming Architecture

## Overview

Replace full-state updates with delta-based streaming to achieve O(n) complexity instead of O(n²).

## Architecture

### Current Flow (O(n²))

```
Token arrives in Zed
    ↓
emit EntryUpdated(idx)
    ↓
Iterate ALL entries after last user message
    ↓
Serialize cumulative content (grows with N)
    ↓
WebSocket: send full content
    ↓
Helix: MessageProcessor.process(full text)
    ↓
Helix: react-markdown parses full text
```

### Proposed Flow (O(n))

```
Token arrives in Zed
    ↓
emit EntryUpdated(idx) with delta info
    ↓
Extract ONLY the new content (delta)
    ↓
WebSocket: send delta OR full state (based on message type)
    ↓
Helix: append delta to existing content
    ↓
Helix: process only the delta for most operations
    ↓
Helix: incremental render (only re-parse when necessary)
```

## Key Changes

### 1. Zed Side: Delta Extraction

**Location**: `zed/crates/agent_ui/src/acp/thread_view.rs`

Track last sent position per entry. On `EntryUpdated`:
- If entry is being appended to (streaming text), send only the new characters
- If entry structure changed (new tool call), send full entry state

```rust
// Pseudocode
struct StreamingState {
    last_sent_length: HashMap<usize, usize>,
}

fn on_entry_updated(index: usize, thread: &AcpThread) {
    let content = get_entry_content(index);
    let last_len = self.last_sent_length.get(&index).unwrap_or(&0);
    
    if content.len() > *last_len {
        // Streaming append - send delta
        let delta = &content[*last_len..];
        send_delta(index, delta);
        self.last_sent_length.insert(index, content.len());
    } else {
        // Structure change - send full
        send_full(index, content);
        self.last_sent_length.insert(index, content.len());
    }
}
```

### 2. WebSocket Protocol: Add Delta Message Type

**New event type**: `MessageDelta`

```typescript
// Existing (keep for full syncs)
interface MessageAdded {
    type: "message_added";
    acp_thread_id: string;
    message_id: string;
    role: string;
    content: string;  // Full content
    timestamp: number;
}

// New (for incremental updates)
interface MessageDelta {
    type: "message_delta";
    acp_thread_id: string;
    message_id: string;
    delta: string;     // Only new characters
    offset: number;    // Position where delta starts
    timestamp: number;
}
```

### 3. Helix Frontend: Accumulate Deltas

**Location**: `helix/frontend/src/hooks/useLiveInteraction.ts` (or similar)

```typescript
// Maintain accumulated content
const [accumulatedContent, setAccumulatedContent] = useState('');

function handleWebSocketMessage(event) {
    if (event.type === 'message_delta') {
        // O(1) append
        setAccumulatedContent(prev => prev + event.delta);
    } else if (event.type === 'message_added') {
        // Full replacement (reconnect, structure change)
        setAccumulatedContent(event.content);
    }
}
```

### 4. Helix Frontend: Optimize MessageProcessor

**Location**: `helix/frontend/src/components/session/Markdown.tsx`

Most processing only needs to happen on:
- Final render (streaming complete)
- Periodic intervals during streaming (e.g., every 500ms)
- NOT on every delta

```typescript
// Throttle expensive processing
const PROCESS_INTERVAL_MS = 500;

const debouncedContent = useDebouncedValue(text, PROCESS_INTERVAL_MS);

// Only run full MessageProcessor on debounced content
const processedContent = useMemo(() => {
    if (!isStreaming) {
        // Final: full processing
        return new MessageProcessor(text, options).process();
    }
    // Streaming: simplified processing (no citations, etc.)
    return text + (showBlinker ? '┃' : '');
}, [debouncedContent, isStreaming]);
```

## Fallback Strategy

For robustness, keep the ability to do full-state sync:
- On WebSocket reconnect
- When delta sequence number mismatch is detected
- Every N deltas (e.g., every 100) send a full sync as checkpoint

## Performance Comparison

| Scenario | Current | Proposed |
|----------|---------|----------|
| 100 token response | 5,050 chars sent | ~100 chars sent |
| 1,000 token response | 500,500 chars sent | ~1,000 chars sent |
| 10,000 token response | 50,005,000 chars sent | ~10,000 chars sent |

## Risks

1. **Out-of-order deltas**: WebSocket is ordered, but add sequence numbers for safety
2. **Reconnection**: Must request full state on reconnect
3. **Content modification**: If agent modifies earlier content (rare), need full sync

## Decision

Start with **Zed-side optimization only** (cheapest fix):
1. Track last-sent length per entry
2. Send deltas for streaming appends
3. Helix naturally handles this (appends to existing message)

This gives 90% of the benefit with minimal protocol changes.