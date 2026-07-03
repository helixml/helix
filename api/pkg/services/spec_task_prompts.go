package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/helixml/helix/api/pkg/types"
)

// PlanningPromptData contains all data needed for the planning prompt template
type PlanningPromptData struct {
	Guidelines         string // Formatted guidelines section (includes header if non-empty)
	KoditSection       string // Dynamic MCP tool documentation from kodit (empty when disabled)
	RepositorySection  string // Available repositories section (local + Kodit repos)
	AttachmentsSection string // User-attached files section (empty when no attachments)
	TaskDirName        string // Directory name for task (e.g., "0042-add-dark-mode")
	ProjectID          string
	TaskType           string
	Priority           string
	TaskName           string // Human-readable task name for commit message
	ClonedTaskPreamble string // Special instructions for cloned tasks (non-empty if task was cloned)
}

// planningPromptTemplate is the compiled template for planning prompts
var planningPromptTemplate = template.Must(template.New("planning").Parse(`## CURRENT PHASE: PLANNING/SPEC-WRITING

You are in the PLANNING phase. You are NOT in the implementation phase.
- Your job now: Write specification documents (requirements.md, design.md, tasks.md)
- Do NOT implement anything yet - no code changes, no file edits to the codebase
- Implementation begins LATER, after this phase is complete and the user approves your plan

---
{{.ClonedTaskPreamble}}
You are a software specification expert. Create SHORT, SIMPLE spec documents as Markdown files, then push to Git.

Speak English.
{{.Guidelines}}

## CRITICAL: Where To Work

ALL work happens in /home/retro/work/. No other paths.

- /home/retro/work/helix-specs/ = Your design docs go here (ALREADY EXISTS - don't create it)
- /home/retro/work/<repo>/ = Code repos (don't touch these - implementation happens later)
{{.RepositorySection}}
{{.AttachmentsSection}}## Your Task Directory

Create exactly 3 files in /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/ (directory already exists):
1. requirements.md - User stories + acceptance criteria
2. design.md - Architecture + key decisions
3. tasks.md - Checklist of implementation tasks using [ ] format

## Open Questions (requirements.md)

End ` + "`requirements.md`" + ` with an ` + "`## Open Questions`" + ` section listing any genuine
questions or uncertain assumptions you have for the user - anything you would
otherwise have to guess. This surfaces guesses so the user can correct them at
review time instead of them silently becoming the spec. Only list real
uncertainties; if there are none, write "None".

## CRITICAL: Title Format

Each of the three files MUST start with an H1 in this exact format:

- requirements.md → ` + "`# Requirements: <Descriptive Title>`" + `
- design.md       → ` + "`# Design: <Descriptive Title>`" + `
- tasks.md        → ` + "`# Implementation Tasks: <Descriptive Title>`" + `

Use the SAME ` + "`<Descriptive Title>`" + ` across all three files. The descriptive title
must summarise the actual subject of the task (e.g. "Add Dark Mode Toggle"),
not the type of document and not a section name.

GOOD: ` + "`# Requirements: Add Dark Mode Toggle to Settings Page`" + `
BAD:  ` + "`# Requirements`" + `              ← no descriptive title, breaks downstream naming
BAD:  ` + "`# Background`" + `                ← wrong document-type prefix
BAD:  ` + "`# Requirements: Background`" + `  ← describes the section, not the task

**Why this matters:** the title from ` + "`requirements.md`" + ` is parsed and used
for the spec-task name displayed in the UI and for the eventual git feature
branch name. A missing or generic title produces a meaningless branch name
(e.g. ` + "`feature/NNNNNN-background`" + `) and a meaningless task in the UI.

## CRITICAL: Don't Over-Engineer

Match solution complexity to task complexity:
- "Start a container" → docker-compose.yaml, NOT a Python wrapper
- "Create sample data" → write files directly, NOT a generator script
- Simple task = minimal docs (1-2 paragraphs per section)

## Git Workflow

` + "```bash" + `
cd /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}

# Create requirements.md, design.md, tasks.md here

cd /home/retro/work/helix-specs
git add -A && git commit -m "Design docs for {{.TaskName}}" && git push origin helix-specs
` + "```" + `

If push fails (another agent pushed first):
` + "```bash" + `
git pull origin helix-specs --rebase && git push origin helix-specs
` + "```" + `

## tasks.md Format

` + "```markdown" + `
# Implementation Tasks: <Descriptive Title>

- [ ] First task
- [ ] Second task
- [ ] Third task
` + "```" + `

{{if .KoditSection}}
{{.KoditSection}}
{{end}}## Visual Testing (Optional - For UI/Frontend Tasks)

You have tools to explore and screenshot the application during planning:

**Browser automation:** ` + "`chrome-devtools`" + ` MCP server
- Navigate pages, click elements, fill forms
- Useful for understanding current UI behavior

**Screenshots:** ` + "`helix-desktop`" + ` MCP server
- ` + "`list_windows`" + ` - Find browser window ID
- ` + "`focus_window`" + ` - Bring window to front (REQUIRED before screenshot)
- ` + "`save_screenshot`" + ` - Save to file

**When to use:** Understanding existing UI, documenting current state, exploring edge cases.

**Screenshot workflow:**
1. Open the app in browser (if applicable)
2. ` + "`list_windows`" + ` → find browser window ID
3. ` + "`focus_window`" + ` → bring to front
4. ` + "`save_screenshot`" + ` with path: ` + "`/home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/screenshots/01-description.png`" + `

Screenshots are optional but valuable for UI tasks - save them in your task's screenshots/ folder.

## Web Search

You can use the ` + "`chrome-devtools`" + ` MCP server to search the web via DuckDuckGo. Navigate to ` + "`https://duckduckgo.com`" + `, type your query, and read the results. Use this to look up documentation, APIs, or solutions.

## Startup Script

The project startup script (installs deps, starts dev servers) runs automatically at session start:
- **Location:** ` + "`/home/retro/work/helix-specs/.helix/startup.sh`" + `
- **Log:** ` + "`cat /tmp/helix-startup.log`" + ` (written when the script runs at startup)

If the startup script hasn't run yet, the log won't exist. You can re-run it manually: ` + "`bash /home/retro/work/helix-specs/.helix/startup.sh`" + `

## Document Your Learnings

**Your design docs may be cloned to similar projects.** Write down what you discover:

- Patterns you found in the codebase ("This project uses X pattern for Y")
- Decisions and rationale ("Chose A over B because...")
- Things you learned from code searches ("Found existing utility Z in repo W")
- Constraints or gotchas you identified ("Note: this codebase requires X")

Future agents implementing similar tasks will read your notes and skip the discovery process.

## After Pushing

Tell the user the design is ready for review. The backend detects your push and moves the task to review status.

---

**Project ID:** {{.ProjectID}} | **Type:** {{.TaskType}} | **Priority:** {{.Priority}}
`))

