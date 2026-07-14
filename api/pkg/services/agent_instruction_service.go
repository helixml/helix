package services

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AgentInstructionService sends automated instructions to agent sessions
type AgentInstructionService struct {
	store        store.Store
	enqueuer     SpecTaskMessageEnqueuer
	koditService KoditServicer
}

// NewAgentInstructionService creates a new agent instruction service
func NewAgentInstructionService(store store.Store, enqueuer SpecTaskMessageEnqueuer, koditService KoditServicer) *AgentInstructionService {
	return &AgentInstructionService{
		store:        store,
		enqueuer:     enqueuer,
		koditService: koditService,
	}
}

// GetTaskDirName returns the task directory name, preferring DesignDocPath with fallback to task.ID
func GetTaskDirName(task *types.SpecTask) string {
	if task.DesignDocPath != "" {
		return task.DesignDocPath
	}
	return task.ID // Backwards compatibility for old tasks
}

// GetRawScreenshotBaseURL returns a raw content URL for a screenshot on the
// helix-specs branch in the external repo. The returned URL contains the
// placeholder SCREENSHOT_FILENAME which the agent replaces with real filenames.
// This works across all providers including ADO (which uses query parameters).
func GetRawScreenshotBaseURL(repo *types.GitRepository, taskDirName string) string {
	if repo == nil || repo.ExternalURL == "" {
		return ""
	}

	baseURL := strings.TrimSuffix(repo.ExternalURL, ".git")
	screenshotPath := fmt.Sprintf("design/tasks/%s/screenshots", taskDirName)

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeGitHub:
		// {baseURL}/raw/branch/path works on both github.com (redirects to raw.githubusercontent.com)
		// and GitHub Enterprise (serves raw content natively)
		return fmt.Sprintf("%s/raw/helix-specs/%s/SCREENSHOT_FILENAME", baseURL, screenshotPath)
	case types.ExternalRepositoryTypeGitLab:
		// Works for both gitlab.com and self-hosted GitLab
		return fmt.Sprintf("%s/-/raw/helix-specs/%s/SCREENSHOT_FILENAME", baseURL, screenshotPath)
	case types.ExternalRepositoryTypeADO:
		// Transform _git/repo to _apis/git/repositories/repo/items for raw content
		// e.g. https://dev.azure.com/org/project/_git/repo -> .../org/project/_apis/git/repositories/repo/items?path=...
		adoURL := strings.Replace(baseURL, "/_git/", "/_apis/git/repositories/", 1)
		return fmt.Sprintf("%s/items?path=/%s/SCREENSHOT_FILENAME&versionDescriptor.version=helix-specs&versionDescriptor.versionType=branch", adoURL, screenshotPath)
	case types.ExternalRepositoryTypeBitbucket:
		return fmt.Sprintf("%s/raw/helix-specs/%s/SCREENSHOT_FILENAME", baseURL, screenshotPath)
	default:
		return ""
	}
}

// =============================================================================
// Template Data Structures
// =============================================================================

// ApprovalPromptData contains all data for the approval/implementation prompt
type ApprovalPromptData struct {
	Guidelines            string   // Formatted guidelines section
	KoditSection          string   // Dynamic MCP tool documentation from kodit (empty when disabled)
	RepositorySection     string   // Available repositories section (local + Kodit repos)
	PrimaryRepoName       string   // Name of the primary repository (e.g., "my-app")
	NonPrimaryRepoNames   []string // Names of non-primary repositories (for per-repo PR descriptions)
	TaskDirName           string   // Design doc directory name
	BranchName            string   // Feature branch name
	BaseBranch            string   // Base branch (e.g., "main")
	TaskName              string   // Human-readable task name
	OriginalPromptSection string   // Formatted original request section (different for cloned vs normal)
	ClonedTaskPreamble    string   // Extra instructions for cloned tasks (empty if not cloned)
	ApprovalComments      string   // Reviewer's comments when approving (may be empty)
	ScreenshotBaseURL     string   // Raw content URL prefix for screenshots on helix-specs branch (empty if no external repo)
}

// CommentPromptData contains data for design review comment prompts
type CommentPromptData struct {
	DocumentLabel string
	SectionPath   string
	LineNumber    int
	QuotedText    string
	CommentText   string
	TaskDirName   string
}

// RevisionPromptData contains data for revision instruction prompts
type RevisionPromptData struct {
	TaskDirName string
	Comments    string
}

