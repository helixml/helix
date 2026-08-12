# Stale-frame flash: why it's hard, why OBS doesn't have it, and the fix

Continuation of `2026-06-19-video-judder-kodit-cpu-and-pts-zero.md`. After proving
the stream is NOT reordered on the wire (oooprobe: ooo=0 even under the window-
switcher repro), the residual symptom is a **stale-CONTENT flash**: one frame
shows old pixels under a correct, monotonically-increasing PTS. It appears under
heavy compositor damage (window switcher, CSS-animation scroll) + GPU load. The
GL→CUDA fence (`frame.finish().wait()`, in main) fixed the *other* half
(producer→NVENC); this is the **import side** (Mutter → our GL sample).

## Why it's a stale frame (mechanism)
Zero-copy capture: PipeWire hands us a dma-buf slot Mutter just rendered into;
`gl_blit.rs` imports it as a GL texture and samples it. On NVIDIA there is **no
implicit dma-buf sync** — the GPU doesn't know our sample must wait for Mutter's
write to finish. Under load, Mutter's write isn't complete when we sample → we
encode stale/partial pixels for one frame. Correct PTS, wrong content → "flash,"
not a reorder.

## Why our two fixes made it WORSE (the real lesson)
Both prior attempts added the wait **inside the PipeWire `process()` callback**,
which runs on the single PipeWire loop thread. That thread is coupled to Mutter's
frame pacing: we must `queue_raw_buffer()` (return the slot) promptly or Mutter's
buffer pool starves and it throttles. So:
- **CPU `poll(POLLIN)`** (branch `fix/4k-import-acquire-fence`): blocked the loop
  → delayed buffer return → Mutter throttled → stutter, which *looked* like
  out-of-order even on drag.
- **GPU `eglWaitSync` on the IMPLICIT fence** (branch `fix/4k-import-glwaitsync`):
  the implicit fence (`DMA_BUF_IOCTL_EXPORT_SYNC_FILE`) is **empty on NVIDIA**, so
  it didn't actually wait (flash unfixed); and its per-frame setup + the existing
  CPU-blocking `frame.finish().wait()` perturbed startup → out-of-order at start.

## Why OBS captures the SAME stack with no stale frames
The decisive difference is **architecture, not a single API call**:
1. **OBS decouples capture from GPU work.** Its PipeWire `process()` callback only
   dequeues the buffer (and its sync metadata) and hands it off; the dma-buf
   import + sync-wait + render happen on OBS's **separate graphics thread**. So
   OBS can *wait for the buffer to be ready* without ever blocking the PipeWire
   loop / throttling the compositor. WE do import + blit + CUDA copy + fence wait
   **inline on the capture thread**, so every wait we add throttles Mutter. That
   is the core reason our waits backfire and OBS's don't.
2. **The correct fence is the EXPLICIT one.** On NVIDIA the implicit dma-buf fence
   is empty; the real "Mutter finished writing" signal is the explicit acquire
   fence delivered via `SPA_META_SyncTimeline` (a drm_syncobj acquire/release
   timeline). PipeWire 1.4.7 + Mutter 49 support it. We never negotiated or read
   it — our one GPU-wait attempt waited on the empty implicit fence.

So: hard because we put the (mandatory) synchronization on the one thread where
waiting is forbidden, and when we did wait we waited on the wrong (empty) fence.

## The fix (two correct shapes)
**Targeted (preferred first):** explicit-sync GPU wait, non-blocking on the CPU.
1. Negotiate `SPA_META_SyncTimeline` in the buffer params (alongside Header/Crop/Cursor).
2. In `process()`, read the **acquire** drm_syncobj point from the meta; convert to
   a sync_file fd (`drmSyncobjExportSyncFile` / timeline wait) and `eglWaitSync`
   (server-side: the GPU waits, the CPU returns immediately — does NOT throttle).
3. Sample/blit, then signal the **release** point so Mutter can reuse the slot.
4. If the meta is absent (non-explicit-sync compositor) skip → identical to today.
This keeps the single-threaded model but makes the wait GPU-side on a VALID fence,
which is the thing both prior attempts lacked.

**Structural (fallback / longer term):** adopt OBS's split — capture thread only
dequeues; a render thread does import+wait+blit+encode. Removes the coupling
entirely so even a CPU wait is safe. Bigger refactor.

## Verification plan
- First build: instrument `process()` to log the buffer's meta types → CONFIRM
  `SPA_META_SyncTimeline` is actually delivered by Mutter's screencast at runtime
  (de-risks the whole approach before building the wait).
- oooprobe must still show ooo=0 (no reorder regression) and clean arrival.
- Stale-flash itself is not probe-detectable (correct PTS) → needs Luke's eyes on
  the window-switcher repro.
- Build: `./stack build-ubuntu`; sandbox is `235491e76be1_helix-sandbox-nvidia-1`
  (recreated, hash prefix) so finish the transfer manually (see prior doc).
```
