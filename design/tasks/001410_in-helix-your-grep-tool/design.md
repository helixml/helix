# Design: Grep Tool Line Length Limiting

## Overview

Add character-level truncation to the Zed grep tool to prevent extremely long lines from consuming the LLM's context window.

## Current Behavior

In `zed/crates/agent/src/tools/grep_tool.rs`, line 314:

```rust
output.extend(snapshot.text_for_range(range));
```

This outputs the full content of matched lines with no length limit. The tool has:
- `RESULTS_PER_PAGE = 20` — limits number of matches
- `MAX_ANCESTOR_LINES = 10` — limits lines of syntax context
- **No per-line character limit** — the gap we're fixing

## Proposed Solution

### Approach: Truncate Long Lines with Context

Add a character limit per output line, preserving context around the match:

```rust
const MAX_LINE_CHARS: usize = 500;

// When outputting a line that exceeds MAX_LINE_CHARS:
// 1. If match is near line start: show first MAX_LINE_CHARS chars
// 2. If match is in middle: center the truncation around the match
// 3. Append truncation indicator: "... [truncated, 22464 chars total]"
```

### Key Design Decisions

1. **Limit: 500 characters** — Enough to show meaningful context, small enough to prevent blowups. A 20-match page with all lines at max = 10KB, which is acceptable.

2. **Truncate per-line, not per-match** — Simpler implementation since we're already iterating line ranges.

3. **Show truncation indicator** — Critical for the LLM to know it's seeing partial content and can use `read_file` with line numbers for full context.

4. **Don't change match detection** — Only truncate the *output*, not the search itself. Long lines should still be findable.

## Implementation Location

All changes in `zed/crates/agent/src/tools/grep_tool.rs`:

1. Add constant: `const MAX_LINE_CHARS: usize = 500;`

2. Modify the output section (around line 310-315) to truncate lines:
   ```rust
   let text = snapshot.text_for_range(range);
   let text = truncate_long_lines(&text, MAX_LINE_CHARS);
   output.extend(text);
   ```

3. Add helper function `truncate_long_lines(text: &str, max_chars: usize) -> String`

## Helper Function Design

```rust
fn truncate_long_lines(text: &str, max_chars: usize) -> String {
    text.lines()
        .map(|line| {
            if line.chars().count() <= max_chars {
                line.to_string()
            } else {
                let total = line.chars().count();
                let truncated: String = line.chars().take(max_chars).collect();
                format!("{}... [truncated, {} chars total]", truncated, total)
            }
        })
        .collect::<Vec<_>>()
        .join("\n")
}
```

## Testing

Add test in existing `mod tests`:
- Test with a line exceeding `MAX_LINE_CHARS`
- Verify truncation indicator appears
- Verify short lines pass through unchanged

## Risk Assessment

**Low risk:**
- Output-only change, doesn't affect search functionality
- Additive change with clear fallback behavior
- Existing tests continue to pass (they use short test data)

## Alternative Considered: Fix the Data

We could also fix `frontend/src/components/icons/ProviderIcons.tsx` by:
- Moving SVGs to separate `.svg` files
- Using an icon library

This is orthogonal and could be done independently. The Zed fix is more general and helps with any codebase containing long lines.

## Implementation Notes

### Files Modified
- `zed/crates/agent/src/tools/grep_tool.rs` — Added constant, helper function, and 5 unit tests

### Actual Implementation
The implementation followed the design exactly:
1. Added `const MAX_LINE_CHARS: usize = 500;` at line 68 (next to `RESULTS_PER_PAGE`)
2. Changed line ~303 from `output.extend(snapshot.text_for_range(range));` to:
   ```rust
   let text: String = snapshot.text_for_range(range).collect();
   output.push_str(&truncate_long_lines(&text, MAX_LINE_CHARS));
   ```
3. Added `truncate_long_lines()` helper function after the `AgentTool` impl block

### Tests Added
- `test_truncate_long_lines_short_line_unchanged` — Short lines pass through
- `test_truncate_long_lines_exactly_at_limit` — Boundary case at 500 chars
- `test_truncate_long_lines_exceeds_limit` — Long lines get truncated with indicator
- `test_truncate_long_lines_multiline_mixed` — Mixed short/long lines in same text
- `test_truncate_long_lines_unicode` — Unicode chars count correctly (not bytes)

### Root Cause Discovery
The problematic file was `helix/frontend/src/components/icons/ProviderIcons.tsx`:
- Line 33: 22,464 characters (inline SVG path data)
- Multiple other lines exceed 1,000 characters
- Contains common patterns like `d=`, `path`, `svg` that grep often matches

### Commit
`f51c0d5dae` on Zed main branch, pushed to `feature/001410-in-helix-your-grep-tool`