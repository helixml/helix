# Findings: Helix-in-Helix Boot — Pull Output Buffering & Desktop-Image Transfer

**Date:** 2026-07-02
**Context:** Surfaced while waiting for the inner Helix stack to boot during the
VCS-lozenge spec work (below). Two Helix-in-Helix dev-startup issues, unrelated
to the lozenge but **implemented as part of this same task** (workstreams C and D
in `design.md` / `tasks.md`). This document is the detailed backing evidence for
those two workstreams.

---

## 0. The original issue that brought us here (recap)

This spec task is *"implement `design/2026-07-02-project-vcs-connection-lozenge.md`"*.
The motivating problem: **spec-task pushes to external repos can fail silently.**
The internal git server returns HTTP 200 to the git client *before* mirroring to
the external provider (`git_http_server.go:614-617`); when the external push
fails it rolls the refs back (`rollbackBranchRefs`, `:775-805`) but the client
already got 200 — so the agent reports "pushed and ready for review", the UI
shows nothing, and the user is left guessing. Root cause in the incident: the
acting user's connected GitHub account had no access to the private repo, and
GitHub returned a misleading `404 Repository not found`.

Full design is in this task's **`requirements.md` / `design.md` / `tasks.md`**
(workstream A = loud push-failure surfacing; workstream B = the generic
per-provider connection lozenge). This document does **not** restate it.

---

## 1. `docker pull` output looks half-rendered / unresponsive

### Symptom
The Ghostty terminal (and `/tmp/helix-startup.log`) show the image pull ending
mid-line, e.g. `008906cd1cbe: Pul`, `ca5f5089f3a2: Downloa`, with long stalls —
looks hung.

### Root cause — buffering, not a hang
1. `docker pull` to a **non-TTY** doesn't do the in-place progress redraw; it
   emits one plaintext line per layer state change (`Downloading` → `Verifying
   Checksum` → `Download complete` → `Pull complete`). Hence thousands of lines.
2. The output is piped through `grep -v "^$"`, and **grep block-buffers stdout**
   (~4–64 KB) when its output isn't a terminal. Lines arrive in bursts; any
   snapshot lands mid-block, truncating the last line.

Evidence it was never stuck: the layer that appeared truncated
(`ca5f5089f3a2: Downloa`) later logged `Pull complete`, and the log grew in
bursts (`82073 → 98355 → 98680` bytes).

### Where
- `stack:1098`, `stack:1139`, `stack:1176` — `sandbox_docker exec … docker pull … 2>&1 | grep -v "^$"`
- `sandbox/04-start-dockerd.sh:266`, `:285` — same shape on sandbox boot

### Fix options (line-based, not char-by-char)
- **Cleanest:** `docker pull --quiet` → prints only the final digest (one line).
- **Keep progress but flush per line:** `stdbuf -oL docker pull … 2>&1 | grep --line-buffered -v "^$"`. The `--line-buffered` on grep is the actual lever.

---

## 2. ~7-minute desktop-image transfer on every fresh session

### Symptom
`helix-ubuntu:828ce7` (7.67 GB) is pushed to `registry:5000` then pulled back
into the sandbox on session start — `transfer-ubuntu +0s → +411s` (~7 min),
inside Step 3 (`./stack build-sandbox`).

### What actually persists (correcting an earlier wrong assumption)
The **build cache persists correctly.** The `helix-ubuntu` build this session was
**100 % `CACHED`** (`#8`–`#24 CACHED`, only re-exported the manifest). The golden
ZFS-clone cache (`design/2026-03-16-zfs-clone-golden-cache.md`) *is* carrying the
desktop's inner-dockerd/buildkit state across sessions as designed.

### The real gap — a second, separate Docker store
Per `design/2026-02-14-sandbox-docker-storage-split.md` there are **two** stores:

| Store | Backing | Golden-cloned? | This session |
|---|---|---|---|
| Desktop inner dockerd (inner compose stack + **buildkit cache**) | ZFS zvol / golden clone | ✅ | warm → build `CACHED` |
| **Sandbox container's own dockerd** (`helix_sandbox-docker-storage`; where `helix-ubuntu` must physically live for Hydra) | named volume | ❌ | **empty**, created 14:34:20 |

The image was in the desktop's buildkit but **not inside the sandbox's dockerd**,
forcing the `push → registry:5000 → pull-back` transfer (the 411 s). The docs say
this is by design for the inner sandbox: *"inner sandboxes pull from the
registry"* (`2026-02-14`, Helix-in-Helix section). The golden captured the build
cache but not the post-transfer sandbox image store, so the transfer re-runs each
fresh session.

### Correction: not a double-transfer
`grep -c "transferred via registry"` returns 2, but the second block
(`/tmp/helix-startup.log:2293-2306`) is the `build-sandbox` **timing log being
replayed** at the end (`📊 Timing log saved to …`), not a second run. It
transferred **once** (~7 min).

### Fix lever
Make the golden snapshot already contain `helix-ubuntu:<tag>` **inside**
`sandbox-docker-storage` — i.e. have the golden build run through the
desktop-image transfer *before* promotion. A fresh session then clones a golden
where the image is already in the sandbox's dockerd, and the existing skip-checks
short-circuit:
- `sandbox/04-start-dockerd.sh:220-224` (skip pull when exact tag present)
- the `./stack` transfer path (`stack:1074-1145`) only re-pushes/pulls when the tag is absent

That turns the 7-min transfer into a no-op on warm sessions. Persisting
`sandbox-docker-storage` independently of golden would fight the split
architecture, so the golden route is the aligned fix.

### Open item to confirm
How far the golden build runs for this project type — check
`api/pkg/services/golden_build_service.go` to confirm whether it can be extended
to include the transfer step before promotion.

---

## Key references
- `stack:1074-1145` (desktop image transfer), `:1098/:1139/:1176` (pull output)
- `sandbox/04-start-dockerd.sh:220-224` (skip-check), `:266/:285` (pull output)
- `api/pkg/hydra/golden.go` (golden copy/clone lifecycle)
- `api/pkg/services/golden_build_service.go` (golden build)
- `design/2026-02-14-sandbox-docker-storage-split.md`
- `design/2026-03-16-zfs-clone-golden-cache.md`
- `design/2026-01-25-helix-in-helix-development.md`
- VCS-lozenge original issue: this task's `requirements.md` / `design.md` / `tasks.md`
