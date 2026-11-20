#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -r "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/docker" <<'EOF'
#!/bin/sh
[ "$TEST_IMAGE_LAYERS" = inspect-fail ] && exit 1
printf '%s\n' "$TEST_IMAGE_LAYERS"
EOF
chmod +x "$work/bin/docker"

check() {
  want=$1
  layers=$2
  if TEST_IMAGE_LAYERS=$layers PATH="$work/bin:$PATH" \
    "$repo/scripts/check-image-layer-depth.sh" test-image >"$work/output" 2>&1; then
    got=0
  else
    got=$?
  fi
  if [ "$got" -ne "$want" ]; then
    echo "FAIL: $layers layers returned $got, want $want" >&2
    sed 's/^/  /' "$work/output" >&2
    exit 1
  fi
}

check 0 124
check 0 125
check 1 126
check 1 invalid
check 1 inspect-fail

src="$work/src"
mkdir -p "$src/scripts" "$src/zed-build"
cp "$repo/stack" "$repo/Dockerfile.ubuntu-helix" "$repo/go.mod" \
  "$repo/sandbox-versions.txt" "$src/"
cp "$repo/scripts/check-image-layer-depth.sh" "$src/scripts/"
: > "$src/zed-build/zed"

cat > "$work/bin/desktop-docker" <<'EOF'
#!/bin/sh
case "${1:-} ${2:-}" in
  'image inspect')
    [ "$TEST_IMAGE_LAYERS" = inspect-fail ] && exit 1
    printf '%s\n' "$TEST_IMAGE_LAYERS"
    ;;
  images\ *) printf '%s\n' 'sha256:abcdef123456' ;;
esac
EOF
chmod +x "$work/bin/desktop-docker"

run_desktop() {
  layers=$1
  PATH="$work/bin:$PATH" TEST_IMAGE_LAYERS=$layers bash "$src/stack" '
    docker() { "$TEST_DOCKER" "$@"; }
    build-qwen-code() { return 0; }
    transfer-desktop-to-sandbox() { : > "$TRANSFER_MARKER"; return 1; }
    build-desktop ubuntu
  ' >"$work/desktop-output" 2>&1
}

export TEST_DOCKER="$work/bin/desktop-docker"
export TRANSFER_MARKER="$work/transfer-called"
if (cd "$src" && run_desktop 126); then
  echo "FAIL: build-desktop accepted an image over the layer limit" >&2
  exit 1
fi
[ ! -e "$src/sandbox-images/helix-ubuntu.version" ] || {
  echo "FAIL: invalid image was published to the version file" >&2
  exit 1
}
[ ! -e "$TRANSFER_MARKER" ] || {
  echo "FAIL: invalid image reached desktop transfer" >&2
  exit 1
}

if (cd "$src" && run_desktop 124); then
  echo "FAIL: build-desktop ignored a transfer failure" >&2
  exit 1
fi
[ -e "$TRANSFER_MARKER" ] || {
  echo "FAIL: valid image did not reach desktop transfer" >&2
  exit 1
}

if bash "$repo/stack" '
  build-zed() { return 0; }
  build-desktop() { return 0; }
  verify-desktop-build() { return 0; }
  sandbox-unchanged() { return 0; }
  docker() { if [[ "$1" == images ]]; then echo sha256:test; fi; return 0; }
  push-desktop-to-registry() { return 0; }
  transfer-desktop-to-sandbox() { return 1; }
  build-sandbox
' >"$work/stack-output" 2>&1; then
  echo "FAIL: build-sandbox ignored a required desktop transfer failure" >&2
  exit 1
fi
grep -q 'ERROR: Failed to transfer required production desktop helix-ubuntu' "$work/stack-output"

echo "image layer depth tests passed"
