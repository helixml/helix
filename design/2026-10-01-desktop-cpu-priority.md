# Desktop CPU priority: keep the display path smooth while agents build

## Problem

Measured on meta/node01 (2026-10-01): when a desktop's agent runs heavy builds (Go
`compile`/`vet`, `cargo`/`rustc`, inner `docker build`), scrolling and window dragging
in the streamed desktop lag badly. Not the GPU (RTX 2000 Ada at 3–7% SM, NVENC ~4ms
per frame) and not a quota (desktop cgroup `cpu.max` 16 cores, `nr_throttled=0`).
It is CPU scheduling: every process in the desktop ran at nice 0 in one cgroup, so the
compositor competed equally with dozens of compile threads.

- gnome-shell main thread: 199ms run, **2079ms waited runnable** in 5s
- zed main thread: 1.9s run, 3.1s waited
- NVENC sessions got 1–5 fps from the compositor

## Goal

The interactive display path gets the CPU first; agent work gets what is left. Builds
may get slower — that is the trade.

| Display tier (normal) | Agent tier (idle) |
|---|---|
| gnome-shell / sway, Xwayland | ACP agents (claude-agent-acp, qwen, codex, goose, opencode, dsh) and everything they spawn |
| pipewire, wireplumber | MCP servers Zed starts (incl. chrome-devtools-mcp → the agent's Chrome) |
| desktop-bridge (capture, encode, WebSocket) | Zed terminals and tasks |
| Zed's UI process | Setup terminal: `helix-workspace-setup.sh` → `.helix/startup.sh` |
| settings-sync-daemon | inner dockerd, containerd, every inner container and BuildKit step |

## What the desktop container can do

Checked inside a running desktop (`grep Cap /proc/self/status`, `mount`, `/sys/fs/cgroup`):

- Desktops with a container engine (the default) run **privileged**: full capability
  set incl. `CAP_SYS_NICE`, private cgroup namespace, **cgroup2 mounted read-write**,
  every controller available. `17-start-dockerd.sh` already moved all processes into
  `/init.scope` and enabled every controller so inner containers get a delegated tree.
- Inner containers live in child cgroups (`/docker/<id>`, BuildKit steps in
  `/docker/buildkit/<id>`). They compete with the desktop's own processes by **cgroup
  weight**, not nice: renicing a shell does nothing for `docker build`.
- Unprivileged sandboxes (headless rootless-Podman agents, bot instances with no
  container engine) drop `CAP_SYS_NICE` and get cgroup2 **read-only**. Headless ones
  have no display path. See "Not covered".

## Options considered

1. **Raise the display path** (negative nice / `SCHED_FIFO` on gnome-shell, pipewire,
   desktop-bridge). Needs `CAP_SYS_NICE`, does nothing against inner containers in
   other cgroups, and RT-class compositor bugs can lock the container up. Rejected.
2. **Lower agent processes** with `nice 19` or `SCHED_IDLE` policy. No capability
   needed and inherited by children, but only orders tasks *within one cgroup*.
   Inner `docker build` is in a sibling cgroup with the same weight as the whole
   desktop, so builds through dockerd would still take half the CPU. Fixing that needs
   a cgroup knob anyway — two mechanisms, and agent processes (one cgroup, weight 100)
   would then starve the agent's own docker work (idle group, weight 3) rather than
   share with it.
3. **cgroup v2 tiers inside the desktop** — chosen. One mechanism covers processes and
   inner containers alike, needs no capability beyond what desktops already have,
   is inherited by every descendant, and gives per-tier accounting (`cpu.stat`,
   `cpu.pressure`) for free.

## Design

`/etc/cont-init.d/16-cpu-tiers.sh` runs as root in the entrypoint, before privileges
drop to `retro`:

```
/sys/fs/cgroup (the desktop container's cgroup namespace root)
└── desktop                 retro may move processes within this subtree
    ├── display             every process starts here (PID 1 is moved here)
    └── agent   cpu.idle=1, cpu.max = container quota minus one core
        ├── procs           agent processes
        └── docker          inner dockerd "cgroup-parent"
```

- **`cpu.idle=1` rather than a low `cpu.weight`.** An idle group gets the minimum
  weight *and* the scheduler's idle-class rules: a waking non-idle task (the compositor
  at frame time) preempts an idle one immediately, and wakeup placement treats a CPU
  running only idle-class work as idle. A low weight only changes the share over time;
  the compositor would still wait out the current slice. Agent work still gets all
  CPU the display path doesn't use, so a build on an otherwise idle desktop runs at
  full speed.
