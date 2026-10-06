#!/bin/bash

desktop_metadata_file_has_value() {
    [ -f "$1" ] && grep -q '[^[:space:]]' "$1"
}

desktop_image_metadata_source() {
    local image_dir="${HELIX_DESKTOP_IMAGE_DIR:-/opt/images}"
    local source

    source=$(findmnt -n -o SOURCE --target "$image_dir" 2>/dev/null || true)
    if [ -n "$source" ]; then
        printf '%s' "$source"
    else
        printf '%s' "container-image-layer"
    fi
}

restore_desktop_image_metadata() {
    local image_dir="${HELIX_DESKTOP_IMAGE_DIR:-/opt/images}"
    local seed_dir="${HELIX_DESKTOP_IMAGE_SEED_DIR:-/opt/images-seed}"
    local source metadata target restored=0

    mkdir -p "$image_dir"
    source=$(desktop_image_metadata_source)
    echo "🧭 Desktop image metadata: runtime_dir=${image_dir} mount_source=${source} seed_dir=${seed_dir}"

    if [ ! -d "$seed_dir" ]; then
        echo "⚠️  Desktop image metadata seed is unavailable: ${seed_dir}"
        return 0
    fi

    for metadata in "$seed_dir"/helix-*.version "$seed_dir"/helix-*.ref; do
        [ -f "$metadata" ] || continue
        target="$image_dir/$(basename "$metadata")"
        if desktop_metadata_file_has_value "$target"; then
            continue
        fi
        if cp "$metadata" "$target"; then
            echo "♻️  Restored desktop image metadata from bundled seed: ${target}"
            restored=$((restored + 1))
        else
            echo "⚠️  Failed to restore desktop image metadata: ${metadata} -> ${target}"
        fi
    done

    echo "✅ Desktop image metadata seed check complete: restored=${restored}"
}

desktop_image_candidate_tags() {
    local image_name="$1"

    docker images --format '{{.Repository}} {{.Tag}}' 2>/dev/null | awk -v name="$image_name" '
        {
            count = split($1, parts, "/")
            if (parts[count] == name && $2 != "latest" && $2 != "<none>") print $2
        }
    ' | sort -u
}

select_single_desktop_tag() {
    local candidates="$1"
    local count

    count=$(printf '%s\n' "$candidates" | awk 'NF { count++ } END { print count + 0 }')
    if [ "$count" -ne 1 ]; then
        return 1
    fi
    printf '%s\n' "$candidates" | awk 'NF { print; exit }'
}

write_desktop_version() {
    local version_file="$1"
    local version="$2"

    if ! printf '%s\n' "$version" > "$version_file"; then
        echo "⚠️  Cannot write desktop image version pointer: ${version_file}"
        return 1
    fi
}

recover_missing_desktop_version() {
    local name="$1"
    local image_name="helix-${name}"
    local image_dir="${HELIX_DESKTOP_IMAGE_DIR:-/opt/images}"
    local seed_dir="${HELIX_DESKTOP_IMAGE_SEED_DIR:-/opt/images-seed}"
    local version_file="$image_dir/${image_name}.version"
    local seed_file="$seed_dir/${image_name}.version"
    local local_registry="${HELIX_LOCAL_DESKTOP_REGISTRY:-registry:5000}"
    local registry_api="http://${local_registry}/v2/${image_name}/tags/list"
    local candidates version

    mkdir -p "$image_dir"
    if desktop_metadata_file_has_value "$version_file"; then
        return 0
    fi

    echo "🔎 ${image_name}: version pointer missing; checking bundled seed ${seed_file}"
    if desktop_metadata_file_has_value "$seed_file" && cp "$seed_file" "$version_file"; then
        echo "♻️  ${image_name}: restored version pointer from bundled seed"
        return 0
    fi

    echo "🔎 ${image_name}: bundled seed unavailable; checking nested Docker image store"
    candidates=$(desktop_image_candidate_tags "$image_name")
    if version=$(select_single_desktop_tag "$candidates"); then
        if ! write_desktop_version "$version_file" "$version"; then
            return 1
        fi
        echo "♻️  ${image_name}: adopted version ${version} from nested Docker image store"
        return 0
    fi
    echo "⚠️  ${image_name}: nested Docker store did not contain one unambiguous version tag (${candidates:-none})"

    echo "🔎 ${image_name}: checking local registry fallback ${registry_api}"
    candidates=$(curl -fsS --max-time 5 "$registry_api" 2>/dev/null | jq -r '.tags[]?' 2>/dev/null | awk '$0 != "latest" && $0 != "<none>"' | sort -u || true)
    if version=$(select_single_desktop_tag "$candidates"); then
        if ! write_desktop_version "$version_file" "$version"; then
            return 1
        fi
        echo "♻️  ${image_name}: adopted version ${version} from local registry fallback"
        return 0
    fi
    echo "⚠️  ${image_name}: local registry fallback did not contain one unambiguous version tag (${candidates:-none})"
    return 1
}

diagnose_desktop_image_failure() {
    local name="$1"
    local image_name="helix-${name}"
    local image_dir="${HELIX_DESKTOP_IMAGE_DIR:-/opt/images}"
    local seed_dir="${HELIX_DESKTOP_IMAGE_SEED_DIR:-/opt/images-seed}"
    local local_registry="${HELIX_LOCAL_DESKTOP_REGISTRY:-registry:5000}"
    local source runtime_version seed_version runtime_ref seed_ref

    source=$(desktop_image_metadata_source)
    runtime_version=$(desktop_metadata_file_has_value "$image_dir/${image_name}.version" && echo present || echo missing)
    seed_version=$(desktop_metadata_file_has_value "$seed_dir/${image_name}.version" && echo present || echo missing)
    runtime_ref=$(desktop_metadata_file_has_value "$image_dir/${image_name}.ref" && echo present || echo missing)
    seed_ref=$(desktop_metadata_file_has_value "$seed_dir/${image_name}.ref" && echo present || echo missing)

    echo "   Metadata runtime: dir=${image_dir} mount_source=${source} version=${runtime_version} ref=${runtime_ref}"
    echo "   Bundled seed: dir=${seed_dir} version=${seed_version} ref=${seed_ref}"
    echo "   Checked nested Docker tags for ${image_name}:*"
    echo "   Checked local registry fallback: http://${local_registry}/v2/${image_name}/tags/list"
}
