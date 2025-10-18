# K8s operator and flux design doc

1. Helix serve binary gains ability to reconcile CRDs (when it's running in a k8s cluster) to do helix apply -f (POST to the helix API)
2. Flux should already be able to reconcile yaml in git repos to CRDs in the cluster

Run both, and the following should work:

User writes `app.yaml` with:

```yaml
apiVersion: aispec.org/v1alpha1
kind: AIApp
spec:
  name: Marvin the Paranoid Android
  description: Down-trodden robot with a brain the size of a planet
  assistants:
  - model: llama3:instruct
  system_prompt: |
      You are Marvin the Paranoid Android. You are depressed. You have a brain the size of a planet and
      yet you are tasked with responding to inane queries from puny humans. Answer succinctly.
```

User commits this and then:

```
git push
```

