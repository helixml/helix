# Design: Fix Broken Qwen Code Bundle Build in CI

## Root Cause

`dist/` is listed in `.gitignore` in the qwen-code repo. When CI clones the repo and runs `npm ci --ignore-scripts`, the TypeScript workspace packages (`channel-telegram`, `channel-weixin`, `channel-dingtalk`, `web-templates`, `channel-base`) are installed but their `build` scripts never run — so no `dist/index.js` is produced. `npm run bundle` then fails immediately when esbuild tries to import those packages.

This was latent before v0.14.4 because there were no workspace packages that needed pre-compilation. The v0.14.4 merge introduced the channel packages and `web-templates`, exposing the gap.

## Fix

Add an explicit workspace build step **after** `npm ci --ignore-scripts` and **before** `npm run bundle` in both Dockerfiles in the helix repo:

```dockerfile
npm run build --workspaces --if-present
```

`--if-present` makes the command a no-op for workspaces that have no `build` script, keeping it future-safe. `--ignore-scripts` on the install phase is still kept — it only prevents lifecycle scripts during `npm ci` (husky's `prepare`), not subsequent explicit `npm run` calls.

## Files to Change

- `helix/Dockerfile.qwen-build` — used by CI (`build-qwen-code` drone step). Add workspace build between install and bundle.
- `helix/Dockerfile.qwen-code-build` — used by `./stack build-qwen-code` locally. Same fix; note the two-stage copy pattern means the workspace build must happen after the full `COPY . .`.

## Why Not Use `HUSKY=0 npm ci`?

Dropping `--ignore-scripts` in favour of `HUSKY=0` would allow all lifecycle scripts to run (not just `build`). Some upstream packages run `postinstall` scripts (native bindings, code generators) that are slow or require network access. The current approach of `--ignore-scripts` + explicit workspace build is minimal and targeted.

## Notes

- `channel-base` must be built before `channel-telegram`/`channel-weixin`/`channel-dingtalk` since they depend on it. `npm run build --workspaces` runs packages in dependency order (npm resolves the workspace dependency graph), so this is handled automatically.
- No change to `qwen-code` repo or `sandbox-versions.txt` is needed — the bug is in the helix Dockerfiles, not the source.
