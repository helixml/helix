package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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

	systemPrompt := ""

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

		systemPrompt = assistant.SystemPrompt

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

	message := fmt.Sprintf("%s %s", systemPrompt, userInteraction.Message)
	log.Info().Str("tool", tool.Name).Str("action", action).Str("message", message).Msg("Running tool action")
	resp, err := c.ToolsPlanner.RunAction(ctx, tool, history, message, action)
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
