# HelixML runner on k8s

Once you have the control-plane running, install the runner with the following command:

```bash
helm upgrade --install helix-runner \
  ./helix-runner -f helix-runner/values.yaml \
  -f helix-runner/values-example.yaml
```

## Install from repository

```bash
helm repo add helix https://charts.helix.ml 
helm repo update
```

Then, install the runner:

```bash
helm upgrade --install helix-3090-runner helix/helix-runner \
  --set runner.host="<host>" \
  --set runner.token="<token>" \
  --set runner.memory=24GB \
  --set replicaCount=4 \
  --set nodeSelector."nvidia\.com/gpu\.product"="NVIDIA-GeForce-RTX-3090-Ti"
```