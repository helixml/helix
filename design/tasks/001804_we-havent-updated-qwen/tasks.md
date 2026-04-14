# Implementation Tasks

## Preparation

- [~] Add upstream remote: `git remote add upstream https://github.com/QwenLM/qwen-code.git`
- [ ] Fetch upstream: `git fetch upstream`
- [ ] Create a merge branch: `git checkout -b merge-upstream-v0.14.4`
- [ ] Review upstream's ACP SDK types to decide if our `schema.ts` is still needed

## Merge

- [ ] Run `git merge upstream/main --no-commit` and inventory conflicts
- [ ] Resolve ACP integration conflicts (`acpAgent.ts`, `schema.ts`, `HistoryReplayer.ts`, `Session.ts`) — prefer upstream, re-apply only KEEP/ADAPT changes
- [ ] Resolve shell security conflicts (`shellReadOnlyChecker.ts`, `shell-utils.ts`) — re-apply sandbox security disabling on top of upstream's AST-based checker
- [ ] Resolve storage conflict (`storage.ts`) — migrate `QWEN_DATA_DIR` to `QWEN_RUNTIME_DIR` or add alongside it
- [ ] Resolve tool conflicts (`edit.ts`, `glob.ts`, `ls.ts`, `write-file.ts`, `smart-edit.ts`) — prefer upstream, check if our error handling improvements are still needed
- [ ] Resolve prompt/config conflicts (`prompts.ts`, `config.ts`) — re-apply `is_background` and XML tool call instructions if still needed
- [ ] Resolve path normalization (`paths.ts`) — keep bind-mount normalization if not upstream
- [ ] Drop all debug logging commits (6 commits) — not needed in merged code

## Validation

- [ ] Run `npm install` successfully
- [ ] Run `npm run build` successfully
- [ ] Run existing tests (`npm test`)
- [ ] Run ACP integration tests
- [ ] Manually verify sandbox security disabling works in container environment

## Finalize

- [ ] Commit the merge with a clear message documenting what was kept/dropped/adapted
- [ ] Push to origin and create PR for review
