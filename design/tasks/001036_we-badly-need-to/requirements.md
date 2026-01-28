# Requirements: Markdown Renderer Performance Optimization

## Problem Statement

The markdown renderer (`InteractionMarkdown` component in `frontend/src/components/session/Markdown.tsx`) causes UI lag and poor responsiveness when an AI agent is actively streaming responses. This makes the UI "pretty unusable" during agent interactions.

## User Stories

1. **As a user**, I want the UI to remain responsive while an agent is streaming markdown content, so I can scroll, click, and interact without lag.

2. **As a user**, I want to see streaming responses update smoothly without the browser freezing or stuttering.

3. **As a developer**, I want to understand where the performance bottlenecks are so we can make targeted optimizations.

## Acceptance Criteria

1. **Responsiveness**: UI remains interactive during streaming (scrolling, clicking work without noticeable delay)
2. **Frame rate**: Maintain 30+ FPS during streaming updates
3. **Memory**: No memory leaks from repeated renders
4. **Correctness**: All existing markdown features continue to work (citations, code blocks, thinking tags, blinker)
5. **Tests**: Existing tests pass, no regressions

## Current Pain Points

1. **MessageProcessor runs on every text change** - Complex regex operations, HTML sanitization, and citation processing happen every 150ms during streaming
2. **react-markdown re-parses entire content** - Full markdown parsing on every update, even for small text additions
3. **SyntaxHighlighter re-highlights all code** - Prism.js re-processes all code blocks on every render
4. **DOM thrashing** - Large amounts of DOM nodes created/destroyed during updates

## Scope

**In scope:**
- `Markdown.tsx` component optimization
- `MessageProcessor` class optimization
- Streaming update throttling improvements
- Code block rendering optimization

**Out of scope:**
- Backend streaming changes
- Other markdown renderers in the app (e.g., DesignDocPage)
- Citation modal performance