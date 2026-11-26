# Helix for Mac: QEMU main loop spinning at 100% CPU on an idle VM

Fix: https://github.com/helixml/qemu-utm/pull/10 (QEMU v19), pinned here via
`for-mac/qemu-helix/QEMU_UTM_COMMIT`.

## Symptom

`qemu-system-aarch64` at ~110% CPU in Activity Monitor while the guest was idle
(guest ~98% idle, load 0.1, all vCPUs parked in `hvf_wfi`). It had been running for
3 days 20 hours.

## Diagnosis

- `ps -M`: one QEMU thread at ~99%, mostly system time.
- `sample` (5s): that thread is `qemu_main_loop`. 1504 of its 3662 samples were in
  `slirp_pollfds_poll → soread`, which called `recvfrom`, `getpeername` and
  `shutdown` in roughly equal numbers. The rest of the CPU was virtio-gpu.
- `lsof`: QEMU held 25 host sockets in `CLOSED` state. 17 were to `api.github.com:443`
  (from `helix-api`) and 8 were hostfwd SSH connections on `:41222`. The guest had no
  record of any of them.

## Root cause (libslirp 4.9.5, vendored in qemu-utm `subprojects/slirp`)

On macOS, a host TCP socket whose read side slirp has shut down (`sofcantrcvmore`),
and which the kernel has since disconnected (reset or timed out), behaves like this:

- `poll()` reports `POLLHUP` even when only `POLLOUT` was requested.
- After the pending error has been consumed once, `recv()` returns 0.
- `getpeername()` fails with `EINVAL`, not `ENOTCONN`.

slirp treats `POLLHUP` as readable and calls `soread()`. `EINVAL` is not in its list
of fatal errors, so it calls `tcp_sockclosed()` instead of `tcp_drop()`. That
re-arms the 2MSL linger timer and returns -1, and the dispatch then `continue`s past
the write path that would have finished the close. The next `poll()` returns `POLLHUP`
immediately, so the loop repeats forever and the socket is never freed. Each leaked
connection makes the spin worse.

One way in: a host socket that macOS times out. `recv()` returns `ETIMEDOUT`, which is
also not fatal to slirp, and guest data still waiting to be sent keeps the socket in
the poll set. A laptop that sleeps or changes networks does this to connections that
stay open (`helix-api` → GitHub).

## Fix

- `soread()`: treat `EINVAL` as a dead connection (`tcp_drop`).
- `tcp_sockclosed()`: don't re-arm a 2MSL timer that is already running.

## Verification (2026-11-17, Apple silicon, macOS 26)

A/B with a minimal test VM (busybox initramfs, `-netdev user`). 3 guest connections
stream to a host server; a pf rule then silently drops that port on `lo0` until macOS
times the host sockets out. Run twice per binary:

| QEMU | after the timeout | CPU | busiest thread |
|---|---|---|---|
| shipped v18 | 3 `CLOSED` sockets never freed | 100–101% | ~100% |
| v19 | freed | 0% | 0% |

Real Helix VM on v19, launched with `HELIX_QEMU_PATH` (no change to the installed app):

- Idle: 22% average over 60s, busiest thread under 11%. In a 5s sample, 1 main-loop
  sample in slirp (v18: 1504).
- `CLOSED` sockets come only from the app's SSH status polling. Over 3 minutes every
  one was freed within 15s; none survived to the next 15s check.
- The same pf timeout scenario inside the real VM: the sockets are freed and CPU stays
  at baseline.
- Networking works: guest → `api.github.com` 200, host → API via hostfwd `:41080` 200.
