# Bump ZED_COMMIT for external ACP permission auto-approve

## Summary

Picks up the Zed fix that stops Qwen Code spec tasks stalling on
"Awaiting Confirmation / Allow all edits / Reject" prompts.

Helix already did everything it could on its side: it launches qwen with
`--yolo` and `default_mode: "yolo"`
(`api/cmd/settings-sync-daemon/main.go`) and writes
`agent.tool_permissions.default = "allow"` into Zed's settings
(`injectAgentToolPermissions`). The problem was that Zed only consulted that
setting for its *native* agent — the external-ACP handler ignored it and always
rendered an interactive dialog, which nothing clicks in a headless sandbox.

The fix is entirely in Zed
(`crates/agent_servers/src/acp.rs`); this PR only moves the pin forward. **No
Helix code change is required.**

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT` → `4d248e320eeab82a8dd3d86d8c83c01f92b712b7`

## Notes for reviewers

- Merge the Zed PR first, then this one (per the ordering rule in `CLAUDE.md`).
- The merge with `main` had a conflict in `sandbox-versions.txt` because main had
  independently bumped `ZED_COMMIT` to `06e9ce8059…`. Resolved in favour of the
  new hash, which descends from `06e9ce8059…` (verified with
  `git merge-base --is-ancestor`), so nothing is dropped.
