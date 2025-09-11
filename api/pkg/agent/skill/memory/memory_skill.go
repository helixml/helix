package memory

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"

	"github.com/sashabaranov/go-openai"
)

func NewAddMemorySkill(store store.Store) agent.Skill {
	return agent.Skill{
		Name:          "AddMemory",
		Description:   addMemorySkillDescription,
		SystemPrompt:  "n/a",
		Parameters:    addMemorySkillParameters,
		Direct:        true,
		ProcessOutput: false,
		Tools: []agent.Tool{
			&addMemoryTool{
				store: store,
			},
		},
	}
}

const addMemorySkillDescription = `## Purpose
The AddMemory skill allows an LLM-based agent to persist knowledge across conversation sessions and turns. 
Use this tool whenever the user shares information that would be valuable to remember in future conversations, including:

- Personal preferences, settings, or configurations
- Important facts about the user's work, projects, or interests  
- User goals, objectives, or ongoing tasks
- Contact information, deadlines, or important dates
- User feedback, corrections, or specific instructions
- Context about previous conversations or decisions made
- Any information the user explicitly asks you to remember

The stored memories will be accessible in future interactions via <memory></memory>, enabling the agent to maintain consistent state, 
remember user preferences, and build cumulative knowledge across sessions. Only store information that would genuinely be useful 
for future conversations - avoid storing trivial or temporary information.`

var addMemorySkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"memory_entry": {
			Type:        jsonschema.String,
			Description: "The exact text (sentences, key-value pairs, or structured JSON) to store in long-term memory. It should be concise, self-contained, and contain no personally identifying information unless explicitly permitted.",
		},
	},
	Required: []string{"memory_entry"},
}

type addMemoryTool struct {
	store store.Store
}

func (t *addMemoryTool) Name() string {
	return "Memory"
}

func (t *addMemoryTool) Description() string {
	return "Use the memory to store information"
}

func (t *addMemoryTool) String() string {
	return "Memory"
}

func (t *addMemoryTool) StatusMessage() string {
	return "Storing memory information"
}

func (t *addMemoryTool) Icon() string {
	return "MemoryIcon"
}

func (t *addMemoryTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "AddMemory",
				Description: addMemorySkillDescription,
				Parameters:  addMemorySkillParameters,
			},
		},
	}
}

func (t *addMemoryTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	contents, ok := args["memory_entry"].(string)
	if !ok {
		return "", fmt.Errorf("url is required")
	}

	if meta.UserID == "" {
		// No user detected
		log.Error().Msg("no user ID detected, cannot add memory")
		return "", nil
	}

	memory, err := t.store.CreateMemory(ctx, &types.Memory{
		UserID:   meta.UserID,
		AppID:    meta.AppID,
		Contents: contents,
	})
	if err != nil {
		return "", fmt.Errorf("failed to add memory: %w", err)
	}

	return fmt.Sprintf("memory %s added", memory.ID), nil
}
