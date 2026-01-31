#!/bin/sh
set -e

echo "Installing curl"
apt update
apt install curl -y

echo "Installing helm"
# curl https://raw.githubusercontent.com/kubernetes/helm/master/scripts/get | bash
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh

echo "Packaging charts from source code"
mkdir -p temp
for d in charts/*
do
 # shellcheck disable=SC3010
 if [[ -d $d ]]
 then
    # Will generate a helm package per chart in a folder
    echo "$d"

    # Fetch Helm chart dependencies from OCI registries before packaging
    # This is required for charts that reference remote dependencies (e.g., bitnami charts)
    # without bundled .tgz files in the charts/ subdirectory.
    #
    # The helix-controlplane chart declares dependencies like:
    #   - postgresql: oci://registry-1.docker.io/bitnamicharts
    #   - common: oci://registry-1.docker.io/bitnamicharts
    #
    # Running `helm dependency update` downloads these from the remote registry
    # and creates them in the chart's charts/ directory so they can be packaged
    # together with the main chart.
    helm dependency update "$d" || true

    helm package "$d"
    # shellcheck disable=SC2035
    mv *.tgz temp/
  fi
done