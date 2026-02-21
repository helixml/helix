# Requirements: Build Sandbox Timing

## User Story

As a developer, I need to run `./stack build-sandbox` and capture timing information to understand build performance.

## Acceptance Criteria

- [x] Run `./stack build-sandbox` command successfully
- [x] Capture output to `/tmp/build-timing.log`
- [x] Extract timing markers (⏱️) from the log
- [x] Document the timing breakdown for each build phase

## Results

Build completed successfully in **1718 seconds (~28.6 minutes)**.

### Timing Breakdown

| Phase | Duration |
|-------|----------|
| Check Zed binary | 0s |
| Build helix-sway | 474s (8 min) |
| Build helix-ubuntu | 745s (12 min) |
| Build helix-sandbox | 72s |
| Transfer helix-sway | 149s |
| Transfer helix-ubuntu | 264s |