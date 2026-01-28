# Implementation Tasks

## Phase 1: Fix Error Handling Bug (Primary Fix)

- [ ] Fix `processContent()` in `InteractionMarkdown` component:
  - [ ] Move `content.replace()` BEFORE the try/catch block so markers are always removed
  - [ ] Current bug: when JSON.parse fails, catch block sets `setCitationData(null)` but leaves `__CITATION_DATA__` markers in content

## Phase 2: Change Placeholder Syntax (Defensive Fix)

- [ ] Update `sanitizeHtml()` method:
  - [ ] Change `__CODE_BLOCK_${index}__` → `\x00CB${index}\x00`
  - [ ] Change `__INLINE_CODE_${index}__` → `\x00IC${index}\x00`
  - [ ] Update restore logic to match new patterns

- [ ] Update `addCitationData()` method:
  - [ ] Change `__CITATION_DATA__${json}__CITATION_DATA__` → `\x00CD\x00${json}\x00/CD\x00`

- [ ] Update `processThinkingTags()` method:
  - [ ] Change `__THINKING_WIDGET__` markers → `\x00TW\x00` markers

- [ ] Update `processContent()` in `InteractionMarkdown` component:
  - [ ] Update citation extraction regex to match new `\x00CD\x00` pattern
  - [ ] Update thinking widget extraction regex to match new `\x00TW\x00` pattern

## Update Tests

- [ ] Update `MessageProcessor.test.tsx`:
  - [ ] Update all `__CITATION_DATA__` pattern assertions to use new syntax
  - [ ] Add test: JSON parse failure should NOT leak markers into output
  - [ ] Add test: placeholder syntax is not interpreted as markdown bold

## Verification

- [ ] Run `cd frontend && yarn test && yarn build` to verify no regressions
- [ ] Manual test: streaming responses with inline code render correctly
- [ ] Manual test: citation data markers don't appear in rendered output
- [ ] Manual test: trigger JSON parse error and verify no marker leakage