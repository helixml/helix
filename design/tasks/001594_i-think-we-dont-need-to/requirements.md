# Requirements: Restrict Sandbox CI Build Trigger

## User Story

As a developer, I want the sandbox Docker image to only build on pushes to `main` and on tags — not on every branch commit — so that CI runs significantly faster for feature branch work.

## Background

The `build-runner`, `build-runner-small`, and `build-runner-large` pipelines already restrict their triggers to `refs/heads/main` and `refs/tags/*`. The sandbox pipelines (`build-sandbox-amd64`, `build-sandbox-arm64`) currently trigger on every push to every branch, making CI slow.

## Acceptance Criteria

- `build-sandbox-amd64` pipeline does NOT run on pushes to feature branches
- `build-sandbox-arm64` pipeline does NOT run on pushes to feature branches
- Both sandbox pipelines continue to run on pushes to `main` and on tags
- The `manifest-sandbox` pipeline is unchanged (it already uses the correct trigger)
