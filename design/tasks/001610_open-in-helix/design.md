# Design: Reformat PR Footer

## Overview

Two functions generate the PR footer — both need updating to produce the new multiline format. The change is purely cosmetic: no logic changes, just string formatting.

## Files to Change

| File | Function |
|------|----------|
| `api/pkg/server/spec_task_workflow_handlers.go` | `buildPRFooterForTask()` lines 408–438 |
| `api/pkg/services/git_http_server.go` | `buildPRFooter()` lines 1125–1154 |

## New Footer Format

Replace the `strings.Join(parts, " | ")` approach with a fixed multiline template. Build each section independently and join with `\n\n`:

```go
// Current (single line):
return "---\n" + strings.Join(parts, " | ")

// New (multiline):
return "---\n" + strings.Join(parts, "\n\n")
```

Each part changes as follows:

**Open in Helix** (unchanged string):
```
🔗 [Open in Helix](https://...)
```

**Spec links** (was inline, now a labeled list):
```go
// Old:
fmt.Sprintf("📋 [Requirements](%s/requirements.md) | [Design](%s/design.md) | [Tasks](%s/tasks.md)", ...)

// New:
fmt.Sprintf("📋 Spec:\n- [Requirements](%s/requirements.md)\n- [Design](%s/design.md)\n- [Tasks](%s/tasks.md)", ...)
```

**Built with Helix** (unchanged string):
```
🚀 Built with [Helix](https://helix.ml)
```

## Rendered Output (GitHub Markdown)

```
---
🔗 [Open in Helix](...)

📋 Spec:
- [Requirements](...)
- [Design](...)
- [Tasks](...)

🚀 Built with [Helix](https://helix.ml)
```

## Notes

- Both functions are near-identical — apply the same change to both
- No tests cover this output directly; verify by inspecting a PR after deploy
- The `---` separator is rendered as a horizontal rule in GitHub Markdown, which gives natural visual separation above the footer
