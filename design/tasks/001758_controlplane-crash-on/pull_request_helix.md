# Fix controlplane crash when Kodit is enabled

## Summary
The production Docker image was missing `libonnxruntime.so`, causing a CrashLoopBackOff when `KODIT_ENABLED=true`. The binary is compiled with `-tags ORT` which requires this library at runtime, but the final image stage omitted the COPY from the `tokenizers-lib` build stage.

## Changes
- Added `COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/` to the production image stage in `Dockerfile`
