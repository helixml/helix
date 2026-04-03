# Requirements: Fix Text Highlighting in Bullet Point Lists

## User Story

As a spec reviewer, when I select text inside a bullet point list item, I want to see the text highlighted in blue so I can create an inline comment on that specific text.

## Current Buggy Behavior

1. Selecting text inside a `<li>` element causes the selected text to render as plain black text (no visible highlight).
2. A new, spurious list item is created in the DOM, breaking the list structure.
3. The inline comment form does appear, but the visual highlight is broken/missing.

## Acceptance Criteria

- [ ] Selecting text inside a `<li>` element highlights it with the same blue background (`#b3d7ff`) as text in paragraphs.
- [ ] No new list items are created as a side effect of highlighting.
- [ ] The list structure is preserved visually after highlighting.
- [ ] Removing the highlight (on mousedown or after comment submission) correctly restores the original list structure.
- [ ] Cross-element selections that span list item boundaries fail gracefully (no DOM corruption) — the comment form still opens without a visual highlight applied.