// =============================================================================
// Compiled Templates
// =============================================================================

var approvalPromptTemplate = template.Must(template.New("approval").Parse(`## CURRENT PHASE: IMPLEMENTATION

You are now in the IMPLEMENTATION phase. The planning/spec-writing phase is complete.
- Your design has been approved - now implement the code changes
- You MAY now ask the user questions that were deferred from planning (e.g., preferences, clarifications)
- You MAY now make code changes, edit files, and modify the codebase
{{.ClonedTaskPreamble}}
---

# Design Approved - Begin Implementation

Speak English.
{{.Guidelines}}

Your design has been approved. Implement the code changes now.

## CRITICAL RULES

1. **PUSH after every task** - The UI tracks progress via git pushes to helix-specs
2. **Do the bare minimum** - Simple tasks = simple solutions. No over-engineering.
3. **Update tasks.md** - Mark [~] when you START a task, [x] when DONE, push immediately
4. **Update design docs as you go** - Modify requirements.md, design.md, tasks.md when you learn something new
5. **Use conventional commit format** - ` + "`type(scope): description`" + ` (e.g., ` + "`feat(api): add X`" + `, ` + "`fix(frontend): handle Y`" + `, ` + "`docs(specs): update progress`" + `). Types: feat, fix, refactor, chore, docs, test, style, perf, ci, build, revert. The ` + "`commit-msg`" + ` hook in code repos enforces this.

## CRITICAL: Use tasks.md, NOT Internal To-Do Tools

You may have access to an internal to-do list tool (TodoWrite, todo_write, or similar). **IGNORE IT.**

Your ONLY to-do list is **/home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/tasks.md**.

- Track ALL progress in tasks.md (mark [~] when starting, [x] when done)
- Update tasks.md as your plan evolves (add new tasks, remove unnecessary ones, reorder)
- Commit and push to helix-specs after EVERY change to tasks.md
- The UI reads tasks.md to show progress - internal tools are invisible to users

If you use an internal to-do tool instead of tasks.md, users cannot see your progress.

## Two Repositories - Don't Confuse Them

1. **/home/retro/work/helix-specs/** = Design docs and progress tracking (push to helix-specs branch)
2. **/home/retro/work/{{.PrimaryRepoName}}/** = Code changes (push to feature branch) - THIS IS YOUR PRIMARY PROJECT
{{.RepositorySection}}
## Task Checklist

Your checklist: /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/tasks.md

- [ ] = not started
- [~] = in progress (currently working on)
- [x] = done

When you START a task, change [ ] to [~] and push. When DONE, change [~] to [x] and push.
Small frequent pushes are better than one big push at the end.

` + "```bash" + `
cd /home/retro/work/helix-specs
git add -A && git commit -m "chore(specs): update progress" && git push origin helix-specs
` + "```" + `

## Steps

1. Read design docs: /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/
2. Verify branch: ` + "`cd /home/retro/work/{{.PrimaryRepoName}} && git branch --show-current`" + ` (should be {{.BranchName}})
3. For each task in tasks.md: mark [~], push helix-specs, do the work, mark [x], push again
4. Before pushing code, merge the latest default branch into your feature branch in every repo that has changes:
   ` + "`cd /home/retro/work/{{.PrimaryRepoName}} && git fetch origin {{.BaseBranch}} && git merge origin/{{.BaseBranch}}`" + `
   Resolve any conflicts and commit before pushing.
5. When all tasks done, push code: ` + "`git push origin {{.BranchName}}`" + `
6. **Do NOT create pull requests yourself** (no ` + "`gh pr create`" + `, no GitHub MCP tools). Pushing to the branch is sufficient. The Helix platform creates the GitHub PR automatically when the user clicks "Open PR" in the UI.

## How Pushing Works (Read This Before Debugging Any Push Failure)

` + "`origin`" + ` in every repo under ` + "`/home/retro/work/`" + ` points at the Helix-hosted intermediate git server over HTTPS. Credentials come from ` + "`~/.git-credentials`" + ` via the ` + "`store`" + ` credential helper. There is no SSH, no SSH agent, no SSH keys, and no GitHub CLI in this environment. The Helix API relays your pushes to the external GitHub/GitLab/ADO repo using the OAuth credential the user configured.

**Do NOT run any of these to "debug" a push failure:** ` + "`gh`" + ` (any subcommand), ` + "`ssh-add`" + `, ` + "`ssh-keygen`" + `, ` + "`ssh-agent`" + `, ` + "`eval $(ssh-agent)`" + `, or anything that touches ` + "`~/.ssh/`" + `. None of those tools or files exist or apply here. Running them is a waste of turns and produces misleading output.

If ` + "`git push`" + ` fails: paste the full verbatim stderr to the user in the chat, then stop. Do not guess at the cause, do not invent "SSH authentication issues" or "credential" explanations, and do not retry with alternative tools. The user (or a Helix engineer) will diagnose the underlying issue from the actual error message.

{{if .KoditSection}}
{{.KoditSection}}
{{end}}## Visual Testing & Screenshots (For UI/Frontend Tasks)

You can test your UI changes and capture screenshots as proof of work:

**Browser automation:** ` + "`chrome-devtools`" + ` MCP server
- Navigate pages, click elements, fill forms, run the app

**Screenshots:** ` + "`helix-desktop`" + ` MCP server
- ` + "`list_windows`" + ` - Find browser window ID
- ` + "`focus_window`" + ` - Bring window to front (REQUIRED before screenshot)
- ` + "`save_screenshot`" + ` - Save to file

**Screenshot workflow:**
1. Run the app / open in browser
2. Navigate to the page you changed
3. ` + "`list_windows`" + ` → find browser window ID
4. ` + "`focus_window`" + ` → bring to front (window must be visible!)
5. ` + "`save_screenshot`" + ` with path: ` + "`/home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/screenshots/01-description.png`" + `

**Naming convention:** ` + "`01-feature-before.png`" + `, ` + "`02-feature-after.png`" + `, ` + "`03-error-state.png`" + `

**After taking screenshots:**
- Reference them in design.md or create screenshots.md
- Mention them in your PR description
- They serve as visual proof that your implementation works

Screenshots are optional but valuable for UI work - they help reviewers see what changed.

## Web Search

You can use the ` + "`chrome-devtools`" + ` MCP server to search the web via DuckDuckGo. Navigate to ` + "`https://duckduckgo.com`" + `, type your query, and read the results. Use this to look up documentation, APIs, or solutions.

## Don't Over-Engineer

- "Start a container" → docker-compose.yaml, NOT a Python wrapper
- "Create sample data" → write files directly, NOT a generator script
- "Run X at startup" → /home/retro/work/helix-specs/.helix/startup.sh (idempotent), NOT a service framework
- If it can be a one-liner, use a one-liner

## Update Design Docs As You Go (CRITICAL)

**Your design docs will be used to clone this task to similar projects.** Future agents will
read your notes to skip the discovery process. Write down everything you learn!

**What to capture in design.md:**

1. **User-specified values** - Any explicit choices made
   - "User specified primary color: #3B82F6"
   - "User confirmed: use PostgreSQL, not SQLite"

2. **Implementation approach** - How you solved it
   - Which files you modified and why
   - The pattern/architecture you used
   - The order of changes that worked

3. **Discovery learnings** - What you figured out
   - "Tried X, but Y worked better because..."
   - "This codebase uses pattern Z, so we adapted by..."
   - "Watch out for edge case: ..."

4. **Gotchas and blockers** - Problems and solutions
   - "Blocker: config parser doesn't support X, used workaround Y"
   - "Note: must restart service after changing Z"

**Also update:**
- **requirements.md** - Clarifications or discovered constraints
- **tasks.md** - Add new tasks, remove unnecessary ones, reorder as needed

Push to helix-specs after every update so the record is saved.

Example addition to design.md:
` + "```markdown" + `
## Implementation Notes

- Found existing utility AuthHelper, reusing instead of building new
- User specified: JWT expiry = 24 hours
- Chose middleware approach over decorator because this codebase uses Express patterns
- Gotcha: the config loader caches values, must call config.reload() after changes
- Added task: also need to update the logout endpoint to invalidate tokens
` + "```" + `

Don't treat the original plan as fixed - update it based on what you learn.

## Pull Request Description (IMPORTANT)

Before you finish, create PR description files in your task directory. These will be used as the PR title and description when pull requests are created.
{{if .NonPrimaryRepoNames}}
**This is a multi-repo project.** Create a separate PR description for EACH repository, describing only the changes in that repo:

- ` + "`pull_request_{{.PrimaryRepoName}}.md`" + ` — for the primary repo
{{- range .NonPrimaryRepoNames}}
- ` + "`pull_request_{{.}}.md`" + ` — for {{.}}
{{- end}}

You may also create a generic ` + "`pull_request.md`" + ` as a fallback for any repo without its own file.

` + "```bash" + `
# Example: write per-repo PR descriptions
cat > /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/pull_request_{{.PrimaryRepoName}}.md << 'EOF'
# PR title for {{.PrimaryRepoName}}

## Summary
What changed in {{.PrimaryRepoName}} and why.

## Changes
- Key change 1
- Key change 2
EOF
{{range .NonPrimaryRepoNames}}
cat > /home/retro/work/helix-specs/design/tasks/{{$.TaskDirName}}/pull_request_{{.}}.md << 'EOF'
# PR title for {{.}}

## Summary
What changed in {{.}} and why.

## Changes
- Key change 1
- Key change 2
EOF
{{end}}
cd /home/retro/work/helix-specs && git add -A && git commit -m "docs(specs): add PR descriptions" && git push origin helix-specs
` + "```" + `
{{else}}
` + "```bash" + `
cat > /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/pull_request.md << 'EOF'
# Clear, concise PR title (50 chars or less)

## Summary
Brief description of what this PR does and why.

## Changes
- Key change 1
- Key change 2

## Testing
How this was tested (if applicable).
EOF
cd /home/retro/work/helix-specs && git add -A && git commit -m "docs(specs): add PR description" && git push origin helix-specs
` + "```" + `
{{end}}
**Tips for good PR descriptions:**
- Title should be imperative ("Add feature" not "Added feature")
- Summary explains the "what" and "why"
- Changes list the key modifications
- Keep it concise - reviewers appreciate brevity
- **For UI/frontend changes:** Include a "Screenshots" section in the PR description. Save screenshots to the ` + "`screenshots/`" + ` folder in your task directory — they get pushed to the helix-specs branch.{{if .ScreenshotBaseURL}} Use markdown image syntax so they render inline. The URL pattern for this repo is:
  ` + "`" + `{{.ScreenshotBaseURL}}` + "`" + `
  Replace ` + "`SCREENSHOT_FILENAME`" + ` with the actual filename (e.g. ` + "`01-before.png`" + `). Example:
  ` + "```" + `
  ## Screenshots
  ![Feature OFF](url-with-01-off.png)
  ![Feature ON](url-with-02-on.png)
  ` + "```" + `{{else}} Reference them by path:
  ` + "```" + `
  ## Screenshots
  See screenshots in helix-specs branch: design/tasks/{{.TaskDirName}}/screenshots/
  ` + "```" + `{{end}}

---

**Task:** {{.TaskName}}
**Feature Branch:** {{.BranchName}} (base: {{.BaseBranch}})
**Design Docs:** /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/
{{if .ApprovalComments}}
## Reviewer Comments

The reviewer included these comments when approving the design:

{{.ApprovalComments}}

Take these comments into account during implementation.
{{end}}{{if .OriginalPromptSection}}
{{.OriginalPromptSection}}
{{end}}
**Primary Project Directory:** /home/retro/work/{{.PrimaryRepoName}}/
`))

