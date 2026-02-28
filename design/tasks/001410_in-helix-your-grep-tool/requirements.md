# Requirements: Grep Tool Line Length Limiting

## Problem Statement

The Zed grep tool can blow up the context window when it matches lines with extremely long content (e.g., inline SVG paths with 22,000+ characters). The tool currently limits output by **line count** but not by **character/byte count per line**.

### Root Cause

In the Helix codebase, `frontend/src/components/icons/ProviderIcons.tsx` contains inline SVG icon definitions with very long `<path d="...">` attributes:
- Line 33: 22,464 characters (a single SVG path)
- Multiple other lines exceed 1,000 characters

When grep matches common patterns (like `d=`, `path`, `svg`, etc.), it returns these massive lines verbatim, exhausting the LLM's context window.

## User Stories

1. **As an LLM agent**, I need grep results to be bounded in size so that a single unlucky match doesn't consume my entire context window.

2. **As a developer**, I want to see enough of a long line to understand the match context, but not the entire 22KB line.

## Acceptance Criteria

### Must Have

- [ ] Long lines in grep output are truncated to a reasonable character limit (e.g., 500-1000 chars)
- [ ] Truncated lines include an indicator showing they were truncated (e.g., `... [truncated, 22464 chars total]`)
- [ ] The truncation preserves context around the actual match position, not just the line start

### Should Have

- [ ] Configurable truncation limit (compile-time constant is acceptable)
- [ ] Total output size is also bounded (already partially addressed by `RESULTS_PER_PAGE`)

### Nice to Have

- [ ] Consider removing/refactoring the problematic SVG icons in Helix to use external files or a more compact format

## Alternative: Fix the Data

Instead of (or in addition to) fixing Zed, we could:
- Move inline SVGs to separate `.svg` files
- Use an icon library that doesn't embed SVG paths inline
- Add the icons file to `.gitattributes` to exclude from grep

This is a simpler fix for Helix specifically but doesn't solve the general problem for other codebases.