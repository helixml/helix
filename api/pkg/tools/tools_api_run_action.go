package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type RunActionResponse struct {
	Message    string `json:"message"`     // Interpreted message
	RawMessage string `json:"raw_message"` // Raw message from the API
	Error      string `json:"error"`
}

func (c *ChainStrategy) RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	switch tool.ToolType {
	case types.ToolTypeFunction:
		return nil, fmt.Errorf("function tool type is not supported yet")
	case types.ToolTypeAPI:
		return c.runApiAction(ctx, tool, history, currentMessage, action)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", tool.ToolType)
	}
}

func (c *ChainStrategy) runApiAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	// Validate whether action is valid
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	found := false

	for _, ac := range tool.Config.API.Actions {
		if ac.Name == action {
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("action %s is not found in the tool %s", action, tool.Name)
	}

	started := time.Now()

	// Get API request parameters
	params, err := c.getAPIRequestParameters(ctx, tool, history, currentMessage, action)
	if err != nil {
		return nil, fmt.Errorf("failed to get api request parameters: %w", err)
	}

	log.Info().
		Str("tool", tool.Name).
		Str("action", action).
		Dur("time_taken", time.Since(started)).
		Msg("API request parameters prepared")

	started = time.Now()

	req, err := c.prepareRequest(ctx, tool, action, params)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}

	log.Info().
		Str("tool", tool.Name).
		Str("action", action).
		Dur("time_taken", time.Since(started)).
		Msg("API request prepared")

	started = time.Now()

	// Make API call
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make api call: %w", err)
	}

	log.Info().
		Str("tool", tool.Name).
		Str("action", action).
		Str("url", req.URL.String()).
		Dur("time_taken", time.Since(started)).
		Msg("API call done")

	defer resp.Body.Close()

	return c.interpretResponse(ctx, tool, currentMessage, resp)
}
