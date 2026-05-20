#!/bin/bash
# helix-npx — drop-in shim installed as /usr/local/bin/npx that shadows the
# system /usr/bin/npx in PATH. Behaves exactly like npx but gives each
# invocation its own NPM_CONFIG_CACHE so parallel npx spawns don't race.
#
# Background: npm's `_npx/<hash>` directory under NPM_CONFIG_CACHE does a
# "reify mark retired" rename dance every spawn (rename
# node_modules/<pkg> → .<pkg>-<rand>, reinstall, rename back). When two
# npx invocations target the same package against the same cache (e.g.
# Zed and Claude Code both spawning `npx -y chrome-devtools-mcp` at
# session start, or two parallel Claude sessions starting `npx -y
# @modelcontextprotocol/server-github`), the renames race and the
# spawned MCP server's JSON-RPC `initialize` never returns. Spec-task
# logs then surface `<server> context server failed to start: Context
# server request timeout` after the 180s context_server_timeout fires.
#
# Workaround: each invocation gets its own NPM_CONFIG_CACHE under
# /tmp. The cache is fully isolated (no symlinks into the user's
# shared `~/.npm/_cacache` — that path is root-owned in our spec-task
# images so symlinking breaks npm with EACCES on first cacache write).
# The cost is one tarball download per cold spawn (~2-3s for typical
# MCP packages), which is the only the ONE-TIME startup cost per
# MCP per session — long-running stdio MCPs stay attached for the
# whole session.
#
# This shim is installed at /usr/local/bin/npx in Dockerfile.ubuntu-helix
# and takes precedence over /usr/bin/npx via standard PATH order. Zed's
# own ACP-wrapper bootstrapping calls npm directly via absolute path so
# it bypasses this shim — only stdio MCPs whose `command: "npx"`
# (resolved via PATH at spawn time) are routed through here, which is
# exactly the case that needed fixing.

set -e

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
