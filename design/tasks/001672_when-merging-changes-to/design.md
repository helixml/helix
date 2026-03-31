# Design: Bump sandbox-versions.txt on merge

## Approach

Add a short reminder block to `CLAUDE.md` in the Helix repo. No automation, no scripts — just a human-readable note in the right place.

The reminder belongs in the **Build Pipeline** section near the "When to Rebuild What" table, since that's where developers look when thinking about builds after merging.

## sandbox-versions.txt format

```
ZED_COMMIT=<full git sha from zed repo main>
QWEN_COMMIT=<full git sha from qwen-code repo main>
```

CI reads these to clone the correct commit before building the sandbox image.

## Decision

A CLAUDE.md entry is the right solution because:
- The step is inherently manual (someone must decide when a merge is "done")
- Automation would require cross-repo CI hooks with more complexity than warranted
- CLAUDE.md is already the authoritative reference agents check before any build/merge work

## Pattern discovered

`sandbox-versions.txt` is documented in the CI section of CLAUDE.md under "Zed WebSocket Sync E2E Testing → Key Files". The new reminder should go in the **Build Pipeline** section where rebuild instructions already live, making it visible to developers doing post-merge work.
