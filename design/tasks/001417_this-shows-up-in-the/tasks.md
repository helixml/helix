# Implementation Tasks

## Streaming Indicator Implementation
- [ ] Choose indicator style (gradient wave bar, pulsing dots, or glow cursor)
- [ ] Create `StreamingIndicator` component with chosen animation
- [ ] Use Helix brand colors (#00D4FF cyan, #7C3AED purple) if using gradient

## Remove Old Blinker
- [ ] Remove `addBlinker()` call from `MessageProcessor.process()` method
- [ ] Remove `addBlinker()` method from `MessageProcessor` class
- [ ] Remove old blinker CSS styles from component (`.blinker-class` rules)
- [ ] Remove blinker CSS from `index.html` if present

## Add New Indicator
- [ ] Pass `isStreaming` prop through to `MemoizedMarkdownRenderer`
- [ ] Add `StreamingIndicator` as sibling React element after `<Markdown>` component
- [ ] Style with appropriate spacing (`mt: 1` or similar)

## Testing
- [ ] Update existing blinker unit tests to test new indicator behavior
- [ ] Run `yarn build` to check for TypeScript errors
- [ ] Manual test: streaming response with code blocks shows indicator correctly
- [ ] Manual test: indicator appears during streaming, disappears when done
- [ ] Manual test: indicator looks good after various content types (text, code, lists)
- [ ] Manual test: animation is smooth and not distracting