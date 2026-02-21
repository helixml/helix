# Requirements: Test Zed Builds Correctly with Shared Cache

## User Stories

1. As a developer, I want to verify that Zed builds correctly using the shared BuildKit cache mounts so I can trust incremental builds work.
2. As a CI system, I want to confirm the cached build produces a valid working binary so failed builds don't propagate.

## Acceptance Criteria

- [ ] Zed binary compiles successfully with `--features external_websocket_sync`
- [ ] Build uses shared cache directories (cargo registry, git deps, rustup toolchain, target dir)
- [ ] Output binary is executable and can print version info
- [ ] Build works in both `dev` and `release` modes
- [ ] Cache is correctly populated after first build
- [ ] Subsequent builds are faster due to cache hits

## Scope

**In scope:**
- Running `./stack build-zed` (dev and release)
- Verifying binary output at `./zed-build/zed`
- Checking cache mount directories are used

**Out of scope:**
- E2E WebSocket tests (covered by separate CI step)
- Running the full desktop image build