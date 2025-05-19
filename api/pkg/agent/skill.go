// Package agent - skill.go
// Defines the Skill structure, grouping Tools and domain-specific logic.

package agent

import (
	"fmt"
	"strings"

	"github.com/openai/openai-go"
)

// Skill holds a set of tools and a domain-specific prompt/description.
type Skill struct {
	Name         string
	Description  string
	SystemPrompt string
	Tools        []Tool
}

func (s *Skill) GetTools() []openai.ChatCompletionToolParam {
	tools := []openai.ChatCompletionToolParam{}
	for _, tool := range s.Tools {
		tools = append(tools, tool.OpenAI()...)
	}
	return tools
}

func (s *Skill) GetTool(name string) (Tool, error) {
	for _, tool := range s.Tools {
		if tool.Name() == name {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("tool %s not found", name)
}

// Spec converts the skill and all the tools into a string so we can pass the spec into LLM
func (s *Skill) Spec() string {
	var toolsDescription strings.Builder
	toolsDescription.WriteString("Available tools:\n\n")

	for _, toolParam := range s.GetTools() {
		// Extract tool information from the function definition
		toolName := toolParam.Function.Name
		toolDescription := toolParam.Function.Description.Value

		toolsDescription.WriteString(fmt.Sprintf("Tool: %s\n", toolName))
		toolsDescription.WriteString(fmt.Sprintf("Description: %s\n", toolDescription))

		// Get parameters information if available
		params := toolParam.Function.Parameters

		// Extract properties and required fields from parameters
		if properties, ok := params["properties"].(map[string]interface{}); ok {
			toolsDescription.WriteString("Parameters:\n")

			for paramName, paramDetails := range properties {
				paramInfo, ok := paramDetails.(map[string]interface{})
				if ok {
					paramType := paramInfo["type"]
					paramDesc := paramInfo["description"]

					toolsDescription.WriteString(fmt.Sprintf("  - %s (%v): %v\n",
						paramName,
						paramType,
						paramDesc))
				}
			}
		}

		// Add required parameters if available
		if required, ok := params["required"].([]string); ok && len(required) > 0 {
			toolsDescription.WriteString("Required parameters: ")
			toolsDescription.WriteString(strings.Join(required, ", "))
			toolsDescription.WriteString("\n")
		}

		toolsDescription.WriteString("\n")
	}

	return toolsDescription.String()
}
