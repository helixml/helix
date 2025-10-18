package tools

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

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
	var oauthTokens map[string]string

	// First check if OAuth tokens were directly provided in options
	if len(opts.oauthTokens) > 0 {
		oauthTokens = opts.oauthTokens
		log.Info().
			Str("session_id", sessionID).
			Str("tool_name", tool.Name).
			Int("token_count", len(oauthTokens)).
			Msg("Using OAuth tokens from options")
	} else if c.oauthManager != nil || opts.oauthManager != nil {
		// Use OAuth manager from options or from the ChainStrategy
		manager := c.oauthManager
		if opts.oauthManager != nil {
			manager = opts.oauthManager
		}

		// Try to get app ID from context
		appID, ok := oai.GetContextAppID(ctx)
		if !ok || appID == "" {
			// If no app ID in context, try to get it from the session
			if sessionID != "" && manager != nil {
				// Try to get user and app from session ID
				userID, err := c.getUserIDFromSessionID(ctx, sessionID)
				if err != nil {
					log.Warn().
						Err(err).
						Str("session_id", sessionID).
						Str("tool_name", tool.Name).
						Msg("Failed to get user ID from session for OAuth tokens")
				} else if userID != "" {
					// Get the session to look up the app ID
					session, err := c.sessionStore.GetSession(ctx, sessionID)
					if err != nil {
						log.Warn().
							Err(err).
							Str("session_id", sessionID).
							Str("tool_name", tool.Name).
							Msg("Failed to get session for OAuth tokens")
					} else if session.ParentApp != "" {
						appID = session.ParentApp
						log.Info().
							Str("session_id", sessionID).
							Str("app_id", appID).
							Str("user_id", userID).
							Msg("Found app ID from session for OAuth tokens")
					}
				}
			}
		}

		// Get OAuth tokens if we have an app ID and user ID
		if appID != "" && manager != nil {
			// Initialize map for tokens
			oauthTokens = make(map[string]string)

			// Get the app to find owner
			app, err := c.appStore.GetApp(ctx, appID)
			if err != nil {
				log.Warn().
					Err(err).
					Str("app_id", appID).
					Str("session_id", sessionID).
					Str("tool_name", tool.Name).
					Msg("Failed to get app for OAuth tokens")
			} else if app.Owner != "" && tool.Config.API != nil && tool.Config.API.OAuthProvider != "" {
				// Get token for this specific provider
				token, err := manager.GetTokenForTool(ctx, app.Owner, tool.Config.API.OAuthProvider, tool.Config.API.OAuthScopes)
				if err != nil {
					log.Warn().
						Err(err).
						Str("app_id", appID).
						Str("user_id", app.Owner).
						Str("provider", tool.Config.API.OAuthProvider).
						Str("session_id", sessionID).
						Str("tool_name", tool.Name).
						Msg("Failed to get OAuth token for tool")
				} else if token != "" {
					// Add the token to our map
					oauthTokens[tool.Config.API.OAuthProvider] = token

					log.Info().
						Str("app_id", appID).
						Str("user_id", app.Owner).
						Str("provider", tool.Config.API.OAuthProvider).
						Str("session_id", sessionID).
						Str("tool_name", tool.Name).
						Str("token_prefix", token[:5]+"...").
						Msg("Retrieved OAuth token for provider")
				}
			}
		} else {
			log.Warn().
				Str("app_id", appID).
				Str("session_id", sessionID).
				Bool("has_oauth_manager", manager != nil).
				Msg("No app ID available for OAuth token retrieval")
		}
	}

	// Process the OAuth tokens if we have any
	if len(oauthTokens) > 0 && tool.ToolType == types.ToolTypeAPI {
		processOAuthTokens(tool, oauthTokens)
		log.Info().
			Str("session_id", sessionID).
			Str("tool_name", tool.Name).
			Int("token_count", len(oauthTokens)).
			Str("oauth_provider", tool.Config.API.OAuthProvider).
			Msg("Processed OAuth tokens for API tool")
	}

	switch tool.ToolType {
	case types.ToolTypeAPI:
		return c.runAPIAction(ctx, opts.client, sessionID, interactionID, tool, history, action)
	case types.ToolTypeZapier:
		return c.RunZapierAction(ctx, opts.client, tool, history, action)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", tool.ToolType)
	}
}

