#!/bin/bash
set -xeuo pipefail

#curl -fsSL https://raw.githubusercontent.com/helixml/llms-on-k8s/refs/heads/main/helix/values-vllm.yaml -o values-vllm.yaml

export KUBECONFIG=/etc/rancher/k3s/k3s.yaml && \
IP=$(curl -s https://ip4only.me/api/ | cut -d, -f2) && \
HYPHENATED_IP=$(echo $IP | tr . -) && \
HTTPS_URL="https://${HYPHENATED_IP}.helix.cluster.world" && \
LATEST_RELEASE=$(curl -s https://get.helixml.tech/latest.txt) && \
helm upgrade --install my-helix-controlplane ../charts/helix-controlplane \
-f values-vllm.yaml \
--set global.serverUrl="${HTTPS_URL}" \
--set image.tag="${LATEST_RELEASE}" \
--set envVariables.APP_URL="${HTTPS_URL}" \
--set envVariables.KEYCLOAK_FRONTEND_URL="${HTTPS_URL}/auth"
