# Implementation Tasks

- [ ] Remove `addBlinker()` call from `MessageProcessor.process()` method
- [ ] Remove `addBlinker()` method from `MessageProcessor` class
- [ ] Pass `isStreaming` prop through to `MemoizedMarkdownRenderer`
- [ ] Add streaming spinner as sibling React element after `<Markdown>` component
- [ ] Choose and implement spinner style (CircularProgress, pulsing dots, or similar)
- [ ] Style spinner with appropriate spacing (`mt: 1` or similar)
- [ ] Remove old blinker CSS styles from component (`.blinker-class` rules)
- [ ] Remove blinker CSS from `index.html` if present
- [ ] Update existing blinker unit tests to test spinner behavior instead
- [ ] Run `yarn build` to check for TypeScript errors
- [ ] Manual test: streaming response with code blocks shows spinner correctly
- [ ] Manual test: spinner appears during streaming, disappears when done
- [ ] Manual test: spinner looks good after various content types (text, code, lists)