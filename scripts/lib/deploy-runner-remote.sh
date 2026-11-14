#!/bin/bash
# Runs ON a runner host, fed over `ssh host <script>` by scripts/deploy-prod.sh,
# which prepends these assignments:
#   VERSION SANDBOX_IMAGE LAUNCHER LAUNCHER_KIND CONTAINER FORCE_RECREATE
#
# A sandbox recreate stops and removes the container, which ends every desktop
# on the host, so recreate only when what the sandbox image ships changed. Every
# amd64 release image built since the label was introduced carries
# helix.sandbox.fingerprint, the output of scripts/sandbox-fingerprint.sh for
# its source. Desktop and Zed inputs are excluded from that fingerprint
# (sandbox-images/ is gitignored and skipped), so an equal fingerprint means
# the release changed nothing the sandbox runs.
#
# Equal fingerprints: roll the desktop image only. Pull the release's own
# helix-ubuntu into the sandbox's dockerd, tag it the way the sandbox's boot
# does, then atomically swap /opt/images/helix-ubuntu.{ref,runtime-ref,version}.
# Hydra reads the version file on every desktop create
# (api/pkg/hydra/devcontainer.go: getDesktopVersion) and sandbox-heartbeat
# re-reads it every heartbeat (api/cmd/sandbox-heartbeat/main.go:
# discoverDesktopVersions, 30s), so the control plane starts placing new
# desktops on it; desktops already running keep their image.
#
# Everything else - either image unlabelled, no running sandbox, unreadable
# desktop pins, FORCE_RECREATE=true - recreates, which is what the deploy did
# before fingerprints existed. A failed desktop pull fails the deploy before the
# version file is touched.

