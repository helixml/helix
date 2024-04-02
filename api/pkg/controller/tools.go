package controller

import (
	"github.com/helixml/helix/api/pkg/types"
)

func getToolFromAction(tools []*types.Tool, action string) (*types.Tool, bool) {
	for _, tool := range tools {
		switch tool.ToolType {
		case types.ToolTypeAPI:
			for _, a := range tool.Config.API.Actions {
				if a.Name == action {
					return tool, true
				}
			}
		case types.ToolTypeGPTScript:
			if tool.Name == action {
				return tool, true
			}
		}
	}
	return nil, false
}
