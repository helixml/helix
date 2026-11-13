#!/bin/sh
# Fail when a docker-compose*.yaml (or a .drone.yml service) defaults to an
# older PostgreSQL major than the Helm chart (charts/helix-controlplane
# values.yaml, postgresql.image.tag). Explicit overrides such as
# ${HELIX_POSTGRES_IMAGE:-...} are checked by their default.
#
# Usage: scripts/check-postgres-defaults.sh [file ...]
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VALUES="$ROOT/charts/helix-controlplane/values.yaml"

chart_tag=$(awk '
  /^postgresql:/ { in_pg = 1; next }
  in_pg && /^[^ #]/ { in_pg = 0 }
  in_pg && /^  image:/ { in_img = 1; next }
  in_pg && in_img && /^  [^ ]/ { in_img = 0 }
  in_pg && in_img && /^    tag:/ { gsub(/["'\'' ]/, "", $2); print $2; exit }
' FS=: "$VALUES")
chart_major=$(echo "$chart_tag" | sed -nE 's/^([0-9]+).*/\1/p')
if [ -z "$chart_major" ]; then
  echo "Could not read postgresql.image.tag from $VALUES (got '$chart_tag')" >&2
  exit 1
fi
echo "Chart PostgreSQL major: $chart_major ($chart_tag)"

if [ "$#" -eq 0 ]; then
  set -- "$ROOT"/docker-compose*.yaml "$ROOT/.drone.yml"
fi

status=0
matches=$(mktemp)
trap 'rm -f "$matches"' EXIT
for file in "$@"; do
  # image: postgres:12.22-alpine, image: ${VAR:-postgres:17-alpine},
  # image: docker.io/library/postgres:16 (commented lines are skipped).
  grep -nE '^[[:space:]]*image:.*(^|[:/ -])postgres:' "$file" 2>/dev/null \
    | grep -vE '^[0-9]+:[[:space:]]*#' > "$matches" || true
  while IFS= read -r line; do
    lineno=${line%%:*}
    ref=$(echo "$line" | sed -nE 's#.*(^|[:/ -])postgres:([^}" ]+).*#\2#p')
    major=$(echo "$ref" | sed -nE 's/^([0-9]+).*/\1/p')
    name="${file#"$ROOT"/}:$lineno"
    if [ -z "$major" ]; then
      echo "FAIL $name: postgres:$ref has no numeric major" >&2
      status=1
    elif [ "$major" -lt "$chart_major" ]; then
      echo "FAIL $name: postgres:$ref is older than the chart's $chart_major" >&2
      status=1
    else
      echo "ok   $name: postgres:$ref"
    fi
  done < "$matches"
done

exit "$status"
