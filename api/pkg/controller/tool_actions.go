package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *Controller) runActionInteraction(ctx context.Context, session *types.Session, systemInteraction *types.Interaction) (*types.Session, error) {
	action, ok := systemInteraction.Metadata["tool_action"]
	if !ok {
		return nil, fmt.Errorf("action not found in interaction metadata")
	}

	toolID, ok := systemInteraction.Metadata["tool_id"]
	if !ok {
		return nil, fmt.Errorf("tool ID not found in interaction metadata")
	}

	tool, err := c.Options.Store.GetTool(ctx, toolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool %s: %w", toolID, err)
	}

	userInteraction, err := data.GetLastUserInteraction(session.Interactions)
	if err != nil {
		return nil, fmt.Errorf("failed to get last user interaction: %w", err)
	}

	fmt.Printf("XX running action '%s', message: '%s' \n", action, userInteraction.Message)

	var updated *types.Session

	resp, err := c.Options.Planner.RunAction(ctx, tool, []*types.Interaction{}, userInteraction.Message, action)
	if err != nil {
		return nil, fmt.Errorf("failed to perform action: %w", err)
	}
	updated, err = data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.Finished = true
		systemInteraction.Message = resp.Message
		systemInteraction.Metadata["raw_message"] = resp.RawMessage
		systemInteraction.Metadata["error"] = resp.Error
		systemInteraction.Metadata["tool_id"] = toolID
		systemInteraction.Metadata["tool_action"] = action

		return systemInteraction, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update system interaction: %w", err)
	}

	c.WriteSession(updated)

	return updated, nil
}
