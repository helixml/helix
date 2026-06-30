# Requirements: Make Comment Resolve Button Discoverable on Narrow Screens

## Background

In the spec-task design review UI, each inline comment is shown in a bubble
(`InlineCommentBubble`) and in the comment log sidebar (`CommentLogSidebar`).
The action to mark a comment resolved is currently a bare, unlabeled green
check-circle `IconButton` in the top-right corner of the comment header. It has
no tooltip and no text label.

On narrow / less-wide screens the comment bubbles stack below the document
(`isNarrowViewport` mode), and the small unlabeled icon is easy to miss. Users
report having to "dig around for ages" before finding the Resolve action.

A key part of the symptom (per review feedback) is that on less-wide screens the
user **needed a horizontal scroll** to reach the button at all. In the
side-positioned (wide) layout the bubble is absolutely positioned to the right of
the 800px document column (`left: 820px`, `width: 300px`), so its right edge sits
at ~1120px+ relative to the centred column. But the layout only switches to the
stacked in-flow mode below 1000px (`useMediaQuery(theme.breakpoints.down(1000))`).
In the medium-width band the bubble — and the Resolve button at its top-right —
overflows past the right edge of the viewport, hidden until you scroll
horizontally.

## User Stories

### US-1: Find the Resolve action quickly
As a reviewer on a narrow screen, I want the Resolve action on a comment to be
clearly labeled and easy to spot, so I can resolve comments without hunting for
a tiny icon.

### US-2: Understand what the button does
As a reviewer, I want to know what the green check icon does before I click it,
so I don't have to guess.

## Acceptance Criteria

- AC-1: The Resolve control on a comment bubble is clearly identifiable as
  "Resolve" — either via a visible text label or an always-available tooltip on
  hover/focus.
- AC-2: On narrow viewports (`isNarrowViewport`, viewport < 1000px), the Resolve
  control shows a visible "Resolve" text label (not icon-only), so it is
  immediately discoverable.
- AC-3: The same treatment is applied consistently in both the inline comment
  bubble (`InlineCommentBubble.tsx`) and the comment log sidebar
  (`CommentLogSidebar.tsx`).
- AC-4: Clicking the control still resolves the comment exactly as before
  (calls `onResolve(comment.id)` / `onResolveComment(comment.id)`); no change to
  resolve behaviour, API, or data.
- AC-5: On wide viewports the header layout is not visually broken — the Resolve
  control remains in the top-right of the comment header and does not overflow
  or push other content.
- AC-6: The comment bubble (and therefore the Resolve control) is reachable
  **without horizontal scrolling** at all viewport widths. The stacked in-flow
  layout must engage before the side-positioned bubble would overflow the right
  edge of the viewport — i.e. the `isNarrowViewport` threshold is raised so there
  is no medium-width band where the bubble is pushed off-screen.

## Out of Scope

- Changing the resolve API, backend, or auto-resolve logic.
- Redesigning the comment bubble beyond the Resolve control's label/tooltip.
- Adding a bulk "resolve all" action.
