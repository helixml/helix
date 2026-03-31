# Implementation Tasks

- [x] Add a warning to `CLAUDE.md` in the Helix repo (Build Pipeline section) that states: when modifying Zed or Qwen, **open the Helix PR to bump `sandbox-versions.txt` first**, before merging the Zed/Qwen PR. Include the reason (spec task system marks tasks done when all PRs merge — if Zed PR merges first, the hash bump may never happen) and the correct key format (`ZED_COMMIT=`, `QWEN_COMMIT=`)
- [x] Commit and push the CLAUDE.md change to the Helix repo
