# GPU block view for sandbox + profile management

**Status:** design / not started.
**Owner:** unassigned.
**Triggered by:** the "where do I assign a profile to a sandbox?"
question. The just-landed dropdown in `AgentSandboxes.tsx` covers the
operational gap, but it treats GPUs as an opaque count — it doesn't
show *which* GPU runs *which* model, where headroom for desktops sits,
or how a 7-of-8 profile maps onto an 8-GPU host. A graphical
block view of "GPUs in the system, with their allocations" is what
the operator actually wants when running multi-tenant inference.

## Problem

After today's dropdown, the workflow is:

1. Operator opens Admin → Agent Sandboxes
2. Sees a "Profile: 8xRTX6000Pro-vllm" line on a card
3. Has to mentally cross-reference the profile's compose YAML with the
   host's GPU inventory to know which GPU is doing what
4. Has no visual cue for "GPU 7 is free for desktops," "GPU 0 is
   shared between two embedders at 45% util each," or "TP=4 spans
   GPUs 2-5"

Per-GPU activity (utilisation / VRAM / temperature) IS already
fetched by `GPUStatsCard`. The missing piece is showing *what's
assigned* to each GPU alongside the live stats — a unified
**inventory view**.

## Proposed layout

A new top-level card on the same Admin page (or a new tab):

```
┌─ Sandbox: cloud-rtx6kpro-1 (8× NVIDIA RTX PRO 6000 Blackwell) ─┐
│ Profile: 8xRTX6000Pro-vllm  [Reassign]  [Clear]                │
│ Status: running   |   Models served: 5                         │
│                                                                │
│ ┌─GPU 0────┐ ┌─GPU 1────┐ ┌─GPU 2────┐ ┌─GPU 3────┐            │
│ │ qwen3-vl │ │ qwen3.5- │ │  minimax │ │  minimax │            │
│ │   -embed │ │      35b │ │   -m2.7  │ │   -m2.7  │            │
│ │ + qwen3- │ │          │ │   (TP=4) │ │   (TP=4) │            │
│ │   text-  │ │          │ │          │ │          │            │
│ │   embed  │ │          │ │          │ │          │            │
│ │  ▓▓▓░ 78%│ │  ▓▓▓▓ 90%│ │  ▓▓▓░ 72%│ │  ▓▓▓░ 72%│            │
│ │   72°C   │ │   75°C   │ │   68°C   │ │   69°C   │            │
│ └──────────┘ └──────────┘ └──────────┘ └──────────┘            │
│ ┌─GPU 4────┐ ┌─GPU 5────┐ ┌─GPU 6────┐ ┌─GPU 7────┐            │
│ │  minimax │ │  minimax │ │  gemma-4 │ │ ┌──────┐ │            │
│ │   -m2.7  │ │   -m2.7  │ │     -26b │ │ │ FREE │ │            │
│ │   (TP=4) │ │   (TP=4) │ │          │ │ │  for │ │            │
│ │          │ │          │ │          │ │ │  Hydra│ │           │
│ │          │ │          │ │          │ │ │ desktop│ │          │
│ │  ▓▓▓░ 72%│ │  ▓▓▓░ 72%│ │  ▓▓▓▓ 88%│ │ │ ░ 0%  │ │           │
│ │   68°C   │ │   69°C   │ │   80°C   │ │ └──────┘ │            │
│ └──────────┘ └──────────┘ └──────────┘ └──────────┘            │
└────────────────────────────────────────────────────────────────┘
```

Each GPU card shows:

- **Allocation label** — derived from joining the profile's compose
  YAML's `device_ids` (or AMD `/dev/dri/render*` mounts) with the
  sandbox's GPU inventory. Multiple services on one GPU stack
  vertically (e.g. GPU 0 hosts both embeddings).
- **Tensor-parallel grouping** — services declared with the same set
  of `device_ids` (e.g. `["2", "3", "4", "5"]`) are visually linked
  with a coloured outline or shared header.
- **Live stats** — VRAM bar + temperature, sourced from the existing
  `GPUStatsCard` data.
