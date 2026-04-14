# Design: Merge Upstream qwen-code

## Current State

- **Our fork**: v0.4.1 + 25 custom commits (406 insertions, 107 deletions across 20 files)
- **Upstream**: v0.14.4 (~10 major versions ahead)
- **No `upstream` remote** configured — only `origin` pointing to our internal git server
- **No GitHub fork relationship** — repo lives on internal Helix git, not as a GitHub fork

## Fork Change Inventory

Our 25 commits on top of v0.4.1, categorized by likely outcome after merge:

### Likely DROP (upstream supersedes)

| Commit | Description | Reason |
|--------|-------------|--------|
| `b91d9a7b` | Align session/list with ACP v0.10.0 | Upstream ACP SDK is at v0.14.1 — far past v0.10.0 |
| `99a19a5b` | camelCase for ACP session list response | Upstream ACP module is mature; likely fixed |
| `2bfae703` | Race conditions in session history replay | Upstream `HistoryReplayer.ts` is 6.9KB (ours patched a smaller version) |
| `f6cdc989` | Defer history replay until after loadSession | Likely fixed in upstream's rewritten Session.ts (44.8KB!) |
| `8659ef8d` | Use stored callIds in history replay | Same — upstream rewrote history replay |
| `acc9c9c0` | `QWEN_DATA_DIR` env var | Upstream has `QWEN_RUNTIME_DIR` serving same purpose |
| `e3d57245` | Use `getErrorMessage()` consistently | Upstream likely has this or equivalent error handling |
| `d269feb6` / `8e6862ae` | Show errno instead of [object Object] | Bug likely fixed upstream in 10 versions |
| `9cd5d289` | Debug logging for ACP session load | Debug-only; upstream has its own logging |
| 6 debug commits | Various debug logging additions | Temporary debugging; not needed in merge |

### Likely KEEP (fork-specific needs)

| Commit | Description | Reason |
|--------|-------------|--------|
| `4db0162d` / `5ec60340` | Disable write redirection security | Container sandbox-specific; upstream uses Docker sandbox differently |
| `b67b46de` / `a8bc8ce0` | Disable command substitution security | Same container need |
| `49677ed1` | Normalize paths for bind-mounts | Container-specific; unlikely upstream addresses this |
| `6d2f4457` | Instruction to not output XML tool calls | Qwen-model-specific prompt tweak for our deployment |
| `94c3b97d` | `is_background` parameter instruction | May still be needed for our prompt customization |

### Likely ADAPT (needs rework to fit upstream)

| Commit | Description | Reason |
|--------|-------------|--------|
| `14ebe78c` | Extend ACP schema for HTTP/SSE MCP servers | Our `schema.ts` doesn't exist upstream; need to check if upstream handles this differently |

## Merge Strategy

**Approach: Add upstream remote, merge with conflict resolution.**

```
git remote add upstream https://github.com/QwenLM/qwen-code.git
git fetch upstream
git merge upstream/main --no-commit
# Resolve conflicts, review each file
git commit
```

Why merge (not rebase):
- 25 commits on top of 2600+ upstream commits — rebasing would be painful and rewrite history
- Merge preserves our commit history for future reference
- Standard approach for long-lived forks

## Key Conflict Areas (Expected)

Based on our 20 changed files vs. upstream's 10 major versions:

1. **`packages/cli/src/acp-integration/`** — Highest conflict area. Upstream rewrote/expanded the entire ACP module. Our `schema.ts`, `acpAgent.ts`, `HistoryReplayer.ts`, and `Session.ts` changes will conflict.
2. **`packages/core/src/utils/shellReadOnlyChecker.ts`** — Our security disabling vs. upstream's AST-based shell parser rewrite.
3. **`packages/core/src/config/storage.ts`** — Our `QWEN_DATA_DIR` vs. upstream's `QWEN_RUNTIME_DIR`.
4. **`packages/core/src/tools/`** — Minor conflicts in edit.ts, glob.ts, ls.ts, etc.

## Key Decision: `schema.ts`

Our fork has a custom `packages/cli/src/acp-integration/schema.ts` (Zod-based ACP protocol definitions). **This file does not exist upstream** — upstream defines ACP types differently (likely via the `@agentclientprotocol/sdk` package directly). During merge:

- Check if upstream's ACP SDK types cover what our `schema.ts` provides
- If so, drop `schema.ts` and use SDK types
- If not, keep `schema.ts` but update it for ACP v0.14.1

## Sandbox Security Approach

Upstream uses a Docker sandbox image (`ghcr.io/qwenlm/qwen-code:0.14.4`) and `sandboxConfig.ts`. Our approach disables shell security checks at the code level. Post-merge:

- Check if upstream's sandbox config has a flag to disable security checks
- If yes, use that instead of our code-level patches
- If no, re-apply our security disabling patches on top of the merged code
