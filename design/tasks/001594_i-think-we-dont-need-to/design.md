# Design: Restrict Sandbox CI Build Trigger

## Current State

In `.drone.yml`, the sandbox pipelines use an `event`-based trigger:

```yaml
# build-sandbox-amd64 (line ~1815) and build-sandbox-arm64 (line ~2248)
trigger:
  event:
    - push
    - tag
```

This fires on every push to every branch.

## Target State

Match the runner pipeline pattern:

```yaml
trigger:
  ref:
    include:
      - refs/heads/main
      - refs/tags/*
```

## Key Decision

Using `ref` with an `include` list is the correct Drone CI pattern for "main + tags only". The runner pipelines already use this pattern. The `manifest-sandbox` pipeline (which runs after both sandbox builds) already uses this pattern too — so it will naturally only run when the sandbox builds run.

## Files to Change

- `.drone.yml` — two trigger blocks to update:
  1. `build-sandbox-amd64` pipeline trigger (around line 1815)
  2. `build-sandbox-arm64` pipeline trigger (around line 2248)

## Codebase Pattern Note

The runner pipelines (`build-runner`, `build-runner-small`, `build-runner-large`) all use `trigger.ref.include` with `refs/heads/main` and `refs/tags/*` as the gating pattern for "main+tags only" builds. Follow the same pattern for sandbox.
