# Implementation Tasks

- [ ] Remove `addBlinker()` call from `MessageProcessor.process()` method
- [ ] Pass `showBlinker` and `isStreaming` props through to `MemoizedMarkdownRenderer`
- [ ] Add blinker as sibling React element after `<Markdown>` component in `MemoizedMarkdownRenderer`
- [ ] Verify blinker CSS class (`blinker-class`) still applies correctly
- [ ] Update/remove `addBlinker()` method in `MessageProcessor` class
- [ ] Update existing blinker unit tests to reflect new rendering approach
- [ ] Add test: blinker renders outside code block context
- [ ] Add test: blinker does not appear in monospace font
- [ ] Run `yarn build` to check for TypeScript errors
- [ ] Manual test: streaming response with code blocks shows blinker correctly
- [ ] Manual test: blinker animation works and disappears when streaming ends