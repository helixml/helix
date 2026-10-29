#!/bin/bash
set -euo pipefail

repo=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -r "$work"' EXIT
mkdir -p "$work/bin"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cat > "$work/bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail
case "$*" in
  "ps -aq")
    printf 'running\nstopped\n'
    ;;
  "inspect --format {{.Image}} running stopped")
    [ "${FAIL_INSPECT:-false}" != "true" ] || exit 1
    printf 'sha256:current\nsha256:stopped\n'
    ;;
  "image ls --no-trunc --format {{.Repository}}\\t{{.Tag}}\\t{{.ID}} ghcr.io/helixml/helix-sandbox")
    if [ "${MISSING_CURRENT:-false}" != "true" ]; then
      printf 'ghcr.io/helixml/helix-sandbox\t2.12.45\tsha256:current\n'
    fi
    cat <<IMAGES
ghcr.io/helixml/helix-sandbox	2.12.44	sha256:rollback
ghcr.io/helixml/helix-sandbox	2.12.43	sha256:old
ghcr.io/helixml/helix-sandbox	2.12.42	sha256:stopped
ghcr.io/helixml/helix-sandbox	2.12.41	sha256:current
ghcr.io/helixml/helix-sandbox	2.13.0	sha256:future
ghcr.io/helixml/helix-sandbox	latest	sha256:latest
ghcr.io/helixml/helix-sandbox	broken	sha256:unknown
other/repository	1.0.0	sha256:other
IMAGES
    case "${MALFORMED_INVENTORY:-}" in
      missing_id) printf 'ghcr.io/helixml/helix-sandbox\t2.12.40\t\n' ;;
      truncated) printf 'ghcr.io/helixml/helix-sandbox\t2.12.40\n' ;;
      extra) printf 'ghcr.io/helixml/helix-sandbox\t2.12.40\tsha256:bad\textra\n' ;;
    esac
    ;;
  "image rm ghcr.io/helixml/helix-sandbox:2.12.43")
    printf '%s\n' "$*" >> "$DOCKER_REMOVALS"
    ;;
  *)
    echo "unexpected docker call: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$work/bin/docker"

export PATH="$work/bin:$PATH"
export DOCKER_REMOVALS="$work/removals"
: > "$DOCKER_REMOVALS"

output=$("$repo/scripts/cleanup-runner-images.sh" 2.12.45)
printf '%s\n' "$output" | grep -q 'Would remove obsolete runner image: ghcr.io/helixml/helix-sandbox:2.12.43' ||
  fail "dry run omitted the unreferenced old tag"
[ ! -s "$DOCKER_REMOVALS" ] || fail "dry run removed an image"
for protected in 2.12.45 2.12.44 2.12.42 2.12.41 2.13.0 latest broken; do
  if printf '%s\n' "$output" | grep -q "Would remove.*:$protected"; then
    fail "dry run selected protected tag $protected"
  fi
done

"$repo/scripts/cleanup-runner-images.sh" 2.12.45 --apply >/dev/null
[ "$(cat "$DOCKER_REMOVALS")" = "image rm ghcr.io/helixml/helix-sandbox:2.12.43" ] ||
  fail "apply did not remove exactly the dry-run candidate"

: > "$DOCKER_REMOVALS"
if FAIL_INSPECT=true "$repo/scripts/cleanup-runner-images.sh" 2.12.45 --apply >/dev/null 2>&1; then
  fail "container inspection failure did not fail closed"
fi
[ ! -s "$DOCKER_REMOVALS" ] || fail "inspection failure removed an image"

if MISSING_CURRENT=true "$repo/scripts/cleanup-runner-images.sh" 2.12.45 --apply >/dev/null 2>&1; then
  fail "missing current image did not fail closed"
fi
[ ! -s "$DOCKER_REMOVALS" ] || fail "missing current image removed an image"

for malformed in missing_id truncated extra; do
  if MALFORMED_INVENTORY="$malformed" "$repo/scripts/cleanup-runner-images.sh" 2.12.45 --apply >/dev/null 2>&1; then
    fail "$malformed inventory row did not fail closed"
  fi
  [ ! -s "$DOCKER_REMOVALS" ] || fail "$malformed inventory row removed an image"
done

echo "runner image cleanup test passed"
