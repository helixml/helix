

# HelixML on k8s

## Setup Keycloak

Helix uses keycloak for authentication. If you have one already, you can skip this step. Otherwise, to install one through Helm ([chart info](https://bitnami.com/stack/keycloak/helm), [repo](https://github.com/bitnami/charts/tree/main/bitnami/keycloak/#installing-the-chart)), do this:

**Note:** Helix includes a custom Keycloak image with the Helix theme pre-installed. Use the following configuration to share the same PostgreSQL database as Helix:

```bash
export LATEST_RELEASE=$(curl -s https://get.helixml.tech/latest.txt)

helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --version "24.3.1" \
  --set global.security.allowInsecureImages=true \
  --set image.registry=registry.helixml.tech \
  --set image.repository=helix/keycloak-bitnami \
  --set image.tag="${HELIX_VERSION}" \
  --set auth.adminUser=admin \
  --set auth.adminPassword=oh-hallo-insecure-password \
  --set httpRelativePath="/auth/" \
  --set postgresql.enabled=false \
  --set externalDatabase.host=my-postgres-host \
  --set externalDatabase.port=5432 \
  --set externalDatabase.user=helix \
  --set externalDatabase.database=helix \
  --set externalDatabase.password=my-secure-password \
  --set externalDatabase.existingSecret=my-postgres-secret \
  --set externalDatabase.existingSecretPasswordKey=password
```

**Note for upgrading from before v2.0:** If you're upgrading from a version before Helix 2.0 and want to keep your existing separate Keycloak database, omit the `postgresql.enabled=false` and `externalDatabase.*` settings from the above command.

Note the pinned version of the chart and the image tag. These are versions that we have tested and are known to work. Newer versions may work, but we have not tested them. [Raise an issue if you have any issues.](https://github.com/helixml/helix/issues)

You do not need to expose a service to access Keycloak from outside the cluster - it is used as an internal implementation detail of Helix (and Helix manages the `helix` Keycloak realm via admin access).

Wait until the Keycloak is running:

```
kubectl get pods
NAME                    READY   STATUS    RESTARTS   AGE
keycloak-0              0/1     Running   0          61s
keycloak-postgresql-0   1/1     Running   0          61s
```

Both pods should turn 1/1 running.

## Database Configuration

Helix requires PostgreSQL for application data and optionally PostgreSQL with the PGVector extension for RAG (Retrieval-Augmented Generation) functionality. You can use vanilla PostgreSQL for the main database and PostgreSQL with the PGVector extension for vectors. Both configurations support bundled deployment or external connection with comprehensive secret support.

### PostgreSQL Configuration

**Bundled PostgreSQL (default)**:
```yaml
postgresql:
  enabled: true
  auth:
    username: helix
    password: "secure-password"
    database: helix
    # Optional: Use existing secret
    # existingSecret: "postgresql-auth-secret"
    # usernameKey: "username"      # defaults to "username"
    # passwordKey: "password"      # defaults to "password"
    # databaseKey: "database"      # defaults to "database"
```

**External PostgreSQL**:
```yaml
postgresql:
  enabled: false
  external:
    host: "my-postgres.example.com"
    port: 5432
    user: "helix"
    password: "secure-password"
    database: "helix"
    # Optional: Use existing secret
    # existingSecret: "postgresql-external-secret"
    # existingSecretHostKey: "host"
    # existingSecretUserKey: "user"
    # existingSecretPasswordKey: "password"
    # existingSecretDatabaseKey: "database"
```

### PostgreSQL with PGVector Extension Configuration (for RAG)

**Bundled PostgreSQL with PGVector**:
```yaml
pgvector:
  enabled: true
  auth:
    username: postgres
    password: "secure-password"
    database: postgres
    # Optional: Use existing secret
    # existingSecret: "pgvector-auth-secret"
    # usernameKey: "username"      # defaults to "username"
    # passwordKey: "password"      # defaults to "password"
    # databaseKey: "database"      # defaults to "database"
```

**External PostgreSQL with PGVector**:
```yaml
pgvector:
  enabled: false
  external:
    host: "my-pgvector.example.com"
    port: 5432
    user: "postgres"
    password: "secure-password"
    database: "postgres"
    # Optional: Use existing secret
    # existingSecret: "pgvector-external-secret"
    # existingSecretHostKey: "host"
    # existingSecretUserKey: "user"
    # existingSecretPasswordKey: "password"
    # existingSecretDatabaseKey: "database"
```

**Important**: External PostgreSQL with PGVector must have the `vector`, `vectorchord`, and `vectorchord-bm25` extensions installed. The bundled PostgreSQL with PGVector uses an image which includes all required extensions.

### Database Secrets Management

For production deployments, use Kubernetes secrets instead of plain text passwords:

```bash
# Create PostgreSQL secret
kubectl create secret generic postgresql-auth-secret \
  --from-literal=username=helix \
  --from-literal=password=secure-password \
  --from-literal=database=helix

# Create PostgreSQL with PGVector secret  
kubectl create secret generic pgvector-auth-secret \
  --from-literal=username=postgres \
  --from-literal=password=secure-password \
  --from-literal=database=postgres

# Create controlplane secrets
kubectl create secret generic runner-token-secret \
  --from-literal=token=your-secure-runner-token-here

kubectl create secret generic keycloak-auth-secret \
  --from-literal=user=admin \
  --from-literal=password=your-secure-keycloak-admin-password

# Create provider API key secrets
kubectl create secret generic openai-credentials \
  --from-literal=api-key=sk-your-openai-api-key

kubectl create secret generic anthropic-credentials \
  --from-literal=api-key=sk-ant-your-anthropic-api-key

kubectl create secret generic together-credentials \
  --from-literal=api-key=your-together-api-key

kubectl create secret generic vllm-credentials \
  --from-literal=api-key=your-vllm-api-key
```

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
