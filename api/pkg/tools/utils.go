package tools

import (
	"encoding/json"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

func AttemptFixJSON(data string) string {
	// sometimes LLM just gives us a single ``` line at the start; just strip that off
	if strings.HasPrefix(data, "```\n") {
		data = strings.Split(data, "```\n")[1]
	}

	if strings.Contains(data, "```json") {
		data = strings.Split(data, "```json")[1]
	}
	// sometimes LLMs in their wisdom puts a message after the enclosing ```json``` block
	parts := strings.Split(data, "```")
	data = parts[0]

	// sometimes LLMs return inconsistent number of backticks (e.g., 2 instead of 3)
	// trim any trailing backticks that weren't caught by the split above
	data = strings.TrimRight(data, "`")

	return data
}

func unmarshalJSON(data string, v interface{}) error {
	fixedData := AttemptFixJSON(data)
	return json.Unmarshal([]byte(fixedData), v)
}

func GetToolFromAction(tools []*types.Tool, action string) (*types.Tool, bool) {
	for _, tool := range tools {
		switch tool.ToolType {
		case types.ToolTypeAPI:
			for _, a := range tool.Config.API.Actions {
				if a.Name == action {
					return tool, true
				}
			}

		case types.ToolTypeZapier:
			if tool.Name == action {
				return tool, true
			}
		}
	}
	return nil, false
}
