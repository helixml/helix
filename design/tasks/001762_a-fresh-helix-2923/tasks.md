# Implementation Tasks

- [~] Verify root cause: run the 2.9.23 production image and confirm `libonnxruntime.so` is missing from `/lib/` and/or `/usr/lib/` — determine whether the file is absent entirely or just at the wrong path
- [ ] Dockerfile: add `RUN ln -sf /usr/lib/libonnxruntime.so /lib/libonnxruntime.so` after the ORT library COPY in the production image stage (line ~126)
- [ ] Dockerfile: apply the same symlink fix to the dev stage (line ~49) for consistency
- [ ] `api/pkg/server/kodit_init.go`: add a pre-flight check before `kodit.New()` that stats the expected ORT library path and returns a clear error message if missing (e.g., "kodit is enabled but libonnxruntime.so not found — ensure image was built with ORT stage")
- [ ] Build the image locally and verify the controlplane starts with `KODIT_ENABLED=true`
- [ ] Push and verify CI (Drone) builds and passes
