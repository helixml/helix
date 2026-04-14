# Implementation Tasks

## Preparation

- [x] Add upstream remote: `git remote add upstream https://github.com/QwenLM/qwen-code.git`
- [x] Fetch upstream: `git fetch upstream`
- [x] Create feature branch: `git checkout -b feature/001804-we-havent-updated-qwen`
- [x] Review upstream's ACP SDK types — upstream uses `@agentclientprotocol/sdk` types directly, our `schema.ts` is fork-only

## Merge

- [~] Run `git merge upstream/main --no-commit` and inventory conflicts
- [ ] Resolve all conflicts — prefer upstream, re-apply only fork-specific changes (sandbox security, path normalization, prompt tweaks)
- [ ] Drop debug logging commits (they'll be superseded by upstream's approach)

## Validation

- [ ] Run `npm install` successfully
- [ ] Run `npm run build` successfully
- [ ] Run existing tests (`npm test`)

## Finalize

- [ ] Commit the merge with a clear message documenting what was kept/dropped/adapted
- [ ] Push to origin and create PR description
