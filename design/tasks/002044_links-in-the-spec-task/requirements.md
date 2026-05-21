# Requirements: Open Chat Links in New Tab

## Background

Markdown links rendered inside the spec task chat (inline comment bubbles
and the comment log sidebar) currently open in the **same tab**, which
navigates the user away from Helix and discards their place in the spec
review.

The user noted this is "probably a common interaction display component
that's used in sessions as well" — and that is correct. The spec task
chat and the regular session chat share a single markdown renderer
(`InteractionMarkdown` in `frontend/src/components/session/Markdown.tsx`),
so the fix applies everywhere the renderer is used.

## User Stories

### US-1: Open external links in a new tab from spec task chat
**As a** user reviewing a spec task,
**I want** clicking a hyperlink in a comment/message to open in a new tab,
**so that** I don't lose my place in the review or the chat scroll position.

### US-2: Same behaviour in session chat
**As a** user chatting with an agent in a regular session,
**I want** the same new-tab behaviour for any link the model or I post,
**so that** clicking a citation, reference, or documentation URL doesn't
navigate the chat away.

## Acceptance Criteria

### AC-1: External markdown links open in a new tab
- **Given** a message contains `[example](https://example.com)`
- **When** the user clicks the rendered link
- **Then** the link opens in a new browser tab
- **And** the existing chat tab remains on the same page/scroll position

### AC-2: `rel` attribute is set for security
- **Given** any link is opened in a new tab via this change
- **When** the rendered HTML is inspected
- **Then** the anchor includes `rel="noopener noreferrer"` to prevent
  `window.opener` tab-nabbing and referrer leaks

### AC-3: Internal action links continue to work
- **Given** a message contains an internal action link such as a filter
  mention (`<a href="#" class="filter-mention">`) or document group link
  (`<a href="#" class="doc-group-link">`)
- **When** the user clicks it
- **Then** it does **NOT** open a new blank tab
- **And** the existing click handlers / styles continue to apply

### AC-4: Existing citation links keep their behaviour
- **Given** a message contains a document citation
  (rendered as `<a target="_blank" class="doc-citation">`)
- **When** the user clicks it
- **Then** it still opens the document in a new tab (no regression)

### AC-5: Applies to both spec task chat and session chat
- **Given** the renderer change is in `Markdown.tsx`
- **When** the user views markdown content in either
  `InlineCommentBubble`, `CommentLogSidebar`, or `InteractionInference`
- **Then** all three surfaces gain the new-tab behaviour from a single
  code change (single shared component, single source of truth)

## Out of Scope

- Changing where citation/filter/group-link click handlers navigate to
- Adding link previews, hover cards, or other rich-link affordances
- Markdown rendering anywhere outside `InteractionMarkdown` (e.g. the
  documentation viewer, the welcome screen)