// BuildAttachmentsSection builds a markdown section pointing the agent at user-uploaded
// attachment files for the task. Files are checked into the helix-specs branch under
// design/tasks/<taskDirName>/attachments/, so they appear in the agent's workspace at
// /home/retro/work/helix-specs/design/tasks/<taskDirName>/attachments/.
// Returns empty string when there are no attachments (so the planning prompt stays clean).
func BuildAttachmentsSection(attachments []*types.SpecTaskAttachment, taskDirName string) string {
	if len(attachments) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Attachments\n\n")
	b.WriteString(fmt.Sprintf("The user attached %d file(s) for context. They are in your workspace at:\n\n", len(attachments)))
	for _, a := range attachments {
		path := fmt.Sprintf("/home/retro/work/helix-specs/design/tasks/%s/attachments/%s", taskDirName, a.Filename)
		line := fmt.Sprintf("- `%s` (%s, %s)", path, a.MimeType, humanSize(a.SizeBytes))
		if a.Caption != "" {
			line += fmt.Sprintf(" — %q", a.Caption)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n**Read or view them BEFORE asking clarifying questions.** They are evidence of the bug or feature, not decoration. For images, the Read tool can view PNG/JPG/GIF/WebP directly.\n\n")
	return b.String()
}

// humanSize renders a byte count compactly: "248 KB", "1.2 MB".
func humanSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * 1024
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%d KB", n/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// BuildPlanningPrompt creates the planning phase prompt for the Zed agent.
// This is the canonical planning prompt - used by both:
// - SpecDrivenTaskService.StartSpecGeneration (explicit user action)
// - SpecTaskOrchestrator.handleBacklog (auto-start when enabled)
// guidelines contains concatenated organization + project guidelines (can be empty)
// repoSection is the pre-built repository access section (from BuildRepositorySection)
// attachmentsSection is the pre-built attachments section (from BuildAttachmentsSection)
func BuildPlanningPrompt(task *types.SpecTask, guidelines, koditSection, repoSection, attachmentsSection string) string {
	// Use DesignDocPath if set (new human-readable format), fall back to task ID
	taskDirName := task.DesignDocPath
	if taskDirName == "" {
		taskDirName = task.ID // Backwards compatibility for old tasks
	}

	// Build guidelines section if provided
	guidelinesSection := ""
	if guidelines != "" {
		guidelinesSection = `
## Guidelines

Follow these guidelines when creating specifications:

` + guidelines + `

---

`
	}

	// Build cloned task preamble if this task was cloned from another
	clonedTaskPreamble := ""
	if task.ClonedFromID != "" {
		clonedTaskPreamble = `
## CLONED TASK - Transfer Implementation Knowledge

This task was cloned from a completed task in another (similar) project. The design docs
contain everything learned during the original implementation - use this knowledge directly.

**What the design docs contain:**

1. **User-specified values** - Explicit choices made during the original task
   - Look for: "User specified...", "User confirmed...", specific values (hex codes, URLs, etc.)
   - Use these values directly - don't re-ask questions that were already answered

2. **Implementation approach** - How the original task was solved
   - The architecture/pattern that worked
   - Which files were modified and why
   - The order of changes that made sense

3. **Discovery learnings** - Things figured out during implementation
   - "We tried X, but Y worked better because..."
   - "This codebase uses pattern Z, so we..."
   - Gotchas, edge cases, and workarounds discovered

4. **Working solution** - The actual implementation that succeeded
   - This is a proven approach for similar codebases
   - Adapt file paths, but keep the core approach

**BEFORE creating new specs, you MUST:**

1. **Read the existing design docs carefully** at /home/retro/work/helix-specs/design/tasks/` + taskDirName + `/
   - requirements.md, design.md, and tasks.md are already populated
   - They contain the complete knowledge from the original implementation
   - This is your guide - you're adapting a working solution, not starting fresh

2. **Adapt for this repository:**
   - Update file paths and structure for the target codebase
   - Verify naming conventions match this project
   - The core approach stays the same, only repo-specific details change
   - Keep all user-specified values unchanged (they apply across cloned tasks)

3. **Reset tasks.md:**
   - All checkboxes may be marked [x] complete from the original task
   - Change [x] back to [ ] (unchecked) for tasks that need to be done
   - REMOVE tasks that don't apply to this repository
   - ADD new tasks if this repo needs different work

4. **Push the adapted specs:**
   - Even if you make minimal changes, you MUST git add, commit, and push
   - The push is what signals to Helix that specs are ready for review

---

`
	}

	data := PlanningPromptData{
		Guidelines:         guidelinesSection,
		KoditSection:       koditSection,
		RepositorySection:  repoSection,
		AttachmentsSection: attachmentsSection,
		ClonedTaskPreamble: clonedTaskPreamble,
		TaskDirName:        taskDirName,
		ProjectID:          task.ProjectID,
		TaskType:           string(task.Type),
		Priority:           string(task.Priority),
		TaskName:           task.Name,
	}

	var buf bytes.Buffer
	if err := planningPromptTemplate.Execute(&buf, data); err != nil {
		// Fallback to a simple error message if template fails
		return "Error generating planning prompt: " + err.Error()
	}
	return buf.String()
}

// BuildRepositorySection builds a markdown section listing available repositories
// for injection into spec task prompts. Returns empty string when both lists are empty.
//
// projectRepos: repos cloned locally (from store.ListGitRepositories by ProjectID)
// koditOrgRepos: org repos with KoditIndexing=true (from store.ListGitRepositories by OrgID, pre-filtered)
// primaryRepoID: the project's DefaultRepoID (to mark primary)
func BuildRepositorySection(projectRepos []*types.GitRepository, koditOrgRepos []*types.GitRepository, primaryRepoID string) string {
	if len(projectRepos) == 0 && len(koditOrgRepos) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Repositories\n\n")

	// Local repositories (read/write)
	if len(projectRepos) > 0 {
		b.WriteString("### Local Repositories (Read/Write)\n")
		b.WriteString("These are cloned locally. You can read, edit, and commit.\n")
		for _, repo := range projectRepos {
			isPrimary := repo.ID == primaryRepoID
			if isPrimary {
				b.WriteString(fmt.Sprintf("- `%s` at `/home/retro/work/%s/` <- primary project\n", repo.Name, repo.Name))
			} else {
				b.WriteString(fmt.Sprintf("- `%s` at `/home/retro/work/%s/`\n", repo.Name, repo.Name))
			}
		}
		b.WriteString("\n")
	}

	// Build set of local repo IDs to exclude from Kodit list
	localRepoIDs := make(map[string]bool, len(projectRepos))
	for _, repo := range projectRepos {
		localRepoIDs[repo.ID] = true
	}

	// Kodit repositories (read-only via Kodit MCP)
	var koditRepos []*types.GitRepository
	for _, repo := range koditOrgRepos {
		if localRepoIDs[repo.ID] {
			continue
		}
		koditID := extractKoditRepoIDFromMetadata(repo.Metadata)
		if koditID <= 0 {
			continue
		}
		koditRepos = append(koditRepos, repo)
	}

	if len(koditRepos) > 0 {
		b.WriteString("### Kodit Repositories (Read-Only via Kodit MCP)\n")
		b.WriteString("These are accessible read-only through the Kodit MCP tools (semantic_search, keyword_search, grep, list_files, read_file).\n")
		for _, repo := range koditRepos {
			koditID := extractKoditRepoIDFromMetadata(repo.Metadata)
			b.WriteString(fmt.Sprintf("- `%s` (Kodit repo ID: %d)\n", repo.Name, koditID))
		}
		b.WriteString("\n")
	}

	b.WriteString("**IMPORTANT:** Only directories listed under \"Local Repositories\" exist on this machine.\n")
	b.WriteString("Do NOT attempt to access repo directories that are not listed. Use Kodit MCP tools for Kodit repositories.\n\n")

	return b.String()
}

// extractKoditRepoIDFromMetadata extracts the kodit_repo_id from metadata,
// handling int64, float64, int, string, and json.Number formats.
// Same logic as extractKoditRepoID in server/kodit_handlers.go but in the services package.
func extractKoditRepoIDFromMetadata(metadata map[string]interface{}) int64 {
	raw, ok := metadata["kodit_repo_id"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	case json.Number:
		if id, err := v.Int64(); err == nil {
			return id
		}
	}
	return 0
}
