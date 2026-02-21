# Design: Build Sandbox Timing

## Overview

This task ran `./stack build-sandbox` with timing capture to measure build performance.

## Approach

Simple command execution with output capture via `tee` and timing extraction via `grep`.

## Build Pipeline Architecture

The `build-sandbox` command executes a multi-stage build:

1. **Zed Binary Check** - Uses existing binary if available
2. **Desktop Images** - Builds helix-sway and helix-ubuntu containers
3. **Sandbox Container** - Builds the DinD (Docker-in-Docker) sandbox
4. **Image Transfer** - Pushes desktop images to local registry, pulls into sandbox

## Timing Results

| Stage | Duration |
|-------|----------|
| helix-sway build | 474s (~8 min) |
| helix-ubuntu build | 745s (~12 min) |
| helix-sandbox build | 72s |
| Image transfers | 413s (~7 min) |
| **Total** | **1718s (~29 min)** |

## Key Observations

- Desktop image builds are the slowest steps (20 min combined)
- Image transfer via registry adds ~7 minutes overhead
- Builds use existing Zed binary and qwen-code when unchanged (incremental)
- BuildKit cache mounts significantly reduce rebuild times

## No Code Changes Required

This was a diagnostic/measurement task only.