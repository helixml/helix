package services

import (
	"bytes"
	"text/template"

	"github.com/helixml/helix/api/pkg/types"
)

// PlanningPromptData contains all data needed for the planning prompt template
type PlanningPromptData struct {
	Guidelines         string // Formatted guidelines section (includes header if non-empty)
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

## Your Task Directory

Create exactly 3 files in /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/ (directory already exists):
1. requirements.md - User stories + acceptance criteria
2. design.md - Architecture + key decisions
3. tasks.md - Checklist of implementation tasks using [ ] format

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
# Implementation Tasks

- [ ] First task
- [ ] Second task
- [ ] Third task
` + "```" + `

## After Pushing

Tell the user the design is ready for review. The backend detects your push and moves the task to review status.

---

**Project ID:** {{.ProjectID}} | **Type:** {{.TaskType}} | **Priority:** {{.Priority}}
`))

// BuildPlanningPrompt creates the planning phase prompt for the Zed agent.
// This is the canonical planning prompt - used by both:
// - SpecDrivenTaskService.StartSpecGeneration (explicit user action)
// - SpecTaskOrchestrator.handleBacklog (auto-start when enabled)
// guidelines contains concatenated organization + project guidelines (can be empty)
// primaryRepoName is the name of the primary code repository (e.g., "my-app")
func BuildPlanningPrompt(task *types.SpecTask, guidelines string) string {
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
## CLONED TASK - Adapt Existing Specs

This task was cloned from a completed task in another project. Design docs already exist.

**CRITICAL: User-Provided Values Transfer - Don't Re-Ask**

The original task may have discovered information through user interaction. Look for phrases like:
- "User specified...", "User confirmed...", "User chose..."
- Specific values like hex codes, URLs, API keys, version numbers
- Decisions marked as "confirmed" or "approved"

**These discovered values override generic instructions in the task description.** For example:
- If the task says "ask the user for X" but design.md already has "User specified X = Y" → use Y directly
- If the task says "determine the API endpoint" but design.md has "Using endpoint: https://..." → use that endpoint
- The whole point of cloning is to SKIP the discovery process that already happened

**BEFORE creating new specs, you MUST:**

1. **Read the existing design docs** at /home/retro/work/helix-specs/design/tasks/` + taskDirName + `/
   - requirements.md, design.md, and tasks.md are already populated
   - They contain learnings from the original implementation
   - **Preserve all user-specified values** - copy them exactly to your adapted specs

2. **Adapt for this repository:**
   - Update file paths and structure for the target codebase
   - Verify naming conventions match this project
   - Update repo-specific references
   - **Keep user-specified values unchanged** (they apply to all cloned tasks)

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
