#!/usr/bin/env bash
# GOW utility functions

gow_log() {
    echo "$(date +"[%Y-%m-%d %H:%M:%S]") $*"
}

join_by() { local IFS="$1"; shift; echo "$*"; }

github_download(){
    curl -H "X-GitHub-Api-Version: "${GITHUB_REST_VERSION:-2022-11-28}"" \
        https://api.github.com/repos/$1/releases/latest | \
        jq $2 | \
        xargs wget -O $3
}
