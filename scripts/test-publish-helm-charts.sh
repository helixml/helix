#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -r "$work"' EXIT

mkdir -p "$work/repo/scripts" "$work/repo/charts/helix-controlplane" \
  "$work/repo/charts/helix-sandbox" "$work/bin"
cp "$repo/scripts/publish-helm-charts.sh" "$work/repo/scripts/"
cp "$repo/scripts/render-charts.sh" "$work/repo/scripts/"

for chart in helix-controlplane helix-sandbox; do
  cp "$repo/charts/$chart/Chart.yaml.tmpl" "$work/repo/charts/$chart/"
done

cat > "$work/bin/gcloud" <<'EOF'
#!/bin/sh
exit 0
EOF
cat > "$work/bin/helm" <<'EOF'
#!/bin/sh
set -eu
if [ "$1" = package ]; then
  destination="$3"
  chart="$4"
  name=$(basename "$chart")
  version=$(sed -n 's/^version: //p' "$chart/Chart.yaml")
  : > "$destination/$name-$version.tgz"
  exit 0
fi
if [ "$1 $2" = "repo index" ]; then
  eval "directory=\${$#}"
  : > "$directory/index.yaml"
  exit 0
fi
exit 1
EOF
cat > "$work/bin/gsutil" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CALLS"
case "$*" in
  "cp gs://charts.helixml.tech/index.yaml "*)
    if [ "${FAIL_FETCH:-0}" = 1 ]; then
      exit 1
    fi
    eval "destination=\${$#}"
    : > "$destination"
    ;;
esac
exit 0
EOF
chmod +x "$work/bin/gcloud" "$work/bin/helm" "$work/bin/gsutil" \
  "$work/repo/scripts/publish-helm-charts.sh" "$work/repo/scripts/render-charts.sh"

(
  cd "$work/repo"
  PATH="$work/bin:$PATH" CALLS="$work/calls" DRONE_TAG=2.12.3 \
    GCS_SERVICE_ACCOUNT_KEY=e30= scripts/publish-helm-charts.sh
)

index_fetch=$(sed -n '1p' "$work/calls")
archive_one=$(sed -n '2p' "$work/calls")
archive_two=$(sed -n '3p' "$work/calls")
index_upload=$(sed -n '4p' "$work/calls")

echo "$archive_one" | grep -q 'helix-controlplane-2.12.3.tgz gs://charts.helixml.tech/helix-controlplane-2.12.3.tgz$'
echo "$archive_two" | grep -q 'helix-sandbox-2.12.3.tgz gs://charts.helixml.tech/helix-sandbox-2.12.3.tgz$'
echo "$index_fetch" | grep -q '^cp gs://charts.helixml.tech/index.yaml '
echo "$index_upload" | grep -q '^-h Cache-Control:no-cache,no-store,max-age=0 cp .*index.yaml gs://charts.helixml.tech/index.yaml$'

if (
  cd "$work/repo"
  PATH="$work/bin:$PATH" CALLS="$work/fail-calls" FAIL_FETCH=1 DRONE_TAG=2.12.3 \
    GCS_SERVICE_ACCOUNT_KEY=e30= scripts/publish-helm-charts.sh
) >/dev/null 2>&1; then
  echo "index fetch failure did not stop publication" >&2
  exit 1
fi
[ "$(wc -l < "$work/fail-calls" | tr -d ' ')" -eq 1 ]
grep -q '^cp gs://charts.helixml.tech/index.yaml ' "$work/fail-calls"

echo "publish helm charts test passed"
