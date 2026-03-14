# Implementation Tasks

- [~] In `processThinkingTags()` (`Markdown.tsx`), normalise `<thinking>` → `<think>` and `</thinking>` → `</think>` before the existing parsing logic runs
- [ ] Add test cases in `Markdown.test.tsx` for the `<thinking>` tag variant (streaming/unclosed, completed, multiple blocks)
- [ ] Manually verify: run a SpecTask with Claude Code agent and confirm thinking output renders as a "💡 Thoughts" widget
