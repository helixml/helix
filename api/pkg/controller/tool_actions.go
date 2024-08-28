package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const actionContextHistorySize = 6

func (c *Controller) runActionInteraction(ctx context.Context, session *types.Session, assistantInteraction *types.Interaction) (*types.Session, error) {
	action, ok := assistantInteraction.Metadata["tool_action"]
	if !ok {
		return nil, fmt.Errorf("action not found in interaction metadata")
	}

	var tool *types.Tool
	var err error

	toolID, ok := assistantInteraction.Metadata["tool_id"]
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

		assistantID := session.Metadata.AssistantID
		if assistantID == "" {
			assistantID = "0"
		}
		assistant := data.GetAssistant(app, assistantID)
		if assistant == nil {
			return nil, fmt.Errorf("we could not find the assistant with the id: %s", assistantID)
		}

		for _, appTool := range assistant.Tools {
			if appTool.ID == toolID {
				tool = appTool
			}
		}
	} else {
		tool, err = c.Options.Store.GetTool(ctx, toolID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool %s: %w", toolID, err)
		}
	}

	// Override query parameters if the user has specified them
	for paramName, paramValue := range session.Metadata.AppQueryParams {
		for queryName, queryValue := range tool.Config.API.Query {
			// If the request query params match something in the tool query params, override it
			if queryName == paramName {
				tool.Config.API.Query[queryName] = paramValue
				log.Debug().Msgf("Overriding default tool query param: %s=%s with %s=%s", queryName, queryValue, paramName, tool.Config.API.Query[queryName])
			}
		}
	}

	var updated *types.Session

	history := data.GetLastInteractions(session, actionContextHistorySize)

	messageHistory := types.HistoryFromInteractions(history)

	log.Info().Str("tool", tool.Name).Str("action", action).Str("history", fmt.Sprintf("%+v", messageHistory)).Msg("Running tool action")
	resp, err := c.ToolsPlanner.RunAction(ctx, session.ID, assistantInteraction.ID, tool, messageHistory, action)
	if err != nil {
		return nil, fmt.Errorf("failed to perform action: %w", err)
	}

	updated, err = data.UpdateAssistantInteraction(session, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
		assistantInteraction.Finished = true
		assistantInteraction.Message = resp.Message
		assistantInteraction.Metadata["raw_message"] = resp.RawMessage
		assistantInteraction.Metadata["error"] = resp.Error
		assistantInteraction.Metadata["tool_id"] = toolID
		assistantInteraction.Metadata["tool_app_id"] = session.ParentApp
		assistantInteraction.Metadata["tool_action"] = action
		assistantInteraction.State = types.InteractionStateComplete

		return assistantInteraction, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update assistant interaction: %w", err)
	}

	c.WriteSession(updated)

	return updated, nil
}
