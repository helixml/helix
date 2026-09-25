#!/usr/bin/env bash
# Remove git worktrees whose branch is already merged — safely.
#
#   scripts/prune-merged-worktrees.sh            # dry run: show what would be removed and why
#   scripts/prune-merged-worktrees.sh --apply    # remove them
#
# A worktree is removed only when ALL hold:
#   - its branch is merged: HEAD is an ancestor of origin/<default>, or GitHub has a MERGED PR
#     for the branch (squash/rebase merges leave no ancestor link)
#   - no uncommitted changes and no untracked (non-ignored) files
#   - no commits that exist on no remote branch
# Never touched: the main checkout, detached HEADs, app-managed worktrees (T3 Code under
# ~/.t3/worktrees), and anything with local work. Stale entries whose directory is gone are
# pruned. Build output (target/, node_modules/) goes with the removed worktree.
set -uo pipefail

apply=false
[[ "${1:-}" == "--apply" ]] && apply=true

repo_root=$(git rev-parse --path-format=absolute --git-common-dir | sed 's#/\.git$##')
cd "$repo_root" || exit 1
git fetch --quiet --prune origin 2>/dev/null || echo "warning: fetch failed; merged checks use the last fetched origin state"
default=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || echo origin/main)
have_gh=false
command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1 && have_gh=true

git worktree prune --verbose
removed=0 kept=0
while IFS= read -r line; do
  case "$line" in
    worktree\ *) wt=${line#worktree }; branch=""; detached=false ;;
    branch\ *) branch=${line#branch refs/heads/} ;;
    detached) detached=true ;;
    "")
      [[ -z "${wt:-}" || "$wt" == "$repo_root" ]] && { wt=""; continue; }
      reason=""
      if [[ "$wt" == "$HOME/.t3/worktrees/"* ]]; then reason="app-managed (T3 Code)"
      elif $detached || [[ -z "$branch" ]]; then reason="detached HEAD"
      elif [[ -n $(git -C "$wt" status --porcelain --untracked-files=normal 2>/dev/null) ]]; then reason="uncommitted or untracked changes"
      elif [[ $(git -C "$wt" rev-list --count HEAD --not --remotes 2>/dev/null || echo 1) != 0 ]]; then reason="commits on no remote"
      else
        merged=""
        if git merge-base --is-ancestor "$branch" "$default" 2>/dev/null; then merged="ancestor of $default"
        elif $have_gh; then
          pr=$(gh pr list --state merged --head "$branch" --json number --jq '.[0].number' 2>/dev/null)
          [[ -n "$pr" ]] && merged="PR #$pr merged"
        fi
        [[ -z "$merged" ]] && reason="not merged"
      fi
      if [[ -n "$reason" ]]; then
        kept=$((kept + 1)); printf 'keep    %-60s %s\n' "$wt" "($reason)"
      else
        removed=$((removed + 1)); size=$(du -sh "$wt" 2>/dev/null | cut -f1)
        printf 'remove  %-60s [%s] %s, %s\n' "$wt" "$branch" "$merged" "${size:-?}"
        if $apply; then
          git worktree remove --force "$wt" 2>/dev/null
          if [[ -d "$wt" ]]; then  # root-owned files from container builds (target/, api/tmp/)
            docker run --rm -v "$(dirname "$wt")":/w alpine:3.22 rm -rf "/w/$(basename "$wt")" >/dev/null 2>&1 \
              || echo "  failed: $wt — remove root-owned files, then: git worktree prune"
          fi
        fi
      fi
      wt="" ;;
  esac
done < <(git worktree list --porcelain; echo)

$apply || echo "dry run: $removed to remove, $kept kept — re-run with --apply"
$apply && echo "removed $removed, kept $kept"
