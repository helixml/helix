#!/bin/sh
# Renders charts/*/Chart.yaml from charts/*/Chart.yaml.tmpl by stamping the
# release version into the __VERSION__ placeholder.
#
# This chart source is NOT meant for direct installation. Install Helix from
# the published helm repository:
#
#   helm repo add helix https://charts.helixml.tech
#   helm install ... helix/helix-controlplane
#
# Render version is taken from ${DRONE_TAG} or ${TAG_NAME} (CI tag builds),
# leading 'v' stripped. Without a tag, the script renders with a sentinel
# version "0.0.0-source-not-installable" so any accidental local install
# produces a chart that obviously isn't a real release.

set -e

VERSION="${DRONE_TAG:-${TAG_NAME:-}}"
VERSION="${VERSION#v}"

if [ -z "${VERSION}" ]; then
  VERSION="0.0.0-source-not-installable"
  echo "WARN: no DRONE_TAG/TAG_NAME set; rendering sentinel VERSION=${VERSION}." >&2
  echo "WARN: this output is for CI lint/template use only — do NOT helm install from source." >&2
  echo "WARN: real installs use https://charts.helixml.tech." >&2
fi

if ! echo "${VERSION}" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'; then
  echo "ERROR: VERSION '${VERSION}' is not valid semver" >&2
  exit 1
fi

echo "Rendering Chart.yaml files with VERSION=${VERSION}"

for tmpl in charts/*/Chart.yaml.tmpl; do
  [ -f "${tmpl}" ] || continue
  out="${tmpl%.tmpl}"
  sed "s/__VERSION__/${VERSION}/g" "${tmpl}" > "${out}"
  echo "  ${out}"
done
