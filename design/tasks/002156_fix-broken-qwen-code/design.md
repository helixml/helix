# Design: Fix Broken Qwen Code Bundle Build in CI

## Root Cause

`dist/` is listed in `.gitignore` in the qwen-code repo. When CI clones the repo and runs `npm ci --ignore-scripts`, the TypeScript workspace packages (`channel-telegram`, `channel-weixin`, `channel-dingtalk`, `web-templates`, `channel-base`) are installed but their `build` scripts never run — so no `dist/index.js` is produced. `npm run bundle` then fails immediately when esbuild tries to import those packages.

This was latent before v0.14.4 because there were no workspace packages that needed pre-compilation. The v0.14.4 merge introduced the channel packages and `web-templates`, exposing the gap.

## Fix

Add an explicit workspace build step **after** `npm ci --ignore-scripts` and **before** `npm run bundle` in both Dockerfiles in the helix repo, building **only the packages the bundle consumes as built modules**, in dependency order:

```dockerfile
npm run build \
  --workspace=@qwen-code/web-templates \
  --workspace=@qwen-code/channel-base \
  --workspace=@qwen-code/channel-telegram \
  --workspace=@qwen-code/channel-weixin \
  --workspace=@qwen-code/channel-dingtalk
```

`--ignore-scripts` on the install phase is still kept — it only prevents lifecycle scripts during `npm ci` (husky's `prepare`), not subsequent explicit `npm run` calls.

### Why NOT `npm run build --workspaces --if-present`

The first attempt used `--workspaces` (all packages). **It failed in CI (build #2396)** for two reasons:

1. **`npm run --workspaces` runs packages in directory order, not dependency order.** `packages/cli` built before `web-templates`/`webui`, failing with `TS2307: Cannot find module '@qwen-code/web-templates'`. It also requires `npm run generate` (for `generated/git-commit.js`) to have run first.
2. **It builds packages the bundle doesn't need** — `cli`, `webui`, `vscode-ide-companion`, `sdk-*` — and those have their own prerequisites (`vscode-ide-companion` needs `webui` built).

The canonical `scripts/build.js` confirms the real dependency order (core → web-templates → channels/base → channel adapters → cli → webui → sdk → vscode-ide-companion) and runs `npm run generate` first. But the **bundle** (`npm run bundle` = `generate` + esbuild + copy assets) bundles `cli`/`core` from *source* and only needs the workspace packages it imports as built modules — exactly `web-templates` + the channel adapters (the 9 esbuild errors in #2350 named only those). Those packages built cleanly in #2396; only the unneeded `cli`/`vscode` packages failed. So we build just that minimal set, in dependency order (`channel-base` before the adapters).

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

### Decision (final)

Run the **source-build steps** of `build-sandbox-amd64` on PR branches; keep
everything heavy or publishing on main + tags.

- `build-sandbox-amd64` trigger changed from `ref: [main, tags]` to `event: [push, tag]`.
- **Ungated (run on PRs too):** `clone-dependencies`, `build-qwen-code`, `build-zed`.
  These push nothing. `build-qwen-code` catches the exact bug class. `build-zed` is a
  cache hit keyed by `ZED_COMMIT` (build #1900: 8-line `cp` from cache), so it only does
  the ~12 min Rust build when a PR bumps the zed pin — exactly when you want it validated.
- **Gated to `refs/heads/main` + `refs/tags/*` via per-step `when:`:** `zed-e2e-test`
  (real LLM cost), `build-desktops` (pushes to registry), `build-sandbox` (needs
  build-desktops' image refs), `push-sandbox` (publishes).
- `build-sandbox-arm64` left unchanged (main + tags only) — one macOS Docker Desktop
  runner shouldn't take every PR; the qwen Dockerfile is arch-agnostic for this failure
  so amd64 coverage is sufficient.
- `build-macos-dmg` unchanged (tags-only).

**Why not run `build-desktops`/`build-sandbox` on PRs too:** they are coupled to
registry pushes (`build-desktops` does `docker login` + `push` inline and writes the
`sandbox-images/*.ref` files that `build-sandbox` bakes in). Covering the desktop/sandbox
*image* build on PRs would need a build/push split refactor of the release path — out of
scope here. Residual gap: breakage in `Dockerfile.ubuntu-helix` / `Dockerfile.sway-helix`
/ `Dockerfile.sandbox` still only surfaces on main. The reported bug (qwen bundle) is
fully covered.
