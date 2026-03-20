# Design: Better Pull Request Titles and Descriptions

## Overview

Enable agents to write professional PR titles and descriptions by having them create a `pull_request.md` file during implementation. The backend reads this file when creating PRs instead of using the raw task name/description.

## Current Behavior

In `git_http_server.go` and `spec_task_workflow_handlers.go`:
```go
description := fmt.Sprintf("> **Helix**: %s\n", task.Description)
prID, err := s.gitRepoService.CreatePullRequest(ctx, repo.ID, task.Name, description, branch, repo.DefaultBranch)
```

**Problem**: `task.Name` and `task.Description` come from the original user prompt, which is often informal or verbose.

## Proposed Solution

### 1. File Convention

**Location:** `/home/retro/work/helix-specs/design/tasks/{task_dir}/pull_request.md`

**Format:**
```markdown
# Short PR title here

Description body goes here. Can be multiple paragraphs.

## What Changed
- Change 1
- Change 2

## Testing
How it was tested.
```

**Parsing rules:**
- First line starting with `# ` = PR title (strip the `# ` prefix)
- First non-empty line if no `#` = PR title
- Everything after first blank line = PR description

### 2. Agent Prompt Update

Add to `approvalPromptTemplate` in `agent_instruction_service.go`:

```markdown
## Pull Request Content

Before you finish, create a **pull_request.md** file in your task directory:

` + "```" + `bash
cat > /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/pull_request.md << 'EOF'
# Clear, concise PR title (50 chars or less)

## Summary
Brief description of what this PR does.

## Changes
- List key changes made

## Testing
How this was tested (if applicable).
EOF
cd /home/retro/work/helix-specs && git add -A && git commit -m "Add PR description" && git push origin helix-specs
` + "```" + `

This file will be used as the PR title and description when the PR is created.
Write it as if you're explaining the change to a code reviewer.
```

### 3. Backend Changes

**New function in `git_http_server.go`:**

```go
// getPullRequestContent reads pull_request.md from helix-specs branch
// Returns (title, description, found). If not found, returns empty strings and false.
func (s *GitHTTPServer) getPullRequestContent(ctx context.Context, repoPath string, task *types.SpecTask) (string, string, bool) {
    if task.DesignDocPath == "" {
        return "", "", false
    }
    
    // Read from helix-specs branch
    filePath := fmt.Sprintf("design/tasks/%s/pull_request.md", task.DesignDocPath)
    cmd := exec.Command("git", "show", "helix-specs:"+filePath)
    cmd.Dir = repoPath
    output, err := cmd.Output()
    if err != nil {
        return "", "", false
    }
    
    return parsePullRequestMarkdown(string(output))
}

func parsePullRequestMarkdown(content string) (string, string, bool) {
    lines := strings.Split(strings.TrimSpace(content), "\n")
    if len(lines) == 0 {
        return "", "", false
    }
    
    // First line is title (strip # prefix if present)
    title := strings.TrimSpace(lines[0])
    title = strings.TrimPrefix(title, "# ")
    title = strings.TrimSpace(title)
    
    if title == "" {
        return "", "", false
    }
    
    // Find first blank line, everything after is description
    var descLines []string
    foundBlank := false
    for i := 1; i < len(lines); i++ {
        if !foundBlank && strings.TrimSpace(lines[i]) == "" {
            foundBlank = true
            continue
        }
        if foundBlank {
            descLines = append(descLines, lines[i])
        }
    }
    
    description := strings.TrimSpace(strings.Join(descLines, "\n"))
    return title, description, true
}
```

**Update `ensurePullRequest` in both files:**

```go
// Try to get custom PR content from pull_request.md
title, description, found := s.getPullRequestContent(ctx, repo.LocalPath, task)
if !found {
    // Fallback to existing behavior
    title = task.Name
    description = task.Description
    log.Debug().Str("task_id", task.ID).Msg("No pull_request.md found, using task name/description")
} else {
    log.Info().Str("task_id", task.ID).Msg("Using pull_request.md for PR content")
}

// Get org name for "Open in Helix" link
orgName := ""
if task.OrganizationID != "" {
    if org, err := s.store.GetOrganization(ctx, task.OrganizationID); err == nil && org != nil {
        orgName = org.Name
    }
}

// Append footer with "Open in Helix" link, spec doc links, and branding
footer := buildPRFooter(repo, task, orgName, s.serverBaseURL)
description = description + "\n\n" + footer

prID, err := s.gitRepoService.CreatePullRequest(ctx, repo.ID, title, description, branch, repo.DefaultBranch)
```

**New helper to build PR footer:**

```go
// buildPRFooter generates the PR description footer with:
// - "Open in Helix" link to the task in Helix UI
// - Spec doc links (if available for the repo type)
// - Helix branding
func buildPRFooter(repo *types.GitRepository, task *types.SpecTask, orgName, helixBaseURL string) string {
    var parts []string
    
    // "Open in Helix" link - always include if we have the necessary info
    if helixBaseURL != "" && orgName != "" && task.ProjectID != "" && task.ID != "" {
        helixTaskURL := fmt.Sprintf("%s/orgs/%s/projects/%s/tasks/%s", 
            strings.TrimSuffix(helixBaseURL, "/"), orgName, task.ProjectID, task.ID)
        parts = append(parts, fmt.Sprintf("🔗 [Open in Helix](%s)", helixTaskURL))
    }
    
    // Spec doc links (if available for this repo type)
    specDocsURL := ""
    if task.DesignDocPath != "" {
        specDocsURL = getSpecDocsBaseURL(repo, task.DesignDocPath)
    }
    
    if specDocsURL != "" {
        parts = append(parts, fmt.Sprintf(`📋 **Spec Documents**
