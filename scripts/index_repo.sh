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
if [ -f index.yaml ]; then
  helm repo index --url "${REPO_URL}" --merge index.yaml ./temp
else
  helm repo index --url "${REPO_URL}" ./temp
fi
