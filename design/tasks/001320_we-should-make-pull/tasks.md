# Implementation Tasks

- [ ] Add `parsePullRequestMarkdown` function in `git_http_server.go` to parse title/description from markdown content
- [ ] Add `getPullRequestContent` function in `git_http_server.go` to read `pull_request.md` from helix-specs branch
- [ ] Update `ensurePullRequest` in `git_http_server.go` to use custom PR content when available
- [ ] Add similar `getPullRequestContent` helper in `spec_task_workflow_handlers.go`
- [ ] Update `ensurePullRequestForTask` in `spec_task_workflow_handlers.go` to use custom PR content
- [ ] Update `approvalPromptTemplate` in `agent_instruction_service.go` to instruct agent to write `pull_request.md`
- [ ] Add unit tests for `parsePullRequestMarkdown` (empty, no title, title only, full content)
- [ ] Build and verify no compile errors
- [ ] Manual test: create task, complete implementation, verify pull_request.md is created and PR uses it