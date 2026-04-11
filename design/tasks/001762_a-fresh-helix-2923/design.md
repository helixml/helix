# Design: Controlplane CrashLoopBackOff — Missing ONNX Runtime Library

## Root Cause Analysis

The startup path is: `serve.go` calls `InitKodit()` (`api/pkg/server/kodit_init.go:80`) which creates a `kodit.New()` client, which calls `provider.NewHugotEmbedding()` to load the ONNX embedding model, which internally creates a hugot session via the `knights-analytics/hugot` library. Hugot uses the `knights-analytics/onnxruntime` Go bindings, which look for `libonnxruntime.so` at a specific filesystem path.

**The path mismatch:**
- The Dockerfile (`Dockerfile:126`) copies the library to `/usr/lib/libonnxruntime.so`.
- The hugot/ORT Go library checks `/lib/libonnxruntime.so` by default.
- On Debian bookworm-slim, `/lib` is a symlink to `/usr/lib` (usrmerge), so this *should* work. If it doesn't, the most likely causes are:
  1. The `download-ort` build stage failed silently, producing no library file, and the `COPY` instruction silently succeeded with nothing to copy.
  2. The hugot/ORT library version was bumped and changed its default search path or validation logic.
  3. The image was built from a variant pipeline that skipped the `tokenizers-lib` stage.

**Key codebase patterns discovered:**
- Kodit is conditionally compiled via Go build tags: `!nokodit` (default, includes kodit) vs `nokodit` (stubs everything out). The error proves the binary *was* built with kodit support.
- The CI pipeline (`.drone.yml`) uses `ORT_LIB_DIR=/usr/lib` when running `download-ort`, but the Dockerfile does NOT set this env var — the `download-ort` tool defaults to `/app/lib/` as its output directory.
- The Dockerfile's multi-stage build downloads the library in a `tokenizers-lib` stage, then copies it in both dev and prod stages.
- `KODIT_ENABLED` defaults to `false` (`api/pkg/config/config.go:79`), so this only affects deployments that explicitly enable Kodit.

## Fix

**Ensure the library is at the path hugot expects.** Two complementary changes:

1. **Dockerfile** — add a symlink as a safety net, or copy to both `/usr/lib/` and `/lib/`:
   ```dockerfile
   COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
   RUN ln -sf /usr/lib/libonnxruntime.so /lib/libonnxruntime.so
   ```
   On usrmerge systems the symlink is redundant (already exists); on non-usrmerge systems it ensures compatibility.

2. **Validate at startup** — before calling `kodit.New()`, check that the ORT library exists at the expected path and produce a clear error message. This is already somewhat handled by the hugot error, but wrapping it with more context would help operators:
   ```
   kodit is enabled but libonnxruntime.so is missing — ensure the container image
   was built with the ORT build stage (see Dockerfile)
   ```

3. **CI verification** — add a build step that asserts `/usr/lib/libonnxruntime.so` exists in the final image before pushing.

## Decision: Symlink vs. Dual Copy

**Chose symlink** because:
- Avoids duplicating a ~200MB shared library inside the image.
- Works on both usrmerge and non-usrmerge Debian variants.
- The symlink is a no-op on bookworm (where `/lib` → `/usr/lib` already), so it's harmless.

## Scope

This fix is limited to the Dockerfile and optionally a startup validation check in `kodit_init.go`. No changes to the Helm chart, Go dependencies, or hugot library itself.
