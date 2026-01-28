# Design: Markdown Renderer Performance Optimization

## Architecture Overview

The current rendering pipeline:
```
Text Stream → InteractionMarkdown → MessageProcessor → react-markdown → SyntaxHighlighter → DOM
```

Each component in this chain has performance issues during streaming.

## Root Cause Analysis

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

**Cost**: ~5-15ms per call × updates every 150ms = significant CPU load

### 2. react-markdown Re-parsing
`react-markdown` parses the **entire** markdown string on every render, even if only a few characters were appended. No incremental parsing.

### 3. SyntaxHighlighter Recalculation
Prism.js tokenizes and highlights **all** code blocks on every render. A response with 3-4 code blocks can take 20-50ms per render.

### 4. React Reconciliation
The entire component tree re-renders, creating new DOM nodes for unchanged content.

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

### Phase 1: Low-Hanging Fruit in MessageProcessor

Based on code review, likely optimizations:

1. **Early exit checks** - Skip processing steps when markers aren't present:
   ```typescript
   // Before: always runs complex regex
   processXmlCitations(message)
   
   // After: skip if no markers
   if (!message.includes('<excerpts>')) return message;
   ```

2. **Cache compiled regexes** - Move regex patterns to class-level constants:
   ```typescript
   // Before: creates new RegExp on each call
   const citationRegex = /<excerpts>([\s\S]*?)<\/excerpts>/g;
   
   // After: compile once
   private static readonly CITATION_REGEX = /<excerpts>([\s\S]*?)<\/excerpts>/g;
   ```

3. **Optimize processThinkingTags** - Currently splits into lines, maps, joins. Could use single-pass regex.

4. **Reduce string allocations** - Each processing step creates a new string. Consider StringBuilder pattern or processing in-place where possible.

### Phase 2: Throttling Improvements

Current throttle: 150ms. Options:

1. **Adaptive throttling** - Increase to 250-300ms during rapid updates:
   ```typescript
   const isRapidStreaming = (Date.now() - lastUpdate) < 100;
   const throttle = isRapidStreaming ? 300 : 150;
   ```

2. **requestIdleCallback** - Process during browser idle time instead of fixed interval

### Phase 3: React Rendering Optimizations

1. **Memoize CodeBlockWithCopy** - Already wrapped in function, add `React.memo()`

2. **Lazy load SyntaxHighlighter** - Prism is heavy, defer loading for off-screen code blocks

3. **Consider lighter markdown parser** - If benchmarks show react-markdown is the bottleneck, evaluate alternatives like `marked` + custom React renderer

## Performance Budget

| Metric | Current (estimated) | Target |
|--------|---------------------|--------|
| MessageProcessor time | 5-15ms | <2ms |
| React render time | 20-50ms | <10ms |
| Total update latency | 50-100ms | <20ms |

## Success Criteria

1. Benchmarks show 5-10x improvement in MessageProcessor throughput
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
3. **Web Workers** - Adds latency and complexity; try synchronous optimizations first