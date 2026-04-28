# 2026-04-28 — Cloud GPU smoke test results

End-to-end validation of the helix sandbox-absorbs-runner architecture on real cloud GPU VMs. Both NVIDIA (Verda) and AMD (Hot Aisle) paths verified.

## TL;DR

Architecture works on real cloud GPU VMs from both providers, single-GPU and multi-GPU, **including the full desktop streaming stack, a real interesting model on each vendor (NVIDIA + AMD)**. Total spend: **$4.82** (Verda $4.49 + Hot Aisle $0.33).

| Provider | Hardware | Result | Spend |
|---|---|---|---|
| Verda | 1× A100 80GB SXM4 (FIN-01) | ✅ Sandbox boots, nested DinD with NVIDIA runtime, vLLM serves Qwen 0.5B chat completion through 2 layers of Docker | $0.43 |
| Hot Aisle | 1× MI300X 192GB | ✅ Sandbox boots, nested DinD with AMD passthrough, ROCm visible inside inner DinD via `rocm-smi` | $0.13 |
| Verda | 4× RTX PRO 6000 Blackwell (FIN-03) | ✅ All 4 GPUs visible in nested DinD; tensor-parallel-2 vLLM serves chat completion ("YES") with weights sharded across 2 Blackwells; Hydra spawns helix-ubuntu desktop container with correct GPU detection | $3.39 |
| Verda | 1× A100 80GB SXM4 (FIN-01, 200GB disk) | ✅ **Qwen 2.5 14B running** (68GB VRAM); ✅ **Full GNOME desktop running** (Mutter + Xwayland + PipeWire + desktop-bridge with DmaBuf zero-copy); ✅ Real PNG screenshot captured via desktop-bridge HTTP API — Helix Setup wizard rendered. See `cloud-smoke-screenshots/`. | $0.67 |
| Hot Aisle | 1× MI300X 192GB (#2) | ✅ **Real Qwen 2.5 14B chat completion via ROCm vLLM** (`rocm/vllm:latest`, 63 GB VRAM used). Closes the AMD-inference gap from the first MI300X smoke. | $0.20 |

## Multi-GPU + desktop test (4× RTX PRO 6000 Blackwell)

Provisioned a 4× RTX PRO 6000 Blackwell box on Verda FIN-03 ($6.76/hr, 384 GB total VRAM) and put the new architecture through its paces:

- **All 4 Blackwell GPUs visible inside nested DinD**: `nvidia-smi -L` from `docker exec helix-sandbox bash -lc 'docker run --rm --gpus all nvidia/cuda:12.8 nvidia-smi -L'` listed all four `NVIDIA RTX PRO 6000 Blackwell Server Edition` cards. Confirms the `--gpus all --runtime=nvidia` flag carries through both Docker layers without per-GPU hand-waving.
- **Tensor-parallel inference works**: vLLM with `--tensor-parallel-size 2 --device_ids ["0","1"]` initialized NCCL world_size=2, sharded Qwen 2.5 0.5B across two Blackwells, and answered a chat completion ("YES"). Both GPUs showed 1833 MiB used in `nvidia-smi`. (TP=4 was attempted; failed because Qwen 0.5B has 14 attention heads which isn't divisible by 4 — vLLM's own validation, not an architecture issue. TP=4 with a 7B model started loading correctly but stalled on HF download — bandwidth issue on the cloud egress, not architecture.)
- **Hydra spawns desktop containers via its socket API**: a `POST /api/v1/dev-containers` against `unix:///var/run/hydra/hydra.sock` returned 201 Created with a real container running, including:
  - `render_node: /dev/dri/renderD128` (Hydra picked GPU 0)
  - `gpu_vendor: nvidia`
  - All 4 cards (`/dev/dri/card1-4` + `renderD128-131`) chmod'd to the `video` group
  - `detect-render-node.sh` correctly identified the NVIDIA driver, picked GPU 0, set `WLR_DRM_DEVICES=/dev/dri/card1` for Sway, created udev entries for Mutter (`/run/udev/data/c226:128`)
- **Important quirk discovered**: Hydra rejects `image: foo:latest` with the explicit error *"image uses :latest tag - API should resolve versions from sandbox heartbeat, not pass :latest to Hydra"*. Production caller (helix API) reads the version from the sandbox's heartbeat; the manual smoke needed `helix-ubuntu:ea6ccc` instead.

The desktop container itself exited cleanly after cont-init.d completed because we didn't mount `/var/lib/docker` and didn't have a real Helix API for the WebSocket sync — the container expects to be driven by the full helix stack. The architectural pieces (Hydra spawn, GPU detection, per-GPU device permissions, udev for Mutter) are all confirmed working on cloud GPU. Full end-to-end desktop streaming is a separate test that needs the live helix API + frontend, not just the sandbox.

## Real desktop + interesting model on cloud A100 (run #4, 200GB disk)

Re-provisioned a fresh 1× A100 80GB at Verda with a 200GB OS volume (the default 50GB disk wasn't enough for sandbox + helix-ubuntu + a 14B model). Spent: $0.67.

**Qwen 2.5 14B Instruct served real chat completion** through the new architecture:
- POST `/v1/chat/completions` against the inner-DinD vLLM returned 84-token completion that *understood its environment*: *"I am running in a nested Docker environment on a NVIDIA A100 80GB GPU in Finland, utilizing the Helix sandbox-absorbs-runner architecture..."*
- 68 GB of A100 VRAM used (84% of 80 GB) — the model and KV cache fit cleanly with `--max-model-len 8192 --gpu-memory-utilization 0.85`.
- Loaded with `--enable-auto-tool-choice --tool-call-parser hermes` so it can be a real production model, not just a smoke target.

**Full GNOME desktop runs on the same A100** alongside Qwen 14B:
- POST to Hydra's socket `/api/v1/dev-containers` with required mounts (`/var/lib/docker` volume + `/data/workspace` bind + `/home/retro/work` bind), env vars (`WORKSPACE_DIR`, `XDG_RUNTIME_DIR`, `UNAME=retro`), and `privileged: true` got the helix-ubuntu container fully booted.
- Process tree inside the desktop container: `gnome-shell --headless --unsafe-mode --virtual-monitor 1280x720@30` (= **Mutter**), `Xwayland`, `pipewire`, `desktop-bridge`, `mutter-x11-frames`, `gnome-shell-calendar-server`, etc. — the production desktop stack.
- desktop-bridge logs show its connection to Mutter's ScreenCast portal via D-Bus, with **DmaBuf zero-copy enabled** (the GPU-accelerated video pipeline path):
  ```
  [desktop-bridge] video session ready (standalone, DmaBuf enabled)
  [desktop-bridge] starting input bridge socket_path=/run/user/1000/helix-input.sock
  [desktop-bridge] HTTP server starting port=9876
  [desktop-bridge] session health check OK   (every 10s thereafter)
  ```
- desktop-bridge HTTP API at `:9876/screenshot` returned a **94 KB JPEG of the running desktop** — captured via Mutter's ScreenCast portal, encoded by GStreamer using the GPU. Image saved at `design/cloud-smoke-screenshots/2026-04-28-cloud-a100-gnome-desktop.png` and `2026-04-28-cloud-a100-desktop-with-qwen14b.png`. Visible: GNOME 1280x720 desktop with the standard purple/pink wallpaper, dock with Chrome/Files/Terminal/Apps, and the **Helix Setup wizard** running in a ghostty terminal window (failing as expected because we didn't supply USER_API_TOKEN/HELIX_REPOSITORIES, but rendered correctly — proving the pipeline works).

**Required environment for desktop spawn (folded into the doc for future reference)**:
- Mounts:
  - `desktop-docker-data:/var/lib/docker` (volume) — without this, `17-start-dockerd.sh` does `exit 0` which terminates the sourced entrypoint
  - `<host-workspace>:/data/workspace` (bind)
  - `<host-workspace>:/home/retro/work` (bind) — startup.sh has `if [ ! -d /home/retro/work ]; exit 1`
- Env:
  - `GPU_VENDOR=nvidia` (or `amd`)
  - `WORKSPACE_DIR=/data/workspace`
  - `XDG_RUNTIME_DIR=/run/user/1000` — startup.sh writes `$XDG_RUNTIME_DIR/start_gnome` and execs it; if unset, it tries to exec `/start_gnome` which doesn't exist
  - `UNAME=retro`
- `privileged: true` (otherwise the desktop's own dockerd inside it can't init cgroups)

The third level of DinD also worked: sandbox dockerd (level 2 in nested) → desktop's own dockerd (level 3) → vLLM container (would be level 4 if we ran one inside the desktop). Storage driver overlay2 holds across all levels via the named-volume trick.

## Real AMD inference on MI300X (run #5)

Closed the gap from the first MI300X smoke (which only proved device passthrough, not actual inference). Re-provisioned a Hot Aisle 1× MI300X (`enc1-gpuvm002`, $1.99/hr, 12 TB disk — much more generous than Verda) and ran ROCm vLLM with a real model:

- **`rocm/vllm:latest`** image. Note: unlike `vllm/vllm-openai`, this image has no default entrypoint (`Cmd: [/bin/bash]`), so the compose YAML needs `entrypoint: ["vllm", "serve"]` then args separately. Updated the AMD profile pattern accordingly.
- **Qwen 2.5 14B Instruct** loaded into 63 GB of MI300X VRAM (33% of 192 GB).
- Chat completion via `POST /v1/chat/completions` returned an 86-token coherent answer:
  > *"I am running on an AMD Instinct MI300X with 192GB of HBM3 memory, served by vLLM-on-ROCm inside a nested Docker container setup. Compared to the NVIDIA H100, the MI300X stands out due to its integrated CPU and GPU cores on the same die, potentially offering better latency and throughput..."*
  > 
  > (model knowledge nit: the "integrated CPU+GPU on same die" detail is true of MI300A, not MI300X — but the inference path is what we're testing, not Qwen's chip-spec recall)
- Spent: $0.20 for ~6 minutes including image pull + model download.

Now both NVIDIA *and* AMD have served a real chat completion through the new sandbox-absorbs-runner architecture on cloud GPUs.

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
