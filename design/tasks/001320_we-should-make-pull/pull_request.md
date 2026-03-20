# Add agent-written PR titles and descriptions

## Summary
Enable agents to write professional PR titles and descriptions by creating a `pull_request.md` file during implementation. The backend reads this file when creating PRs instead of using the raw task name/description.

## Changes
- Add `parsePullRequestMarkdown` and `getPullRequestContent` functions to read PR content from helix-specs branch
- Add `buildPRFooter` with "Open in Helix" link, spec doc links (GitHub, GitLab, ADO, Bitbucket), and Helix branding
- Update `ensurePullRequest` in both `git_http_server.go` and `spec_task_workflow_handlers.go` to use custom PR content
- Add instructions to `approvalPromptTemplate` telling agents to write `pull_request.md` before finishing
- Add unit tests for markdown parsing and URL generation

## Testing
- All unit tests pass (`TestParsePullRequestMarkdown`, `TestGetSpecDocsBaseURL`)
- Build verified with `go build ./pkg/services/ ./pkg/server/`
