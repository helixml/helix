#!/bin/bash
set -euo pipefail

repository="ghcr.io/helixml/helix-sandbox"
current="${1:-${HELIX_RUNNER_CURRENT_VERSION:-}}"
mode="${2:---dry-run}"
semver_re='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'

if ! [[ "$current" =~ $semver_re ]]; then
  echo "usage: $0 CURRENT_VERSION [--dry-run|--apply]" >&2
  exit 2
fi
if [ "$mode" != "--dry-run" ] && [ "$mode" != "--apply" ]; then
  echo "usage: $0 CURRENT_VERSION [--dry-run|--apply]" >&2
  exit 2
fi

container_ids=$(docker ps -aq)
protected_ids=""
if [ -n "$container_ids" ]; then
  # Container IDs are hex, so unquoted expansion is safe and avoids one inspect per container.
  # shellcheck disable=SC2086
  protected_ids=$(docker inspect --format '{{.Image}}' $container_ids)
fi
images=$(docker image ls --no-trunc --format '{{.Repository}}\t{{.Tag}}\t{{.ID}}' "$repository")

image_row_re=$'^([^\t]+)\t([^\t]+)\t([^\t]+)$'
while IFS= read -r line; do
  case "$line" in
    "$repository"|"$repository"$'\t'*)
      if ! [[ "$line" =~ $image_row_re ]]; then
        echo "invalid runner image inventory row; refusing cleanup" >&2
        exit 1
      fi
      ;;
  esac
done <<< "$images"

current_found=false
while IFS=$'\t' read -r image_repository tag image_id; do
  if [ "$image_repository" = "$repository" ] && [ "$tag" = "$current" ]; then
    current_found=true
    break
  fi
done <<< "$images"
if [ "$current_found" != "true" ]; then
  echo "current runner image $repository:$current is not installed; refusing cleanup" >&2
  exit 1
fi

version_less() {
  local left_major left_minor left_patch right_major right_minor right_patch
  IFS=. read -r left_major left_minor left_patch <<< "$1"
  IFS=. read -r right_major right_minor right_patch <<< "$2"
  ((left_major < right_major ||
    (left_major == right_major && left_minor < right_minor) ||
    (left_major == right_major && left_minor == right_minor && left_patch < right_patch)))
}

rollback=""
while IFS=$'\t' read -r image_repository tag image_id; do
  [ "$image_repository" = "$repository" ] || continue
  if [[ "$tag" =~ $semver_re ]] && version_less "$tag" "$current"; then
    if [ -z "$rollback" ] || version_less "$rollback" "$tag"; then
      rollback="$tag"
    fi
  fi
done <<< "$images"

candidate_count=0
while IFS=$'\t' read -r image_repository tag image_id; do
  [ "$image_repository" = "$repository" ] || continue
  [[ "$tag" =~ $semver_re ]] || continue
  version_less "$tag" "$current" || continue
  [ "$tag" != "$rollback" ] || continue
  if printf '%s\n' "$protected_ids" | grep -Fxq "$image_id"; then
    continue
  fi

  ref="$repository:$tag"
  candidate_count=$((candidate_count + 1))
  if [ "$mode" = "--apply" ]; then
    echo "Removing obsolete runner image: $ref"
    docker image rm "$ref"
  else
    echo "Would remove obsolete runner image: $ref"
  fi
done <<< "$images"

echo "Runner image retention: $candidate_count candidate tag(s); keeping current $current and rollback ${rollback:-none}"
