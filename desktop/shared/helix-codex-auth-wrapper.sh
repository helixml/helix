#!/bin/bash
set -euo pipefail

rm -f /tmp/codex-auth-error.txt /tmp/codex-auth-status.txt /tmp/codex-auth-stdout.txt
mkdir -p "$HOME/.local" "$HOME/.codex"
export NPM_CONFIG_CACHE=/tmp/helix-codex-npm-cache
for i in 1 2 3 4 5; do
    npm install -g --prefix "$HOME/.local" @openai/codex@latest 2>>/tmp/codex-npm-install.log && break
    [ "$i" -lt 5 ] && sleep 3
done
export PATH="$HOME/.local/bin:$PATH"

if ! command -v codex >/dev/null 2>&1; then
    echo "Codex CLI installation failed" > /tmp/codex-auth-error.txt
    exit 1
fi

cat > "$HOME/.codex/config.toml" <<'EOF'
cli_auth_credentials_store = "file"
forced_login_method = "chatgpt"
EOF

echo "starting" > /tmp/codex-auth-status.txt
script -qefc "codex login --device-auth" /tmp/codex-auth-stdout.txt
codex login status > /tmp/codex-auth-status.txt 2>&1
