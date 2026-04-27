# HelixML runner on k8s

> **Do not install this chart from source.** `Chart.yaml` is generated from `Chart.yaml.tmpl` by CI at release time and is not committed to git — installing from a clone produces a chart with a sentinel version that won't match any release. Use the published Helm repository below.

## Install from repository

Once you have the control-plane running:

```bash
helm repo add helix https://charts.helixml.tech
helm repo update

helm upgrade --install helix-runner helix/helix-runner \
  --set runner.host="<host>" \
  --set runner.token="<token>" \
  --set replicaCount=4
```

Set `replicaCount` to the number of runner pods you want to deploy. You can also target specific GPUs, e.g. `--set nodeSelector."nvidia\.com/gpu\.product"="NVIDIA-GeForce-RTX-3090-Ti"`

## Developing on the chart

`Chart.yaml` is generated from `Chart.yaml.tmpl` and gitignored, so `helm lint`
and `helm template` will fail with `Chart.yaml file is missing` on a fresh
clone. Render it first from the repo root:

```bash
sh scripts/render-charts.sh
helm lint charts/helix-runner
```

Without `DRONE_TAG`/`TAG_NAME`, the script stamps a sentinel version that is
fine for lint/template iteration but will not produce a usable release.

## Multi-GPU Support

Each runner pod can manage multiple GPUs. Configure this with the `gpuCount` setting:

```bash
# Single runner managing 4 GPUs
helm upgrade --install helix-runner helix/helix-runner \
  --set runner.host="<host>" \
  --set runner.token="<token>" \
  --set gpuCount=4 \
  --set replicaCount=1

# Or 2 runners each managing 2 GPUs  
helm upgrade --install helix-runner helix/helix-runner \
  --set runner.host="<host>" \
  --set runner.token="<token>" \
  --set gpuCount=2 \
  --set replicaCount=2
```

The runner will automatically detect and use all GPUs allocated to it by Kubernetes. VLLM and Ollama will handle model loading across multiple GPUs on the same runner.

## AMD GPUs (ROCm)

By default the chart requests `nvidia.com/gpu`. On AMD clusters using the ROCm device plugin, override the resource key:

```bash
helm upgrade --install helix-runner helix/helix-runner \
  --set runner.host="<host>" \
  --set runner.token="<token>" \
  --set gpuResourceKey="amd.com/gpu" \
  --set gpuCount=1
```

The runner pod will request `amd.com/gpu: "1"` instead of `nvidia.com/gpu: "1"`. No other changes to `resources:` are needed — that block is only for non-GPU resources (CPU, memory, hugepages, etc.).

## Troubleshooting

### GPU Upgrade Deadlocks

If you're running with GPUs and experiencing deadlocks during upgrades, this is caused by Kubernetes' default RollingUpdate strategy trying to start a new pod before terminating the old one. Since both pods compete for the same GPU resources, the deployment gets stuck.

**Solution**: The chart automatically detects GPU configurations and uses the `Recreate` deployment strategy to prevent this issue. You can also manually control this behavior:

```yaml
# In your values file
deploymentStrategy: "Recreate"  # Force recreate strategy
# or
deploymentStrategy: "auto"      # Auto-detect (default)
# or  
deploymentStrategy: "RollingUpdate"  # Force rolling update
```

The `auto` setting (default) will:
- Use `Recreate` strategy when `gpuCount > 0` (any GPU allocation)
- Use `RollingUpdate` strategy for CPU-only setups (`gpuCount: 0`)
