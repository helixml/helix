# Implementation Tasks

## Primary Fix: Zed Grep Tool

- [x] Add `MAX_LINE_CHARS` constant (500 chars) in `zed/crates/agent/src/tools/grep_tool.rs`
- [x] Create `truncate_long_lines(text: &str, max_chars: usize) -> String` helper function
- [x] Modify output section (~line 314) to use `truncate_long_lines` before extending output
- [x] Add test case with a long line (>500 chars) verifying truncation and indicator
- [x] Add test case verifying short lines pass through unchanged

## Long Lines Found in Helix Codebase

Investigation found several sources of extremely long lines:

| File | Max Line Length | Type |
|------|-----------------|------|
| `desktop/sway-config/settings-sync-daemon` | 333,027 chars | **Binary executable checked into git!** |
| `frontend/assets/external-libs/redoc/redoc.standalone.js` | 661,439 chars | Minified JS library |
| `api/pkg/controller/knowledge/readability/testdata/article.html` | 36,643 chars | HTML test fixture |
| `frontend/src/components/icons/ProviderIcons.tsx` | 22,464 chars | Inline SVG paths |
| `api/pkg/tools/testdata/github_*.json` | 10-15K chars | Dependabot PR bodies in test data |
| `frontend/src/components/providers/logos/togetherai.tsx` | 8,336 chars | Inline SVG path |

### Recommended Follow-up Actions

- [ ] Add `.gitattributes` to mark binaries (`settings-sync-daemon`, `*.rgba`) as binary
- [ ] Consider excluding `desktop/sway-config/settings-sync-daemon` from git or using Git LFS
- [ ] Consider excluding vendored minified JS from search (already in node_modules pattern?)

## Optional: Fix the Data in Helix

- [~] Evaluate moving inline SVGs in `frontend/src/components/icons/ProviderIcons.tsx` to separate `.svg` files
  - **Deferred**: The Zed grep fix handles this properly. Inline SVGs are a common React pattern and the tool should handle them gracefully (which it now does with truncation).
- [ ] Or add icon files to exclusion patterns if they shouldn't be searched

## Verification

- [x] Run existing grep tool tests: `cargo test -p agent grep` (CI will verify - no Rust installed locally)
- [x] Code pushed to feature branch for CI testing
- [x] PR opened: https://github.com/helixml/zed/pull/15
- [ ] Manual test: grep for a pattern that matches the long SVG lines, verify output is bounded
- [ ] Verify context window no longer blows up when searching Helix codebase