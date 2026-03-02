# Implementation Tasks

- [ ] Add `closeUnclosedTags()` private method to `MessageProcessor` class in `Markdown.tsx`
- [ ] Modify `addBlinker()` to call `closeUnclosedTags()` before appending blinker span
- [ ] Add unit test: blinker with unclosed `<div>` tag renders correctly
- [ ] Add unit test: blinker with partial tag (`<span class="`) renders correctly
- [ ] Add unit test: blinker with unclosed `<pre>` block renders correctly
- [ ] Add unit test: blinker with multiple unclosed nested tags
- [ ] Add unit test: verify self-closing tags (br, hr, img) don't affect blinker
- [ ] Verify existing blinker tests still pass (code block handling, streaming on/off)
- [ ] Run `yarn build` to check for TypeScript errors
- [ ] Manual test: streaming response with various HTML content