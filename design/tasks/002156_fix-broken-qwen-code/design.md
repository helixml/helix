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

## CI Coverage Gap (why this reached `main`)

The qwen bundle build only runs in the `build-sandbox-amd64` / `build-sandbox-arm64`
pipelines, which trigger **only on `refs/heads/main` and `refs/tags/*`**. PR/branch
pushes run only the `default` pipeline, so a Dockerfile/bundle breakage is invisible
until it merges to main. Same gap applies to `build-controlplane-*`.

### Pipeline trigger map (`.drone.yml`)

| Pipeline | Trigger | Notes |
|---|---|---|
| `default` | every `push`/`tag` | tests, frontend, api binary |
| `build-controlplane-amd64/arm64` | main + tags only | builds + **publishes** controlplane image |
| `build-sandbox-amd64/arm64` | main + tags only | qwen + zed + desktops + sandbox |
| `build-macos-dmg` | **tags only** | heaviest; provisions VM, builds Mac app |

### Timing of `build-sandbox-*` (build #1900, `ZED_COMMIT` unchanged → zed cached)

| Step | Duration | Cost notes |
|---|---|---|
| `build-zed` | ~seconds (cache hit) | full Rust build ~12 min only when `ZED_COMMIT` bumped |
| `build-qwen-code` | ~10s | BuildKit npm cache; the step we fixed |
| `zed-e2e-test` | **~7 min, always runs** | builds Go test image + 10-phase E2E for zed-agent **and** Claude; needs `ANTHROPIC_API_KEY` → real LLM spend every run |
| `build-desktops` | ~few min | docker build embedding zed + qwen |
| `build-sandbox` | ~1–2 min | assembles sandbox image |
| `push-sandbox` | publish | must stay main/tags-only |

Runs as two parallel pipelines (amd64 + arm64).

### Decision (pending user confirmation)

Run the **build** steps on PR branches so build/bundle breakage is caught pre-merge,
while keeping publish/push and the Mac DMG release-only. Open question: whether to
also run `zed-e2e-test` on every PR push (correctness vs ~7 min + per-push Anthropic
API cost). Leaning: include all build steps; gate `zed-e2e-test` + all publish/push
steps to main/tags. (To be finalised before editing `.drone.yml`.)
