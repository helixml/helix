# HelixML runner on k8s

Once you have the control-plane running, install the runner with the following command:

```bash
helm upgrade --install helix-runner \
  ./helix-runner -f helix-runner/values.yaml \
  -f helix-runner/values-example.yaml
```

