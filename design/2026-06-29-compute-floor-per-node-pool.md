# Compute Floor per Node Pool (discovery-driven)

Date: 2026-06-29
Status: Implemented. Discovery is the only mode - the single-Manager / single-worker-tag path has been removed.
Scope: control-plane compute provisioning (`api/pkg/sandbox/compute`)

## Summary

- Goal: with one NVIDIA pool and one Neuron pool brought up by the operator,
  Helix keeps a floor of runners in EACH, without the operator declaring the
  pools in config and without per-pool scaling policy.
- Approach: the ComputeManager DISCOVERS pools at runtime from the YD API
  (the operator's `yd-provision` is the only declaration), and one unchanged
  Manager runs per discovered pool under ONE global scaling policy.
- Status: discovery primitive, per-pool supervisor, AND the bootstrap/serve
  wiring are implemented and unit-tested. Discovery is the ONLY mode - there is
  no flag and no single-Manager fallback (that path, incl. worker-tag
  resolution, was deleted). The neuron device plumbing it depends on is in main.

## Why discovery, not a config pool list

A first cut declared each pool in config (`pools: [...]`). Rejected by the
operator on two grounds, both fair:

1. Per-pool scaling policy is unwanted - Floor/Max/idle should be ONE global
   policy, not tuned per pool.
2. Declaring every pool ahead of time is heavy and duplicates what the
   operator already expressed by running `yd-provision`.

So the running pools are the source of truth. Helix asks the YD API "what
pools exist right now?" each cycle and maintains the global floor in each.

## What is a "node pool" here

A distinct group of currently-online YD nodes sharing a `(workerTag,
instanceType)`. The worker tag is what a work requirement must target; the
instance type classifies the accelerator (which selects the sandbox image /
GPU vendor). No new identity, no schema - both fields already ride on the YD
`Node.details` object Helix was already fetching.

## Implemented (this change)

### 1. Discovery primitive - `yellowdog/node_discovery.go`

Helix already called `GET /workerPools/nodes` (for worker-tag auto-discovery)
but decoded only `workerTag`. Extended:

- `nodeDetails` now also decodes `instanceType` (a real YD `NodeDetails`
  field).
- `fetchOnlineNodes()` extracted; both helpers share the one API call.
- `DiscoverNodePools(ctx, cfg) []NodePool` groups online nodes by
  `(workerTag, instanceType)` with a node count. Tag-less nodes dropped
  (a WR can't target them). Sorted for stable output.
- `DiscoverOnlineWorkerTags` refactored onto the shared fetch; behaviour and
  existing tests unchanged.

Tested: `TestDiscoverNodePools` (grouping, same-tag-different-type as distinct
pools, tag-less drop).

### 2. Per-pool supervisor - `compute/pool_supervisor.go`

`PoolSupervisor` keeps one running Manager per discovered pool:

- `PoolDiscoverer` interface (injected; keeps `compute` free of a YD import -
  `yellowdog` imports `compute`, so the dependency cannot go the other way).
- `ManagerFactory` interface - builds a Manager scoped to one pool, owning the
  per-pool provider and applying the ONE global `ManagerConfig`.
- Each reconcile diffs live pools against running Managers: start a Manager
  when a pool appears, cancel its context when the pool's nodes are gone.
- A discovery error leaves the current Manager set intact (transient YD blip
  must not tear down healthy pre-warming).
- `AcceleratorForInstanceType()` - prefix heuristic: `inf*/trn*` -> neuron,
  `g*/p*` -> nvidia, else "". The small static map the operator agreed to.

The existing `Manager` (Floor/D3/D4, all its guarded invariants) is NOT
modified. The supervisor instantiates it N times.

Tested: `TestPoolSupervisorStartsAndStopsManagers`,
`...DiscoveryErrorKeepsManagers`, `...SkipsUnbuildablePool`,
`TestAcceleratorForInstanceType`.

## The isolation invariant (unchanged from the config-list design)

Each pool's Manager must only see its own SandboxInstance rows.
`Manager.ownedRows` filters by `provider.Name()` which is
`"yellowdog-" + DeploymentTag`. So the factory MUST give each pool a distinct
DeploymentTag (derived from the base tag + the worker tag). Otherwise pool A's
Manager counts pool B's rows toward A's floor and they fight. This is the same
worker-tag/deployment-tag separation the YD-side POC already relies on.

## Dependency already in main: YD provider neuron plumbing

The neuron path the supervisor relies on is ALREADY in main, so it is NOT part
of this change:
- `bash_script.sh` has a `GPU_VENDOR=neuron` branch (drops `--gpus`, enumerates
  `/dev/neuron*`), and auto-detects the vendor on the host when unset.
- `provider.taskEnvironment()` emits `GPU_VENDOR` from `Spec.GPUVendor`.
- `config.Compute.GPUVendor` (`HELIX_COMPUTE_GPU_VENDOR`) is an optional
  override, not the switch.

The factory (below) sets `Spec.GPUVendor` per pool from the instance type,
reusing that existing plumbing.

## Wiring (implemented)

The supervisor is the production path whenever the compute subsystem is enabled
(`HELIX_COMPUTE_PROVIDER=yellowdog`). No flag, no single-Manager fallback:

- `bootstrap.Bootstrap` returns a `compute.Service` (the shared `Run` contract;
  here always a `*PoolSupervisor`). The old single-Manager construction and the
  `buildProvider`/`resolveWorkerTag`/`discoverFn` worker-tag machinery were
  deleted. `server.go`'s `computeManager` field is now `compute.Service`; the
  boot site is unchanged (`go computeManager.Run(ctx)`).
- `bootstrap/pool_discovery.go`: `ydPoolDiscoverer` adapts `DiscoverNodePools`
  to `[]compute.DiscoveredPool` (Key = `workerTag|instanceType`);
  `ydManagerFactory` builds one Manager per pool with the pool's worker tag, an
  isolated `DeploymentTag` (`<base>-<sanitised key>`) for row-ownership, and
  `Spec.GPUVendor` from `AcceleratorForInstanceType`. An unclassifiable
  instance type errors and the supervisor skips that pool.
- `managerConfig(cfg, spec)` is shared by the single and per-pool paths, so
  Floor/Max/idle are identical everywhere - only the Spec (GPU vendor) differs.

**Global config knobs.** Floor/Max/idle stay on `config.Compute` and are
copied verbatim into every pool's Manager. No per-pool config. (The whole
point: one policy, N pools.)

## Edge cases / caveats

- Pool removed from discovery -> its Manager is cancelled, but the Manager's
  owned rows (now offline, since the nodes are gone) are not actively reaped
  by anyone. The sandbox reaper flips them offline; a follow-up may want the
  supervisor to drain a stopped pool's rows. Logged, not solved.
- Mixed instance types under one worker tag show up as two pools, but YD
  work-requirement placement is by worker tag ONLY - there is no instance-type
  constraint on a WR. So if one tag genuinely spans accelerators (g5 + inf2),
  the supervisor would build an nvidia Manager and a neuron Manager that both
  submit WRs tagged the same, and YD could schedule the neuron task onto the
  nvidia node (wrong `--gpus`/device flags). This is unsafe, not just untidy:
  the factory MUST refuse to build per-accelerator Managers for a tag that
  spans accelerators (or WRs must gain an instance-type constraint). Operators
  should run one worker tag per accelerator (the natural setup). The
  `DiscoveredPool.Key` contract reflects this - it is keyed on
  (workerTag, instanceType) so the reconcile diff never collides two pools.
- `AcceleratorForInstanceType` is a heuristic: `g4ad` (AMD) would mis-map to
  nvidia, and an unrecognised family returns "" so the factory errors and the
  supervisor **skips that pool** (logged). There is no manual vendor override
  anymore (`HELIX_COMPUTE_GPU_VENDOR` is vestigial). Acceptable for this POC
  (g5 + inf2); add an explicit case when a new family appears.
- **Zero online nodes => no pools => nothing provisions.** Discovery only sees
  RUNNING nodes, so the floor is maintained only on pools that already have a
  node. This is intended for the model "the operator brings up the YD pool;
  Helix maintains a sandbox floor on whatever nodes exist" - Helix does not
  bring the first node up from zero. (Behaviour change from the old
  single-Manager path, which submitted floor WRs against a derived tag even at
  zero - those just sat starved.)
- **Shared API key sees foreign pools.** The old single-Manager path hard-erred
  on multiple distinct worker tags; the supervisor instead manages *every*
  discovered pool. If one YD API key's visibility spans another Helix install's
  pools, this install will manage them too. Scope the API key per install.
  (`HELIX_YD_WORKER_TAG` was removed, so there is no per-install tag filter.)

## Review fixes applied

- **Injective deployment tag.** `poolDeploymentTag` hashes the raw pool Key
  (fnv-32a) into the tag, so two Keys that sanitise to the same readable string
  (`team.a|...` vs `team_a|...`) no longer collide on `provider.Name()` and
  share row ownership. Unit-tested (`TestPoolDeploymentTagInjective`).
- **Eager Namespace validation** in `buildSupervisor` - empty namespace fails
  boot instead of silently skipping every pool at reconcile.
- **Context cleanup**: the self-remove goroutine calls `run.cancel()` so a
  Manager that returns for a non-cancel reason doesn't leak its child context.
- **Dropped the redundant `Service` interface** (identical to `PoolManager`
  once the single-Manager path was gone); Bootstrap returns `*PoolSupervisor`.
- Removed the now-dead `DiscoverOnlineWorkerTags`; `config.Yellowdog.WorkerTag`
  (`HELIX_YD_WORKER_TAG`) deleted; pure `toDiscoveredPools` extracted + tested.

## Follow-ups (not in this change)

- **Reject a worker tag that spans accelerators** in the factory - currently
  relies on the operator using one tag per accelerator.
- **Drain a stopped pool's rows** when a pool disappears from discovery.
- Remove `config.Compute.GPUVendor` (`HELIX_COMPUTE_GPU_VENDOR`), now vestigial.
