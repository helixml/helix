# Implementation Tasks

## Update Placeholder Syntax in Markdown.tsx

- [ ] Update `sanitizeHtml()` method:
  - [ ] Change `__CODE_BLOCK_${index}__` → `<<<CODE_BLOCK_${index}>>>`
  - [ ] Change `__INLINE_CODE_${index}__` → `<<<INLINE_CODE_${index}>>>`
  - [ ] Update restore logic to match new patterns

- [ ] Update `addCitationData()` method:
  - [ ] Change `__CITATION_DATA__${json}__CITATION_DATA__` → `<<<CITATION_DATA>>>${json}<<</CITATION_DATA>>>`

- [ ] Update `processThinkingTags()` method:
  - [ ] Change `__THINKING_WIDGET__${content}__THINKING_WIDGET__` → `<<<THINKING_WIDGET>>>${content}<<</THINKING_WIDGET>>>`

- [ ] Update `processContent()` in `InteractionMarkdown` component:
  - [ ] Update citation extraction pattern to match `<<<CITATION_DATA>>>...<<</CITATION_DATA>>>`
  - [ ] Update thinking widget extraction pattern to match `<<<THINKING_WIDGET>>>...<<</THINKING_WIDGET>>>`

## Update Tests

- [ ] Update `MessageProcessor.test.tsx`:
  - [ ] Change all `__CITATION_DATA__` pattern assertions to use new `<<<CITATION_DATA>>>` syntax
  - [ ] Add test case: placeholder syntax doesn't get interpreted as markdown bold

## Verification

- [ ] Run `cd frontend && yarn test && yarn build` to verify no regressions
- [ ] Manual test: streaming responses with inline code render correctly
- [ ] Manual test: citation data markers don't appear in rendered output