#!/bin/bash
# Browser languages: websites see the user's languages, not the runner's location.
#
# Many sites pick their language from the request's IP address when nothing
# else says otherwise, so a desktop on a runner in Germany comes up in German.
# The API passes the user's own browser languages (navigator.languages) as
# HELIX_BROWSER_LANGUAGES="en-GB,en". We set them as Chrome's preferred
# languages via the ForcedLanguages policy, which drives both the
# Accept-Language header and navigator.languages. Languages the user adds in
# chrome://settings/languages still apply, listed after these.
#
# The entrypoint sources this file, so skipping must `return`, not `exit`.

if [ -z "${HELIX_BROWSER_LANGUAGES:-}" ]; then
    echo "[browser-languages] HELIX_BROWSER_LANGUAGES not set; using browser defaults"
    return 0 2>/dev/null || exit 0
fi

# The API already sanitizes the list; re-check each tag before writing JSON.
browser_languages_json=""
IFS=',' read -ra browser_language_tags <<< "${HELIX_BROWSER_LANGUAGES}"
for tag in "${browser_language_tags[@]}"; do
    if [[ "$tag" =~ ^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8}){0,3}$ ]]; then
        browser_languages_json+="${browser_languages_json:+, }\"${tag}\""
    fi
done
unset browser_language_tags tag

if [ -z "$browser_languages_json" ]; then
    echo "[browser-languages] no valid languages in HELIX_BROWSER_LANGUAGES='${HELIX_BROWSER_LANGUAGES}'"
    unset browser_languages_json
    return 0 2>/dev/null || exit 0
fi

# Google Chrome (amd64) and Chromium (arm64) read different policy directories.
for policy_dir in /etc/opt/chrome/policies/managed /etc/chromium/policies/managed; do
    mkdir -p "$policy_dir"
    echo "{\"ForcedLanguages\": [${browser_languages_json}]}" > "$policy_dir/helix-languages.json"
    chmod 644 "$policy_dir/helix-languages.json"
done
echo "[browser-languages] browser languages set to [${browser_languages_json}]"
unset browser_languages_json policy_dir
