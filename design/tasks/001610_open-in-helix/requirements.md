# Requirements: Reformat PR Footer

## User Story

As a developer reviewing a Helix-generated PR, I want the footer to be clearly formatted across multiple lines, so it's easy to read without being cluttered.

## Current Behavior

The PR footer renders as a single long pipe-separated line:

```
---
🔗 [Open in Helix](...) | 📋 [Requirements](...) | [Design](...) | [Tasks](...) | 🚀 Built with [Helix](https://helix.ml)
```

## Desired Behavior

The PR footer renders as structured multiline Markdown:

```
---
🔗 [Open in Helix](...)

📋 Spec:
- [Requirements](...)
- [Design](...)
- [Tasks](...)

🚀 Built with [Helix](https://helix.ml)
```

## Acceptance Criteria

- [ ] "Open in Helix" link appears on its own line
- [ ] "Spec:" section appears as a labeled list with Requirements, Design, Tasks as bullet points
- [ ] "Built with Helix" appears on its own line with the Helix link
- [ ] Each section is separated by a blank line for readability
- [ ] The `---` divider still precedes the footer
- [ ] If the spec docs URL is not available, that section is omitted (same as before)
- [ ] If the Helix task URL is not available, the Open in Helix line is omitted (same as before)
- [ ] Both `buildPRFooter` (git_http_server.go) and `buildPRFooterForTask` (spec_task_workflow_handlers.go) are updated consistently
