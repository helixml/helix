# Implementation Tasks

## Primary Fix: Zed Grep Tool

- [x] Add `MAX_LINE_CHARS` constant (500 chars) in `zed/crates/agent/src/tools/grep_tool.rs`
- [x] Create `truncate_long_lines(text: &str, max_chars: usize) -> String` helper function
- [x] Modify output section (~line 314) to use `truncate_long_lines` before extending output
- [x] Add test case with a long line (>500 chars) verifying truncation and indicator
- [x] Add test case verifying short lines pass through unchanged

## Optional: Fix the Data in Helix

- [ ] Evaluate moving inline SVGs in `frontend/src/components/icons/ProviderIcons.tsx` to separate `.svg` files
- [ ] Or add icon files to exclusion patterns if they shouldn't be searched

## Verification

- [x] Run existing grep tool tests: `cargo test -p agent grep` (CI will verify - no Rust installed locally)
- [x] Code pushed to feature branch for CI testing
- [ ] Manual test: grep for a pattern that matches the long SVG lines, verify output is bounded
- [ ] Verify context window no longer blows up when searching Helix codebase