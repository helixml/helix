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

## Proposed Solutions

### Solution 1: Incremental Message Processing (Primary)

**Concept**: Only process the **delta** (new text) during streaming, not the entire message.

```typescript
class IncrementalMessageProcessor {
  private processedUpTo: number = 0;
  private cachedResult: string = '';
  
  processIncremental(fullText: string): string {
    if (fullText.length < this.processedUpTo) {
      // Text was reset, reprocess everything
      return this.processAll(fullText);
    }
    
    const delta = fullText.slice(this.processedUpTo);
    const processedDelta = this.processDelta(delta);
    this.cachedResult += processedDelta;
    this.processedUpTo = fullText.length;
    
    return this.cachedResult;
  }
}
```

**Limitation**: Some features (thinking tags, citations) span multiple chunks and need full reprocessing. Use a hybrid approach - process delta for simple text, full reprocess only when special markers detected.

### Solution 2: Increase Throttle During Active Streaming

Current throttle: 150ms. During rapid streaming, increase to 250-300ms.

```typescript
const STREAMING_THROTTLE_MS = 150;
const FAST_STREAMING_THROTTLE_MS = 300;

// Detect rapid streaming (>10 updates/sec)
const isRapidStreaming = (Date.now() - lastUpdate) < 100;
const throttle = isRapidStreaming ? FAST_STREAMING_THROTTLE_MS : STREAMING_THROTTLE_MS;
```

### Solution 3: Virtualized Code Block Rendering

Use `React.lazy` and visibility detection to only render code blocks that are in/near viewport:

```typescript
const LazyCodeBlock = React.lazy(() => import('./CodeBlockWithCopy'));

const CodeBlockWrapper = ({ inView, ...props }) => {
  if (!inView) {
    return <pre className="code-placeholder">Loading...</pre>;
  }
  return (
    <Suspense fallback={<pre>...</pre>}>
      <LazyCodeBlock {...props} />
    </Suspense>
  );
};
```

### Solution 4: Memoize Static Content

Split content into "finalized" (closed code blocks, completed paragraphs) and "streaming" (current line/block being written):

```typescript
const MemoizedContent = React.memo(({ content }) => (
  <Markdown>{content}</Markdown>
));

// Only re-render the streaming tail
<MemoizedContent content={finalizedContent} />
<Markdown>{streamingTail}</Markdown>
```

### Solution 5: Web Worker for MessageProcessor

Move expensive regex operations to a Web Worker to avoid blocking the main thread:

```typescript
// worker.ts
self.onmessage = (e) => {
  const { text, options } = e.data;
  const processor = new MessageProcessor(text, options);
  const result = processor.process();
  self.postMessage(result);
};
```

**Trade-off**: Adds latency (worker communication) but frees main thread for UI interactions.

## Recommended Implementation Order

1. **Quick wins** (Solution 2): Adaptive throttling - low risk, immediate improvement
2. **Medium effort** (Solution 4): Content splitting - moderate refactor, good gains
3. **Higher effort** (Solution 1): Incremental processing - requires careful handling of edge cases
4. **Optional** (Solutions 3, 5): Only if above insufficient

## Performance Budget

| Metric | Current | Target |
|--------|---------|--------|
| MessageProcessor time | 5-15ms | <3ms |
| React render time | 20-50ms | <16ms (60fps) |
| Total update latency | 50-100ms | <32ms |

## Risks

1. **Feature regressions** - Citations, thinking tags, and blinker depend on full-text processing
2. **Edge cases** - Partial markdown (unclosed code blocks) during streaming
3. **Browser compatibility** - Web Workers may have issues in some environments

## Alternatives Considered

1. **Replace react-markdown with mdx-js** - Similar performance, more complexity
2. **Custom markdown parser** - High effort, maintenance burden
3. **Server-side rendering** - Doesn't help with streaming