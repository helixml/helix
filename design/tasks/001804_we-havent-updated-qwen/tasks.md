# Implementation Tasks

## Preparation

- [x] Add upstream remote: `git remote add upstream https://github.com/QwenLM/qwen-code.git`
- [x] Fetch upstream: `git fetch upstream`
- [x] Create feature branch: `git checkout -b feature/001804-we-havent-updated-qwen`
- [x] Review upstream's ACP SDK types — upstream uses `@agentclientprotocol/sdk` types directly, our `schema.ts` is fork-only

## Merge

- [x] Run `git merge upstream/main --no-commit` and inventory conflicts (11 conflicted files)
- [x] Resolve all conflicts:
  - Take upstream: acpAgent.ts, cli/errors.ts, edit.ts, glob.ts, shell-utils.ts, tools.ts
  - Delete (upstream removed): schema.ts, smart-edit.ts
  - Manual merge: chatRecordingService.ts (both imports), ls.ts (both imports), paths.ts (keep normalizeProjectPath + add sanitizeCwd)
- [x] Clean up fork debug logging (console.error in HistoryReplayer.ts, Session.ts, config.ts, sessionService.ts)

## Validation

- [x] Run `npm install` successfully
- [x] Run `npm run build` successfully
- [x] Run existing tests (`npm test`) — 28 test files, 188 tests pass

## Finalize

- [x] Commit the merge with detailed message documenting what was kept/dropped/adapted
- [x] Push to origin: `feature/001804-we-havent-updated-qwen`
- [x] Create PR description
