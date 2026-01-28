# Implementation Tasks

## Fix Inline Code Placeholder Collision

- [ ] Modify `sanitizeHtml()` in `frontend/src/components/session/Markdown.tsx`:
  - [ ] Change `inlineCode` from array to `Map<string, string>`
  - [ ] Change `codeBlocks` from array to `Map<string, string>`
  - [ ] Generate UUID-based placeholders using `crypto.randomUUID()`
  - [ ] Update restore logic to iterate over Map entries

## Testing

- [ ] Add unit test: LLM output containing literal `__INLINE_CODE_N__` text renders correctly
- [ ] Add unit test: Normal inline code renders correctly during streaming
- [ ] Add unit test: Multiple inline code blocks in single message render correctly
- [ ] Manual test: Verify streaming markdown renders without placeholder leakage

## Verification

- [ ] Run `cd frontend && yarn test && yarn build` to verify no regressions
- [ ] Test in dev environment with streaming responses containing inline code