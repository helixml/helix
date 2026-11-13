#!/bin/sh
# Shallow-clone helixml/zed at the ZED_COMMIT pinned in sandbox-versions.txt
# into external/zed-source. Run from the repository root.
set -eu
. ./sandbox-versions.txt
src=external/zed-source
git init -q "$src"
git -C "$src" fetch -q --depth 1 https://github.com/helixml/zed.git "$ZED_COMMIT"
git -C "$src" checkout -q FETCH_HEAD
echo "Zed source at $ZED_COMMIT"
