# Implementation Tasks: Fix Broken Qwen Code Bundle Build in CI

- [x] In `helix/Dockerfile.qwen-build`, add `npm run build --workspaces --if-present` after `npm ci --ignore-scripts` and before `npm run bundle`
- [x] In `helix/Dockerfile.qwen-code-build`, add `npm run build --workspaces --if-present` after `npm ci --ignore-scripts` and before `npm run bundle`
- [x] Push to a feature branch and verify the `build-qwen-code` Drone step passes on both amd64 and arm64
- [x] Run the source-build steps (`clone-dependencies`, `build-qwen-code`, `build-zed`) of `build-sandbox-amd64` on PR branches by switching its trigger to `event: [push, tag]`, and gate `zed-e2e-test`, `build-desktops`, `build-sandbox`, `push-sandbox` to `refs/heads/main` + `refs/tags/*` (so PRs build/bundle but never run E2E or push)
- [~] Verify the amd64 sandbox pipeline runs the build steps (and skips e2e/push) on the feature branch
- [ ] Merge to main