- **Quota reserve (`agent/cpu.max`).** Desktops run with a CPU quota (`cpu.max` =
  vCPUs, e.g. 12). `cpu.idle` decides who runs *inside* the quota, but idle-class work
  still spends it, and then CFS throttles the whole container, compositor included,
  until the next period. Measured with 24 agent threads on a 12-vCPU desktop and only
  `cpu.idle`: the container ran at 12.00 cores, 45 of 50 periods throttled, display
  tier got 0.31 cores, gnome-shell wait/run 3.68. So the agent tier is capped at the
  quota minus two cores (a quarter of the quota on small presets: 1→0.25, 4→1,
  8/12/16→2; no cap when the container has no quota). Two, not one: the display tier
  uses ~0.9 cores for gnome-shell + Zed + desktop-bridge and 2.4 cores while streaming
  vkcube. A/B on the same desktop under the same hog + 1080p stream: 1-core reserve →
  gnome-shell wait/run 1.72, 37 FPS; 2-core reserve → 0.29–0.33, 56.5 FPS. The cost is
  up to two cores of build throughput while the display is idle. `nproc` and Go's
  GOMAXPROCS read the tier's quota, so builds size their parallelism to it.
- **Delegation.** Moving a process needs write access to the destination's and the
  common ancestor's `cgroup.procs`. Only `desktop/cgroup.procs` and
  `desktop/agent/procs/cgroup.procs` are chowned to retro: agent work can move *into*
  the agent tier but cannot move itself back into `display` (its `cgroup.procs`
  stays root-owned). `cpu.idle` stays root-owned.
- **No internal processes.** cgroup v2 forbids processes in a cgroup whose subtree has
  controllers enabled, hence the `procs` leaf next to `docker`, and the root's
  processes move to `display` before controllers are enabled (this replaces the
  `init.scope` block that used to live in `17-start-dockerd.sh`). Every controller is
  still delegated down to `agent/docker`, so Kind/systemd inner containers work.

### How processes reach the agent tier

- **Zed's children.** Zed spawns ACP agents (registry *and* custom), stdio MCP context
  servers, terminals and tasks through the system shell (`ShellBuilder::new(&Shell::System)`
  → `$SHELL -c …`; `crates/agent_servers/src/acp.rs`,
  `crates/context_server/src/transport/stdio_transport.rs`). `run_zed_restart_loop`
  starts Zed with `SHELL=/usr/local/libexec/helix/agent-tier/bash`, a 3-line bash that
  writes itself into `desktop/agent/procs` and `exec`s `/bin/bash`. Zed itself stays in
  `display`. Every spawn passes through it, so it survives Zed restarts
  (`run_zed_restart_loop`, settings-sync `restartZed()`) and agent switches, and covers
  registry agents (claude-acp, codex-acp) whose command Helix cannot configure.
  It is named `bash` because Zed and Claude Code choose shell syntax from the file name.
  SHELL is left pointing at it so Zed's login-shell environment capture cannot reset it.
- **Setup terminal.** `launch_setup_terminal` runs `helix-workspace-setup.sh` (and thus
  `.helix/startup.sh`) under the same bash.
- **Inner dockerd.** `17-start-dockerd.sh` puts the dockerd restart loop in
  `agent/procs` and sets `"cgroup-parent": "/desktop/agent/docker"`, so containers and
  BuildKit `RUN` steps are in the agent tier and share it fairly with the agent's own
  compiles.
- Inheritance does the rest: builds, tests, browsers and `sudo` children stay in the
  cgroup of the process that spawned them.

### chrome-devtools MCP → agent tier

The agent's Chrome is started by the first chrome-devtools-mcp server
(`helix-chrome-devtools-mcp.sh`), which Zed and the agent both launch through the
agent-tier shell. It belongs on the agent side: it renders whatever the agent is
testing (often heavy — the inner Helix frontend, video streams), so it is agent load.
Its *frames* are composited by gnome-shell in the display tier, so the user still sees
smooth window movement; the page itself updates at whatever rate the agent tier allows.

### Not changed

- Audio/input: pipewire, wireplumber, the compositor and desktop-bridge's input path
  stay in `display`.
- Zerocopy: no change to the capture/encode pipeline or its processes.
- No new daemon: an init script plus a launcher, configured through existing entry
  points.

## Not covered (follow-ups)

