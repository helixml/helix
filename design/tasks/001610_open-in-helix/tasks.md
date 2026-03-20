# Implementation Tasks

- [ ] Update `buildPRFooterForTask()` in `api/pkg/server/spec_task_workflow_handlers.go`: change spec links format from inline pipe-separated to labeled bullet list, and join parts with `\n\n` instead of `" | "`
- [ ] Update `buildPRFooter()` in `api/pkg/services/git_http_server.go`: same changes as above
- [ ] Verify rendered output looks correct on a test PR in GitHub
