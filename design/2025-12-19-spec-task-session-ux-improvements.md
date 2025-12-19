# Spec Task Session UX Improvements

**Date:** 2025-12-19
**Status:** Partially Implemented

## Overview

This document describes two UX improvements to reduce reliance on desktop streaming and make the Helix spec task interface more usable on slow/unreliable connections.

## Implemented: Session-First Tab Layout

### Problem

The previous spec task detail dialog showed the video stream (IDE) as the primary interface. This created several issues:

1. **Bandwidth-dependent UX** - Users on slow connections couldn't interact with tasks effectively
2. **Desktop streaming as a bottleneck** - Video streaming issues blocked the entire workflow
3. **Helix UI buried** - The chat/message interaction (the core Helix experience) was secondary to the video

### Solution

Restructured the spec task detail dialog tabs:

| Tab Order | Tab Name | Content | Streaming Required? |
|-----------|----------|---------|---------------------|
| 0 | **Session** | Chat/message thread + input | No |
| 1 | **IDE** | Desktop video stream | Yes |
| 2 | **Details** | Task metadata, settings | No |

When no session exists, only the Details tab is shown.

### Implementation

1. Created `EmbeddedSessionView` component (`frontend/src/components/session/EmbeddedSessionView.tsx`)
   - Lightweight session message thread viewer
   - Auto-scrolls to bottom on new messages
   - Auto-refreshes session data every 2 seconds
   - Renders interactions with streaming support

2. Modified `SpecTaskDetailDialog` to use three tabs:
   - Session tab uses `EmbeddedSessionView` + `RobustPromptInput`
   - IDE tab uses `ExternalAgentDesktopViewer` + `RobustPromptInput`
   - Details tab unchanged

### Benefits

- **Works on slow connections** - Chat interface loads instantly, no video required
- **Progressive enhancement** - Users can optionally view IDE when bandwidth allows
- **Helix-first experience** - Core AI interaction is front and center
- **Same functionality** - Message input available on both Session and IDE tabs

---

## Roadmap: Headless Agent Mode

### Problem

The current architecture requires a full desktop environment (Sway + Wolf streaming) for every agent session. This creates:

1. **Resource overhead** - GPU resources used even when user doesn't need visual output
2. **Latency** - Desktop container startup adds delay before agent can work
3. **Complexity** - Multiple layers (Wolf, Moonlight, desktop compositor) that can fail
4. **Connection dependency** - Agent cannot work effectively without user being connected

### Proposed Solution: Headless-First Agent Architecture

Restructure the agent architecture to be headless by default, with desktop environment as an optional escalation.

```
┌─────────────────────────────────────────────────────────────────────┐
│ Current Architecture (Desktop-First)                                 │
├─────────────────────────────────────────────────────────────────────┤
│   User connects → Desktop starts → Agent runs inside desktop        │
│                                                                      │
│   [Wolf + Sway Container]                                           │
│    ├── Zed (editor with AI agent)                                   │
│    ├── Browser (for web dev)                                        │
│    └── Terminal                                                     │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ Proposed Architecture (Headless-First)                               │
├─────────────────────────────────────────────────────────────────────┤
│   Agent starts → Works headlessly → Desktop available on-demand     │
│                                                                      │
│   [Lightweight Agent Container]                                      │
│    ├── ACP-compatible agent (Claude Code / Qwen Code CLI)          │
│    ├── Git operations                                               │
│    └── File system access                                           │
│                                                                      │
│   [Optional Desktop Container] (spawned on-demand)                  │
│    ├── Zed / VS Code (iframed or streamed)                         │
│    ├── Browser for web app testing                                  │
│    └── Visual debugging tools                                       │
└─────────────────────────────────────────────────────────────────────┘
```

### New Tab Structure (Future)

| Tab | Content | When Available |
|-----|---------|----------------|
| **Session** | Chat + message thread | Always |
| **Terminal/SSH** | Terminal access to container | When container running |
| **IDE** | VS Code in iframe or Zed stream | On-demand |
| **Preview** | Iframed web app (dev server) | When dev server running |

