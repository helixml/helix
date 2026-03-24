# Design: Consistent Conventional Commits in Helix

## Investigation Summary

### What Was Found

| Mechanism | Location | Effect on Commits |
|-----------|----------|-------------------|
| CLAUDE.md | `/home/retro/work/helix-4/CLAUDE.md` | Read by Claude Code on start. Says "commit atomically" but **no conventional format** |
| AGENTS.md | Symlink → CLAUDE.md | Same file, both read by different agents |
| `commit-msg` hook | `.git/hooks/commit-msg` | Adds `Spec-Ref:` trailer only, **no format enforcement** |
| Agent prompts | `agent_instruction_service.go` | Uses generic `"Progress update"` in examples, **no conventional format** |
| PR intercept | `pull_request.md` convention | Affects PR title/description only, **not commits** |

### Why Commits Are Inconsistent

Claude Code uses conventional commits when it "feels right" based on training — not because helix enforces it. All three control points (CLAUDE.md, hook, agent prompts) are silent on the format, so each agent session makes its own call.

### The PR Intercept Does Not Affect Commits

Task 001320 implemented a system where agents write `pull_request.md` to helix-specs, and the backend reads it to set the PR title/description when the PR is created. This is purely about PR metadata — commit messages are separate and unaffected.

## Proposed Solution

### Layer 1: CLAUDE.md (highest leverage, immediate effect)

Add a clear rule to the "Commits & Debugging" section:

```markdown
### Commits & Debugging
- **Use conventional commit format**: `type(scope): description`
  - Types: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `style`
  - Examples: `feat(api): add PR content reading from helix-specs`
  - Keep subject line ≤ 72 chars, imperative mood
- Commit and push frequently, keep commits atomic...
```

### Layer 2: Agent Prompts in agent_instruction_service.go

Update the example commit commands in the approval prompt template to use conventional format:

```go
// Before:
git add -A && git commit -m "Progress update" && git push origin helix-specs

// After:
git add -A && git commit -m "chore(specs): update progress" && git push origin helix-specs
```

Similarly update other example commits in the template.

### Layer 3: commit-msg Hook Enforcement (strongest enforcement)

Extend the existing `commit-msg` hook to validate format before adding the `Spec-Ref` trailer:

```bash
# Validate conventional commit format
COMMIT_MSG=$(grep -v '^#' "$COMMIT_MSG_FILE" | head -1)
CONVENTIONAL_RE='^(feat|fix|refactor|chore|docs|test|style|perf|ci|build|revert)(\([a-z0-9/_-]+\))?: .+'
if ! echo "$COMMIT_MSG" | grep -qE "$CONVENTIONAL_RE"; then
    echo "ERROR: Commit message must use conventional format:"
    echo "  type(scope): description"
    echo "  e.g.: feat(api): add user authentication"
    exit 1
fi
```

**Tradeoffs:**
- Hard enforcement via hook is the most reliable but can frustrate agents if they don't know the rule
- Soft enforcement via CLAUDE.md alone relies on Claude following instructions (usually works)
- Recommended: do both — CLAUDE.md for understanding, hook for enforcement

### Layer 4: PR Titles — Keep As-Is

PR titles come from `pull_request.md` first line. These should remain descriptive prose (e.g., "Add conventional commit enforcement to helix"), **not** conventional commit format. PRs summarize a body of work; conventional commits describe atomic changes.

## Key Files to Modify

1. **`/home/retro/work/helix-4/CLAUDE.md`** — Add conventional commit requirement to "Commits & Debugging" section
2. **`/home/retro/work/helix-4/.git/hooks/commit-msg`** — Add format validation before `Spec-Ref` trailer
3. **`/home/retro/work/helix-4/api/pkg/services/agent_instruction_service.go`** — Update example commit messages in `approvalPromptTemplate` to use conventional format

## Patterns Discovered

- **CLAUDE.md is the primary Claude Code configuration** — changes here take effect immediately for all new sessions
- **`.git/hooks/commit-msg` is the only active git hook** — the other hooks are all `.sample` files
- **Agent prompts in `agent_instruction_service.go`** use template variables like `{{.TaskDirName}}` and are sent by the helix backend during planning/implementation phases
- **helix-specs is a git worktree** of the helix-4 repo (not a standalone repo)
- **PR intercept** reads `pull_request.md` via `git show helix-specs:{path}` — it's purely about PR metadata