var commentPromptTemplate = template.Must(template.New("comment").Parse(`# Review Comment

Speak English.

**Document:** {{.DocumentLabel}}
{{if .SectionPath}}**Section:** {{.SectionPath}}
{{end}}{{if gt .LineNumber 0}}**Line:** {{.LineNumber}}
{{end}}
{{if .QuotedText}}> {{.QuotedText}}

{{end}}**Comment:** {{.CommentText}}

---

If changes are needed, update /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/ and push:
` + "```bash" + `
cd /home/retro/work/helix-specs && git add -A && git commit -m "docs(specs): address feedback" && git push origin helix-specs
` + "```" + `
`))

var revisionPromptTemplate = template.Must(template.New("revision").Parse(`# Changes Requested

Speak English.

Update your design based on this feedback:

{{.Comments}}

---

**Your docs are in:** /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/

After updating, push immediately:
` + "```bash" + `
cd /home/retro/work/helix-specs && git add -A && git commit -m "docs(specs): address feedback" && git push origin helix-specs
` + "```" + `
`))

// =============================================================================
// Prompt Builder Functions
// =============================================================================

// BuildApprovalInstructionPrompt builds the approval instruction prompt for an agent
// This is the single source of truth for this prompt - used by WebSocket and database approaches
// guidelines contains concatenated organization + project guidelines (can be empty)
// primaryRepoName is the name of the primary project repository (e.g., "my-app")
// repoSection is the pre-built repository access section (from BuildRepositorySection)
func BuildApprovalInstructionPrompt(task *types.SpecTask, branchName, baseBranch, guidelines, primaryRepoName, koditSection, repoSection string, nonPrimaryRepoNames []string, screenshotBaseURL string) string {
	taskDirName := GetTaskDirName(task)

	// Build guidelines section if provided
	guidelinesSection := ""
	if guidelines != "" {
		guidelinesSection = `
## Guidelines

Follow these guidelines when implementing:

` + guidelines + `

---
`
	}

	// Build cloned task preamble if this task was cloned from another
	clonedTaskPreamble := ""
	if task.ClonedFromID != "" {
		clonedTaskPreamble = `

## CLONED TASK - User-Provided Values Already Known

This task was cloned from a completed task. The specs contain values discovered through user interaction.

**CRITICAL: Use Discovered Values Directly**

Look in design.md for phrases like "User specified...", "User confirmed...", or specific values
(hex codes, URLs, etc.). These values override any generic instructions in the task description.

If the task says "ask the user for X" but design.md already has "User specified X = Y" → use Y directly.
The whole point of cloning is to SKIP re-asking questions that were already answered.

**Before implementing:**

1. **Read design.md carefully** - Extract all user-specified values and use them directly
2. **Reset tasks.md** - Change [x] back to [ ], remove/add tasks as needed for this repo
3. **Adapt paths for this repository** - File paths may differ, but user-specified values stay the same

`
	}

	// Format original prompt section - for cloned tasks, reframe as historical context
	// Only include original prompt for cloned tasks (where the agent hasn't seen it before)
	// For normal tasks, the agent already has the original prompt from the planning phase
	var originalPromptSection string
	if task.ClonedFromID != "" {
		originalPromptSection = "**Original Request (for context only - any questions have already been resolved in the specs):**\n> \"" + task.OriginalPrompt + "\""
	}

	// Extract approval comments from the spec approval if available
	var approvalComments string
	if task.SpecApproval != nil && task.SpecApproval.Comments != "" {
		approvalComments = task.SpecApproval.Comments
	}

	data := ApprovalPromptData{
		Guidelines:            guidelinesSection,
		KoditSection:          koditSection,
		RepositorySection:     repoSection,
		PrimaryRepoName:       primaryRepoName,
		NonPrimaryRepoNames:   nonPrimaryRepoNames,
		TaskDirName:           taskDirName,
		BranchName:            branchName,
		BaseBranch:            baseBranch,
		TaskName:              task.Name,
		OriginalPromptSection: originalPromptSection,
		ClonedTaskPreamble:    clonedTaskPreamble,
		ApprovalComments:      approvalComments,
		ScreenshotBaseURL:     screenshotBaseURL,
	}

	var buf bytes.Buffer
	if err := approvalPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating approval prompt: " + err.Error()
	}
	return buf.String()
}

