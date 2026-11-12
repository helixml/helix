#!/bin/sh
# Tests scripts/publish-image.sh, ghcr-push.sh and ghcr-manifest.sh against a
# fake docker.
# FAKE_DOCKER_FAIL lists docker subcommands ("login", "push", "manifest") that
# fail; FAKE_LOGIN_OUTPUT is what a failing login prints.
set -u
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/bin"
cat > "$WORK/bin/docker" <<'FAKE'
#!/bin/sh
echo "$1" >> "$FAKE_DOCKER_CALLS"
case "$1" in login|build) cat > /dev/null ;; esac
case " ${FAKE_DOCKER_FAIL:-} " in
  *" $1 "*)
    [ "$1" = login ] && echo "${FAKE_LOGIN_OUTPUT:-}"
    exit 1 ;;
esac
exit 0
FAKE
printf '#!/bin/sh\nexit 0\n' > "$WORK/bin/sleep"
chmod +x "$WORK/bin/docker" "$WORK/bin/sleep"

failures=0
# expect <name> <want-exit> <want-login-calls> <script> <args...>
expect() {
  name="$1"; want="$2"; want_logins="$3"; shift 3
  export FAKE_DOCKER_CALLS="$WORK/calls"
  : > "$FAKE_DOCKER_CALLS"
  PATH="$WORK/bin:$PATH" GITHUB_TOKEN=t sh "$@" > "$WORK/out" 2>&1
  got=$?
  logins=$(grep -c '^login$' "$FAKE_DOCKER_CALLS")
  pushes=$(grep -c '^push$' "$FAKE_DOCKER_CALLS")
  if [ "$got" -ne "$want" ] || [ "$logins" -ne "$want_logins" ]; then
    echo "FAIL: $name (exit $got want $want, logins $logins want $want_logins)"
    sed 's/^/    /' "$WORK/out"
    failures=$((failures + 1))
  else
    echo "ok: $name"
  fi
}

IMG=registry.helixml.tech/helix/controlplane:abc-linux-amd64
REPO=registry.helixml.tech/helix/controlplane

FAKE_DOCKER_FAIL="" expect "push succeeds" 0 1 "$ROOT/scripts/ghcr-push.sh" "$IMG"
FAKE_DOCKER_FAIL="push" expect "push failure fails the build" 1 1 "$ROOT/scripts/ghcr-push.sh" "$IMG"
FAKE_DOCKER_FAIL="login" FAKE_LOGIN_OUTPUT='Error response from daemon: Get "https://ghcr.io/v2/": denied: denied' \
  expect "rejected token fails without retrying" 1 1 "$ROOT/scripts/ghcr-push.sh" "$IMG"
FAKE_DOCKER_FAIL="login" FAKE_LOGIN_OUTPUT='net/http: TLS handshake timeout' \
  expect "transient login failure is retried" 1 6 "$ROOT/scripts/ghcr-push.sh" "$IMG"
FAKE_DOCKER_FAIL="" expect "publish pushes to both registries" 0 1 "$ROOT/scripts/publish-image.sh" "$IMG"
[ "$pushes" -eq 2 ] || { echo "FAIL: publish made $pushes pushes, want 2"; failures=$((failures + 1)); }
FAKE_DOCKER_FAIL="push" expect "publish: primary push failure fails before mirroring" 1 0 "$ROOT/scripts/publish-image.sh" "$IMG"
[ "$pushes" -eq 4 ] || { echo "FAIL: publish retried $pushes pushes, want 4"; failures=$((failures + 1)); }
FAKE_DOCKER_FAIL="" expect "publish without images is an error" 1 0 "$ROOT/scripts/publish-image.sh"
FAKE_DOCKER_FAIL="" expect "mirror without images is an error" 1 0 "$ROOT/scripts/ghcr-push.sh"
FAKE_DOCKER_FAIL="" expect "manifest succeeds" 0 1 "$ROOT/scripts/ghcr-manifest.sh" "$REPO" abc
FAKE_DOCKER_FAIL="manifest" expect "manifest failure fails the build" 1 1 "$ROOT/scripts/ghcr-manifest.sh" "$REPO" abc
FAKE_DOCKER_FAIL="login" FAKE_LOGIN_OUTPUT='unauthorized: authentication required' \
  expect "manifest: rejected token fails without retrying" 1 1 "$ROOT/scripts/ghcr-manifest.sh" "$REPO" abc

[ "$failures" -eq 0 ] || { echo "$failures test(s) failed"; exit 1; }
echo "all image publish script tests passed"
