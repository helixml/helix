

# HelixML on k8s

## Setup Keycloak

Helix uses keycloak for authentication. If you have one already, you can skip this step. Otherwise, to install one through Helm ([chart info](https://bitnami.com/stack/keycloak/helm), [repo](https://github.com/bitnami/charts/tree/main/bitnami/keycloak/#installing-the-chart)), do this:

**Note:** Helix includes a custom Keycloak image with the Helix theme pre-installed. Use the following configuration:

```bash
export LATEST_RELEASE=$(curl -s https://get.helixml.tech/latest.txt)

helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --version "24.3.1" \
  --set auth.adminUser=admin \
  --set auth.adminPassword=oh-hallo-insecure-password \
  --set image.registry=registry.helixml.tech \
  --set image.repository=helix/keycloak \
  --set image.tag="${LATEST_RELEASE}" \
  --set httpRelativePath="/auth/" 
```

By default it only has ClusterIP service, in order to expose it, you can either port-forward or create a load balancer to access it if you are on k3s or minikube:

```
kubectl expose pod keycloak-0 --port 8888 --target-port 8080 --name keycloak-ext --type=LoadBalancer
```

Alternatively, if you run on k3s:

```
export LATEST_RELEASE=$(curl -s https://get.helixml.tech/latest.txt)

helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --version "24.3.1" \
  --set auth.adminUser=admin \
  --set auth.adminPassword=oh-hallo-insecure-password \
  --set image.registry=registry.helixml.tech \
  --set image.repository=helix/keycloak \
  --set image.tag="${LATEST_RELEASE}" \
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

Get your License key from https://deploy.helix.ml/licenses. And create a secret with it:

```bash
kubectl create secret generic helix-license --from-literal=license="<base64 encoded secret contents here>"
```

Copy the values-example.yaml to values-your-env.yaml and update the values as needed. Then run the following command (just with your own file):

```bash
export LATEST_RELEASE=$(curl -s https://get.helixml.tech/latest.txt)

helm upgrade --install helix \
  ./helix-controlplane \
  -f helix-controlplane/values.yaml \
  -f helix-controlplane/values-example.yaml \
  --set image.tag="${LATEST_RELEASE}"
```

Use port-forward to access the service.

## Connecting runners

You can connect runners through [Docker](https://docs.helixml.tech/helix/private-deployment/docker/), [Docker Compose](https://github.com/helixml/helix/blob/main/docker-compose.runner.yaml), [Synpse](https://cloud.synpse.net/templates?id=helix-runner), [Runpod](https://docs.helixml.tech/helix/private-deployment/runpod/), [LambdaLabs](https://docs.helixml.tech/helix/private-deployment/lambdalabs/) or [Kubernetes chart](../helix-runner) 
