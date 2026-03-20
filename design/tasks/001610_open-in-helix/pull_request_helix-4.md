# Reformat PR footer to multiline Markdown layout

## Summary

The PR footer added by Helix was a single noisy pipe-separated line. This reformats it into a clean multiline layout that renders well in GitHub.

## Before

```
---
🔗 [Open in Helix](...) | 📋 [Requirements](...) | [Design](...) | [Tasks](...) | 🚀 Built with [Helix](https://helix.ml)
```

## After

```
---
🔗 [Open in Helix](...)

📋 Spec:
- [Requirements](...)
- [Design](...)
- [Tasks](...)

🚀 Built with [Helix](https://helix.ml)
```

## Changes

- `api/pkg/server/spec_task_workflow_handlers.go`: spec links now formatted as a labeled bullet list; sections joined with `\n\n`
- `api/pkg/services/git_http_server.go`: same changes
