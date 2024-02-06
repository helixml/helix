package controller

import (
	"github.com/helixml/helix/api/pkg/types"
)

func getToolFromAction(tools []*types.Tool, action string) *types.Tool {
	for _, tool := range tools {
		if tool.ToolType == types.ToolTypeAPI {
			for _, a := range tool.Config.API.Actions {
				if a.Name == action {
					return tool
				}
			}
		}
	}
	return nil
}
