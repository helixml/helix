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

| File | Hardware | Models |
|------|----------|--------|
| `8xH100-vllm.yaml` | 8x NVIDIA H100 80GB | qwen3 embeddings, qwen3.5-35b, minimax-m2.7, gemma-4-26b |
| `any-nvidia-blackwell-4gpu.yaml` | 4x NVIDIA Blackwell | qwen3.5-72b (TP=4) |
| `any-nvidia-dev-single-gpu.yaml` | 1x NVIDIA, ≥24 GiB | qwen2.5-7b |
| `amd-mi300x-vllm.yaml` | 1x AMD MI300X | qwen2.5-72b |
| `dev-spike-tiny.yaml` | 1x NVIDIA, ≥4 GiB (shared) | qwen2.5-0.5b |

`dev-spike-tiny.yaml` is what the GPU-passthrough-into-DinD spike uses. If
you are validating this design on a small dev GPU shared with desktop
workloads, start with this profile.
