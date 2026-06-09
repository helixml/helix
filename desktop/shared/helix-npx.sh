#!/bin/bash
# helix-npx — drop-in shim installed as /usr/local/bin/npx that shadows the
# system /usr/bin/npx in PATH. Two responsibilities:
#
# 1) Short-circuit when the requested package is already a binary on PATH.
#    Upstream npm's exec path detects "binary already installed" and shells
#    out via `sh -c "<pkg>"` as a child, then npm itself EXITS. MCP clients
#    that track the spawned PID (Claude Code via --mcp-config, Zed via
#    context_server_store) see the npx PID exit and report `Connection
#    closed`/`Context server request timeout` even though the MCP child is
#    alive. Exec the binary directly so the PID the caller monitors IS the
#    MCP process.
#
# 2) For real npm-install paths (scoped packages, packages not on PATH),
#    give each invocation its own NPM_CONFIG_CACHE so parallel npx spawns
#    don't race in npm's `_npx/<hash>` rename dance. Background: npm's
#    "reify mark retired" rename (node_modules/<pkg> → .<pkg>-<rand>,
#    reinstall, rename back) corrupts state when two npx invocations
#    target the same package against the same cache (e.g. two parallel
#    Claude sessions both spawning `npx -y @modelcontextprotocol/server-
#    everything`). The cache is fully isolated under /tmp (no symlinks
#    into the shared `~/.npm/_cacache` — that path is root-owned in our
#    spec-task images so symlinking breaks npm with EACCES on first
#    cacache write). The cost is one tarball download per cold spawn
#    (~2-3s for typical MCP packages), paid once per MCP per session.
#
# This shim is installed at /usr/local/bin/npx in Dockerfile.ubuntu-helix
# and takes precedence over /usr/bin/npx via standard PATH order.

set -e

# Parse: find the first positional (non-flag) argument — that's the
# <pkg> in `npx [flags...] <pkg> [pkg-args...]`. Conservative: any flag
# we don't recognise, or any "advanced" flag that changes resolution
# semantics (-p/--package/--call), bails out to real npx.
SHORTCUT_PKG=""
SHORTCUT_SHIFT=0
i=0
for arg in "$@"; do
    i=$((i+1))
    case "$arg" in
        -y|--yes|--no|--no-install|-q|--quiet|--silent|--no-shell|--shell-auto-fallback|--ignore-existing|--prefer-online|--prefer-offline|--offline|--legacy-peer-deps)
            continue
            ;;
        -p|--package|--package=*|--call|-c|--with|-w|--workspace|--workspace=*|--workspaces|-ws|--include-workspace-root|--if-present|--node-options|--node-options=*)
            SHORTCUT_PKG=""
            break
            ;;
        --)
            # Everything after `--` is positional but the FIRST one might
            # not be the package name in some npx invocations — be safe.
            SHORTCUT_PKG=""
            break
            ;;
        -*)
            # Unknown flag: don't try to be clever.
            SHORTCUT_PKG=""
            break
            ;;
        *)
            SHORTCUT_PKG="$arg"
            SHORTCUT_SHIFT=$i
            break
            ;;
    esac
done

if [ -n "$SHORTCUT_PKG" ] && command -v "$SHORTCUT_PKG" >/dev/null 2>&1; then
    shift "$SHORTCUT_SHIFT"
    exec "$SHORTCUT_PKG" "$@"
fi

# Slow path: real npm install via isolated cache.
ISOLATED_CACHE=$(mktemp -d -t helix-npx-XXXXXX)

# Run the real npx (absolute path so we don't recurse into ourselves)
# with the isolated cache. Forward signals to the child so MCP shutdown
# is clean and the temp cache is removed even on TERM/INT.
NPM_CONFIG_CACHE="$ISOLATED_CACHE" /usr/bin/npx "$@" &
CHILD=$!

trap 'kill -TERM "$CHILD" 2>/dev/null || true' TERM INT HUP

wait "$CHILD"
exit_code=$?

rm -rf "$ISOLATED_CACHE"
exit $exit_code
