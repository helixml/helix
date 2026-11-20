#!/bin/sh
set -eu

image=${1:?Usage: check-image-layer-depth.sh IMAGE}
max_layers=125

layers=$(docker image inspect --format '{{len .RootFS.Layers}}' "$image")
case "$layers" in
  ''|*[!0-9]*)
    echo "Invalid layer count for $image: $layers" >&2
    exit 1
    ;;
esac

if [ "$layers" -gt "$max_layers" ]; then
  echo "$image has $layers filesystem layers; Docker supports at most $max_layers" >&2
  exit 1
fi

echo "$image has $layers/$max_layers filesystem layers"
