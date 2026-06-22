# Requirements: Fix Broken Qwen Code Bundle Build in CI

## Problem

The `build-qwen-code` step fails on both amd64 and arm64 sandbox pipelines on the `main` branch (Drone build #2350). `esbuild` cannot resolve the following workspace packages during `npm run bundle`:

- `@qwen-code/channel-telegram`
- `@qwen-code/channel-weixin`
- `@qwen-code/channel-dingtalk`
- `@qwen-code/web-templates` (`./generated/exportHtmlTemplate.js`)

These packages were added as part of the upstream qwen-code v0.14.4 merge. Their `dist/` directories are gitignored and must be compiled from TypeScript source before bundling. The helix Dockerfiles use `npm ci --ignore-scripts`, which prevents workspace `build` scripts from running during install — so no `dist/` is ever produced, and the bundle fails.

## User Story

As a developer, when I push to `main`, the qwen-code sandbox build should succeed so the desktop image can be deployed.

## Acceptance Criteria

- `npm run bundle` in `Dockerfile.qwen-build` completes without esbuild resolution errors for channel or web-templates packages.
- `npm run bundle` in `Dockerfile.qwen-code-build` also completes cleanly.
- The `build-qwen-code` step passes on both amd64 and arm64 in Drone CI on `main`.
- `./stack build-qwen-code` continues to work locally.
- The husky `prepare` hook is still not executed during `npm ci` (no regression on that).
