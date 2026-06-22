# Implementation Tasks: Fix Broken Qwen Code Bundle Build in CI

- [x] In `helix/Dockerfile.qwen-build`, add `npm run build --workspaces --if-present` after `npm ci --ignore-scripts` and before `npm run bundle`
- [x] In `helix/Dockerfile.qwen-code-build`, add `npm run build --workspaces --if-present` after `npm ci --ignore-scripts` and before `npm run bundle`
- [x] Push to a feature branch and verify the `build-qwen-code` Drone step passes on both amd64 and arm64
- [~] Add a `smoke-test-qwen-code` step to the `default` pipeline so the qwen-code bundle build is exercised on every branch/PR (not just main/tag releases)
- [ ] Verify the smoke-test step runs and passes on the feature branch
- [ ] Merge to main
