# Multi-node scheduling for GPU-less hosts + headless agent sessions

**Date:** 2026-08-21
**Status:** Partially landed upstream; remainder in `feature/capability-aware-sandbox-scheduling` + follow-ups
**Author:** Waqas Ahmad

## Motivation

A common Helix topology is one GPU workstation plus one or more cheap CPU-only
machines (a NAS, spare servers, ordinary cloud VMs), all reachable over a
private network such as Tailscale. We want:

1. CPU-only hosts to join the cluster and receive the workloads they can
   actually run.
2. The user (or Helix) to decide per-session where it runs and whether it
   needs a watchable desktop at all.
3. Headless agent sessions — agent + toolchain without a compositor or
   encoder — to land on the cheap hosts, keeping GPU capacity free for
   streamed desktops.

Concrete driving setup: `omen` (NVIDIA, runs the control plane + desktop
sessions) and a GPU-less Ubuntu NAS on the same tailnet (runs headless
sessions and headless Sandboxes-API containers).

## Already shipped upstream (no work needed)

- **Cluster join**: `sandbox_instances` registry, outbound-only RevDial
  attach (`api/cmd/hydra/main.go`), auto-registration on connect
  (`api/pkg/server/server.go` `ensureSandboxRegistered`), 30s heartbeat with
  `desktop_versions`/`gpu_vendor`/`render_node`, admin Runners page.
  Adding a node: `./install.sh --sandbox --api-host <url> --runner-token
  <tok>`. Works over Tailscale — no inbound ports on the node.
- **Capability-aware placement**: `SandboxInstance.CanHostDesktop()`
  (`api/pkg/types/types.go`) gates only *streamed desktop* placement;
  `FindAvailableSandboxInstance(ctx, desktopType, requiresDisplay)` places
  headless work on any online host including CPU-only ones.
  `RuntimeSpec.RequiresDisplay` does the same for the Sandboxes API
  (`api/pkg/sandbox/runtimes.go`, `controller_provision.go`).
- **Headless spec-task runtime**: `SandboxRuntimeHeadlessUbuntu` /
  `desktop_type: headless` runs the helix-ubuntu toolchain image with no
  compositor, display devices, or encoder (`hydra_executor.go` — display
  env skipped, placement with `requiresDisplay=false`). Chat rides the
  Zed↔Helix `external_websocket_sync` WebSocket, independent of video.
- **Sticky placement + data safety**: sessions pin to `session.SandboxID`;
  persistent Sandboxes-API sandboxes refuse to move off their bound host
  (workspaces are host-local disk) — `controller_provision.go`
  `pickHostForSandbox`.

## This PR: `feature/capability-aware-sandbox-scheduling`

Closes the remaining dispatch gaps for mixed GPU / CPU-only fleets:

1. **Prefer CPU-only hosts for headless work.** The picker
   (`pickSandboxInstance`, `api/pkg/store/store_sandbox.go`) now places
   `requiresDisplay=false` work on a display-*incapable* host when one
   qualifies, falling back to render-capable hosts otherwise — so headless
   sessions drain to the NAS and GPU slots stay free for desktops. Same
   preference in the custom-image host loop
   (`controller_provision.go` `pickHostForSandbox`).
2. **`max_sandboxes` enforced at dispatch** (was autoscaler-signal only,
   flagged as a follow-up in `config.go`): hosts at their ceiling are
   skipped; a fleet fully at ceiling yields a no-capacity error instead of
   overloading the least-loaded host. `MaxSandboxes == 0` (pre-field rows)
   means no ceiling. Shared predicate: `SandboxInstance.AtMaxCapacity()`.
3. **No more silent `"local"` fallback** (`hydra_executor.go`
   `resolveSandboxID`): when hosts are registered but none is eligible,
   placement fails with an error naming the container type, image key, and
   requirements — instead of dialing the `hydra-local` device key and
   hanging out the RevDial grace period. The `"local"` key remains only for
   the legacy single-node install with an empty registry.
4. **Dev compose**: `SANDBOX_INSTANCE_ID` is overridable
   (`${SANDBOX_INSTANCE_ID:-local}`) so a second dev node doesn't collide
   with the hardcoded `local` row.

Tests: `TestPickSandboxInstance` (store), `TestResolveSandboxID*`
(external-agent, gomock store), `TestSandboxInstanceAtMaxCapacity` (types).

## PR 2: `feature/session-sandbox-host-pinning` — user picks the node (API)

`DesktopAgent.SandboxID` already pinned placement internally (golden builds;
honoured first in `resolveSandboxID`); this exposes it to users:

- `sandbox_host_id` on spec-task creation (`CreateTaskRequest` →
  `SpecTask.SandboxHostID`, GORM AutoMigrate column) and on session start
  (`ExternalAgentConfig.SandboxHostID`).
- Validation at create time via `external_agent.ValidateSandboxHostPin`
  (`api/pkg/external-agent/host_pin.go`): host must exist, be online, and be
  display-capable unless the workload is headless
  (`DesktopTypeRequiresDisplay`). Capacity is deliberately not checked — a
  pin is an explicit "this host, full or not". Invalid pins are rejected,
  never silently rescheduled.
- The pin is honoured on every launch path: task start, just-do-it start,
  restart-after-crash, and session resume (resume reads it off the spec
  task, so the container returns to the host that has its workspace).
- Host discovery for clients: the existing authenticated
  `GET /api/v1/sandboxes` listing — no new endpoint.

Follow-up (PR 3): frontend host dropdown at task/session creation (default
**Auto**), populated from that listing; show hostname, GPU capability, load.

## Mode switching (headless ↔ desktop), for reference

- *Same host*: stop the container, start with the other desktop type for the
  same session — chat history (Postgres) and workspace
  (`/data/workspaces/<sid>` on the host) survive; an in-flight agent turn is
  interrupted (same as any resume).
- *Cross host* (NAS headless → GPU desktop): blocked — workspaces are
  host-local (same rationale as the persistent-sandbox refusal). Workflow:
  agent pushes its branch, reopen on the target host. Cross-host workspace
  sync is out of scope.

## Multi-node operational notes

- A remote node cannot use the dev `registry:5000` image path — it pulls
  from ghcr.io (prod path) or a tailnet-reachable registry via
  `HELIX_SANDBOX_REGISTRY`.
- Upgrades touch the control plane *and* each node (re-run
  `install.sh --sandbox` / bump `SANDBOX_TAG`). A stale node degrades
  gracefully: it stops receiving new sessions once its advertised
  `desktop_versions` no longer match, but keeps serving running ones.
- `MAX_SANDBOXES` on each node is the operator throttle, now enforced at
  dispatch (this PR).
