# Merge upstream QwenLM/qwen-code v0.14.4 into the Helix fork

## Summary

Re-lands the qwen-code upgrade that task 001804 produced but never merged. The fork was pinned at v0.4.1 + 25 custom commits; this merges upstream `QwenLM/qwen-code` main (v0.14.4) — ~10 major versions, ~2800 commits — bringing the mature ACP integration that actually honours approval mode.

This is the qwen-code half of task 002098. Without it, the Helix-side `--yolo` / `default_mode` fix is inert: qwen v0.4.1's ACP `Session.runTool` path has no YOLO gate, so it round-trips a `session/request_permission` for every edit and stalls on an "Allow all edits?" prompt in headless sandboxes. v0.14.4's `Session.ts` L3/L4/L5 permission flow forces `defaultPermission = 'allow'` under YOLO and skips the permission request entirely — verified live in the inner Helix browser (GLM-5.1 + Qwen Code wrote a file with the tool call going straight to Completed, no prompt).

## Changes (3 commits on top of upstream v0.14.4)

- `420b2013a` Merge upstream QwenLM/qwen-code v0.14.4 into fork — resolved 11 conflicts (took upstream for acpAgent.ts/cli errors/edit/glob/shell-utils/tools; deleted upstream-removed schema.ts/smart-edit.ts; manually merged chatRecordingService.ts/ls.ts/paths.ts). Kept fork-specific bind-mount path normalization, `QWEN_DATA_DIR`, prompt customizations. Dropped fork changes superseded upstream (custom ACP Zod schema, debug logging, shell-security patches).
- `f01cdc413` Complete implementation
- `ca5f3a28c` Disable all external telemetry and phone-home in the Helix fork

## Verification

- `npm install && npm run build` succeed; produces `dist/cli.js` reporting `0.14.4`.
- Driven over ACP (`initialize → session/new → session/set_mode yolo → session/prompt`): 0 `session/request_permission` calls, `fs/write_text_file` executed. With `--yolo` on the command line it also auto-approves without any IDE-issued set_mode.
- End-to-end in the inner Helix browser via a GLM-5.1 Qwen Code spec task.

## Companion change

`helix` PR bumps `sandbox-versions.txt` `QWEN_COMMIT` to this branch tip (`ca5f3a28c`) so the desktop image builds this version. Merge order per CLAUDE.md: merge this qwen-code PR, then the helix PR.
