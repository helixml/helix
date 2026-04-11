# Implementation Tasks

- [~] Add `COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/` to the production image stage in `Dockerfile` (after line 124)
- [ ] Build the Docker image and verify `libonnxruntime.so` exists in the production image at `/usr/lib/`
- [ ] Start the stack with `KODIT_ENABLED=true` and confirm the controlplane does not crash
