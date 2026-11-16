#!/bin/bash
#
# Test helix-trusted-startup.sh: a sandbox runs .helix/startup.sh, and every
# file it runs, only from the trusted helix-specs commit — never from the
# agent-writable helix-specs tip.
#
# Usage: ./test-trusted-startup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP=$(mktemp -d)
# Extracted startup trees are read-only.
trap 'chmod -R u+w "$TMP" 2>/dev/null; rm -rf "$TMP"' EXIT
export TMPDIR="$TMP"

source "$SCRIPT_DIR/helix-trusted-startup.sh"

check() {
    local name="$1" expected="$2" actual="$3"
    if [ "${actual}" != "${expected}" ]; then
        echo "FAIL: ${name}: expected '${expected}', got '${actual}'"
        exit 1
    fi
    echo "ok: ${name}"
}

git_q() { git -c user.name=t -c user.email=t@example.com "$@" >/dev/null 2>&1; }

# The sandbox layout: the primary repo clone at $WORK/kit with helix-specs
# checked out as the sibling worktree $WORK/helix-specs.
WORK="$TMP/work"
REPO="$WORK/kit"
SPECS="$WORK/helix-specs"
mkdir -p "$REPO"
git_q -C "$REPO" init -b main
echo code > "$REPO/README.md"
git_q -C "$REPO" add -A
git_q -C "$REPO" commit -m code
git_q -C "$REPO" worktree add --orphan -b helix-specs "$SPECS" \
    || { git_q -C "$REPO" checkout --orphan helix-specs && git_q -C "$REPO" rm -rf . \
         && git_q -C "$REPO" commit --allow-empty -m root && git_q -C "$REPO" checkout main \
         && git_q -C "$REPO" worktree add "$SPECS" helix-specs; }

commit_specs() {
    git_q -C "$SPECS" add -A
    git_q -C "$SPECS" commit -m "$1"
    git -C "$SPECS" rev-parse HEAD
}

# Human commit: startup.sh runs a repo-relative helper, as the PTaaS kit does.
mkdir -p "$SPECS/.helix" "$SPECS/engagement/lane_setup"
cat > "$SPECS/.helix/startup.sh" <<'EOF'
#!/bin/bash
bash "$(dirname "$0")/../engagement/lane_setup/fetch-secrets.sh"
EOF
echo 'echo human-secrets' > "$SPECS/engagement/lane_setup/fetch-secrets.sh"
HUMAN=$(commit_specs "human startup")

# Agent commits on top: one changes the helper (not under .helix/), one the script.
echo 'echo AGENT-HELPER' > "$SPECS/engagement/lane_setup/fetch-secrets.sh"
commit_specs "agent helper" >/dev/null
printf '#!/bin/bash\necho AGENT-SCRIPT\n' > "$SPECS/.helix/startup.sh"
commit_specs "agent script" >/dev/null

resolve() {
    local code=0 out
    out=$(helix_trusted_startup_script "$REPO" "$SPECS" 2>"$TMP/err") || code=$?
    echo "$code|$out"
}

# 1. The next sandbox start runs the human-pushed version, from the code dir.
HELIX_TRUSTED_STARTUP_SHA="$HUMAN"
r=$(resolve)
check "trusted commit resolves" "0" "${r%%|*}"
script="${r#*|}"
check "runs the human script and helper, not the agent's" "human-secrets" "$(cd "$REPO" && bash "$script")"

# 2. The extracted tree cannot be edited in place.
if ( echo 'echo tampered' > "$(dirname "$script")/../engagement/lane_setup/fetch-secrets.sh" ) 2>/dev/null \
    && [ "$(id -u)" != 0 ]; then
    echo "FAIL: trusted tree is writable"; exit 1
fi
echo "ok: trusted tree is read-only"

# 3. No trusted commit: an agent-written startup.sh is refused, nothing to run.
HELIX_TRUSTED_STARTUP_SHA=""
r=$(resolve)
check "no trusted commit refuses the agent's startup.sh" "2|" "$r"
grep -q "no one has vouched" "$TMP/err" || { echo "FAIL: refusal not explained"; cat "$TMP/err"; exit 1; }
echo "ok: refusal explained"

# 4. No trusted commit and no startup.sh: nothing to run, not an error.
mv "$SPECS/.helix" "$TMP/helix-moved"
check "no startup script anywhere" "1|" "$(resolve)"
mv "$TMP/helix-moved" "$SPECS/.helix"

# 5. The commit must be a full id an agent cannot steer to its own commit.
for bogus in helix-specs HEAD "${HUMAN:0:12}" "$HUMAN; echo pwned" "0000000000000000000000000000000000000000"; do
    HELIX_TRUSTED_STARTUP_SHA="$bogus"
    check "refuses trusted commit '$bogus'" "2|" "$(resolve)"
done

# 6. A trusted commit without a startup script runs nothing.
git_q -C "$SPECS" rm -r -q .helix
NOSCRIPT=$(commit_specs "remove startup")
HELIX_TRUSTED_STARTUP_SHA="$NOSCRIPT"
check "trusted commit without startup.sh" "1|" "$(resolve)"

# 7. helix-workspace-setup.sh resolves under set -e without aborting, and logs
# the refusal where agents are told to look.
SETUP_FN=$(sed -n '/^resolve_startup_script() {/,/^}/p' "$SCRIPT_DIR/helix-workspace-setup.sh" \
    | sed "s#/tmp/helix-startup.log#$TMP/startup.log#")
[ -n "$SETUP_FN" ] || { echo "FAIL: resolve_startup_script not found"; exit 1; }
out=$(cd "$WORK" && HOME="$TMP" WORK_DIR="$WORK" HELIX_PRIMARY_REPO_NAME=kit HELIX_TRUSTED_STARTUP_SHA="" bash -e -c "
        source '$SCRIPT_DIR/helix-trusted-startup.sh'
        git -C '$SPECS' checkout -q HEAD~1 -- .helix
        $SETUP_FN
        resolve_startup_script >/dev/null
        echo \"\$STARTUP_STATUS|\$STARTUP_SCRIPT\"
    ")
check "workspace setup refuses without aborting" "2|" "$out"
grep -q "Startup script NOT run" "$TMP/startup.log" || { echo "FAIL: refusal not logged"; exit 1; }
echo "ok: refusal logged to the startup log"

out=$(HOME="$TMP" WORK_DIR="$WORK" HELIX_PRIMARY_REPO_NAME=kit HELIX_TRUSTED_STARTUP_SHA="$HUMAN" bash -e -c "
        source '$SCRIPT_DIR/helix-trusted-startup.sh'
        $SETUP_FN
        resolve_startup_script
        cd '$REPO' && bash \"\$STARTUP_SCRIPT\"
    ")
check "workspace setup runs the trusted script" "human-secrets" "$out"

# 8. No runner executes the live helix-specs worktree.
for f in helix-workspace-setup.sh helix-run-startup-script.sh; do
    if grep -nE 'STARTUP_SCRIPT(_PATH)?=.*helix-specs/\.helix/startup\.sh' "$SCRIPT_DIR/$f"; then
        echo "FAIL: $f runs startup.sh from the live helix-specs worktree"; exit 1
    fi
done
echo "ok: runners never point at the live helix-specs worktree"

echo "All trusted startup tests passed"
