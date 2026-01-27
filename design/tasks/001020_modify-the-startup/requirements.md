# Requirements: Rename Repo Directories in helix-specs Startup

## User Story

As a developer working on helix-in-helix development, I want the repo directories to be named consistently (`helix`, `zed`, `qwen-code`) so that the `./stack` script works correctly since it expects `../zed`, `../qwen-code` etc. to exist.

## Background

When a user already has repos with names like `helix`, `zed`, `qwen-code`, the API's `CreateRepository` function auto-increments to avoid conflicts: `helix-1`, `zed-1`, `qwen-code-1`, then `helix-2`, etc.

The `./stack` script in helix uses `$PROJECTS_ROOT/zed` and `$PROJECTS_ROOT/qwen-code` paths, requiring consistent naming.

**Root cause**: `api/pkg/services/git_repository_service.go` function `CreateRepository` (lines 135-143) auto-increments repo names to avoid duplicates.

## Acceptance Criteria

1. **Startup script in helix-specs renames repos correctly**
   - `helix-1`, `helix-2`, etc. → `helix`
   - `zed-1`, `zed-2`, etc. → `zed`
   - `qwen-code-1`, `qwen-code-2`, etc. → `qwen-code`

2. **Idempotent operation**
   - Script should handle already-renamed repos gracefully
   - Script should handle repos that were never numbered

3. **No hardcoded paths in helix codebase assume numbered names**
   - Verified: prompts use generic names (`helix`, `zed`, `qwen-code`)
   - Verified: desktop container scripts don't assume numbered names

## Out of Scope

- Changing the API's auto-increment logic (that's a separate concern for avoiding DB conflicts)
- Changing agent pool names (`zed-1`, `zed-2` in server.go are agent IDs, not repo names)
- Changing model names (`helix-3.5`, `helix-4` are LLM model identifiers)