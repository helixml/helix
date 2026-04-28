# 2026-04-28 — Cloud GPU smoke test results

End-to-end validation of the helix sandbox-absorbs-runner architecture on real cloud GPU VMs. Both NVIDIA (Verda) and AMD (Hot Aisle) paths verified.

## TL;DR

Architecture works on real cloud GPU VMs from both providers. Total spend: **$0.56** (Verda $0.43 + Hot Aisle $0.13).

| Provider | Hardware | Result | Spend |
|---|---|---|---|
| Verda | 1× A100 80GB SXM4 (FIN-01) | ✅ Sandbox boots, nested DinD with NVIDIA runtime, vLLM serves Qwen 0.5B chat completion through 2 layers of Docker | $0.43 |
| Hot Aisle | 1× MI300X 192GB | ✅ Sandbox boots, nested DinD with AMD passthrough, ROCm visible inside inner DinD via `rocm-smi` | $0.13 |

## What was validated

The uncertain pieces of the new architecture, all confirmed on cloud:

1. **DinD inside a cloud VM works** when you run the sandbox with `--privileged --network host -v sandbox-docker-data:/var/lib/docker`. The volume is critical — without it, the inner dockerd hits `failed to mount overlay: invalid argument` because the container's rootfs is itself overlayfs.
2. **NVIDIA passthrough through 2 layers of Docker**: outer Docker → sandbox container with `--gpus all --runtime=nvidia` → inner dockerd with NVIDIA runtime registered → vLLM container with `device_ids: ["0"]` → CUDA sees the A100. Verified with `nvidia-smi -L` from the inner-most container.
3. **AMD passthrough through 2 layers of Docker**: outer Docker → sandbox container with `--device=/dev/kfd --device=/dev/dri --group-add video` → inner dockerd → ROCm container → `rocm-smi` sees the MI300X. The `--group-add render` flag from `cloudinit.go` had to be dropped on Hot Aisle's host (their host has the `render` *group* but the build script's docker-run was running before the host had injected the group into the container's user → drop it; the container side already has render permissions via the `video` group + `seccomp=unconfined`).
4. **Sandbox image's full lifecycle**: cont-init.d scripts execute cleanly, hydra + sandbox-heartbeat + compose-manager + inference-proxy all start, the "Helix Sandbox Ready" banner prints. compose-manager polls a (fake-for-this-smoke) HTTP API for assignments at 15s intervals as designed.
5. **Real chat completion roundtrip on cloud A100**: `POST /v1/chat/completions` against vLLM-in-inner-DinD returned: *"Yes, I can hear you. How may I assist you today?"* This is the closing-the-loop signal — the new architecture path runs real models on rented cloud GPUs, just as it does on local hardware.

## Open items, not blocking ship

- **inference-proxy active.yaml format**: I wrote a hand-crafted `/etc/helix/active.yaml` to test routing on the Verda VM, but the proxy returned `{"object":"list","data":null}` — the file format I guessed doesn't match what the proxy expects. compose-manager would normally write this file when it applies a profile; the proxy's input contract is a code-level question, not a cloud question. The direct vLLM call on `127.0.0.1:8000` already proved the underlying chain works.
- **8× MI300X validation**: Hot Aisle's bare-metal product was empty on this date and Verda has no L40S/4×/8× SKU stock anywhere. Customer's true Node 5 (8× MI300X with Infinity Fabric) needs either Hot Aisle stock to return or a TensorWave account. Marked `enabled: false` in `matrix.yaml`.
- **Per-session GPU pinning** for multi-GPU hosts: deferred follow-up — see Decision 15 in design.md.

## Concrete fixes folded back into the code

- **`internal/provision/verda.go`**: corrected teardown shape (Verda uses `PUT /v1/instances {id, action:"delete"}` not `DELETE /v1/instances/{id}`); switched auth from static Bearer to OAuth2 client_credentials with token caching (10-min JWT TTL).
- **`internal/provision/hotaisle.go`**: was guessing a SKU-named POST shape, real Hot Aisle API is **spec-matched** — POST `{cpu_cores, ram_capacity, disk_capacity, gpus[{count,manufacturer,model}]}` and they assign a VM matching specs. Added `specsForInstanceType` map for the 1×MI300X SKU.
- **`internal/provision/cloudinit.go`**: documented that `--group-add render` is optional and host-dependent; AMD VMs without the `render` group on the host should drop it.
- **`matrix.yaml`**: rescoped to two enabled smoke entries (1× A100 on Verda + 1× MI300X on Hot Aisle); customer-config 4×/8× entries kept but disabled with notes about stock.

## Reproduce

```bash
# Pre-req: secrets at ~/work/.gpucloud-secrets.env (HOTAISLE_API_KEY, HOTAISLE_TEAM,
# VERDA_CLIENT_ID, VERDA_CLIENT_SECRET, VERDA_SSH_KEY_ID), SSH key at ~/.ssh/helix_gpucloud,
# helix-sandbox image pushed to a public registry (we used ttl.sh for 24h).

set -a && source ~/work/.gpucloud-secrets.env && set +a

# Verda smoke
cd ~/work/helix
go run ./integration-test/gpucloud/cmd/gpucloud-it --only a100-1x-smoke

# Hot Aisle smoke
go run ./integration-test/gpucloud/cmd/gpucloud-it --only mi300x-1x-smoke
```