// BuildCommentPrompt builds a prompt for sending a design review comment to an agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildCommentPrompt(specTask *types.SpecTask, comment *types.SpecTaskDesignReviewComment) string {
	taskDirName := GetTaskDirName(specTask)

	// Map document types to readable labels
	documentTypeLabels := map[string]string{
		"requirements":        "Requirements (requirements.md)",
		"technical_design":    "Technical Design (design.md)",
		"implementation_plan": "Implementation Plan (tasks.md)",
	}
	docLabel := documentTypeLabels[comment.DocumentType]
	if docLabel == "" {
		docLabel = comment.DocumentType
	}

	data := CommentPromptData{
		DocumentLabel: docLabel,
		SectionPath:   comment.SectionPath,
		LineNumber:    comment.LineNumber,
		QuotedText:    comment.QuotedText,
		CommentText:   comment.CommentText,
		TaskDirName:   taskDirName,
	}

	var buf bytes.Buffer
	if err := commentPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating comment prompt: " + err.Error()
	}
	return buf.String()
}

// BuildRevisionInstructionPrompt builds the prompt for sending revision feedback to the agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildRevisionInstructionPrompt(task *types.SpecTask, comments string) string {
	taskDirName := GetTaskDirName(task)

	data := RevisionPromptData{
		TaskDirName: taskDirName,
		Comments:    comments,
	}

	var buf bytes.Buffer
	if err := revisionPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating revision prompt: " + err.Error()
	}
	return buf.String()
}

