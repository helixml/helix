# Implementation Tasks

## Phase 0: Benchmarking (Do This First)

- [ ] **Create benchmark test file** - Create `frontend/src/components/session/Markdown.bench.ts` with performance tests for MessageProcessor
- [ ] **Capture real session data** - Export 4-5 real large sessions (small ~1KB, medium ~10KB, large ~50KB, huge ~200KB with many code blocks) as test fixtures
- [ ] **Benchmark MessageProcessor.process()** - Measure total time and per-step time for each processing method
- [ ] **Benchmark react-markdown rendering** - Measure render time for various content sizes
- [ ] **Benchmark SyntaxHighlighter** - Measure Prism.js highlighting time for different code block counts/sizes
- [ ] **Document baseline metrics** - Record current performance numbers to compare against after optimizations

## Phase 1: MessageProcessor Optimization (Based on Benchmark Results)

- [ ] **Add early-exit checks** - Skip processing steps when their markers aren't present (e.g., skip `processXmlCitations` if no `<excerpts>` in text)
- [ ] **Cache regex objects** - Move regex patterns to static class-level constants instead of creating new RegExp on each call
- [ ] **Optimize processThinkingTags** - Replace line-by-line split/map/join with single-pass regex where possible
- [ ] **Reduce string allocations** - Audit each processing step for unnecessary string copies
- [ ] **Re-run benchmarks** - Verify improvements after each change

## Phase 2: Throttling Improvements

- [ ] **Add adaptive throttling** - Detect rapid streaming (updates <100ms apart) and use longer throttle (250-300ms) automatically
- [ ] **Consider requestIdleCallback** - Evaluate processing during browser idle time instead of fixed interval
- [ ] **Measure UI responsiveness** - Ensure scrolling and clicking remain smooth during streaming

## Phase 3: React Rendering Optimizations

- [ ] **Memoize CodeBlockWithCopy** - Wrap component with `React.memo()` to prevent re-renders when props unchanged
- [ ] **Profile React reconciliation** - Use React DevTools Profiler to identify unnecessary re-renders
- [ ] **Lazy load SyntaxHighlighter** - Defer Prism loading for off-screen code blocks if benchmarks show it's a bottleneck

## Verification

- [ ] **Run existing tests** - Ensure all `Markdown.test.tsx` and `MessageProcessor.test.tsx` tests pass
- [ ] **Manual testing** - Test with long streaming responses (1000+ tokens) with multiple code blocks
- [ ] **Compare before/after benchmarks** - Document improvement (target: 5-10x faster MessageProcessor)
- [ ] **Test edge cases** - Partial citations, unclosed code blocks, parallel tool call outputs changing mid-stream
- [ ] **Cross-browser testing** - Verify performance improvements in Chrome, Firefox, Safari