- **Language servers.** Zed spawns LSPs (rust-analyzer, gopls) directly, not via
  `$SHELL`, so they stay in the display tier. They react to agent edits and can be heavy.
  Moving them needs a Zed change.
- **Unprivileged desktops** (bot instances on a desktop runtime with no container
  engine): cgroup2 is read-only, so there are no tiers and the wrapper is plain bash.
  They have no inner dockerd, so per-process `SCHED_IDLE` would work there; not done
  to keep one mechanism.
- **Disk I/O.** Builds also saturate I/O; `io.weight` on the same tiers is the obvious
  next step if that shows up.
- `docker exec` into a desktop (Hydra tooling) joins PID 1's cgroup, i.e. `display`.

## Testing

- `desktop/shared/test-cpu-tiers.sh` (needs privileged Docker; CI's shell-test step has
  none) runs the init script and wrapper in containers configured like Hydra's
  desktops: tree, `cpu.idle`, quota caps for 12/4/unlimited CPUs, delegation (agent
  work cannot move back to `display`), and the unprivileged no-op path.
- Live in the inner Helix, below.

## Results (inner Helix, 2026-10-01)

Setup: 12-vCPU desktops (`cpu.max` 12 cores) in the inner Helix on a 48-CPU host that
other tenants kept busy (host load 50–300, which is noise these numbers carry — every
condition was run interleaved old/new, 3 rounds). Load: 24 CPU-bound threads started
the way the agent's tools start them (through Zed's `$SHELL`; old image: plain bash in
`/init.scope`), or the same in an inner `docker run` container. Display metric:
gnome-shell main-thread run/wait from `/proc/<pid>/task/<pid>/schedstat` over 5s,
sampled mid-run of `helix spectask benchmark --duration 30 --width 1920 --height 1080`
(vkcube). Old = `537288` (main), new = `f9152a` (this change).

| | gnome-shell run / wait per 5s | wait/run | stream FPS | gaps >100ms |
|---|---|---|---|---|
| old, no load | 2666 / 996 ms | 0.37 | 53.3 | 0.3% |
| new, no load | 2333 / 1477 ms | 0.63 | 46.9 | 2.6% |
| **old, agent load** | **749 / 2811 ms** | **3.75** | **24.0** | **9.9%** |
| **new, agent load** | **1757 / 1882 ms** | **1.07** | **42.8** | **4.1%** |
| old, inner-docker load | 1164 / 3000 ms | 2.58 | 28.2 | 5.2% |
| new, inner-docker load | 1504 / 2519 ms | 1.68 | 43.6 | 3.3% |

In the quietest host window (round 3) new + agent load ran at 59.6 FPS with
gnome-shell wait/run 0.10, against 32.5 FPS / 2.53 for old. The no-load difference
between images is host noise: one new no-load run fell to 27 FPS with nothing of ours
running. What remains under load on new is contention outside the container (other
tenants, SMT siblings) that no in-container priority can remove.

An earlier run with `cpu.idle` but no quota cap (image `5b383c`) still beat old under
agent load (gnome-shell wait/run 2.13 vs 5.99, 27.8 vs 12.9 FPS) but left the container
throttled whenever the agent filled the quota — the reason for `agent/cpu.max`.

`spectask latency` (key-to-eyeball) was too noisy on this host to compare: single
samples ranged 5–596ms on both images.

Verified live on the new image:
- Placement (`ps -eo pid,ni,cls,cgroup,args`): gnome-shell, Xwayland, pipewire,
  wireplumber, desktop-bridge, settings-sync-daemon, Zed in `/desktop/display`;
  claude-agent-acp, the `claude` CLI, its MCP servers, chrome-devtools-mcp and Chrome,
  the setup script, dockerd/containerd in `/desktop/agent/procs`; inner containers in
  `/desktop/agent/docker/<id>`; `docker exec` into the desktop lands in `display`.
- A cold `go build -a std` started by the agent (Claude Code, via its Bash tool):
  `go`/`compile` in `/desktop/agent/procs`.
- Zed restart (`pkill -x zed`, what `restartZed()` does) and an agent switch
  (Zed Agent → Claude Code through `PATCH /spec-tasks/{id}/execution-config`, which made
  settings-sync restart Zed): Zed came back in `display`, the new agent and its
  children in `agent/procs`.
- Agent turns end-to-end after the switch: the agent's Bash tool shell reports
  `0::/desktop/agent/procs`.
