# Sample runner profiles

Each `.yaml` file in this directory is a Docker Compose file describing a set
of model servers to run on a Helix runner. The runner runs them inside a
Docker-in-Docker dockerd; the API server routes inference requests by model
name to the matching container.

Operators write their own profiles for their hardware. These are reference
copies operators can adapt.

## Conventions

All sample profiles follow these conventions; copy them in your own profiles:

- Mount the runner's shared HF cache at `/root/.cache/huggingface`:
  `volumes: - /models:/root/.cache/huggingface`
  `/models` is the canonical mount path inside the runner container; the
  runner manages the underlying named volume (`helix-runner-models`).
- Pass `HUGGING_FACE_HUB_TOKEN` through from the runner's env.
- Set `HF_HUB_OFFLINE=1` once the model is cached locally — required for
  air-gapped operation.
- Use `--served-model-name` so the API-side model name is stable independent
  of the upstream model identifier.

## NVIDIA vs AMD

NVIDIA uses `deploy.resources.reservations.devices`:
```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          device_ids: ["0"]
          capabilities: [gpu]
```

AMD uses top-level `devices` and `group_add`:
```yaml
devices:
  - /dev/kfd
  - /dev/dri/renderD128
group_add:
  - video
  - render
```

Mixing both styles in a single service is rejected by the compose parser.

## GPU compatibility metadata

Each profile is *also* stored with operator-declared GPU compatibility fields
(vendor, architectures, model_match, min_vram_bytes) — these are *not* in the
YAML, they are entered separately in the admin UI when the profile is saved.
The header comment at the top of each sample documents what those fields
should be set to.

## Files

| File | Hardware | Models | Desktop headroom |
|------|----------|--------|------------------|
| `8xH100-vllm.yaml` | 8x NVIDIA H100 80GB | qwen3 embeddings, qwen3.5-35b, minimax-m2.7, gemma-4-26b | GPU 7 free |
| `8xRTX6000Pro-prod-saas.yaml` | 8x NVIDIA RTX PRO 6000 Blackwell 96GB | qwen3 embeddings (×2 sharing GPU 0), qwen3.5-35b, minimax-m2.7 (TP=4 GPUs 2-5), gemma-4-26b | **GPU 7 free for desktops** |
| `any-nvidia-blackwell-4gpu.yaml` | 4x NVIDIA Blackwell | qwen3.5-72b (TP=4) | none |
| `any-nvidia-dev-single-gpu.yaml` | 1x NVIDIA, ≥24 GiB | qwen2.5-7b | none |
| `amd-mi300x-vllm.yaml` | 1x AMD MI300X | qwen2.5-72b | n/a (CDNA can't host desktops — see live-test in design/2026-04-28-cloud-gpu-smoke-results.md) |
| `dev-spike-tiny.yaml` | 1x NVIDIA, ≥4 GiB (shared) | qwen2.5-0.5b | shared (LLM uses 20% VRAM) |

`dev-spike-tiny.yaml` is what the GPU-passthrough-into-DinD spike uses. If
you are validating this design on a small dev GPU shared with desktop
workloads, start with this profile.

`8xRTX6000Pro-prod-saas.yaml` is the canonical multi-tenant production-SaaS
profile. Same 5-service shape as the H100 profile; deliberately uses GPUs 0-6
and **leaves GPU 7 unused** so Hydra can pin agent desktops to it via
`gpu_index: 7` (Decision 15). This is the right starting point for any
operator running production inference + agent-desktop SaaS on the same node.

## Desktop-headroom convention

Profiles that need agent desktops on the same node should claim *N-1* GPUs of
the host's *N*, leaving the highest-index GPU free. Hydra spawns desktop
sessions pinned to the unclaimed GPU via `gpu_index: <N-1>` in the
dev-container request (see Decision 15 in design.md). The compose parser
computes `GPUCount` from the union of `device_ids` across all services — so a
7-of-8 profile reads as `GPUCount: 7` and the compatibility check on an 8-GPU
host passes with explicit headroom for desktops. Both the 8xH100 and
8xRTX6000Pro profiles in this directory follow this convention.
