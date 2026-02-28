# Implementation Tasks

## Primary Fix: Zed Grep Tool

- [ ] Add `MAX_LINE_CHARS` constant (500 chars) in `zed/crates/agent/src/tools/grep_tool.rs`
- [ ] Create `truncate_long_lines(text: &str, max_chars: usize) -> String` helper function
- [ ] Modify output section (~line 314) to use `truncate_long_lines` before extending output
- [ ] Add test case with a long line (>500 chars) verifying truncation and indicator
- [ ] Add test case verifying short lines pass through unchanged

## Optional: Fix the Data in Helix

- [ ] Evaluate moving inline SVGs in `frontend/src/components/icons/ProviderIcons.tsx` to separate `.svg` files
- [ ] Or add icon files to exclusion patterns if they shouldn't be searched

## Verification

- [ ] Run existing grep tool tests: `cargo test -p agent grep`
- [ ] Manual test: grep for a pattern that matches the long SVG lines, verify output is bounded
- [ ] Verify context window no longer blows up when searching Helix codebase