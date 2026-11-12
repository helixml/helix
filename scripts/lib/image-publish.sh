# Shared by scripts/publish-image.sh, ghcr-push.sh and ghcr-manifest.sh (POSIX
# sh: the docker:cli CI image runs busybox ash). Callers set LOG_TAG.

# Exit code for a failure that retrying cannot fix. retry() stops on it
# instead of backing off for minutes.
GHCR_PERMANENT=99

# retry <max-attempts> <sleep-secs-base> <cmd...>
# Linear backoff (base, base*2, ...). Returns the command's final exit code.
retry() {
  max="$1"; base="$2"; shift 2
  attempt=1
  while :; do
    "$@" && return 0
    rc=$?
    if [ "$rc" -eq "$GHCR_PERMANENT" ] || [ "$attempt" -ge "$max" ]; then
      return "$rc"
    fi
    delay=$((base * attempt))
    echo "[$LOG_TAG] attempt $attempt/$max failed (rc=$rc), retrying in ${delay}s: $*" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
  done
}

ghcr_login_once() {
  out=$(echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin 2>&1) && return 0
  echo "$out" >&2
  case "$out" in
    *denied*|*nauthorized*)
      echo "[$LOG_TAG] ERROR: ghcr.io rejected GITHUB_TOKEN; rotate the github_token Drone secret (needs write:packages)" >&2
      return "$GHCR_PERMANENT" ;;
  esac
  return 1
}

# Exits the calling script: nothing can be published without a login.
ghcr_login() {
  if ! retry 6 30 ghcr_login_once; then
    echo "[$LOG_TAG] ERROR: ghcr.io login failed; ghcr.io/helixml is the customer-facing registry, cannot continue" >&2
    exit 1
  fi
}
