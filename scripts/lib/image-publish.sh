# Shared by scripts/publish-image.sh, ghcr-push.sh and ghcr-manifest.sh (POSIX
# sh: the docker:cli CI image runs busybox ash). Callers set LOG_TAG.

# Exit code for a failure that retrying cannot fix. retry() stops on it
# instead of backing off for minutes.
GHCR_PERMANENT=99

# retry <max-attempts> <sleep-secs-base> <cmd...>
# Linear backoff (base, base*2, ...). Returns the command's final exit code.
retry() {
  _retry_max="$1"; _retry_base="$2"; shift 2
  _retry_attempt=1
  while :; do
    "$@" && return 0
    _retry_rc=$?
    if [ "$_retry_rc" -eq "$GHCR_PERMANENT" ] || [ "$_retry_attempt" -ge "$_retry_max" ]; then
      return "$_retry_rc"
    fi
    _retry_delay=$((_retry_base * _retry_attempt))
    echo "[$LOG_TAG] attempt $_retry_attempt/$_retry_max failed (rc=$_retry_rc), retrying in ${_retry_delay}s: $*" >&2
    sleep "$_retry_delay"
    _retry_attempt=$((_retry_attempt + 1))
  done
}

ghcr_login_once() {
  _login_out=$(echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin 2>&1) && return 0
  echo "$_login_out" >&2
  case "$_login_out" in
    *denied*|*nauthorized*)
      echo "[$LOG_TAG] ERROR: ghcr.io rejected GITHUB_TOKEN; rotate the github_token Drone secret (needs write:packages)" >&2
      return "$GHCR_PERMANENT" ;;
  esac
  return 1
}

# Exits the calling script: nothing can be published without a login.
ghcr_login() {
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "[$LOG_TAG] ERROR: GITHUB_TOKEN not set; ghcr.io/helixml cannot be published" >&2
    exit 1
  fi
  if ! retry 6 30 ghcr_login_once; then
    echo "[$LOG_TAG] ERROR: ghcr.io login failed; ghcr.io/helixml is the customer-facing registry, cannot continue" >&2
    exit 1
  fi
}

# require_images <script-name> "$@": an empty image list is a caller bug, not
# a successful publish.
require_images() {
  _name="$1"; shift
  if [ "$#" -eq 0 ]; then
    echo "usage: $_name <image>..." >&2
    exit 1
  fi
}
