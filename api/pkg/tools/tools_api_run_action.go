package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	apiActionRetries       = 3
	delayBetweenAPIRetries = 50 * time.Millisecond
)

type RunActionResponse struct {
	Message    string `json:"message"`     // Interpreted message
	RawMessage string `json:"raw_message"` // Raw message from the API
	Error      string `json:"error"`
}

func (c *ChainStrategy) RunAction(ctx context.Context, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string, options ...Option) (*RunActionResponse, error) {
	opts := c.getDefaultOptions()

	for _, opt := range options {
		if opt != nil {
			if err := opt(&opts); err != nil {
				return nil, err
			}
		}
	}

	switch tool.ToolType {
	case types.ToolTypeGPTScript:
		return c.RunGPTScriptAction(ctx, tool, history, action)
	case types.ToolTypeAPI:
		return retry.DoWithData(
			func() (*RunActionResponse, error) {
				return c.runAPIAction(ctx, opts.client, sessionID, interactionID, tool, history, action)
			},
			retry.Attempts(apiActionRetries),
			retry.Delay(delayBetweenAPIRetries),
			retry.Context(ctx),
		)
	case types.ToolTypeZapier:
		return c.RunZapierAction(ctx, opts.client, tool, history, action)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", tool.ToolType)
	}
}

func (c *ChainStrategy) RunActionStream(ctx context.Context, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string, options ...Option) (*openai.ChatCompletionStream, error) {
	opts := c.getDefaultOptions()

	for _, opt := range options {
		if opt != nil {
			if err := opt(&opts); err != nil {
				return nil, err
			}
		}
	}

	switch tool.ToolType {
	case types.ToolTypeGPTScript:
		return c.RunGPTScriptActionStream(ctx, tool, history, action)
	case types.ToolTypeAPI:
		return c.runAPIActionStream(ctx, opts.client, sessionID, interactionID, tool, history, action)
	case types.ToolTypeZapier:
		return c.RunZapierActionStream(ctx, opts.client, tool, history, action)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", tool.ToolType)
	}
}

func (c *ChainStrategy) runAPIAction(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*RunActionResponse, error) {
	resp, err := c.callAPI(ctx, client, sessionID, interactionID, tool, history, action)
	if err != nil {
		return nil, fmt.Errorf("failed to call api: %w", err)
	}
	defer resp.Body.Close()

	return c.interpretResponse(ctx, client, sessionID, interactionID, tool, history, resp)
}

func (c *ChainStrategy) runAPIActionStream(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*openai.ChatCompletionStream, error) {
	resp, err := c.callAPI(ctx, client, sessionID, interactionID, tool, history, action)
	if err != nil {
		return nil, fmt.Errorf("failed to call api: %w", err)
	}
	defer resp.Body.Close()

	return c.interpretResponseStream(ctx, client, sessionID, interactionID, tool, history, resp)
}

func (c *ChainStrategy) callAPI(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*http.Response, error) {
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
	params, err := c.getAPIRequestParameters(ctx, client, sessionID, interactionID, tool, history, action)
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

	return resp, nil
}

// RunAPIActionWithParameters executes the API request with the given parameters. This method (compared to RunAction) doesn't require
// invoking any LLM, neither for request formation nor for response interpretation.
// In this mode Helix is acting as a plumbing only
func (c *ChainStrategy) RunAPIActionWithParameters(ctx context.Context, req *types.RunAPIActionRequest, options ...Option) (*types.RunAPIActionResponse, error) {
	if req.Tool == nil {
		return nil, fmt.Errorf("tool is required")
	}

	if req.Action == "" {
		return nil, fmt.Errorf("action is required")
	}

	opts := c.getDefaultOptions()

	for _, opt := range options {
		if opt != nil {
			if err := opt(&opts); err != nil {
				return nil, err
			}
		}
	}

	if req.Parameters == nil {
		// Initialize empty parameters map, some API actions don't require parameters
		req.Parameters = make(map[string]string)
	}

	log.Info().
		Str("tool", req.Tool.Name).
		Str("action", req.Action).
		Msg("API request parameters prepared")

	// Process OAuth environment variables if provided
	if len(req.OAuthEnvVars) > 0 {
		log.Debug().Int("count", len(req.OAuthEnvVars)).Msg("Adding OAuth tokens to API request")
		for _, envVar := range req.OAuthEnvVars {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				envKey, envValue := parts[0], parts[1]
				// Only process OAUTH_TOKEN_ variables
				if strings.HasPrefix(envKey, "OAUTH_TOKEN_") {
					// Extract provider type from env var name (e.g., OAUTH_TOKEN_GITHUB -> github)
					providerType := strings.ToLower(strings.TrimPrefix(envKey, "OAUTH_TOKEN_"))
					// Add the token to headers if not already in headers
					authHeaderKey := "Authorization"
					if _, exists := req.Tool.Config.API.Headers[authHeaderKey]; !exists {
						// Add OAuth token as Bearer token if the tool doesn't already have an auth header
						if req.Tool.Config.API.Headers == nil {
							req.Tool.Config.API.Headers = make(map[string]string)
						}
						req.Tool.Config.API.Headers[authHeaderKey] = fmt.Sprintf("Bearer %s", envValue)
						log.Debug().Str("provider", providerType).Msg("Added OAuth token to API request headers")
					}
				}
			}
		}
	}

	httpRequest, err := c.prepareRequest(ctx, req.Tool, req.Action, req.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}

	resp, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to make api call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &types.RunAPIActionResponse{Response: string(body)}, nil
}
