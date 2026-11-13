#!/bin/sh
# Check the crawler browser image's Chromium against chrome-version.env.
#
# Usage: scripts/check-chromium-freshness.sh [--online] <image>
#
# Default (PR builds): deterministic, no network. The image must carry the
# pinned CHROMIUM_VERSION, and the pin must be within CHROMIUM_MAX_LAG majors
# of CHROMIUM_STABLE_MAJOR recorded alongside it.
#
# --online (scheduled builds): also compare against today's Chrome stable from
# Chrome's release feed. If the feed cannot be reached, estimate it from
# CHROMIUM_STABLE_MAJOR/CHROMIUM_STABLE_AT (a new major every 28 days). A
# failure here means it is time to bump the pins in chrome-version.env.
#
# CHROMIUM_STABLE_OVERRIDE skips the lookup and uses that major (tests).
set -eu

ONLINE=false
if [ "${1:-}" = "--online" ]; then
  ONLINE=true
  shift
fi
IMAGE="${1:?usage: $0 [--online] <image>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FEED="https://versionhistory.googleapis.com/v1/chrome/platforms/linux/channels/stable/versions?pageSize=1"

# shellcheck disable=SC1091 # chrome-version.env at the repo root
. "$ROOT/chrome-version.env"
MAX_LAG="${CHROMIUM_MAX_LAG:-2}"

major_of() { echo "$1" | sed -nE 's/^[^0-9]*([0-9]+)\..*/\1/p'; }

version=$(docker run --rm --entrypoint /usr/lib/chromium/chromium-headless-shell "$IMAGE" --version)
upstream=$(echo "$version" | sed -nE 's/^Chromium ([0-9.]+).*/\1/p')
pinned_upstream="${CHROMIUM_VERSION%%-*}"
major=$(major_of "$upstream")
echo "Image Chromium: $version (pinned $CHROMIUM_VERSION)"

if [ -z "$major" ]; then
  echo "Could not read the Chromium version from $IMAGE" >&2
  exit 1
fi
if [ "$upstream" != "$pinned_upstream" ]; then
  echo "Image Chromium $upstream does not match CHROMIUM_VERSION $CHROMIUM_VERSION" >&2
  exit 1
fi

check_lag() { # stable source
  lag=$(($1 - major))
  if [ "$lag" -gt "$MAX_LAG" ]; then
    echo "Chromium $major is $lag majors behind stable $1 ($2); allowed: $MAX_LAG." >&2
    echo "Bump the pins in chrome-version.env." >&2
    exit 1
  fi
  echo "OK: $lag major(s) behind stable $1 ($2); allowed: $MAX_LAG"
}

check_lag "$CHROMIUM_STABLE_MAJOR" "recorded $CHROMIUM_STABLE_AT"
[ "$ONLINE" = true ] || exit 0

stable="${CHROMIUM_STABLE_OVERRIDE:-}"
source="override"
if [ -z "$stable" ]; then
  stable=$(wget -qO- "$FEED" 2>/dev/null || curl -fsS "$FEED" 2>/dev/null || true)
  stable=$(echo "$stable" | sed -nE 's/.*"version": *"([0-9]+)\..*/\1/p' | head -n1)
  source="Chrome release feed"
fi
if [ -z "$stable" ]; then
  recorded=$(date -u -d "$CHROMIUM_STABLE_AT" +%s)
  now=$(date -u +%s)
  stable=$((CHROMIUM_STABLE_MAJOR + (now - recorded) / 86400 / 28))
  source="estimated from $CHROMIUM_STABLE_AT"
fi
check_lag "$stable" "$source"
