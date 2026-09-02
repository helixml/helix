#!/bin/bash

# Garbage-collect obsolete Helix desktop image tags. This file is sourced by
# 40-start-dockerd.sh and kept separate so the selection policy can be tested.
cleanup_desktop_images() {
    local phase="${1:-post-pull}"
    local image_dir="${HELIX_DESKTOP_IMAGE_DIR:-/opt/images}"
    local retention="${HELIX_DESKTOP_IMAGE_RETENTION:-1}"
    local desktop_names=""
    local version_file image_name desktop_name image repository tag expected
    local candidates candidate_versions keep_versions removed_count=0 kept_count=0
    declare -A expected_versions

    if ! [[ "$retention" =~ ^[0-9]+$ ]]; then
        echo "⚠️  Invalid HELIX_DESKTOP_IMAGE_RETENTION='$retention'; using 1"
        retention=1
    fi

    for version_file in "$image_dir"/helix-*.version; do
        [ -f "$version_file" ] || continue
        image_name=$(basename "$version_file" .version)
        expected_versions[$image_name]=$(cat "$version_file")
        desktop_name="${image_name#helix-}"
        desktop_names="${desktop_names:+$desktop_names|}$desktop_name"
    done
    if [ -z "$desktop_names" ]; then
        echo "   No desktop version files found - skipping cleanup"
        return 0
    fi

    candidates=$(docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | awk -F/ -v names="$desktop_names" '
        BEGIN { pattern = "^helix-(" names "):" }
        $NF ~ pattern { print }
    ' | sort -u)

    for image_name in "${!expected_versions[@]}"; do
        expected="${expected_versions[$image_name]}"
        candidate_versions=$(printf '%s\n' "$candidates" | awk -F/ -v name="$image_name" '
            $NF ~ ("^" name ":") { sub(/^.*:/, "", $NF); if ($NF != "latest") print $NF }
        ' | sort -Vu)
        keep_versions=$(printf '%s\n' "$candidate_versions" | grep -vxF "$expected" | sort -Vr | head -n "$retention" || true)

        for image in $candidates; do
            repository="${image%:*}"
            [ "${repository##*/}" = "$image_name" ] || continue
            tag="${image##*:}"
            if [ "$tag" = "$expected" ] || [ "$tag" = "latest" ] || printf '%s\n' "$keep_versions" | grep -qxF "$tag"; then
                kept_count=$((kept_count + 1))
                continue
            fi
            echo "   Removing obsolete desktop image during $phase: $image"
            if docker rmi "$image" >/dev/null 2>&1; then
                removed_count=$((removed_count + 1))
            else
                echo "   ⚠️  Kept $image because Docker reports it is still in use"
            fi
        done
    done
    echo "✅ Desktop image cleanup ($phase): removed $removed_count tag(s), kept $kept_count tag(s), retained $retention previous version(s)"
}
