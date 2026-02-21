# Design: Run ./stack build-sandbox Timing

## Overview

Execute the existing `./stack build-sandbox` command and capture its timing output. This is a read-only operation with no code modifications.

## What build-sandbox Does

The `build-sandbox` function in `helix/stack` performs these steps:

1. **Check Zed binary** - Verifies `./zed-build/zed` exists (builds if missing)
2. **Build production desktops** - Builds `helix-sway` and `helix-ubuntu` images
3. **Build helix-sandbox container** - Docker build from `Dockerfile.sandbox`
4. **Start sandbox and transfer images** - Restarts sandbox container and transfers desktop images via local registry

The script has built-in timing using `$SECONDS` and prints elapsed time at each major step with `[⏱️  +Xs]` markers.

## Execution Approach

Run the command directly from the helix directory:

```bash
cd /home/retro/work/helix
./stack build-sandbox 2>&1 | tee /tmp/build-sandbox-output.log
```

The output will include:
- Per-step timing markers throughout the build
- Final summary: `🎉 Sandbox build completed successfully! (total: Xs)`

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Capture method | Direct execution with tee | Preserves real-time output while logging |
| Desktop scope | Production only (sway, ubuntu) | Default behavior, no EXPERIMENTAL_DESKTOPS set |
| Code changes | None | Task explicitly requires no modifications |

## Risks

- **Long build time**: Desktop image builds can take 10-30+ minutes depending on cache state
- **Disk space**: Desktop images are large; ensure sufficient space available
- **Docker state**: Requires running Docker daemon and compose services