# Requirements: Controlplane CrashLoopBackOff — Missing ONNX Runtime Library

## Context

A fresh Helix 2.9.23 deployment enters CrashLoopBackOff (pod 1/2 Ready) when Kodit is enabled (`KODIT_ENABLED=true`). The controlplane fatally exits during startup with:

```
failed to initialize kodit: failed to initialize kodit client: probe embedding dimension:
initialize hugot: create hugot session: cannot find the ort library at: /lib/libonnxruntime.so
```

## User Stories

**As an operator** deploying Helix with Kodit enabled, I need the controlplane to start successfully so that the platform is usable.

**As an operator**, if the ONNX Runtime library is genuinely missing from the image, I need a clear error message and documentation on how to resolve it, rather than a cryptic crash loop.

## Acceptance Criteria

1. **Controlplane starts successfully** when `KODIT_ENABLED=true` and the image is built from the standard Dockerfile (with `-tags ORT`).
2. **`libonnxruntime.so` is present** at the path the hugot/ORT Go library expects inside the production container image.
3. **No regression** — Kodit embedding and code intelligence features work end-to-end after the fix.
4. **CI validates** — the Drone pipeline builds the image correctly with the ORT library included.
