# Implementation Tasks: Fix Broken Qwen Code Bundle Build in CI

- [x] In `helix/Dockerfile.qwen-build`, add `npm run build --workspaces --if-present` after `npm ci --ignore-scripts` and before `npm run bundle`
- [x] In `helix/Dockerfile.qwen-code-build`, add `npm run build --workspaces --if-present` after `npm ci --ignore-scripts` and before `npm run bundle`
- [~] Push to a feature branch and verify the `build-qwen-code` Drone step passes on both amd64 and arm64
- [ ] Merge to main
