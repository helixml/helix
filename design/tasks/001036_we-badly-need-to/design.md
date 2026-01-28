# Design: Markdown Renderer Performance Optimization

## Architecture Overview

The current rendering pipeline:
```
Text Stream → InteractionMarkdown → MessageProcessor → react-markdown → SyntaxHighlighter → DOM
```

Each component in this chain may have performance issues during streaming. We need to measure before optimizing.

## Root Cause Analysis (Hypothesized)

### 1. MessageProcessor Overhead
The `MessageProcessor` class runs **8 sequential processing steps** on every update:
- `processXmlCitations()` - Multiple regex matches
- `processFilterMentions()` - Regex replacement
- `processDocumentIds()` - Iteration over document_ids map
- `processDocumentGroupIds()` - Regex replacement
- `processThinkingTags()` - Complex regex with line splitting
- `removeTrailingTripleDash()` - Regex
- `sanitizeHtml()` - DOMPurify sanitization + code block protection
- `addBlinker()` / `addCitationData()` - String concatenation

**Estimated cost**: ~5-15ms per call × updates every 150ms = potentially significant CPU load

### 2. react-markdown Re-parsing
`react-markdown` parses the **entire** markdown string on every render, even if only a few characters were appended. No incremental parsing.

### 3. SyntaxHighlighter Recalculation
Prism.js tokenizes and highlights **all** code blocks on every render. A response with 3-4 code blocks may take 20-50ms per render.

### 4. React Reconciliation
The entire component tree re-renders, creating new DOM nodes for unchanged content.

**Note**: These are hypotheses based on code review. Benchmarks will confirm which are actual bottlenecks.

## Important Constraint: External Agent Streaming

External agents send the **entire interaction content** on every update, not just appended text. This is because:
- Multiple tool calls can run in parallel
- Content can change mid-stream (not just at the end)
- Two commands running simultaneously both update the output

This means **incremental/delta processing is risky** - we can't assume text is only appended. The entire content may change anywhere. Focus optimization efforts on making full processing faster, not avoiding it.

## Recommended Approach: Benchmark-Driven Optimization

### Phase 0: Create Benchmarks First

Before optimizing, we need to measure. Create JavaScript benchmarks using real large sessions:

```typescript
// benchmark/messageProcessor.bench.ts
import { MessageProcessor } from '../src/components/session/Markdown';
import { realSessionData } from './fixtures/large-sessions';

const sessions = [
  { name: 'small', text: realSessionData.small },      // ~1KB
  { name: 'medium', text: realSessionData.medium },    // ~10KB  
  { name: 'large', text: realSessionData.large },      // ~50KB
  { name: 'huge', text: realSessionData.huge },        // ~200KB with many code blocks
];

for (const session of sessions) {
  console.time(`MessageProcessor:${session.name}`);
  for (let i = 0; i < 100; i++) {
    const processor = new MessageProcessor(session.text, mockOptions);
    processor.process();
  }
  console.timeEnd(`MessageProcessor:${session.name}`);
}
```

**Goal**: Identify which processing steps are slowest, then optimize those specifically.

### Phase 1: Optimize Based on Benchmark Results

After benchmarking, apply targeted optimizations to the slowest parts. Likely candidates:

1. **Early exit checks** - Skip processing steps when markers aren't present
2. **Cache compiled regexes** - Move regex patterns to static class-level constants
3. **Optimize processThinkingTags** - Replace line-by-line split/map/join with single-pass regex if this is slow
4. **Reduce string allocations** - Audit each processing step for unnecessary string copies

### Phase 2: Throttling Improvements (If Needed)

Current throttle: 150ms. If benchmarks show processing is still too slow:

1. **Adaptive throttling** - Increase to 250-300ms during rapid updates
2. **requestIdleCallback** - Process during browser idle time instead of fixed interval

### Phase 3: React Rendering Optimizations (If Needed)

Only pursue if benchmarks show React rendering is the bottleneck:

1. **Memoize CodeBlockWithCopy** - Add `React.memo()` if re-renders are excessive
2. **Profile with React DevTools** - Identify unnecessary re-renders

**Note**: There may already be virtualization in `session.tsx` - check existing code before adding new implementations.

## Performance Budget

| Metric | Current (to be measured) | Target |
|--------|--------------------------|--------|
| MessageProcessor time | TBD | <2ms |
| React render time | TBD | <10ms |
| Total update latency | TBD | <20ms |

## Success Criteria

1. Benchmarks show measurable improvement in identified bottlenecks
2. UI remains responsive (no dropped frames) during streaming
3. All existing tests pass
4. No feature regressions (citations, thinking tags, code blocks all work)

## Risks

1. **Over-optimization** - Don't add complexity for marginal gains; benchmark first
2. **Feature regressions** - Run full test suite after each change
3. **Browser differences** - Benchmark in Chrome, Firefox, Safari

## What NOT to Do

1. **Incremental/delta processing** - Too risky given external agents can modify content anywhere
2. **Content splitting** - Same reason; can't reliably detect "finalized" content
3. **Premature optimization** - Measure first, then optimize the actual bottlenecks

---

## Implementation Notes (What Was Actually Done)

### Key Finding: MessageProcessor is NOT the Bottleneck

Benchmarking revealed that `MessageProcessor.process()` is extremely fast:

| Content Size | Mean Time | Target | Result |
|--------------|-----------|--------|--------|
| Small (~0.4KB) | 0.004ms | <2ms | ✅ 500x faster than target |
| Medium (~7KB) | 0.014ms | <2ms | ✅ 140x faster |
| Large (~18KB) | 0.065ms | <2ms | ✅ 30x faster |
| Huge (~82KB) | 0.645ms | <2ms | ✅ 3x faster |

**Conclusion**: The UI lag must come from React rendering, not MessageProcessor.

### Changes Made (Commit 3e6096a3a)

1. **Memoized `CodeBlockWithCopy` component**
   - Wrapped with `React.memo()` to prevent re-renders when props unchanged
   - Added `useCallback` for `handleCopy` to stabilize function reference
   - Added `useMemo` for `processedChildren` to avoid recalculating on every render

2. **Extracted `MemoizedMarkdownRenderer` component**
   - Separated markdown rendering into its own memoized component
   - Memoized the `components` object passed to react-markdown (created once, not on every render)
   - Memoized `remarkPlugins` and `rehypePlugins` arrays

3. **Added displayName for debugging**
   - Both memoized components have `displayName` for React DevTools visibility

### Files Modified

- `frontend/src/components/session/Markdown.tsx` - Core optimizations
- `frontend/src/components/session/Markdown.bench.ts` - Comprehensive benchmark suite (created)

### Benchmark File Location

The benchmark file is at `frontend/src/components/session/Markdown.bench.ts`. Run with:

```bash
cd frontend
npx vitest bench --run
```

### Why This Approach Works

The memoization prevents react-markdown from:
1. Recreating the `components` object on every parent render
2. Re-rendering already-rendered code blocks when only text content changes
3. Re-running Prism highlighting for unchanged code blocks

React's reconciliation can now skip diffing entire subtrees when props haven't changed.

### What Wasn't Needed

Based on benchmarks, these optimizations were **skipped** (not worth the complexity):
- Early exit checks in MessageProcessor
- Caching compiled regexes
- Optimizing processThinkingTags
- Reducing string allocations
- Web Workers for processing
- Incremental/delta processing