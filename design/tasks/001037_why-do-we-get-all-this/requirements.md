# Requirements: Markdown Placeholder Leakage Bug

## Problem Statement

During streaming responses, users see corrupted placeholder text in rendered markdown:
- `__INLINE_CODE_43__ensure_go()__INLINE_CODE_44__setup_dev_networking` instead of properly rendered inline code
- `CITATION_DATA{"excerpts":[],"isStreaming":false}CITATION_DATA` appearing as visible text (note: underscores stripped)

## User Stories

1. **As a user**, I want streaming markdown to render inline code correctly, so that I can read responses without seeing internal placeholder tokens.

2. **As a user**, I want citation metadata to be invisible, not displayed as raw text in the response.

## Root Cause Analysis

There are **two distinct bugs** causing placeholder leakage:

### Bug 1: CITATION_DATA markers not being extracted

**Symptom**: User sees `CITATION_DATA{"excerpts":[],"isStreaming":false}CITATION_DATA` (without underscores)

**Flow**:
1. `MessageProcessor.process()` adds `__CITATION_DATA__${json}__CITATION_DATA__` at end of message
2. `processContent()` in `InteractionMarkdown` tries to extract with regex `/__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/`
3. **Regex fails to match for unknown reason**
4. Markers remain in content passed to `<Markdown>` component
5. react-markdown with remarkGfm interprets `__text__` as bold syntax
6. Underscores are consumed, leaving `CITATION_DATA...CITATION_DATA` visible

**Additional bug in error handling**: In the catch block of `processContent()`, if JSON.parse fails, the code sets `setCitationData(null)` but does NOT remove the markers from content.

### Bug 2: INLINE_CODE placeholders not being restored

**Symptom**: User sees `__INLINE_CODE_43__ensure_go()__INLINE_CODE_44__` with underscores intact

**Flow**:
1. `sanitizeHtml()` replaces `` `code` `` with `__INLINE_CODE_N__` placeholders
2. DOMPurify.sanitize() runs
3. Restore loop: `processedMessage.replace(__INLINE_CODE_${index}__, code)`
4. **Replace fails for unknown reason** - placeholder remains in output
5. Content passes to react-markdown (underscores remain visible in this case)

**Key observation**: The placeholder indices are 43 and 44, meaning the message has 45+ inline code blocks. This suggests the bug may be related to messages with many inline code blocks.

## Investigation Needed

The exact failure modes need to be confirmed with failing tests:
- Why does the CITATION_DATA regex fail to match?
- Why does the INLINE_CODE replace() fail to restore placeholders?
- Is DOMPurify modifying the placeholder text somehow?
- Is there a race condition in streaming updates?

## Acceptance Criteria

- [ ] Placeholder tokens (`__INLINE_CODE_N__`, `__CODE_BLOCK_N__`, `__CITATION_DATA__`) never appear in rendered output
- [ ] Inline code renders correctly during streaming and final state
- [ ] Citation data markers are invisible to users
- [ ] Error handling in processContent() removes markers even when JSON parsing fails
- [ ] Solution handles messages with many (50+) inline code blocks
- [ ] No regression in existing markdown rendering functionality