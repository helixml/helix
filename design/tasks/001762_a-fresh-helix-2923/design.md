# Design: Controlplane CrashLoopBackOff — Missing ONNX Runtime Library

## Root Cause Analysis

The startup path is: `serve.go` calls `InitKodit()` (`api/pkg/server/kodit_init.go:80`) which creates a `kodit.New()` client, which calls `provider.NewHugotEmbedding()` to load the ONNX embedding model, which internally creates a hugot session via the `knights-analytics/hugot` library. Hugot uses the `knights-analytics/onnxruntime` Go bindings, which look for `libonnxruntime.so` at a specific filesystem path.

**The path resolution chain (discovered during implementation):**

Kodit's `infrastructure/provider/hugot_ort.go` has a `resolveORTLibDir()` function that:
1. Checks `ORT_LIB_DIR` env var
2. Checks `lib/` alongside the executable — `filepath.Join(filepath.Dir(os.Executable()), "lib")`
3. Checks `lib/` relative to working directory
4. Falls back to empty string → hugot defaults to `/usr/lib/libonnxruntime.so`

**The problem:** The helix binary is at `/helix` in the production container. So step 2 computes `filepath.Dir("/helix")` = `/`, then `filepath.Join("/", "lib")` = `/lib`. Since `/lib` exists as a directory on Debian bookworm-slim (it's a symlink to `/usr/lib` via usrmerge), `resolveORTLibDir()` returns `/lib`. Hugot then looks for `/lib/libonnxruntime.so`.

On usrmerge Debian, `/lib/libonnxruntime.so` should resolve via symlink to `/usr/lib/libonnxruntime.so` (where the Dockerfile copies it). The fact that it fails means the library file is **genuinely absent** from the image — likely a `download-ort` build-stage failure or cache issue.

**Key codebase patterns discovered:**
- Kodit is conditionally compiled via Go build tags: `!nokodit` (default, includes kodit) vs `nokodit` (stubs everything out). The error proves the binary *was* built with kodit support.
- The CI pipeline (`.drone.yml`) uses `ORT_LIB_DIR=/usr/lib` when running `download-ort`, but the Dockerfile does NOT set this env var at runtime — the auto-detection kicks in instead.
- The Dockerfile's multi-stage build downloads the library in a `tokenizers-lib` stage, then copies it in both dev and prod stages.
- `KODIT_ENABLED` defaults to `false` (`api/pkg/config/config.go:79`), so this only affects deployments that explicitly enable Kodit.

## Fix

Two complementary changes:

1. **Dockerfile** — set `ENV ORT_LIB_DIR=/usr/lib` in both dev and production stages. This bypasses the fragile auto-detection logic and points the kodit library directly at the canonical path where the file is installed. This is the same approach CI uses.

2. **Startup validation** — add a pre-flight check in `kodit_init.go` before `kodit.New()` that verifies the ORT library exists and returns a clear, actionable error message if missing.

## Decision: `ORT_LIB_DIR` env var vs. Symlink

**Chose `ORT_LIB_DIR=/usr/lib` env var** because:
- It's the same approach CI already uses (`.drone.yml` line 122).
- It eliminates the fragile auto-detection that accidentally resolves to `/lib` because the binary is at `/helix`.
- No extra filesystem operations or layers (vs. a `RUN ln -sf` instruction).
- Works regardless of usrmerge status.

## Scope

Changes limited to `Dockerfile` (env var) and `api/pkg/server/kodit_init.go` (pre-flight check). No Helm chart, Go dependency, or hugot library changes needed.
