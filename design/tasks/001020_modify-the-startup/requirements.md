# Requirements: Rename Repo Directories in helix-specs Startup

## User Story

As a developer working on helix-in-helix development, I want the repo directories to be named consistently (`helix`, `zed`, `qwen-code`) so that the `./stack` script works correctly since it expects `../zed`, `../qwen-code` etc. to exist.

## Background

Currently, helix-specs may clone repos with numbered suffixes (e.g., `helix-1`, `zed-2`, `qwen-code-3`). However, the `./stack` script in helix uses `$PROJECTS_ROOT/zed` and `$PROJECTS_ROOT/qwen-code` paths, requiring consistent naming.

## Acceptance Criteria

1. **Startup script renames repos correctly**
   - `helix-1`, `helix-2`, etc. → `helix`
   - `zed-1`, `zed-2`, etc. → `zed`
   - `qwen-code-1`, `qwen-code-2`, etc. → `qwen-code`

2. **No hardcoded paths in helix codebase assume numbered names**
   - Verify prompts use generic names (`helix`, `zed`, `qwen-code`)
   - Verify desktop container scripts don't assume numbered names

3. **Idempotent operation**
   - Script should handle already-renamed repos gracefully
   - Script should handle repos that were never numbered

## Out of Scope

- Changing agent pool names (`zed-1`, `zed-2` in server.go are agent IDs, not repo names)
- Changing model names (`helix-3.5`, `helix-4` are LLM model identifiers)