func (c *ChainStrategy) RunActionStream(ctx context.Context, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string, options ...Option) (*openai.ChatCompletionStream, error) {
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
		Msg("Starting RunActionStream with tool configuration")

	opts := c.getDefaultOptions()

	for _, opt := range options {
		if opt != nil {
			if err := opt(&opts); err != nil {
				return nil, err
			}
		}
	}

	// Add OAuth token handling for API tools
	var oauthTokens map[string]string

	// First check if OAuth tokens were directly provided in options
	if len(opts.oauthTokens) > 0 {
		oauthTokens = opts.oauthTokens
		log.Info().
			Str("session_id", sessionID).
			Str("tool_name", tool.Name).
			Int("token_count", len(oauthTokens)).
			Msg("Using OAuth tokens from options in stream")
	} else if c.oauthManager != nil || opts.oauthManager != nil {
		// Use OAuth manager from options or from the ChainStrategy
		manager := c.oauthManager
		if opts.oauthManager != nil {
			manager = opts.oauthManager
		}

		// Try to get app ID from context
		appID, ok := oai.GetContextAppID(ctx)
		if !ok || appID == "" {
			// If no app ID in context, try to get it from the session
			if sessionID != "" && manager != nil {
				// Try to get user and app from session ID
				userID, err := c.getUserIDFromSessionID(ctx, sessionID)
				if err != nil {
					log.Warn().
						Err(err).
						Str("session_id", sessionID).
						Str("tool_name", tool.Name).
						Msg("Failed to get user ID from session for OAuth tokens in stream")
				} else if userID != "" {
					// Get the session to look up the app ID
					session, err := c.sessionStore.GetSession(ctx, sessionID)
					if err != nil {
						log.Warn().
							Err(err).
							Str("session_id", sessionID).
							Str("tool_name", tool.Name).
							Msg("Failed to get session for OAuth tokens in stream")
					} else if session.ParentApp != "" {
						appID = session.ParentApp
						log.Info().
							Str("session_id", sessionID).
							Str("app_id", appID).
							Str("user_id", userID).
							Msg("Found app ID from session for OAuth tokens in stream")
					}
				}
			}
		}

		// Get OAuth tokens if we have an app ID and user ID
		if appID != "" && manager != nil {
			// Note: Manager doesn't have a method to get all tokens for an app
			// So we need to collect tokens for each tool configuration instead
			oauthTokens = make(map[string]string)

			// Get the app to find owner
			app, err := c.appStore.GetApp(ctx, appID)
			if err != nil {
				log.Warn().
					Err(err).
					Str("app_id", appID).
					Str("session_id", sessionID).
					Str("tool_name", tool.Name).
					Msg("Failed to get app for OAuth tokens in stream")
			} else if app.Owner != "" && tool.Config.API != nil && tool.Config.API.OAuthProvider != "" {
				// Get token for this specific provider
				token, err := manager.GetTokenForApp(ctx, app.Owner, tool.Config.API.OAuthProvider)
				if err != nil {
					log.Warn().
						Err(err).
						Str("app_id", appID).
						Str("user_id", app.Owner).
						Str("provider", tool.Config.API.OAuthProvider).
						Str("session_id", sessionID).
						Str("tool_name", tool.Name).
						Msg("Failed to get OAuth token for tool in stream")
				} else if token != "" {
					// Add the token to our map
					oauthTokens[tool.Config.API.OAuthProvider] = token
					log.Info().
						Str("app_id", appID).
						Str("user_id", app.Owner).
						Str("provider", tool.Config.API.OAuthProvider).
						Str("session_id", sessionID).
						Str("tool_name", tool.Name).
						Str("token_prefix", token[:5]+"...").
						Msg("Retrieved OAuth token for provider in stream")
				}
			}
		} else {
			log.Warn().
				Str("app_id", appID).
				Str("session_id", sessionID).
				Bool("has_oauth_manager", manager != nil).
				Msg("No app ID available for OAuth token retrieval in stream")
		}
	}

	// Process the OAuth tokens if we have any
	if len(oauthTokens) > 0 && tool.ToolType == types.ToolTypeAPI {
		processOAuthTokens(tool, oauthTokens)
		log.Info().
			Str("session_id", sessionID).
			Str("tool_name", tool.Name).
			Int("token_count", len(oauthTokens)).
			Str("oauth_provider", tool.Config.API.OAuthProvider).
			Msg("Processed OAuth tokens for API tool in stream")
	}

	switch tool.ToolType {
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
	httpClient := &http.Client{
		Timeout: 120 * time.Second,
	}

	if c.cfg.Tools.TLSSkipVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// Log the HTTP error for debugging
		log.Error().
			Err(err).
			Str("tool", tool.Name).
			Str("action", action).
			Str("method", req.Method).
			Str("url", req.URL.String()).
			Str("host", req.URL.Host).
			Str("error_type", fmt.Sprintf("%T", err)).
			Dur("time_taken", time.Since(started)).
			Msg("HTTP request failed")
		return nil, fmt.Errorf("failed to make api call: %w", err)
	}

	// Always log response details for all API requests (success or failure)
	// Read response body for logging but keep a copy
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().
			Err(err).
			Str("tool", tool.Name).
			Str("action", action).
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Msg("Failed to read API response body for logging")
		// Return the response even if we can't read the body
		return resp, nil
	}

	// Restore the response body for further processing
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

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
		req.Parameters = make(map[string]interface{})
	}

	// Process OAuth tokens if provided
	if len(req.OAuthTokens) > 0 {
		// Extract token keys for logging
		tokenKeys := make([]string, 0, len(req.OAuthTokens))
		for key := range req.OAuthTokens {
			tokenKeys = append(tokenKeys, key)
		}

		// Only proceed if the tool has OAuth provider configured
		if req.Tool.Config.API != nil && req.Tool.Config.API.OAuthProvider != "" {
			toolProviderName := req.Tool.Config.API.OAuthProvider

			// Log all available tokens for debugging
			for key := range req.OAuthTokens {
				log.Debug().
					Str("available_token_key", key).
					Bool("matches_provider", key == toolProviderName).
					Msg("Available OAuth token")
			}

			// Check if we have a matching OAuth token for this provider
			if token, exists := req.OAuthTokens[toolProviderName]; exists {
				log.Debug().
					Str("provider", toolProviderName).
					Bool("token_present", token != "").
					Str("token_prefix", token[:5]+"...").
					Msg("Found matching OAuth token")

				// Add the token to headers if not already in headers
				authHeaderKey := "Authorization"
				if _, exists := req.Tool.Config.API.Headers[authHeaderKey]; !exists {
					// Add OAuth token as Bearer token if the tool doesn't already have an auth header
					if req.Tool.Config.API.Headers == nil {
						log.Debug().Msg("Initializing headers map in tool API config")
						req.Tool.Config.API.Headers = make(map[string]string)
					}

					bearerToken := fmt.Sprintf("Bearer %s", token)
					req.Tool.Config.API.Headers[authHeaderKey] = bearerToken
				}
			} else {
				// This is important - if we don't find a token with the exact provider name
				// Try a case-insensitive match as a fallback

				for tokenKey, tokenValue := range req.OAuthTokens {
					if strings.EqualFold(tokenKey, toolProviderName) {
						log.Debug().
							Str("tool_provider", toolProviderName).
							Str("token_key", tokenKey).
							Bool("case_sensitive_match", tokenKey == toolProviderName).
							Bool("case_insensitive_match", strings.EqualFold(tokenKey, toolProviderName)).
							Msg("Found OAuth token with case-insensitive match")

						// Add the token to headers
						authHeaderKey := "Authorization"
						if req.Tool.Config.API.Headers == nil {
							req.Tool.Config.API.Headers = make(map[string]string)
						}
						req.Tool.Config.API.Headers[authHeaderKey] = fmt.Sprintf("Bearer %s", tokenValue)

						break
					}
				}
			}
		}
	}

	httpRequest, err := c.prepareRequest(ctx, req.Tool, req.Action, req.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}

	// Log the request details to debug OAuth headers
	log.Debug().
		Str("url", httpRequest.URL.String()).
		Str("method", httpRequest.Method).
		Interface("headers", httpRequest.Header).
		Bool("has_auth_header", httpRequest.Header.Get("Authorization") != "").
		Str("auth_value", func() string {
			auth := httpRequest.Header.Get("Authorization")
			if auth != "" && len(auth) > 15 {
				return auth[:15] + "..."
			}
			return auth
		}()).
		Msg("Prepared API request with headers")

	// Log before making HTTP request (agent mode)
	log.Info().
		Str("url", httpRequest.URL.String()).
		Str("method", httpRequest.Method).
		Msg("Making HTTP request (agent mode)")

	resp, err := c.httpClient.Do(httpRequest)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", httpRequest.URL.String()).
			Str("method", httpRequest.Method).
			Msg("HTTP request failed (agent mode)")
		return nil, fmt.Errorf("failed to make api call: %w", err)
	}
	defer resp.Body.Close()

	// Log HTTP response received (agent mode)
	log.Info().
		Str("url", httpRequest.URL.String()).
		Str("method", httpRequest.Method).
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Interface("headers", resp.Header).
		Msg("HTTP response received (agent mode)")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", httpRequest.URL.String()).
			Int("status_code", resp.StatusCode).
			Msg("Failed to read response body (agent mode)")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log complete response details (agent mode)
	log.Info().
		Str("url", httpRequest.URL.String()).
		Str("method", httpRequest.Method).
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Interface("response_headers", resp.Header).
		Str("response_body", string(body)).
		Int("response_body_length", len(body)).
		Msg("Complete API response details (agent mode)")

	// Log API response summary (agent mode)
	log.Info().
		Str("url", httpRequest.URL.String()).
		Int("status_code", resp.StatusCode).
		Bool("success", resp.StatusCode >= 200 && resp.StatusCode < 300).
		Int("body_length", len(body)).
		Msg("API response details (agent mode)")

	// If body is empty but status code is 200, return the status text
	if len(body) == 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &types.RunAPIActionResponse{Response: "OK"}, nil
	}

	if req.Tool.Config.API.SkipUnknownKeys {
		// Remove unknown keys from the response body
		filteredBody, err := removeUnknownKeys(req.Tool, req.Action, resp.StatusCode, body)
		if err != nil {
			log.Error().
				Err(err).
				Str("tool", req.Tool.Name).
				Str("action", req.Action).
				Str("status", resp.Status).
				Msg("Failed to remove unknown keys from response body")
		} else {
			log.Info().Str("tool", req.Tool.Name).
				Str("size_before", strconv.Itoa(len(body))).
				Str("size_after", strconv.Itoa(len(filteredBody))).
				Str("action", req.Action).Msg("Removed unknown keys from response body")
			body = filteredBody
		}
	}

	return &types.RunAPIActionResponse{Response: string(body)}, nil
}