main() {
  set -euo pipefail
  : "${VERSION:?}" "${SANDBOX_IMAGE:?}" "${LAUNCHER:?}" "${LAUNCHER_KIND:?}" \
    "${CONTAINER:?}" "${FORCE_RECREATE:?}"

  FINGERPRINT_LABEL=helix.sandbox.fingerprint
  DESKTOP=helix-ubuntu
  IMAGE_DIR=/opt/images

  log() { echo "runner: $*"; }
  warn() { echo "runner: WARNING: $*" >&2; }

  # fingerprint_of <container|image> <ref>: the sandbox fingerprint, or nothing
  # when the object is gone, unlabelled or docker cannot read it.
  fingerprint_of() {
    local value
    value=$(docker "$1" inspect --format "{{index .Config.Labels \"$FINGERPRINT_LABEL\"}}" "$2" 2>/dev/null || true)
    case "$value" in "" | "<no value>") return 0 ;; esac
    printf '%s' "$value"
  }

  # The launcher names the image the next recreate boots. Updating it without
  # running it only changes what a future restart uses.
  update_launcher() {
    case "$LAUNCHER_KIND" in
      image) sed -i "s|^image=ghcr.io/helixml/helix-sandbox:.*|image=$SANDBOX_IMAGE|" "$LAUNCHER" ;;
      tag) sed -i "s/^SANDBOX_TAG=.*/SANDBOX_TAG=\"$VERSION\"/" "$LAUNCHER" ;;
      *)
        warn "unknown LAUNCHER_KIND '$LAUNCHER_KIND'"
        exit 1
        ;;
    esac
  }

  recreate() { # recreate <reason>
    log "recreating $CONTAINER: $1"
    update_launcher
    if [ "$LAUNCHER_KIND" = image ] && docker container inspect "$CONTAINER" > /dev/null 2>&1; then
      docker stop "$CONTAINER"
      docker rm "$CONTAINER"
    fi
    "$LAUNCHER"
  }

  # The desktop image the release itself would boot: read the pins the sandbox
  # image bakes at /opt/images (Dockerfile.sandbox COPY sandbox-images/), so the
  # registry and the arch-specific tag come from the release, not from here.
  # docker create/rm leave nothing behind: the container never starts.
  desktop_pins() {
    local cid=$1 tmp
    tmp=$(mktemp -d)
    if ! docker cp "$cid:$IMAGE_DIR/$DESKTOP.ref" "$tmp/ref" ||
      ! docker cp "$cid:$IMAGE_DIR/$DESKTOP.version" "$tmp/version"; then
      rm -r "$tmp"
      return 1
    fi
    printf '%s %s\n' "$(cat "$tmp/ref")" "$(cat "$tmp/version")"
    rm -r "$tmp"
  }

  # Swap the three pins together after writing all of them: a new version file
  # with the old runtime-ref is harmless (hydra rejects the stale ref and uses
  # the local tag), a missing version file is not.
  swap_pins() { # swap_pins <ref> <version>
    docker exec "$CONTAINER" sh -c '
      set -e
      dir=${HELIX_DESKTOP_IMAGE_DIR:-/opt/images}
      printf "%s\n" "$1" > "$dir/.helix-ubuntu.ref.new"
      printf "%s\n" "$1" > "$dir/.helix-ubuntu.runtime-ref.new"
      printf "%s\n" "$2" > "$dir/.helix-ubuntu.version.new"
      mv -f "$dir/.helix-ubuntu.ref.new" "$dir/helix-ubuntu.ref"
      mv -f "$dir/.helix-ubuntu.runtime-ref.new" "$dir/helix-ubuntu.runtime-ref"
      mv -f "$dir/.helix-ubuntu.version.new" "$dir/helix-ubuntu.version"
    ' runner "$1" "$2"
  }

  desktop_only() { # desktop_only <fingerprint>
    local cid pins ref version
    log "sandbox artifacts unchanged (fingerprint $1); rolling $DESKTOP only, running desktops stay up"

    cid=$(docker create "$SANDBOX_IMAGE")
    if ! pins=$(desktop_pins "$cid"); then
      docker rm -f "$cid" > /dev/null 2>&1 || true
      recreate "the release image has no readable $DESKTOP pins in $IMAGE_DIR"
      return 0
    fi
    docker rm -f "$cid" > /dev/null
    read -r ref version <<< "$pins"
    if [ -z "$ref" ] || [ -z "$version" ]; then
      recreate "the release image has empty $DESKTOP pins in $IMAGE_DIR"
      return 0
    fi
    log "$DESKTOP $version from $ref"

    # Exactly what the sandbox's boot does (sandbox/04-start-dockerd.sh:
    # load_desktop_image): pull the registry ref, then tag it as the local name
    # hydra resolves. Nothing is swapped unless the image is loaded.
    docker exec "$CONTAINER" docker pull "$ref"
    docker exec "$CONTAINER" docker tag "$ref" "$DESKTOP:$version"
    docker exec "$CONTAINER" docker tag "$ref" "$DESKTOP:latest"
    docker exec "$CONTAINER" docker image inspect "$DESKTOP:$version" > /dev/null

    update_launcher
    swap_pins "$ref" "$version"
    log "$CONTAINER now places new desktops on $DESKTOP:$version (within one 30s heartbeat); desktops already running keep their image"

    # Old desktop images would otherwise sit in the sandbox's dockerd until its
    # next restart. Prune with the sandbox's own policy, which keeps the
    # configured version and every image a desktop container still uses; a GC
    # failure must not fail a deploy whose desktops are already switched.
    if docker exec "$CONTAINER" test -f /usr/local/lib/helix-desktop-image-gc.sh; then
      docker exec "$CONTAINER" bash -c \
        'source /usr/local/lib/helix-desktop-image-gc.sh && cleanup_desktop_images deploy' ||
        warn "desktop image GC failed on $CONTAINER; old desktops stay until its next restart"
    fi
  }

  test -x "$LAUNCHER"
  case "$LAUNCHER_KIND" in
    image) grep -q '^image=ghcr.io/helixml/helix-sandbox:' "$LAUNCHER" ;;
    tag) grep -q '^SANDBOX_TAG=' "$LAUNCHER" ;;
    *) log "unknown LAUNCHER_KIND '$LAUNCHER_KIND'"; exit 1 ;;
  esac

  log "pulling $SANDBOX_IMAGE"
  docker pull "$SANDBOX_IMAGE"

  released=$(fingerprint_of image "$SANDBOX_IMAGE")
  running=$(fingerprint_of container "$CONTAINER")
  state=$(docker container inspect --format '{{.State.Status}}' "$CONTAINER" 2>/dev/null || true)

  if [ "$FORCE_RECREATE" = "true" ]; then
    recreate "FORCE_RECREATE=true"
  elif [ -z "$released" ]; then
    recreate "$SANDBOX_IMAGE carries no $FINGERPRINT_LABEL label (built before the label existed)"
  elif [ -z "$running" ]; then
    recreate "$CONTAINER has no $FINGERPRINT_LABEL label (it still runs a pre-label image)"
  elif [ "$released" != "$running" ]; then
    recreate "the sandbox itself changed: released fingerprint $released, running $running"
  elif [ "$state" != "running" ]; then
    recreate "$CONTAINER is ${state:-missing}, not running"
  else
    desktop_only "$released"
  fi
}

main < /dev/null
