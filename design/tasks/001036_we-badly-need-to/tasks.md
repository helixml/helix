# Implementation Tasks

## ✅ Phase 0: Benchmarking (COMPLETED)

- [x] **Create benchmark test file** - Created `frontend/src/components/session/Markdown.bench.ts`
- [x] **Capture test fixtures** - Generated small (~0.4KB), medium (~7KB), large (~18KB), huge (~82KB) content
- [x] **Benchmark MessageProcessor.process()** - Measured total time for various content sizes
- [x] **Document baseline metrics** - Results embedded below

### Key Finding: MessageProcessor is NOT the bottleneck!

| Content Size | Mean Time | Target | Status |
|--------------|-----------|--------|--------|
| Small (~0.4KB) | 0.004ms | <2ms | ✅ 500x faster |
| Medium (~7KB) | 0.014ms | <2ms | ✅ 140x faster |
| Large (~18KB) | 0.065ms | <2ms | ✅ 30x faster |
| Huge (~82KB) | 0.645ms | <2ms | ✅ 3x faster |

**The UI lag must come from React rendering, react-markdown, or syntax highlighting.**

## ✅ Phase 1: React Rendering Optimizations (COMPLETED)

- [x] **Memoize CodeBlockWithCopy** - Wrapped with `React.memo()` + `useCallback` for handleCopy + `useMemo` for processedChildren
- [x] **Memoize react-markdown components object** - Extracted to `MemoizedMarkdownRenderer` component with `useMemo` for components, remarkPlugins, and rehypePlugins
- [x] **Add displayName for debugging** - Both memoized components have displayName for React DevTools

All changes merged to main in commit `3e6096a3a`.

## 🔄 Phase 2: User Verification Required

These tasks require manual testing in the running application:

- [ ] **Manual testing** - Test with long streaming responses (1000+ tokens) with multiple code blocks - **user should verify UI responsiveness improved**
- [ ] **Profile React rendering** - Use React DevTools Profiler to verify memoization reduces re-renders during streaming
- [ ] **Test edge cases** - Partial citations, unclosed code blocks, parallel tool call outputs changing mid-stream

## Phase 3: Further Optimizations (If Still Needed)

Only pursue these if user reports continued issues after Phase 1 optimizations:

- [ ] **Analyze current throttling** - Current `STREAMING_THROTTLE_MS` is 150ms, may need adjustment
- [ ] **Add adaptive throttling** - Detect rapid streaming (updates <100ms apart) and use longer throttle (250-300ms)
- [ ] **Check existing virtualization** - Review virtualization code in `session.tsx` mentioned in design doc
- [ ] **Lazy load SyntaxHighlighter** - Only import Prism when code blocks are present in content

## ~~MessageProcessor Optimization~~ - NOT NEEDED

Based on benchmarks, these optimizations are **not necessary**:

- ~~Add early-exit checks~~ - Already fast enough (<1ms for 82KB)
- ~~Cache regex objects~~ - Negligible impact at these speeds
- ~~Optimize processThinkingTags~~ - Already sub-millisecond
- ~~Reduce string allocations~~ - Not a bottleneck

## ✅ Verification (Automated)

- [x] **Run existing tests** - All 60 tests pass (4 test files, 1 skipped)
- [x] **Build succeeds** - `npx vite build` completes without errors
- [x] **Benchmarks run** - `npx vitest bench --run` confirms MessageProcessor performance

## Running Benchmarks

```bash
cd frontend
npx vitest bench --run
```

## Summary

The core optimization work is **complete and merged**. The memoization changes should significantly reduce wasted re-renders during streaming. User should test the application to verify the UI is now responsive during agent streaming.