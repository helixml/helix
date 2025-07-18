#!/bin/bash
set -euo pipefail

# USE_LOCAL_HELM_CHART=1
# This option will use the local helm chart in the charts/helix-controlplane directory, rather than
# the official one from the helixml/helix repository.
USE_LOCAL_HELM_CHART=${USE_LOCAL_HELM_CHART:-""}

# USE_EXTERNAL_POSTGRES=1 
# This option is used to test the chart when using an external postgres instance.
# The script will install an independent postgres instance using the official helm chart.
# Both keycloak and helix will be configured to use the external postgres instance.
USE_EXTERNAL_POSTGRES=${USE_EXTERNAL_POSTGRES:-""}

# Function to check if a command is installed
check_command() {
  if ! command -v $1 &> /dev/null
  then
    echo "$1 could not be found, please install it before proceeding."
    exit 1
  else
    echo "$1 is installed."
  fi
}

# Function to check if a pod is ready
wait_for_pod_ready_with_label() {
  # Wait for pod to exist
  echo "Waiting for $1 pod to exist..."
  # Check that there is at least one pod with the label.
  # use jsonpath to get the labels
  until kubectl get pod -l $1 -o jsonpath='{.items[0].metadata.labels}'; do
      sleep 2
      echo -n "."
  done

  # Wait for pod to be ready
  echo "Waiting for $1 pod to be ready..."
  kubectl wait --for=condition=ready pod -l $1 --timeout=300s
}

# Check if kind is installed
check_command "kind"

# Check if Docker is running
check_command "docker"

# Check if kubectl is installed
check_command "kubectl"

# Check if helm is installed
check_command "helm"

# Set the cluster name
CLUSTER_NAME="my-helix-cluster"

# Get a temporary working directory
DIR=$(mktemp -d)

# If the cluster already exists, delete it
if kind get clusters | grep -q $CLUSTER_NAME; then
  echo "Deleting existing cluster named $CLUSTER_NAME..."
  kind delete cluster --name $CLUSTER_NAME
fi

# Create a kind cluster with the specified name
echo "Creating a kind cluster named $CLUSTER_NAME..."
kind create cluster --name $CLUSTER_NAME

# Set kubectl context to the newly created cluster
echo "Setting kubectl context to $CLUSTER_NAME..."
kubectl cluster-info --context kind-$CLUSTER_NAME

# Install external postgres
if [ -n "$USE_EXTERNAL_POSTGRES" ] && [ "$USE_EXTERNAL_POSTGRES" != "false" ] && [ "$USE_EXTERNAL_POSTGRES" != "" ]; then
  echo "Using external postgres..."

  # Use cloudnative-pg to install postgres to match one of our clients.
  kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.26/releases/cnpg-1.26.0.yaml

  # Create a really basic cluster
  cat <<EOF > $DIR/cluster-example.yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: helix-external-postgres
spec:
  instances: 3
  storage:
    size: 1Gi
EOF

  # Apply the cluster, repeat until it succeeds.
  while ! kubectl apply -f $DIR/cluster-example.yaml; do
    sleep 1
  done

  # Verify that the secret is created
  while ! kubectl get secret helix-external-postgres-app; do
    sleep 1
  done

  wait_for_pod_ready_with_label "cnpg.io/cluster=helix-external-postgres"
fi


HELM_VALUES_KEYCLOAK=()
HELM_VALUES_KEYCLOAK+=("--set" "auth.adminUser=admin")
HELM_VALUES_KEYCLOAK+=("--set" "auth.adminPassword=oh-hallo-insecure-password")
HELM_VALUES_KEYCLOAK+=("--set" "httpRelativePath=/auth/")
if [ -n "$USE_EXTERNAL_POSTGRES" ] && [ "$USE_EXTERNAL_POSTGRES" != "false" ] && [ "$USE_EXTERNAL_POSTGRES" != "" ]; then
  HELM_VALUES_KEYCLOAK+=("--set" "postgresql.enabled=false")
  HELM_VALUES_KEYCLOAK+=("--set" "externalDatabase.existingSecret=helix-external-postgres-app")
  HELM_VALUES_KEYCLOAK+=("--set" "externalDatabase.existingSecretHostKey=host")
  HELM_VALUES_KEYCLOAK+=("--set" "externalDatabase.existingSecretPortKey=port")
  HELM_VALUES_KEYCLOAK+=("--set" "externalDatabase.existingSecretUserKey=user")
  HELM_VALUES_KEYCLOAK+=("--set" "externalDatabase.existingSecretDatabaseKey=dbname")
  HELM_VALUES_KEYCLOAK+=("--set" "externalDatabase.existingSecretPasswordKey=password")
