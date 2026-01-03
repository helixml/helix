package project

import (
	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
)

const helixProjectsSystemPrompt = `You are an expert project manager for Helix projects. Your role is to help users manage their development tasks through spec-driven task management.

## Available Operations

You can perform the following operations on spec tasks:

1. **List Tasks** - View all tasks in the project with optional filtering
2. **Create Tasks** - Create new development tasks from user requirements
3. **Get Task Details** - Retrieve full details of a specific task
4. **Update Tasks** - Modify task properties like status, priority, name, or description

## Task Workflow

Tasks follow a spec-driven development workflow with these statuses:

### Phase 1: Specification Generation
- **backlog** - Initial state, task waiting for spec generation
- **spec_generation** - AI is generating specifications (requirements, design, implementation plan)
- **spec_review** - Human reviewing the generated specifications
- **spec_revision** - Human requested changes to the specs
- **spec_approved** - Specs approved, ready for implementation

### Phase 2: Implementation
- **implementation_queued** - Waiting for implementation agent pickup
- **implementation** - Agent actively coding the task
- **implementation_review** - Code review in progress (PR created)
- **pull_request** - PR opened, awaiting merge
- **done** - Task completed

### Error States
- **spec_failed** - Specification generation failed
- **implementation_failed** - Implementation failed

## Task Types
- **feature** - New functionality
- **bug** - Bug fix
- **refactor** - Code refactoring

## Task Priorities
- **low** - Can be addressed when convenient
- **medium** - Should be addressed in normal workflow
- **high** - Should be prioritized
- **critical** - Requires immediate attention

## Best Practices

1. When creating tasks:
   - Use clear, descriptive names
   - Provide detailed descriptions with context
   - Set appropriate priority based on urgency
   - Choose the correct task type

2. When updating tasks:
   - Only change status according to the workflow progression
   - Update priority if urgency changes
   - Keep descriptions updated with relevant context

3. When listing tasks:
   - Use filters to narrow down results
   - Check status distribution to understand project health
   - Review high-priority items first

4. Always provide context when creating or updating tasks to ensure the development agent has enough information to work effectively.`

const helixProjectsSkillDescription = `Manage Helix project tasks through spec-driven development workflow.

This skill allows you to:
- List tasks with optional filtering by status, priority, or type
- Create new development tasks from user requirements
- Get full details of specific tasks
- Update task properties like status, priority, name, or description

Use this tool when users want to manage their development tasks, check project status, or create new work items.`

var helixProjectsSkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"prompt": {
			Type:        jsonschema.String,
			Description: "The instruction describing what task management operation to perform. Examples: 'list all high priority tasks', 'create a new feature task for user authentication', 'update task status to done'",
		},
	},
	Required: []string{"prompt"},
}

type HelixProjectsSkill struct{}

func NewHelixProjectsSkill(projectID string, store store.Store) agent.Skill {
	skillTools := []agent.Tool{
		NewListSpecTasksTool(projectID, store),
		NewCreateSpecTaskTool(projectID, store),
		NewGetSpecTaskTool(projectID, store),
		NewUpdateSpecTaskTool(projectID, store),
	}
	return agent.Skill{
		Name:         "HelixProjects",
		Description:  helixProjectsSkillDescription,
		SystemPrompt: helixProjectsSystemPrompt,
		Parameters:   helixProjectsSkillParameters,
		Direct:       false, // This is not a direct skill, sub-agent context runner will be used
		Tools:        skillTools,
	}
}

func (s *HelixProjectsSkill) Name() string {
	return "HelixProjects"
}
