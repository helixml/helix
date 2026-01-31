# HelixML runner on k8s

Once you have the control-plane running, install the runner with the following command:

```bash
helm upgrade --install helix-runner \
  ./helix-runner -f helix-runner/values.yaml \
  -f helix-runner/values-example.yaml
```

## Install from repository

```bash
helm repo add helix https://charts.helixml.tech 
helm repo update
```

Then, install the runner:

```bash
helm upgrade --install helix-runner helix/helix-runner \
  --set runner.host="<host>" \
  --set runner.token="<token>" \
  --set replicaCount=4
```

Set `replicaCount` to the number of runner pods you want to deploy. You can also target specific GPUs, e.g. `--set nodeSelector."nvidia\.com/gpu\.product"="NVIDIA-GeForce-RTX-3090-Ti"`

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
