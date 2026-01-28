# Implementation Tasks

## Phase 1: Quick Wins (Low Risk)

- [ ] **Profile current performance** - Use React DevTools Profiler and Chrome Performance tab to measure baseline render times, identify which components are slowest
- [ ] **Increase streaming throttle** - Change `STREAMING_THROTTLE_MS` from 150ms to 250ms in `Markdown.tsx` to reduce update frequency
- [ ] **Add adaptive throttling** - Detect rapid streaming (updates <100ms apart) and use longer throttle (300ms) automatically
- [ ] **Memoize CodeBlockWithCopy** - Wrap `CodeBlockWithCopy` component with `React.memo()` to prevent re-renders when props unchanged

## Phase 2: Content Splitting (Medium Effort)

- [ ] **Split finalized vs streaming content** - Detect "safe split points" (closed code blocks, complete paragraphs) and separate into finalized content that can be memoized
- [ ] **Create MemoizedMarkdown wrapper** - Memoized component for rendering finalized content that doesn't change
- [ ] **Render streaming tail separately** - Only pass the "active" portion (last paragraph/code block being written) to non-memoized Markdown
- [ ] **Handle split point detection edge cases** - Ensure unclosed code blocks, partial citations, and thinking tags don't break the split logic

## Phase 3: MessageProcessor Optimization (Higher Effort)

- [ ] **Add early-exit checks** - Skip processing steps when their markers aren't present (e.g., skip `processXmlCitations` if no `<excerpts>` in text)
- [ ] **Cache regex objects** - Move regex patterns to class-level constants instead of creating new RegExp on each call
- [ ] **Optimize processThinkingTags** - Replace line-by-line split/map/join with single-pass regex where possible
- [ ] **Batch DOMPurify calls** - Only sanitize the delta content when possible, not the entire message
- [ ] **Consider incremental processing** - Track processed length and only process new text for simple operations (non-spanning features)

## Phase 4: Code Block Performance (Optional)

- [ ] **Lazy load SyntaxHighlighter** - Use `React.lazy()` for Prism highlighter, show plain `<pre>` as fallback during load
- [ ] **Defer off-screen code blocks** - Use Intersection Observer to only highlight code blocks visible in viewport
- [ ] **Consider lighter highlighter** - Evaluate if `highlight.js` or a subset of Prism languages would be faster

## Phase 5: Web Worker (Optional, if needed)

- [ ] **Create MessageProcessor worker** - Move `MessageProcessor.process()` to a Web Worker
- [ ] **Handle worker communication** - Post message text, receive processed result
- [ ] **Add fallback for worker errors** - Process on main thread if worker fails
- [ ] **Measure worker overhead** - Verify worker communication latency doesn't negate benefits

## Verification

- [ ] **Run existing tests** - Ensure all `Markdown.test.tsx` and `MessageProcessor.test.tsx` tests pass
- [ ] **Manual testing** - Test with long streaming responses (1000+ tokens) with multiple code blocks
- [ ] **Measure improvements** - Profile after each phase, document render time improvements
- [ ] **Test edge cases** - Partial citations, unclosed code blocks, rapid text deletion/replacement
- [ ] **Cross-browser testing** - Verify performance improvements in Chrome, Firefox, Safari