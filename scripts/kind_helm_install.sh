#!/bin/bash

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

# Check if kind is installed
check_command "kind"

# Check if Docker is running
check_command "docker"

# Check if kubectl is installed
check_command "kubectl"

# Check if helm is installed
check_command "helm"

# Check that user has TOGETHER_API_KEY set
if [ -z "$TOGETHER_API_KEY" ]; then
    echo "TOGETHER_API_KEY is not set. Please set it before proceeding."
    exit 1
fi

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

# Install Keycloak using Helm
helm upgrade --install keycloak oci://registry-1.docker.io/bitnamicharts/keycloak \
  --set auth.adminUser=admin \
  --set auth.adminPassword=oh-hallo-insecure-password \
  --set httpRelativePath="/auth/"

# Wait for pod to exist
echo "Waiting for Keycloak pod to exist..."
until echo $(kubectl get pod -l app.kubernetes.io/name=keycloak) | grep "keycloak"; do
    sleep 2
    echo -n "."
done

# Wait for Keycloak to be ready
echo "Waiting for Keycloak to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=keycloak --timeout=300s

# Add the Helix Helm chart repository
echo "Adding the Helix Helm chart repository..."
helm repo add helix https://charts.helix.ml
helm repo update

# Grab the latest values-example.yaml
echo "Downloading the latest values-example.yaml..."
curl -o $DIR/values-example.yaml https://raw.githubusercontent.com/helixml/helix/main/charts/helix-controlplane/values-example.yaml

# Install Helix using Helm
echo "Installing Helix..."
export LATEST_RELEASE=$(curl -s https://get.helix.ml/latest.txt)
helm upgrade --install my-helix-controlplane helix/helix-controlplane \
  -f $DIR/values-example.yaml \
  --set image.tag="${LATEST_RELEASE}" \
  --set envVariables.TOGETHER_API_KEY=${TOGETHER_API_KEY}