// =============================================================================
// Service Methods (Database Interaction)
// =============================================================================

// SendApprovalInstruction sends a message to the agent to start implementation
// NOTE: This creates a database interaction - for WebSocket-connected agents, use BuildApprovalInstructionPrompt
// and send via sendChatMessageToExternalAgent instead
func (s *AgentInstructionService) SendApprovalInstruction(
	ctx context.Context,
	sessionID string,
	userID string,
	task *types.SpecTask,
	branchName string,
	baseBranch string,
	primaryRepoName string,
) error {
	// Fetch guidelines from project and organization
	guidelines, project := s.getGuidelinesForTask(ctx, task)
	koditDoc := ""
	if project != nil && project.KoditEnabled {
		koditDoc = s.koditService.MCPDocumentation()
	}

	// Build repository section
	repoSection := s.buildRepositorySectionForTask(ctx, task, project)

	// Gather non-primary repo names and find primary repo for screenshot URLs
	var nonPrimaryRepoNames []string
	var screenshotBaseURL string
	taskDirName := GetTaskDirName(task)
	if task.ProjectID != "" {
		projectRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			ProjectID: task.ProjectID,
		})
		if err == nil {
			for _, repo := range projectRepos {
				if repo.Name == primaryRepoName && repo.ExternalURL != "" {
					screenshotBaseURL = GetRawScreenshotBaseURL(repo, taskDirName)
				} else if repo.Name != primaryRepoName && repo.ExternalURL != "" {
					nonPrimaryRepoNames = append(nonPrimaryRepoNames, repo.Name)
				}
			}
		}
	}

	message := BuildApprovalInstructionPrompt(task, branchName, baseBranch, guidelines, primaryRepoName, koditDoc, repoSection, nonPrimaryRepoNames, screenshotBaseURL)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending approval instruction to agent")

	// Enqueue onto the session-scoped prompt queue. interrupt=false: approval
	// kickoff begins a new phase and should respect the queue (defer until idle).
	// userID is carried as notifyUserID so the response streams to the same user
	// as before.
	if err := s.enqueuer(ctx, task, message, false, userID); err != nil {
		return fmt.Errorf("failed to enqueue approval instruction to agent: %w", err)
	}

	return nil
}

