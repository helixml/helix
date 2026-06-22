# fix(ci): build qwen workspace packages before bundling

## Summary

`npm run bundle` was failing on both amd64 and arm64 sandbox pipelines (Drone build #2350) with esbuild errors like:

```
Could not resolve "@qwen-code/channel-telegram"
Could not resolve "@qwen-code/channel-weixin"
Could not resolve "@qwen-code/channel-dingtalk"
Could not resolve "./generated/exportHtmlTemplate.js"
```

The upstream qwen-code v0.14.4 merge added several TypeScript workspace packages whose `dist/` directories are gitignored. The Dockerfiles used `npm ci --ignore-scripts`, which prevents workspace `build` scripts from running — so no `dist/index.js` was ever produced for those packages before esbuild tried to import them.

## Changes

**The fix**
- `Dockerfile.qwen-build`: add `npm run build --workspaces --if-present` between `npm ci --ignore-scripts` and `npm run bundle`
- `Dockerfile.qwen-code-build`: same fix in the two-stage build (after install, before bundle)

`--if-present` makes this a no-op for workspaces without a `build` script, keeping it safe for future package additions.

**Catch it on PRs (`.drone.yml`)**

The qwen build only ran in `build-sandbox-*`, which triggered on `main` + tags only — so this breakage was invisible until merge (it failed identically on the regular main merge #2350 and the release build #2384). Shift it left:

- `build-sandbox-amd64` trigger changed to `event: [push, tag]` so it also runs on PR branches.
- Source-build steps run on PRs: `clone-dependencies`, `build-qwen-code`, `build-zed`. (`build-zed` is a cache hit keyed by `ZED_COMMIT`, so it only does the full Rust build when a PR bumps the pin.)
- Heavy/publishing steps gated to `main` + tags via per-step `when:`: `zed-e2e-test` (real LLM cost), `build-desktops` / `build-sandbox` / `push-sandbox` (registry pushes).
- `build-sandbox-arm64` unchanged (main/tags-only) to avoid loading the single macOS runner on every PR.
