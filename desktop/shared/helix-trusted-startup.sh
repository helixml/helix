#!/bin/bash
# Sourced by helix-workspace-setup.sh and helix-run-startup-script.sh.
#
# A sandbox runs .helix/startup.sh only from the helix-specs commit a person
# vouched for: the last one pushed with a human credential or saved from the
# Helix UI/API. The API passes that commit as HELIX_TRUSTED_STARTUP_SHA.
#
# The live helix-specs worktree is never executed. Agents may push helix-specs,
# and a repository can be attached to several projects, so its tip may hold a
# script (or a file a script runs) written by another project's agent.

# helix_trusted_startup_script REPO_DIR SPECS_DIR
#
# REPO_DIR is a clone holding the helix-specs history; SPECS_DIR is the live
# helix-specs worktree. Uses HELIX_TRUSTED_STARTUP_SHA.
#
# Extracts the trusted commit's tree into a fresh read-only directory, so the
# script and every repo-relative path it resolves from "$0" come from that
# commit. Prints the extracted startup.sh path and returns 0. Returns 1 when
# there is no startup script to run, and 2 when one exists but must not run;
# the reason goes to stderr.
helix_trusted_startup_script() {
    local repo_dir="$1" specs_dir="$2"
    local sha="${HELIX_TRUSTED_STARTUP_SHA:-}"

    if [ -z "$sha" ]; then
        if [ -f "$specs_dir/.helix/startup.sh" ]; then
            echo "ERROR: not running .helix/startup.sh: no one has vouched for any helix-specs commit." >&2
            echo "  Only agents have written helix-specs, and an agent's change never runs at sandbox start." >&2
            echo "  Fix: save the startup script in Project Settings, or push helix-specs with your own credentials." >&2
            return 2
        fi
        return 1
    fi

    if ! [[ "$sha" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]]; then
        echo "ERROR: not running .helix/startup.sh: trusted startup commit '$sha' is not a commit id." >&2
        return 2
    fi
    if ! git -C "$repo_dir" cat-file -e "$sha^{commit}" 2>/dev/null; then
        echo "ERROR: not running .helix/startup.sh: trusted startup commit $sha is not in $repo_dir." >&2
        return 2
    fi

    local tree archive
    tree=$(mktemp -d "${TMPDIR:-/tmp}/helix-startup-XXXXXX") || return 2
    archive="$tree.tar"
    if ! git -C "$repo_dir" archive --format=tar --output="$archive" "$sha" \
        || ! tar -xf "$archive" -C "$tree"; then
        rm -f "$archive"
        echo "ERROR: not running .helix/startup.sh: could not extract trusted startup commit $sha." >&2
        return 2
    fi
    rm -f "$archive"
    chmod -R a-w "$tree"

    if [ ! -f "$tree/.helix/startup.sh" ]; then
        return 1
    fi
    echo "$tree/.helix/startup.sh"
}