// getGuidelinesForTask fetches concatenated organization/user + project guidelines
func (s *AgentInstructionService) getGuidelinesForTask(ctx context.Context, task *types.SpecTask) (string, *types.Project) {
	if task.ProjectID == "" {
		return "", nil
	}

	project, err := s.store.GetProject(ctx, task.ProjectID)
	if err != nil || project == nil {
		return "", nil
	}

	guidelines := ""

	// Get organization guidelines (if project belongs to an org)
	if project.OrganizationID != "" {
		org, err := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: project.OrganizationID})
		if err == nil && org != nil && org.Guidelines != "" {
			guidelines = org.Guidelines
		}
	} else if project.UserID != "" {
		// No organization - check for user guidelines (personal workspace)
		userMeta, err := s.store.GetUserMeta(ctx, project.UserID)
		if err == nil && userMeta != nil && userMeta.Guidelines != "" {
			guidelines = userMeta.Guidelines
		}
	}

	// Append project guidelines
	if project.Guidelines != "" {
		if guidelines != "" {
			guidelines += "\n\n---\n\n"
		}
		guidelines += project.Guidelines
	}

	return guidelines, project
}

// buildRepositorySectionForTask fetches project and org repos, then builds the repository section
func (s *AgentInstructionService) buildRepositorySectionForTask(ctx context.Context, task *types.SpecTask, project *types.Project) string {
	if task.ProjectID == "" {
		return ""
	}

	// Fetch project repos
	projectRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		return ""
	}

	// Fetch Kodit org repos if enabled
	var koditOrgRepos []*types.GitRepository
	if project != nil && project.KoditEnabled && project.OrganizationID != "" {
		orgRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			OrganizationID: project.OrganizationID,
		})
		if err == nil {
			for _, repo := range orgRepos {
				if repo.KoditIndexing {
					koditOrgRepos = append(koditOrgRepos, repo)
				}
			}
		}
	}

	primaryRepoID := ""
	if project != nil {
		primaryRepoID = project.DefaultRepoID
	}

	return BuildRepositorySection(projectRepos, koditOrgRepos, primaryRepoID)
}

