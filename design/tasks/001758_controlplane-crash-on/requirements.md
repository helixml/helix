# Requirements: Controlplane crash on startup when Kodit/Kodex is enabled

## Problem

The controlplane container enters `CrashLoopBackOff` when `KODIT_ENABLED=true` because the production Docker image is missing `libonnxruntime.so`. The binary is compiled with `-tags ORT` (which links against ONNX Runtime), but the shared library is never copied into the final production image stage.

## User Stories

- **As an operator**, I want the controlplane to start successfully with Kodit enabled so that code intelligence features work out of the box.

## Acceptance Criteria

- [ ] Controlplane container starts without crashing when `KODIT_ENABLED=true`
- [ ] `libonnxruntime.so` is present in the production image at `/usr/lib/`
- [ ] Existing behavior when `KODIT_ENABLED=false` is unchanged
- [ ] Docker image size increase is minimal (only the single `.so` file added)
