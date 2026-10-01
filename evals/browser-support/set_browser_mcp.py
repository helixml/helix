#!/usr/bin/env python3
"""Override a bot project's chrome-devtools MCP (same name wins over the
built-in entry in zed_config.go) to try another chrome-devtools-mcp version or
tool-category set without rebuilding the desktop image.

    python3 set_browser_mcp.py <bot-id> 1.10.1 --categoryNetwork=false ...
    python3 set_browser_mcp.py <bot-id> --reset
"""

import sys

import run_eval as r

# Installs the requested version once into the persistent workspace volume
# (restarts do not re-download it), then runs it through the image's wrapper,
# so it shares the sandbox's one Chrome like the built-in server does. The
# args are the built-in ones from zed_config.go.
WRAPPER = r'''set -e
P=/home/retro/work/.cdm-$CDM_VERSION
[ -x "$P/bin/chrome-devtools-mcp" ] || npm install -g --silent --cache /home/retro/work/.npm-cache --prefix "$P" "chrome-devtools-mcp@$CDM_VERSION" >/dev/null 2>&1
HELIX_CHROME_DEVTOOLS_MCP="$P/bin/chrome-devtools-mcp" exec /usr/local/bin/helix-chrome-devtools-mcp "$@"'''

BASE_ARGS = [
    "--user-data-dir=/home/retro/work/.chrome-state",
    "--viewport", "1280x800",
    "--chrome-arg=--ozone-platform=wayland",
    "--chrome-arg=--disable-blink-features=AutomationControlled",
    "--chrome-arg=--no-first-run",
    "--chrome-arg=--disable-infobars",
    "--chrome-arg=--disable-extensions",
    "--no-usage-statistics",
    "--no-performance-crux",
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
