# Implementation Tasks

- [x] Verify root cause: trace how kodit resolves the ORT library path — found that `resolveORTLibDir()` in kodit resolves to `/lib` because the binary is at `/helix`, and the library is genuinely missing from the image
- [x] Dockerfile: add `ENV ORT_LIB_DIR=/usr/lib` to the production image stage to bypass fragile auto-detection
- [x] Dockerfile: add `ENV ORT_LIB_DIR=/usr/lib` to the dev stage for consistency
- [x] `api/pkg/server/kodit_init.go`: add a pre-flight check before `kodit.New()` that verifies the ORT library exists and returns a clear error message if missing
- [~] Build the image locally and verify the controlplane starts with `KODIT_ENABLED=true`
- [ ] Push and verify CI (Drone) builds and passes
- [ ] Write PR description