fi

# Install Keycloak using Helm with official image
helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --version "24.3.1" \
  "${HELM_VALUES_KEYCLOAK[@]}"

# # TODO: This is OOMKilled on my machine. Likely best fix is to update the base image.
# # Install Keycloak using Helm with custom Helix image
# export KEYCLOAK_VERSION=${HELIX_VERSION:-$(curl -s https://get.helixml.tech/latest.txt)}
# helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
#   --set global.security.allowInsecureImages=true \
#   --version "24.3.1" \
#   --set auth.adminUser=admin \
#   --set auth.adminPassword=oh-hallo-insecure-password \
#   --set image.registry=registry.helixml.tech \
#   --set image.repository=helix/keycloak-bitnami \
#   --set image.tag="${KEYCLOAK_VERSION}" \
#   --set httpRelativePath="/auth/"

wait_for_pod_ready_with_label "app.kubernetes.io/name=keycloak"

if [ -n "$USE_LOCAL_HELM_CHART" ] && [ "$USE_LOCAL_HELM_CHART" != "false" ] && [ "$USE_LOCAL_HELM_CHART" != "" ]; then
  echo "Using local Helm chart..."

  # Change to the root directory of the Helix project
  cd "$(dirname "$0")/.."

  # Ensure we're in the correct directory
  if [ ! -d "./charts/helix-controlplane" ]; then
    echo "Error: charts/helix-controlplane directory not found. Make sure you're running this script from the root of the Helix project."
    exit 1
  fi

  cp charts/helix-controlplane/values-example.yaml $DIR/values-example.yaml

  CHART=./charts/helix-controlplane
else
  # Add the Helix Helm chart repository
  echo "Adding the Helix Helm chart repository..."
  helm repo add helix --force-update https://charts.helixml.tech
  helm repo update
  # Grab the latest values-example.yaml
  echo "Downloading the latest values-example.yaml..."
  curl -o $DIR/values-example.yaml https://raw.githubusercontent.com/helixml/helix/main/charts/helix-controlplane/values-example.yaml

  CHART=helix/helix-controlplane
fi

# Install Helix using Helm
echo "Installing Helix..."
export HELIX_VERSION=${HELIX_VERSION:-$(curl -s https://get.helixml.tech/latest.txt)}

# Initialize an empty array for dynamic values
HELM_VALUES=()

# Add base values
HELM_VALUES+=("-f" "$DIR/values-example.yaml")
HELM_VALUES+=("--set" "image.tag=${HELIX_VERSION}")
HELM_VALUES+=("--set" "controlplane.keycloak.user=admin")
HELM_VALUES+=("--set" "controlplane.keycloak.password=oh-hallo-insecure-password")

if [ -n "$USE_EXTERNAL_POSTGRES" ] && [ "$USE_EXTERNAL_POSTGRES" != "false" ] && [ "$USE_EXTERNAL_POSTGRES" != "" ]; then
  HELM_VALUES+=("--set" "postgresql.enabled=false")
  HELM_VALUES+=("--set" "postgresql.external.existingSecret=helix-external-postgres-app")
  HELM_VALUES+=("--set" "postgresql.external.existingSecretHostKey=host")
  HELM_VALUES+=("--set" "postgresql.external.existingSecretPortKey=port")
  HELM_VALUES+=("--set" "postgresql.external.existingSecretUserKey=user")
  HELM_VALUES+=("--set" "postgresql.external.existingSecretDatabaseKey=dbname")
  HELM_VALUES+=("--set" "postgresql.external.existingSecretPasswordKey=password")
fi

# Execute helm command with all accumulated values
helm upgrade --install my-helix-controlplane $CHART "${HELM_VALUES[@]}"

wait_for_pod_ready_with_label "app.kubernetes.io/name=helix-controlplane"