### Key Components

1. **Headless Agent Daemon**
   - ACP-compatible agent (could be Claude Code, Qwen Code, or custom)
   - Runs in lightweight container without desktop compositor
   - Communicates via ACP protocol with Helix backend

2. **MCP Binary for Helix Integration**
   - Bridge between headless agent and Helix services
   - Handles file operations, git, and Helix-specific commands
   - Reports progress back to Helix session

3. **On-Demand Desktop Escalation**
   - When user needs visual environment, spin up desktop container
   - Connect to same workspace/volume as headless agent
   - VS Code can be iframed; Zed requires streaming

4. **Web Preview Iframe**
   - For web development tasks, serve dev server through Helix proxy
   - Display in iframe within task dialog
   - No video streaming required for basic web testing

### Implementation Phases

**Phase 1: Terminal/SSH Tab**
- Add SSH/terminal access to existing desktop container
- Alternative to video streaming for command-line work

**Phase 2: Web Preview Iframe**
- Proxy dev server URLs through Helix
- Display web apps in iframe without streaming

**Phase 3: VS Code Iframe**
- Use code-server or similar for VS Code in browser
- No video streaming required

**Phase 4: Headless Agent Container**
- Claude Code / Qwen Code running without desktop
- ACP communication with Helix
- Desktop spawned only when needed

### Trade-offs

| Approach | Pros | Cons |
|----------|------|------|
| **Headless-first** | Fast startup, lower resources, works on slow connections | Can't see AI using the IDE visually |
| **Iframe VS Code** | No streaming, familiar UI | Not as powerful as native Zed |
| **On-demand desktop** | Full visual when needed | Complexity of two modes |

### Open Questions

1. Should we support multiple agent backends (Claude Code, Qwen Code, custom)?
2. How to handle state sync between headless and desktop modes?
3. What's the minimum viable headless agent for spec tasks?
4. Should terminal/SSH be web-based (xterm.js) or require client?

---

## Files Changed

### Implemented
- `frontend/src/components/session/EmbeddedSessionView.tsx` (new)
- `frontend/src/components/tasks/SpecTaskDetailDialog.tsx` (modified)
- `frontend/src/services/sessionService.ts` (modified - added refetchInterval support)

### Zed Changes (~/pm/zed)

**1. Fixed tool calls not being synced to Helix at all**
- `crates/external_websocket_sync/src/thread_service.rs` - The `EntryUpdated` handler was only processing `AssistantMessage` entries and skipping everything else. Tool calls (which contain diffs, terminal output, etc.) are `ToolCall` entries and were silently ignored.

**Before:**
```rust
let content = match entry {
    AgentThreadEntry::AssistantMessage(msg) => msg.content_only(cx),
    _ => return, // ← Tool calls silently skipped!
};
```

**After:**
```rust
let content = match entry {
    AgentThreadEntry::AssistantMessage(msg) => msg.content_only(cx),
    AgentThreadEntry::ToolCall(tool_call) => tool_call.to_markdown(cx), // ← Now synced!
    AgentThreadEntry::UserMessage(_) => return,
};
```

**2. Made ToolCall::to_markdown() public**
- `crates/acp_thread/src/acp_thread.rs` - Changed `fn to_markdown` to `pub fn to_markdown` so external_websocket_sync can call it.

**3. Fixed diff serialization to show actual changes**
- `crates/acp_thread/src/diff.rs` - `Diff::to_markdown()` was only outputting the new file content, not showing what changed. Now uses `language::unified_diff()` to generate proper diff format.

**Before:**
```
Diff: path/to/file
```
(just the new file content - no indication of what changed)
```
```

**After:**
```
Diff: path/to/file
```diff
@@ -1,5 +1,5 @@
 unchanged line
-deleted line
+added line
 unchanged line
```
```

**Tool call output now includes:**
- `**Tool Call: <name>**` header with status
- Diff content in unified diff format
- Terminal output in code blocks
- Any other content blocks as markdown

### Future Work
- TBD based on roadmap decisions
