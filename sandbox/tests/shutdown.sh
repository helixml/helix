#!/usr/bin/env bash

set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -r "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CALLS"
case "$1" in
    info)
        exit 0
        ;;
    ps)
        printf '%s\n' persistent ephemeral unknown
        ;;
    inspect)
        case "${4:-}" in
            persistent) printf '%s\n' true ;;
            ephemeral) printf '%s\n' false ;;
            unknown) exit 1 ;;
        esac
        ;;
esac
EOF

cat > "$work/bin/pgrep" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF

cat > "$work/bin/timeout" <<'EOF'
#!/usr/bin/env bash
shift
exec "$@"
EOF

cat > "$work/bin/touch" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$work/bin/awk" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

chmod +x "$work/bin/"*
export CALLS="$work/calls"
PATH="$work/bin:$PATH" \
    HELIX_SHUTDOWN_DESKTOP_DOCKERD_TIMEOUT=1 \
    HELIX_SHUTDOWN_DESKTOP_STOP_TIMEOUT=1 \
    HELIX_SHUTDOWN_DOCKERD_TIMEOUT=1 \
    HELIX_SHUTDOWN_UNMOUNT_TIMEOUT=1 \
    bash "$repo_dir/sandbox/shutdown.sh" > "$work/output"

for container in persistent ephemeral unknown; do
    if ! grep -q "^exec -u root $container " "$CALLS"; then
        echo "FAIL: inner dockerd was not quiesced for $container" >&2
        exit 1
    fi
done

if ! grep -q '^stop -t 1 ephemeral$' "$CALLS"; then
    echo "FAIL: non-persistent container was not explicitly stopped" >&2
    exit 1
fi
if grep -Eq '^stop .*persistent|^stop .*unknown' "$CALLS"; then
    echo "FAIL: persistent or uninspectable container was explicitly stopped" >&2
    exit 1
fi
if ! grep -q 'Could not inspect container unknown; keeping it for dockerd restart' "$work/output"; then
    echo "FAIL: uninspectable container was not retained fail-closed" >&2
    exit 1
fi

echo "PASS: persistent containers remain eligible for dockerd restart"
