# Fix controlplane CrashLoopBackOff when Kodit is enabled (missing ORT library)

## Summary
A fresh Helix deployment with `KODIT_ENABLED=true` crashes during startup because kodit's ORT library auto-detection resolves to `/lib/libonnxruntime.so` instead of `/usr/lib/libonnxruntime.so`. This happens because the binary sits at `/helix`, making `filepath.Dir("/helix")` = `/`, so the candidate path `/lib` is tried first. Setting `ORT_LIB_DIR=/usr/lib` explicitly bypasses this fragile auto-detection.

## Changes
- **Dockerfile**: Added `ENV ORT_LIB_DIR=/usr/lib` to both dev and production stages, ensuring kodit finds the ORT library at the canonical path where it's installed
- **api/pkg/server/kodit_init.go**: Added a pre-flight check that verifies the ORT library exists before attempting to initialize kodit, producing a clear actionable error message instead of the cryptic hugot error