- **Headroom indicator** — GPUs claimed by no compose service show
  "FREE — Hydra desktops" so the operator knows where new sessions
  land.
- **Click target** — clicking a GPU card opens a side panel with the
  full service definition (image, command flags, env vars,
  `--gpu-memory-utilization`, etc.) for that GPU.

## Cross-sandbox view

For multi-host deployments (the matrix.yaml-style fleet: 4×A100 +
4×L40S + 8×MI300X), a top-level fleet view stacks all sandboxes
vertically, each rendered with the per-sandbox layout above. Operators
can scan total inference capacity at a glance and spot under-utilised
nodes.

## Where the data already lives

- **Per-sandbox GPU inventory:** `sandbox.gpus` (vendor, arch, VRAM,
  index, name) — already in the heartbeat (`api/pkg/types/types.go`
  `GPUStatus`).
- **Profile → GPU mapping:** `composeparse.Parse()` already extracts
  `device_ids` per service. Today the parser only computes a count; we
  need a new field that exposes the per-service assignment — e.g.
  `Models[i].GPUIndices []string`. Trivial extension to the parser.
- **Live GPU stats:** already fetched by `GPUStatsCard` via
  `hydraClient.GetSystemStats()`.
- **Service health:** already in the heartbeat.
- **Download progress:** added in `b1e9cbfdb`.

So the data plumbing is in place. The work is:

1. Extend `composeparse.ParseResult` to expose per-service GPU
   indices.
2. Extend `agent_sandboxes_handlers.go` `SandboxInstanceInfo` to
   include the parsed mapping (so the frontend doesn't have to
   re-parse compose YAML).
3. Build a new `<SandboxGPUBlockView />` component that consumes
   the existing data + the new mapping field.
4. Decide where it lives in nav: replace the current "Inference
   Profiles" card or live alongside it as a richer view.

## Reassign / drag-drop scope

For a v1, reassign goes via the same dropdown the operator already
has (existing `[Reassign]` button opens a modal with the
compatible-profiles dropdown + Assign button). Drag-and-drop
GPU reallocation (e.g. "drag the minimax-m2.7 service from GPUs 2-5
to GPUs 4-7") is **out of scope** — that requires a profile-editor
UI that rewrites compose YAML, which is a much larger surface.

## Tradeoffs

**For:**
- Operator mental model matches what's actually running on the silicon
- Headroom visibility is the single biggest gap today — every operator
  has to learn the "GPU N-1 reserved for desktops" convention by
  reading docs
- Same backend data; pure UI work

**Against:**
- 3-5 days for a polished first cut, depending on layout fidelity
- Need to handle edge cases: no GPUs (CPU-only), unknown GPUs (probe
  failed), heterogeneous mixes (e.g. Azure NV-series with one A100 +
  one virtio_gpu in the same host)
- The GPU block layout assumes ≤16 GPUs per sandbox fits readably in
  one card — for bigger hosts (NVIDIA Bluefield clusters) we may need
  a denser representation

## Spec needed before building

1. Layout for >8 GPUs (does it wrap into rows of 4, or scale down?)
2. CPU-only and virtio_gpu visualisation
3. AMD/MI300X representation: device_ids look like `/dev/dri/renderD128`
   not `0..N`, so we need a normalisation step
4. Click-into-GPU side panel scope: is it just service detail, or
   does it offer per-GPU actions (kill container, restart service)?

## Related

- `frontend/src/components/admin/AgentSandboxes.tsx` —
  `SandboxProfileCard` (today's text-list assignment surface) and
  `GPUStatsCard` (live per-GPU stats) are the building blocks
- `api/pkg/runner/composeparse/parse.go` — parser to extend with
  per-service GPU indices
- `api/pkg/server/agent_sandboxes_handlers.go` `SandboxInstanceInfo` —
  extend with the parsed mapping
- `design/2026-04-30-gpu-onboarding-flow.md` — the prior design note
  for the onboarding wizard; this view is the natural next step after
  a sandbox is attached and a profile is assigned
