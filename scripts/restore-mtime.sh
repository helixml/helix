#!/bin/sh
# Set every tracked file's mtime to the time of the last commit that touched it.
#
# A fresh clone gives every file the checkout time, and go test keys cached
# results on the mtime+size of files a test opens (testdata), so without this
# packages that read files never hit the test cache in CI. Per-file commit
# times keep the key honest: an edited file gets a newer mtime, so a same-size
# edit still invalidates the cached result.
#
# Walks first-parent history and diffs merges against their first parent, so
# a file changed by a merge gets the merge's time, not the (older) time of the
# side-branch commit that would otherwise be the latest one listed for it.
#
# Needs full history: on a shallow clone the boundary commit "adds" every
# file, which would stamp them all with that commit's time.
set -eu

if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
  echo "restore-mtime: shallow clone, per-file commit times unavailable" >&2
  exit 1
fi

git -c core.quotepath=off log --first-parent --diff-merges=first-parent \
  --format='@%ct' --name-only --no-renames HEAD |
  perl -ne '
    chomp;
    if (/^@(\d+)$/) { $t = $1; next }
    next if $_ eq "" || $seen{$_}++;
    if (-f $_) { utime($t, $t, $_) or die "utime $_: $!\n"; $n++ }
    END { print "restore-mtime: set $n file mtimes\n" }
  '
