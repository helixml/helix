package agent

import (
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// Skill holds a set of tools and a domain-specific prompt/description.
type Skill struct {
	Name         string
	Description  string
	Parameters   any // Top level parameters for the skill
	SystemPrompt string
	Tools        []Tool

	Direct bool // If true, the skill can be executed directly without the skill context runner, examples: calculator, browser
	// If true, the skill will process the output of the tool call before returning it to the top loop. Useful for skills that return structured data such as Browser,
	// however not useful for skills that return text such as WebSearch, Calculator, etc.
	ProcessOutput bool
}

func (s *Skill) GetTools() []openai.Tool {
	tools := []openai.Tool{}
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
		toolDescription := toolParam.Function.Description

		toolsDescription.WriteString(fmt.Sprintf("Tool: %s\n", toolName))
		toolsDescription.WriteString(fmt.Sprintf("Description: %s\n", toolDescription))

		// Get parameters information if available, cast to map[string]interface{}
		paramsAny := toolParam.Function.Parameters

		// Convert paramsAny to map[string]interface{}
		params, ok := paramsAny.(map[string]interface{})
		if !ok {
			continue
		}

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
