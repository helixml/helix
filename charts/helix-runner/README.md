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

Set `replicaCount` to the number of GPUs you want to deploy the runner on. You can also target specific GPUs, e.g. `--set nodeSelector."nvidia\.com/gpu\.product"="NVIDIA-GeForce-RTX-3090-Ti"`

## Troubleshooting

### Single GPU Upgrade Deadlocks

If you're running with a single GPU (`nvidia.com/gpu: 1`) and experiencing deadlocks during upgrades, this is caused by Kubernetes' default RollingUpdate strategy trying to start a new pod before terminating the old one. Since both pods compete for the same GPU resource, the deployment gets stuck.

**Solution**: The chart automatically detects single GPU configurations and uses the `Recreate` deployment strategy to prevent this issue. You can also manually control this behavior:

```yaml
# In your values file
deploymentStrategy: "Recreate"  # Force recreate strategy
# or
deploymentStrategy: "auto"      # Auto-detect (default)
# or  
deploymentStrategy: "RollingUpdate"  # Force rolling update
```

The `auto` setting (default) will:
- Use `Recreate` strategy when `nvidia.com/gpu: 1` 
- Use `RollingUpdate` strategy for multi-GPU setups
