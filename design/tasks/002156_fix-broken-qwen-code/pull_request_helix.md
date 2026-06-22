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

- `Dockerfile.qwen-build`: add `npm run build --workspaces --if-present` between `npm ci --ignore-scripts` and `npm run bundle`
- `Dockerfile.qwen-code-build`: same fix in the two-stage build (after install, before bundle)

`--if-present` makes this a no-op for workspaces without a `build` script, keeping it safe for future package additions.
