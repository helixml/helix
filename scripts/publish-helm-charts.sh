#!/bin/sh
set -eu

VERSION="${DRONE_TAG:-${TAG_NAME:-}}"
VERSION="${VERSION#v}"
GCS_BUCKET="gs://charts.helixml.tech"
REPO_URL="https://charts.helixml.tech"

if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'; then
  echo "ERROR: release tag is not valid semver: ${VERSION:-<empty>}" >&2
  exit 1
fi

if [ -z "${GCS_SERVICE_ACCOUNT_KEY:-}" ]; then
  echo "ERROR: GCS_SERVICE_ACCOUNT_KEY is required" >&2
  exit 1
fi

work=$(mktemp -d)
key_file="$work/gcs-key.json"
trap 'rm -r "$work"' EXIT

printf '%s' "$GCS_SERVICE_ACCOUNT_KEY" | base64 -d > "$key_file"
gcloud auth activate-service-account --key-file="$key_file" --quiet
gsutil cp "$GCS_BUCKET/index.yaml" "$work/previous-index.yaml"

DRONE_TAG="$VERSION" scripts/render-charts.sh

for chart in helix-controlplane helix-sandbox; do
  helm package --destination "$work" "charts/$chart"
done

for chart in helix-controlplane helix-sandbox; do
  archive="$work/${chart}-${VERSION}.tgz"
  if [ ! -f "$archive" ]; then
    echo "ERROR: expected chart archive was not created: $archive" >&2
    exit 1
  fi
  gsutil cp "$archive" "$GCS_BUCKET/${chart}-${VERSION}.tgz"
done

helm repo index --url "$REPO_URL" --merge "$work/previous-index.yaml" "$work"

gsutil -h "Cache-Control:no-cache,no-store,max-age=0" cp \
  "$work/index.yaml" "$GCS_BUCKET/index.yaml"
