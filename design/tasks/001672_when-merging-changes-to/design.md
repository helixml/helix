# Design: Bump sandbox-versions.txt on merge

## Approach

Add a clear warning block to `CLAUDE.md` in the Helix repo. No automation — a human-readable note that enforces the correct merge order.

## Critical merge order

This order matters because the spec task system marks a task done when all its PRs are merged. If the Zed PR is merged first, the system may think the task is complete — but `sandbox-versions.txt` in Helix still points to the old commit.

**Required order when modifying Zed or Qwen:**

1. **Open the Helix PR first** (bumping `sandbox-versions.txt` with the expected new commit hash)
2. Merge the Zed/Qwen PR
3. Merge the Helix PR

The Helix PR must exist before any downstream repo PR is merged.

## sandbox-versions.txt format

```
ZED_COMMIT=<full git sha from zed repo main>
QWEN_COMMIT=<full git sha from qwen-code repo main>
```

CI reads these to clone the correct commit before building the sandbox image.

## Placement in CLAUDE.md

The warning belongs in the **Build Pipeline** section under "When to Rebuild What", where developers already look when planning build/merge work. It should be visually prominent (e.g., a bold warning or a dedicated subsection).

## Pattern discovered

`sandbox-versions.txt` is referenced in the CI section of CLAUDE.md under "Zed WebSocket Sync E2E Testing → Key Files". The new warning should be in Build Pipeline so it's seen before the work starts, not after.
