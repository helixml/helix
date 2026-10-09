#!/usr/bin/env bash
# Prints a content fingerprint of everything the helix-sandbox image ships that
# comes from this checkout, so a deploy can recreate the sandbox (killing every
# desktop on it) only when what would run inside it actually changed.
#
# The fingerprint covers:
#   * Dockerfile.sandbox itself (base image digests, ARG pins, RUN steps), and
#   * every file the final stage COPYs/ADDs: the Go binaries from the builder
#     stage and the scripts/overlays from the build context.
# Those files are produced by BuildKit from a generated `FROM scratch` stage
# that repeats the final stage's own COPY/ADD instructions, so the Go toolchain,
# build flags, cache mounts and .dockerignore are exactly the real build's and
# nothing is listed by hand. A change to a shared package (api/pkg/types, ...)
# only changes the fingerprint if it changes the bytes of a shipped binary.
#
# Inputs that are not part of the source tree are deliberately excluded:
#   * APP_VERSION (the commit SHA baked into sandbox-heartbeat) is pinned to a
#     constant; it only feeds the dashboard's version-mismatch hint.
#   * COPY sources that are gitignored (sandbox-images/: desktop image pins
#     written by build-ubuntu, bind-mounted over the baked copy at runtime).
#
# The build context is the working tree's tracked and untracked-but-not-ignored
# files. Output: the fingerprint on stdout; progress on stderr. Exits non-zero
# if it cannot be computed — callers must then assume the sandbox changed.
#
# Usage: scripts/sandbox-fingerprint.sh [repo_dir]
set -euo pipefail

FINGERPRINT_SCHEME=v1
FINGERPRINT_TARGET=helix-sandbox-fingerprint

repo=$(cd "${1:-$(dirname "${BASH_SOURCE[0]}")/..}" && pwd)
dockerfile="$repo/Dockerfile.sandbox"
[[ -f "$dockerfile" ]] || { echo "sandbox-fingerprint: $dockerfile not found" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
ctx="$work/ctx"
mkdir -p "$ctx"

# Build context: the source tree as git sees it (no ignored build outputs).
git -C "$repo" ls-files -z --cached --others --exclude-standard |
  tar -C "$repo" --null --no-recursion --ignore-failed-read -T - -cf - 2>/dev/null |
  tar -C "$ctx" -xf -

# The final stage's COPY/ADD instructions, continuation lines joined.
mapfile -t instructions < <(awk '
  { line = (pending == "" ? $0 : pending " " $0); pending = "" }
  /\\[[:space:]]*$/ { sub(/\\[[:space:]]*$/, "", line); pending = line; next }
  { print line }
' "$dockerfile" | awk '
  toupper($1) == "FROM" { n = 0; delete keep; next }
  toupper($1) == "COPY" || toupper($1) == "ADD" { keep[n++] = $0 }
  END { for (i = 0; i < n; i++) print keep[i] }
')
if [[ ${#instructions[@]} -eq 0 ]]; then
  echo "sandbox-fingerprint: no COPY/ADD found in the final stage of $dockerfile" >&2
  exit 1
fi

{
  cat "$dockerfile"
  printf '\nFROM scratch AS %s\n' "$FINGERPRINT_TARGET"
} > "$work/Dockerfile"
for instruction in "${instructions[@]}"; do
  read -r -a words <<< "$instruction"
  if [[ "$instruction" == *'<<'* || "${words[1]:-}" == '['* ]]; then
    echo "sandbox-fingerprint: unsupported instruction form: $instruction" >&2
    exit 1
  fi
  from_stage=false
  sources=()
  for word in "${words[@]:1:${#words[@]}-2}"; do
    case "$word" in
      --from=*) from_stage=true ;;
      --*) ;;
      *) sources+=("$word") ;;
    esac
  done
  if [[ "$from_stage" == false ]]; then
    missing=()
    for src in "${sources[@]}"; do
      [[ "$src" == http://* || "$src" == https://* ]] && continue
      compgen -G "$ctx/${src%/}" >/dev/null || missing+=("$src")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
      if [[ ${#missing[@]} -ne ${#sources[@]} ]]; then
        echo "sandbox-fingerprint: some sources of '$instruction' are not in the source tree: ${missing[*]}" >&2
        exit 1
      fi
      echo "sandbox-fingerprint: excluding untracked/ignored source(s) ${missing[*]}" >&2
      continue
    fi
  fi
  printf '%s\n' "$instruction" >> "$work/Dockerfile"
done

echo "sandbox-fingerprint: building shipped artifacts from $repo" >&2
docker buildx build --quiet \
  -f "$work/Dockerfile" \
  --target "$FINGERPRINT_TARGET" \
  --build-arg APP_VERSION=fingerprint \
  --output "type=local,dest=$work/out" \
  "$ctx" >/dev/null

# Hash path, type, mode and content of every shipped file, then the Dockerfile.
{
  echo "$FINGERPRINT_SCHEME"
  sha256sum < "$dockerfile"
  (cd "$work/out" && find . -mindepth 1 -printf '%y %m %p -> %l\n' | LC_ALL=C sort)
  (cd "$work/out" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum)
} > "$work/manifest"

if [[ -n "${SANDBOX_FINGERPRINT_MANIFEST:-}" ]]; then
  cp "$work/manifest" "$SANDBOX_FINGERPRINT_MANIFEST"
fi
echo "$FINGERPRINT_SCHEME-$(sha256sum < "$work/manifest" | cut -c1-64)"
