# Design: Controlplane crash on startup when Kodit/Kodex is enabled

## Root Cause

The Dockerfile has a multi-stage build. The `tokenizers-lib` stage downloads `libonnxruntime.so` via Kodit's `download-ort` tool. The dev and build stages correctly copy this library into `/usr/lib/`, but the final production image stage (`FROM debian:bookworm-slim`, line 116) **omits the COPY**.

The binary at `/helix` is compiled with `CGO_ENABLED=1 -tags ORT` (line 80), which creates a runtime dependency on `libonnxruntime.so`. When Kodit initialization calls `kodit.New()` → Hugot embedding provider → ONNX Runtime via CGo, the missing `.so` causes a fatal load error.

### What works (dev/build stages)

```dockerfile
# Line 47 (api-dev-env) and line 72 (api-build-env):
COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
```

### What's missing (production stage, lines 112-130)

```dockerfile
FROM debian:bookworm-slim
# ...
COPY --from=api-build-env /helix /helix
COPY --from=ui-build-env /app/dist /www
COPY --from=embedding-model /build/models/ /kodit-models/
# ❌ Missing: COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
```

## Fix

Add one line to the production image stage in `Dockerfile` after the embedding model COPY (after line 124):

```dockerfile
COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
```

## Codebase Notes

- **Dockerfile**: `/home/retro/work/helix/Dockerfile` — multi-stage build, production image at line 112
- **Kodit init**: `api/pkg/server/kodit_init.go` — `InitKodit()` calls `kodit.New()` which loads ONNX Runtime
- **Config**: `api/pkg/config/config.go` — `KODIT_ENABLED` env var (default: `false`)
- **docker-compose.yaml** sets `KODIT_ENABLED=true` (line 56)
- The `tokenizers-lib` stage (lines 33-36) already downloads the library — we just need to copy it to the final image

## Implementation Notes

- The library (22MB) is loaded at runtime via `dlopen`, not linked at compile time — so `ldd /helix` won't show it, but it must be on the filesystem at `/usr/lib/`
- The dev stack (`docker-compose.dev.yaml`) uses the `api-dev-env` stage which already has the COPY — so the bug only manifests in production images
- Verified: production image now contains `/usr/lib/libonnxruntime.so` (22044576 bytes)
- The fix pattern mirrors exactly what the dev and build stages already do (line 47 and line 72)
