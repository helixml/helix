package services

import (
	"bytes"
	"text/template"

	"github.com/helixml/helix/api/pkg/types"
)

// PlanningPromptData contains all data needed for the planning prompt template
type PlanningPromptData struct {
	Guidelines      string // Formatted guidelines section (includes header if non-empty)
	TaskDirName     string // Directory name for task (e.g., "0042-add-dark-mode")
	ProjectID       string
	TaskType        string
	Priority        string
	TaskName        string // Human-readable task name for commit message
	PrimaryRepoName string // Name of the primary code repository
}

// planningPromptTemplate is the compiled template for planning prompts
var planningPromptTemplate = template.Must(template.New("planning").Parse(`You are a software specification expert. Create SHORT, SIMPLE spec documents as Markdown files, then push to Git.

Speak English.
{{.Guidelines}}

## CRITICAL: Where To Work

ALL work happens in /home/retro/work/. No other paths.

- /home/retro/work/helix-specs/ = Your design docs go here (ALREADY EXISTS - don't create it)
- /home/retro/work/<repo>/ = Code repos (don't touch these - implementation happens later)

Your task directory: /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/

## CRITICAL: What To Create

Create these 3 files in your task directory:
1. requirements.md - User stories + acceptance criteria
2. design.md - Architecture + key decisions
3. tasks.md - Checklist of implementation tasks using [ ] format

## CRITICAL: Don't Over-Engineer

Match solution complexity to task complexity:
- "Start a container" → docker-compose.yaml, NOT a Python wrapper
- "Create sample data" → write files directly, NOT a generator script
- Simple task = minimal docs (1-2 paragraphs per section)

## Git Workflow

**IMPORTANT:** Always include a Code-Ref in your commit messages to link specs to code versions.

` + "```bash" + `
cd /home/retro/work/helix-specs
mkdir -p design/tasks/{{.TaskDirName}}
cd design/tasks/{{.TaskDirName}}

# Create requirements.md, design.md, tasks.md here

# Get the code commit hash you're designing against (from the primary code repo)
cd /home/retro/work/{{.PrimaryRepoName}}
CODE_REF="$(git rev-parse --short HEAD)"
CODE_BRANCH="$(git branch --show-current)"

cd /home/retro/work/helix-specs
git add -A && git commit -m "Design docs for {{.TaskName}}

Code-Ref: {{.PrimaryRepoName}}/${CODE_BRANCH}@${CODE_REF}" && git push origin helix-specs
` + "```" + `

If push fails (another agent pushed first):
` + "```bash" + `
git pull origin helix-specs --rebase && git push origin helix-specs
` + "```" + `

The **Code-Ref** line is machine-parsable and links this spec version to the code state you analyzed.

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
func BuildPlanningPrompt(task *types.SpecTask, guidelines, primaryRepoName string) string {
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

	data := PlanningPromptData{
		Guidelines:      guidelinesSection,
		TaskDirName:     taskDirName,
		ProjectID:       task.ProjectID,
		TaskType:        string(task.Type),
		Priority:        string(task.Priority),
		TaskName:        task.Name,
		PrimaryRepoName: primaryRepoName,
	}

	var buf bytes.Buffer
	if err := planningPromptTemplate.Execute(&buf, data); err != nil {
		// Fallback to a simple error message if template fails
		return "Error generating planning prompt: " + err.Error()
	}
	return buf.String()
}
