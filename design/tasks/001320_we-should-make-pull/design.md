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
    description = fmt.Sprintf("> **Helix**: %s\n", task.Description)
    log.Debug().Str("task_id", task.ID).Msg("No pull_request.md found, using task name/description")
} else {
    log.Info().Str("task_id", task.ID).Msg("Using pull_request.md for PR content")
}

prID, err := s.gitRepoService.CreatePullRequest(ctx, repo.ID, title, description, branch, repo.DefaultBranch)
```

## Key Files to Modify

1. `helix/api/pkg/services/agent_instruction_service.go` - Add PR content instructions to `approvalPromptTemplate`
2. `helix/api/pkg/services/git_http_server.go` - Add `getPullRequestContent`, `parsePullRequestMarkdown`, update `ensurePullRequest`
3. `helix/api/pkg/server/spec_task_workflow_handlers.go` - Update `ensurePullRequestForTask` similarly

## Why This Approach

1. **No LLM at PR time**: Agent writes content during implementation when it has LLM access
2. **Simple file format**: Easy to parse, easy for agent to write
3. **Graceful fallback**: Existing tasks without the file continue to work
4. **Reuses existing infra**: helix-specs branch already synced and readable
5. **Agent has full context**: Can summarize actual changes, not just original prompt

## Testing

1. Create task, implement with agent, verify `pull_request.md` is created
2. Approve implementation, verify PR uses custom title/description
3. Test fallback: task without `pull_request.md` uses task.Name/Description
4. Test parsing edge cases: empty file, no title, no description