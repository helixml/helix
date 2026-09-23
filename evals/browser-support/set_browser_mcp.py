#!/usr/bin/env python3
"""Override a bot project's chrome-devtools MCP (same name wins over the
built-in entry in zed_config.go) to try another chrome-devtools-mcp version or
tool-category set without rebuilding the desktop image.

    python3 set_browser_mcp.py <bot-id> 1.10.1 --categoryNetwork=false ...
    python3 set_browser_mcp.py <bot-id> --reset
"""

import sys

import run_eval as r

# Mirrors desktop/shared/helix-chrome-devtools-mcp.sh plus the built-in args
# from zed_config.go. The package is installed once into the persistent
# workspace volume so restarts do not re-download it.
WRAPPER = r'''set -e
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
if [ -z "${WAYLAND_DISPLAY:-}" ]; then
  for s in "$XDG_RUNTIME_DIR"/wayland-*; do [ -S "$s" ] && WAYLAND_DISPLAY="${s##*/}"; done
  export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-0}"
fi
P=/home/retro/work/.cdm-$CDM_VERSION
[ -x "$P/bin/chrome-devtools-mcp" ] || npm install -g --silent --prefix "$P" "chrome-devtools-mcp@$CDM_VERSION" >/dev/null 2>&1
exec "$P/bin/chrome-devtools-mcp" "$@"'''

BASE_ARGS = [
    "--user-data-dir=/home/retro/work/.chrome-state",
    "--viewport", "1280x800",
    "--chrome-arg=--ozone-platform=wayland",
    "--chrome-arg=--disable-blink-features=AutomationControlled",
    "--chrome-arg=--no-first-run",
    "--chrome-arg=--disable-infobars",
    "--chrome-arg=--disable-extensions",
]


def main():
    bot = sys.argv[1]
    project_id = r.api("GET", f"/orgs/{r.ORG}/bots/{bot}")["project_id"]
    project = r.api("GET", f"/projects/{project_id}")
    skills = project.get("skills") or {}
    mcps = [m for m in (skills.get("mcps") or []) if m.get("name") != "chrome-devtools"]
    if sys.argv[2] != "--reset":
        version, extra = sys.argv[2], sys.argv[3:]
        mcps.append({
            "name": "chrome-devtools",
            "description": f"chrome-devtools-mcp {version} {' '.join(extra)}",
            "transport": "stdio",
            "command": "bash",
            "args": ["-c", WRAPPER, "cdm", *BASE_ARGS, *extra],
            "env": {"CDM_VERSION": version, "CHROME_PATH": "/usr/bin/google-chrome-stable"},
            "tools": [],
        })
    skills["mcps"] = mcps
    r.api("PUT", f"/projects/{project_id}", {"skills": skills})
    print(bot, "chrome-devtools MCP:", mcps[-1]["description"] if sys.argv[2] != "--reset" else "built-in")


if __name__ == "__main__":
    main()
