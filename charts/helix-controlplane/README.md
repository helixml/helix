# Helix.ML on Kubernetes

> **Do not install this chart from source.** `Chart.yaml` is generated from `Chart.yaml.tmpl` by CI at release time and is not committed to git — installing from a clone produces a chart with a sentinel version that won't match any release. Use the published Helm repository:
>
> ```
> helm repo add helix https://charts.helixml.tech
> helm install <release> helix/helix-controlplane
> ```

Please follow the instructions provided on our website to [install Helix.ML on
Kubernetes](https://helix.ml/docs).

## Developing on the chart

`Chart.yaml` is generated from `Chart.yaml.tmpl` and gitignored, so `helm lint`
and `helm template` will fail with `Chart.yaml file is missing` on a fresh
clone. Render it first from the repo root:

```bash
sh scripts/render-charts.sh
helm dependency build charts/helix-controlplane
helm lint charts/helix-controlplane
```

Without `DRONE_TAG`/`TAG_NAME`, the script stamps a sentinel version that is
fine for lint/template iteration but will not produce a usable release.
