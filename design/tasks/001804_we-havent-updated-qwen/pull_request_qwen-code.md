# Merge upstream QwenLM/qwen-code v0.14.4

## Summary
Merges upstream `QwenLM/qwen-code` main (v0.14.4) into our fork, which was pinned at v0.4.1. This is a ~10 major version jump bringing in ~2800 upstream commits with significant improvements to ACP protocol support (now using `@agentclientprotocol/sdk` v0.14.1), new features (fork subagents, startup profiler, hooks), and bug fixes.

## Changes
- Resolved 11 merge conflicts — took upstream for 6 files, deleted 2 files upstream removed (`schema.ts`, `smart-edit.ts`), manually merged 3 files
- Cleaned up fork debug logging (`console.error` statements) from 4 files that the linter flagged
- Preserved fork-specific changes: bind-mount path normalization, `QWEN_DATA_DIR` env var, shell prompt customizations (is_background, XML tool call suppression), session history callId queue fix
- Dropped fork changes superseded by upstream: custom ACP Zod schema types, ACP v0.10.0 alignment, debug logging (6 commits), `[object Object]` error fix, shell security disabling

## What was kept from our fork
- `normalizeProjectPath()` in `paths.ts` — handles bind-mount path equivalence for containerized environments
- `normalizeProjectPath` usage in `chatRecordingService.ts` — ensures session hashing works across bind-mount paths
- `QWEN_DATA_DIR` in `storage.ts` — persistent storage location override (consider migrating to upstream's `QWEN_RUNTIME_DIR`)
- Prompt customizations in `prompts.ts` — `is_background` parameter instruction and XML tool call suppression
- `getErrorMessage` import retained in `ls.ts` where still used

## What was dropped
- `schema.ts` — custom Zod-based ACP type definitions, superseded by `@agentclientprotocol/sdk` types
- `smart-edit.ts` — tool removed by upstream entirely
- Shell security disabling — upstream now has a mature AST-based shell checker; can be re-applied as config if needed
- All debug `console.error` logging — 6 commits worth of temporary debugging statements

## Notes for review
- `QWEN_DATA_DIR` env var may want to be migrated to upstream's `QWEN_RUNTIME_DIR` pattern in a follow-up
- Shell security checks are now re-enabled (upstream approach) — verify this works in our sandbox environment
- Build and all 188 tests pass
