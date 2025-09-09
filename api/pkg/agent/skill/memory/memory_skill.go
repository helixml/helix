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
		Description:   "TODO",
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
The add_memory skill allows an LLM-based agent to persist knowledge across turns. 
When the LLM detects that a user's question or the agent's context requires the retention of new facts, facts about user intent, 
or observations from the environment, it should invoke this tool. The stored entry will be accessible in future interactions via <memory></memory>, 
enabling the agent to maintain consistent state, remember preferences, and build cumulative knowledge in a controlled, structured manner.`

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
	return "Browser"
}

func (t *addMemoryTool) Description() string {
	return "Use the browser to open website URLs"
}

func (t *addMemoryTool) String() string {
	return "Memory"
}

func (t *addMemoryTool) StatusMessage() string {
	return "Storing memory information"
}

func (t *addMemoryTool) Icon() string {
	return "LanguageIcon"
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
