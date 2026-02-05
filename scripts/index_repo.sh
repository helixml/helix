#!/bin/sh
set -e

echo "Installing curl"
apt update
apt install curl -y

echo "Installing helm"
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh

echo "Indexing repository"
# The index.yaml is downloaded to temp/ by the previous gsutil rsync step
if [ -f temp/index.yaml ]; then
  echo "Merging with existing index.yaml"
  helm repo index --url "${REPO_URL}" --merge temp/index.yaml ./temp
else
  echo "Creating new index.yaml"
  helm repo index --url "${REPO_URL}" ./temp
fi
