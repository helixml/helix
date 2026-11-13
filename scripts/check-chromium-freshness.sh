#!/bin/sh
# Fail when the crawler browser image ships a Chromium more than
# CHROMIUM_MAX_LAG majors behind the current stable release.
#
# Usage: scripts/check-chromium-freshness.sh <image>
#
# The stable major comes from Chrome's release feed. When the feed cannot be
# reached, the script estimates it from chrome-version.env: Chrome ships a new
# major every four weeks, so the expected major is CHROMIUM_MIN_MAJOR plus one
# per 28 days since CHROMIUM_RECORDED_AT.
#
# Environment:
#   CHROMIUM_MAX_LAG       majors the image may trail stable by (default 2)
#   CHROMIUM_STABLE_MAJOR  skip the lookup and use this major (tests)
set -eu

IMAGE="${1:?usage: $0 <image>}"
MAX_LAG="${CHROMIUM_MAX_LAG:-2}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FEED="https://versionhistory.googleapis.com/v1/chrome/platforms/linux/channels/stable/versions?pageSize=1"

# shellcheck disable=SC1091 # chrome-version.env at the repo root
. "$ROOT/chrome-version.env"

version=$(docker run --rm --entrypoint /usr/lib/chromium/chromium-headless-shell "$IMAGE" --version)
major=$(echo "$version" | sed -E 's/^[^0-9]*([0-9]+)\..*/\1/')
case "$major" in
  ''|*[!0-9]*) echo "Could not read the Chromium version from $IMAGE: $version" >&2; exit 1 ;;
esac
echo "Image Chromium: $version"

if [ "$major" -lt "$CHROMIUM_MIN_MAJOR" ]; then
  echo "Chromium $major is below the floor in chrome-version.env ($CHROMIUM_MIN_MAJOR)" >&2
  exit 1
fi

stable="${CHROMIUM_STABLE_MAJOR:-}"
source="override"
if [ -z "$stable" ]; then
  stable=$(wget -qO- "$FEED" 2>/dev/null || curl -fsS "$FEED" 2>/dev/null || true)
  stable=$(echo "$stable" | sed -nE 's/.*"version": *"([0-9]+)\..*/\1/p' | head -n1)
  source="Chrome release feed"
fi
if [ -z "$stable" ]; then
  recorded=$(date -u -d "$CHROMIUM_RECORDED_AT" +%s)
  now=$(date -u +%s)
  stable=$((CHROMIUM_MIN_MAJOR + (now - recorded) / 86400 / 28))
  source="estimate from chrome-version.env ($CHROMIUM_RECORDED_AT)"
fi
echo "Current stable major: $stable ($source)"

lag=$((stable - major))
if [ "$lag" -gt "$MAX_LAG" ]; then
  echo "Chromium $major is $lag majors behind stable $stable (allowed: $MAX_LAG)." >&2
  echo "Rebuild Dockerfile.chrome; if Debian has no newer chromium, move the base image." >&2
  exit 1
fi
echo "OK: $lag major(s) behind stable (allowed: $MAX_LAG)"
