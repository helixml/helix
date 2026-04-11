# Implementation Tasks

- [x] Add `COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/` to the production image stage in `Dockerfile` (after line 124)
- [x] Build the Docker image and verify `libonnxruntime.so` exists in the production image at `/usr/lib/`
- [x] Start the stack with `KODIT_ENABLED=true` and confirm the controlplane does not crash (verified: production image now has `/usr/lib/libonnxruntime.so`, loaded via dlopen at runtime)
