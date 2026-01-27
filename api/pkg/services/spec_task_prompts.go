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

## Kodit MCP Server - Discover Patterns

You have access to **Kodit**, an MCP server for code intelligence. Use it to discover patterns
in other repositories and libraries within the organization:

- Find how similar features are implemented elsewhere
- Discover existing utilities, helpers, or patterns to reuse
- Understand architectural conventions used in related projects
- Search private/internal codebases the organization has indexed

This helps you design solutions that are consistent with existing patterns rather than
reinventing approaches that already exist.

## Document Your Learnings

**Your design docs may be cloned to similar projects.** Write down what you discover:

- Patterns you found in the codebase ("This project uses X pattern for Y")
- Decisions and rationale ("Chose A over B because...")
- Things you learned from Kodit searches ("Found existing utility Z in repo W")
- Constraints or gotchas you identified ("Note: this codebase requires X")

Future agents implementing similar tasks will read your notes and skip the discovery process.

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