- [Requirements](%s/requirements.md)
- [Design](%s/design.md)
- [Tasks](%s/tasks.md)`, specDocsURL, specDocsURL, specDocsURL))
    }
    
    // Helix branding - always include
    parts = append(parts, "🚀 Built with [Helix](https://helix.ml)")
    
    return "---\n" + strings.Join(parts, " | ")
}

func getSpecDocsBaseURL(repo *types.GitRepository, designDocPath string) string {
    if repo.ExternalURL == "" {
        return ""
    }
    
    baseURL := strings.TrimSuffix(repo.ExternalURL, ".git")
    
    // Build blob/browse URL based on provider
    // Each provider has a different URL structure for viewing files in a branch
    switch repo.ExternalType {
    case types.ExternalRepositoryTypeGitHub:
        // GitHub: https://github.com/owner/repo/blob/helix-specs/design/tasks/{path}
        return fmt.Sprintf("%s/blob/helix-specs/design/tasks/%s", baseURL, designDocPath)
    case types.ExternalRepositoryTypeGitLab:
        // GitLab: https://gitlab.com/owner/repo/-/blob/helix-specs/design/tasks/{path}
        return fmt.Sprintf("%s/-/blob/helix-specs/design/tasks/%s", baseURL, designDocPath)
    case types.ExternalRepositoryTypeADO:
        // Azure DevOps: https://dev.azure.com/org/project/_git/repo?path=/design/tasks/{path}&version=GBhelix-specs
        // Note: ADO uses query params, not path segments for branch
        return fmt.Sprintf("%s?path=/design/tasks/%s&version=GBhelix-specs", baseURL, designDocPath)
    case types.ExternalRepositoryTypeBitbucket:
        // Bitbucket Cloud: https://bitbucket.org/owner/repo/src/helix-specs/design/tasks/{path}
        // Bitbucket Server uses different format but this covers cloud
        return fmt.Sprintf("%s/src/helix-specs/design/tasks/%s", baseURL, designDocPath)
    default:
        // Unknown provider - skip links rather than generate broken URLs
        return ""
    }
}
```

## Key Files to Modify

1. `helix/api/pkg/services/agent_instruction_service.go` - Add PR content instructions to `approvalPromptTemplate`
2. `helix/api/pkg/services/git_http_server.go` - Add `getPullRequestContent`, `parsePullRequestMarkdown`, `buildPRFooter`, `getSpecDocsBaseURL`, update `ensurePullRequest`
3. `helix/api/pkg/server/spec_task_workflow_handlers.go` - Update `ensurePullRequestForTask` similarly (needs access to org name and server URL)

## Why This Approach

1. **No LLM at PR time**: Agent writes content during implementation when it has LLM access
2. **Simple file format**: Easy to parse, easy for agent to write
3. **Graceful fallback**: Existing tasks without the file continue to work
4. **Reuses existing infra**: helix-specs branch already synced and readable
5. **Agent has full context**: Can summarize actual changes, not just original prompt
6. **Spec links in PR**: Reviewers can easily access requirements, design, and task list (GitHub, GitLab, ADO, Bitbucket supported)
7. **"Open in Helix" link**: Direct link to task in Helix UI for full context and history

## Testing

1. Create task, implement with agent, verify `pull_request.md` is created
2. Approve implementation, verify PR uses custom title/description
3. Test fallback: task without `pull_request.md` uses task.Name/Description
4. Test parsing edge cases: empty file, no title, no description

## Implementation Notes

### Files Modified

1. **`api/pkg/services/git_http_server.go`**
   - Added `parsePullRequestMarkdown` - parses markdown content into title/description
   - Added `getPullRequestContent` - reads `pull_request.md` from helix-specs branch using `git show`
   - Added `getSpecDocsBaseURL` - builds URLs for GitHub, GitLab, ADO, Bitbucket
   - Added `buildPRFooter` - combines "Open in Helix" link, spec doc links, and branding
   - Modified `ensurePullRequest` to use custom content when available

2. **`api/pkg/server/spec_task_workflow_handlers.go`**
   - Added similar helper functions (`getPullRequestContentForTask`, `parsePullRequestMarkdownForTask`, etc.)
   - Modified `ensurePullRequestForTask` to use custom content and footer
   - Added `store` import for `GetOrganizationQuery`

3. **`api/pkg/services/agent_instruction_service.go`**
   - Added "Pull Request Description" section to `approvalPromptTemplate`
   - Instructs agent to create `pull_request.md` with title, summary, changes, testing

4. **`api/pkg/services/git_http_server_test.go`**
   - Added `TestParsePullRequestMarkdown` - 8 test cases covering edge cases
   - Added `TestGetSpecDocsBaseURL` - 7 test cases for all repo types

### Key Decisions

- Used `git show helix-specs:{path}` to read files from helix-specs branch without checkout
- Kept parsing simple: first line = title (strip `# ` prefix), after first blank line = description
- Made "Open in Helix" link use `org.Name` (the URL slug) not `OrganizationID`
- Footer always includes Helix branding; spec doc links only when URL can be constructed
- GetOrganization requires `&store.GetOrganizationQuery{ID: task.OrganizationID}` not just the ID string

### Gotchas

- `store.GetOrganization` takes a pointer to `GetOrganizationQuery`, not a string ID
- Need to import `store` package in `spec_task_workflow_handlers.go` for the query type
- The `os/exec` package needed to be added to imports for `exec.Command`