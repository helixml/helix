# Implementation Tasks

- [x] Add `parsePullRequestMarkdown` function in `git_http_server.go` to parse title/description from markdown content
- [x] Add `getPullRequestContent` function in `git_http_server.go` to read `pull_request.md` from helix-specs branch
- [x] Update `ensurePullRequest` in `git_http_server.go` to use custom PR content when available
- [x] Add `buildPRFooter` and `getSpecDocsBaseURL` helpers to build footer with "Open in Helix" link, spec doc links, and branding
- [x] Add similar `getPullRequestContent` helper in `spec_task_workflow_handlers.go`
- [x] Update `ensurePullRequestForTask` in `spec_task_workflow_handlers.go` to use custom PR content and footer
- [x] Update `approvalPromptTemplate` in `agent_instruction_service.go` to instruct agent to write `pull_request.md`
- [x] Add unit tests for `parsePullRequestMarkdown` (empty, no title, title only, full content)
- [x] Build and verify no compile errors
- [ ] Manual test: create task, complete implementation, verify pull_request.md is created and PR uses it