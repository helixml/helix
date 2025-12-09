package services

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// BuildPlanningPrompt creates the planning phase prompt for the Zed agent.
// This is the canonical planning prompt - used by both:
// - SpecDrivenTaskService.StartSpecGeneration (explicit user action)
// - SpecTaskOrchestrator.handleBacklog (auto-start when enabled)
// guidelines contains concatenated organization + project guidelines (can be empty)
func BuildPlanningPrompt(task *types.SpecTask, guidelines string) string {
	// Use DesignDocPath if set (new human-readable format), fall back to task ID
	taskDirName := task.DesignDocPath
	if taskDirName == "" {
		taskDirName = task.ID // Backwards compatibility for old tasks
	}

	// Build guidelines section if provided
	guidelinesSection := ""
	if guidelines != "" {
		guidelinesSection = fmt.Sprintf(`
## Guidelines

Follow these guidelines when creating specifications:

%s

---

`, guidelines)
	}

	return fmt.Sprintf(`You are a software specification expert. Create SHORT, SIMPLE spec documents as Markdown files, then push to Git.

Speak English.
%[9]s

## CRITICAL: Where To Work

ALL work happens in /home/retro/work/. No other paths.

- /home/retro/work/helix-specs/ = Your design docs go here (ALREADY EXISTS - don't create it)
- /home/retro/work/<repo>/ = Code repos (don't touch these - implementation happens later)

Your task directory: /home/retro/work/helix-specs/design/tasks/%[5]s/

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

%[1]sbash
cd /home/retro/work/helix-specs
mkdir -p design/tasks/%[5]s
cd design/tasks/%[5]s

# Create requirements.md, design.md, tasks.md here

cd /home/retro/work/helix-specs
git add -A && git commit -m "Design docs for %[8]s" && git push origin helix-specs
%[1]s

If push fails (another agent pushed first):
%[1]sbash
git pull origin helix-specs --rebase && git push origin helix-specs
%[1]s

## tasks.md Format

%[1]smarkdown
# Implementation Tasks

- [ ] First task
- [ ] Second task
- [ ] Third task
%[1]s

## After Pushing

Tell the user the design is ready for review. The backend detects your push and moves the task to review status.

---

**Project ID:** %[2]s | **Type:** %[3]s | **Priority:** %[4]s | **SpecTask ID:** %[8]s
`,
		"```",
		task.ProjectID, task.Type, task.Priority, // [2], [3], [4]
		taskDirName,                              // [5] - directory name
		"", "", "",                               // [6], [7] unused
		task.ID,                                  // [8] - task ID
		guidelinesSection)                        // [9] - guidelines
}
