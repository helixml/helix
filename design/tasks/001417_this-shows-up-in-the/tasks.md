# Implementation Tasks

## Streaming Indicator Implementation
- [x] Create `StreamingIndicator` component with infinity/figure-8 animation
  - Two dots (cyan #00d4ff, purple #d8b4fe head / #a855f7 trail)
  - Figure-8 lemniscate path with 48 keyframes
  - 24 trail elements per dot with fading opacity
  - 33% offset between dots to avoid collision
  - 2s animation cycle

## Remove Old Blinker
- [x] Remove `addBlinker()` call from `MessageProcessor.process()` method
- [x] Remove `addBlinker()` method from `MessageProcessor` class
- [x] Remove old blinker CSS styles from component (`.blinker-class` rules)
- [x] Remove blinker CSS from `index.html` if present

## Add New Indicator
- [~] Pass `isStreaming` prop through to `MemoizedMarkdownRenderer`
- [ ] Add `StreamingIndicator` as sibling React element after `<Markdown>` component
- [ ] Style with appropriate spacing

## Testing
- [ ] Update existing blinker unit tests to test new indicator behavior
- [ ] Run `yarn build` to check for TypeScript errors
- [ ] Manual test: streaming response with code blocks shows indicator correctly
- [ ] Manual test: indicator appears during streaming, disappears when done
- [ ] Manual test: indicator looks good after various content types (text, code, lists)
- [ ] Manual test: animation is smooth and not distracting