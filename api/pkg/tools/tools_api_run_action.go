package tools

import (
	"bytes"
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
	// Log the tool configuration at the start
	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", interactionID).
		Str("tool_name", tool.Name).
		Str("tool_type", string(tool.ToolType)).
		Interface("tool_config", tool.Config).
		Bool("has_api_config", tool.Config.API != nil).
		Str("oauth_provider", func() string {
			if tool.Config.API != nil {
				return tool.Config.API.OAuthProvider
			}
			return ""
		}()).
		Msg("Starting RunAction with tool configuration")

	opts := c.getDefaultOptions()

	for _, opt := range options {
		if opt != nil {
			if err := opt(&opts); err != nil {
				return nil, err
			}
		}
	}

	// Add OAuth token handling for API tools
	if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil && tool.Config.API.OAuthProvider != "" {
		log.Info().
			Str("tool_type", string(tool.ToolType)).
			Bool("config_api_exists", tool.Config.API != nil).
			Str("oauth_provider", tool.Config.API.OAuthProvider).
			Interface("tool_config", tool.Config).
			Str("session_id", sessionID).
			Str("interaction_id", interactionID).
			Msg("Starting OAuth token handling for API tool")

		// Get the OAuth provider name and required scopes
		providerName := tool.Config.API.OAuthProvider
		requiredScopes := tool.Config.API.OAuthScopes

		log.Info().
			Str("provider_name", providerName).
			Strs("required_scopes", requiredScopes).
			Msg("OAuth provider details")

		// Try to get user ID from session ID
		var userID string
		if c.oauthManager != nil && sessionID != "" && c.sessionStore != nil && c.appStore != nil {
			var err error
			userID, err = c.getUserIDFromSessionID(ctx, sessionID)
			if err != nil {
				log.Warn().
					Err(err).
					Str("session_id", sessionID).
					Bool("oauth_manager_exists", c.oauthManager != nil).
					Bool("session_store_exists", c.sessionStore != nil).
					Bool("app_store_exists", c.appStore != nil).
					Msg("Failed to get user ID from session for OAuth token")
			} else {
				log.Info().
					Str("session_id", sessionID).
					Str("user_id", userID).
					Msg("Successfully retrieved user ID from session")
			}
		} else {
			log.Warn().
				Bool("oauth_manager_exists", c.oauthManager != nil).
				Bool("session_id_exists", sessionID != "").
				Bool("session_store_exists", c.sessionStore != nil).
				Bool("app_store_exists", c.appStore != nil).
				Msg("Missing required components for OAuth token handling")
		}

		// If we have a user ID and OAuth manager, get the token
		if userID != "" && c.oauthManager != nil {
			log.Info().
				Str("session_id", sessionID).
				Str("user_id", userID).
				Str("provider", providerName).
				Strs("scopes", requiredScopes).
				Bool("oauth_manager_exists", c.oauthManager != nil).
				Msg("Fetching OAuth token for API tool")

			// Get the OAuth token for this tool
			token, err := c.oauthManager.GetTokenForTool(ctx, userID, providerName, requiredScopes)
			if err == nil && token != "" {
				// Initialize headers map if it doesn't exist
				if tool.Config.API.Headers == nil {
					tool.Config.API.Headers = make(map[string]string)
					log.Debug().Msg("Initialized empty headers map for API tool")
				}

				// Add the token to the Authorization header
				authHeaderKey := "Authorization"
				tool.Config.API.Headers[authHeaderKey] = fmt.Sprintf("Bearer %s", token)

				log.Info().
					Str("session_id", sessionID).
					Str("provider", providerName).
					Str("token_prefix", token[:10]+"...").
					Bool("headers_map_exists", tool.Config.API.Headers != nil).
					Interface("all_headers", tool.Config.API.Headers).
					Msg("Added OAuth token to API tool headers")
			} else {
				log.Warn().
					Err(err).
					Str("session_id", sessionID).
					Str("provider", providerName).
					Bool("token_empty", token == "").
					Msg("Failed to get OAuth token for API tool")
			}
		} else {
			log.Warn().
				Str("session_id", sessionID).
				Str("user_id", userID).
				Bool("oauth_manager_exists", c.oauthManager != nil).
				Msg("Cannot fetch OAuth token - missing userID or oauthManager")
		}
	}

	if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil {
		// Log details for all API tools
		log.Info().
			Str("session_id", sessionID).
			Str("interaction_id", interactionID).
			Str("tool", tool.Name).
			Str("action", action).
			Str("provider", tool.Config.API.OAuthProvider).
			Str("api_url", tool.Config.API.URL).
			Msg("RunAction called for API tool")

		// Check for Authorization header
		if tool.Config.API.Headers != nil {
			authHeader := tool.Config.API.Headers["Authorization"]
			if authHeader != "" {
				log.Info().
					Str("session_id", sessionID).
					Str("auth_header_prefix", authHeader[:10]+"...").
					Msg("API tool has Authorization header in RunAction")
			} else {
				log.Warn().
					Str("session_id", sessionID).
					Msg("API tool missing Authorization header in RunAction")
			}
		} else {
			log.Warn().
				Str("session_id", sessionID).
				Msg("API tool has no headers map in RunAction")
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

	// Debug logging for all API tools
	if tool.Config.API != nil {
		log.Info().
			Str("session_id", sessionID).
			Str("interaction_id", interactionID).
			Str("tool", tool.Name).
			Str("action", action).
			Str("provider", tool.Config.API.OAuthProvider).
			Str("api_url", tool.Config.API.URL).
			Msg("callAPI called for API tool")

		// Check for Authorization header
		if tool.Config.API.Headers != nil {
			authHeader := tool.Config.API.Headers["Authorization"]
			if authHeader != "" {
				log.Info().
					Str("session_id", sessionID).
					Str("auth_header_prefix", authHeader[:10]+"...").
					Msg("API tool has Authorization header in callAPI")
			} else {
				log.Warn().
					Str("session_id", sessionID).
					Msg("API tool missing Authorization header in callAPI")
			}
		} else {
			log.Warn().
				Str("session_id", sessionID).
				Msg("API tool has no headers map in callAPI")
		}
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

	// Log response details for all API requests
	// Read response body for logging but keep a copy
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read API response body for logging")
	} else {
		// Log response details
		log.Info().
			Str("tool", tool.Name).
			Str("action", action).
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Str("response_body", string(bodyBytes)).
			Msg("API response details")

		// Restore the response body for further processing
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

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

	// Process OAuth tokens if provided
	if len(req.OAuthTokens) > 0 {
		log.Debug().Int("count", len(req.OAuthTokens)).Msg("Adding OAuth tokens to API request")

		// Only proceed if the tool has OAuth provider configured
		if req.Tool.Config.API != nil && req.Tool.Config.API.OAuthProvider != "" {
			toolProviderType := strings.ToLower(req.Tool.Config.API.OAuthProvider)

			// Check if we have a matching OAuth token for this provider
			if token, exists := req.OAuthTokens[toolProviderType]; exists {
				// Add the token to headers if not already in headers
				authHeaderKey := "Authorization"
				if _, exists := req.Tool.Config.API.Headers[authHeaderKey]; !exists {
					// Add OAuth token as Bearer token if the tool doesn't already have an auth header
					if req.Tool.Config.API.Headers == nil {
						req.Tool.Config.API.Headers = make(map[string]string)
					}
					req.Tool.Config.API.Headers[authHeaderKey] = fmt.Sprintf("Bearer %s", token)
					log.Debug().
						Str("provider", toolProviderType).
						Msg("Added matching OAuth token to API request headers")
				} else {
					log.Debug().
						Str("provider", toolProviderType).
						Msg("Authentication header already exists, not overriding")
				}
			} else {
				log.Debug().
					Str("tool_provider", toolProviderType).
					Msg("No matching OAuth token found for provider")
			}
		} else {
			log.Debug().Msg("Tool has no OAuth provider configured, skipping token injection")
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

// Helper function to extract user ID from session ID
