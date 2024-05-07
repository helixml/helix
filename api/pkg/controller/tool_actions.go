package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
)

const actionContextHistorySize = 6

func (c *Controller) runActionInteraction(ctx context.Context, session *types.Session, systemInteraction *types.Interaction) (*types.Session, error) {
	action, ok := systemInteraction.Metadata["tool_action"]
	if !ok {
		return nil, fmt.Errorf("action not found in interaction metadata")
	}

	var tool *types.Tool
	var err error

	toolID, ok := systemInteraction.Metadata["tool_id"]
	if !ok {
		return nil, fmt.Errorf("tool ID not found in interaction metadata")
	}

	if session.ParentApp != "" {
		app, err := c.Options.Store.GetApp(ctx, session.ParentApp)
		if err != nil {
			return nil, fmt.Errorf("failed to get app %s: %w", session.ParentApp, err)
		}
		if len(app.Config.Helix.Assistants) <= 0 {
			return nil, fmt.Errorf("no assistants found in app %s", session.ParentApp)
		}
		assistant := app.Config.Helix.Assistants[0]

		for _, appTool := range assistant.Tools {
			if appTool.ID == toolID {
				tool = &appTool
			}
		}
	} else {
		tool, err = c.Options.Store.GetTool(ctx, toolID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool %s: %w", toolID, err)
		}
	}

	userInteraction, err := data.GetLastUserInteraction(session.Interactions)
	if err != nil {
		return nil, fmt.Errorf("failed to get last user interaction: %w", err)
	}

	var updated *types.Session

	history := data.GetLastInteractions(session, actionContextHistorySize)

	// If history has more than 2 interactions, remove the last 2 as it's the current user and system interaction
	if len(history) > 2 {
		history = history[:len(history)-2]
	}

	resp, err := c.ToolsPlanner.RunAction(ctx, tool, history, userInteraction.Message, action)
	if err != nil {
		return nil, fmt.Errorf("failed to perform action: %w", err)
	}

	updated, err = data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.Finished = true
		systemInteraction.Message = resp.Message
		systemInteraction.Metadata["raw_message"] = resp.RawMessage
		systemInteraction.Metadata["error"] = resp.Error
		systemInteraction.Metadata["tool_id"] = toolID
		systemInteraction.Metadata["tool_app_id"] = session.ParentApp
		systemInteraction.Metadata["tool_action"] = action
		systemInteraction.State = types.InteractionStateComplete

		return systemInteraction, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update system interaction: %w", err)
	}

	c.WriteSession(updated)

	return updated, nil
}
