

# HelixML on k8s

## Setup Keycloak

Helix uses keycloak for authentication. If you have one already, you can skip this step. Otherwise, to install one through Helm ([chart info](https://bitnami.com/stack/keycloak/helm), [repo](https://github.com/bitnami/charts/tree/main/bitnami/keycloak/#installing-the-chart)), do this:

Some of the values:

```bash
helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --set auth.adminUser=admin \
  --set auth.adminPassword=oh-hallo-insecure-password \
  --set httpRelativePath="/auth/" 
```

By default it only has ClusterIP service, in order to expose it, you can either port-forward or create a load balancer to access it if you are on k3s or minikube:

```
kubectl expose pod keycloak-0 --port 8888 --target-port 8080 --name keycloak-ext --type=LoadBalancer
```

Alternatively, if you run on k3s:

```
helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --set auth.adminUser=admin \
  --set auth.adminPassword=oh-hallo-insecure-password \
  --set httpRelativePath="/auth/" \
  --set service.type=LoadBalancer \
  --set service.ports.http=8888
```

Wait until the Keycloak is running:

```
kubectl get pods
NAME                    READY   STATUS    RESTARTS   AGE
keycloak-0              0/1     Running   0          61s
keycloak-postgresql-0   1/1     Running   0          61s
```

Both pods should turn 1/1 running.

## Setup Helix

Copy the values-example.yaml to values-your-env.yaml and update the values as needed. Then run the following command (just with your own file):

```bash
helm upgrade --install helix \
  ./helix-controlplane \
  -f helix-controlplane/values.yaml \
  -f helix-controlplane/values-example.yaml
```

Use port-forward to access the service.

## Ingress

When configuring ingress, adjust both global.serverUrl to your domain name and keycloak's frontend URL to the same domain name. This is important for the redirects to work.

## Connecting runners

You can connect runners through [Docker](https://docs.helix.ml/helix/private-deployment/docker/), [Docker Compose](https://github.com/helixml/helix/blob/main/docker-compose.runner.yaml), [Synpse](https://cloud.synpse.net/templates?id=helix-runner), [Runpod](https://docs.helix.ml/helix/private-deployment/runpod/), [LambdaLabs](https://docs.helix.ml/helix/private-deployment/lambdalabs/) or [Kubernetes chart](../helix-runner) 
