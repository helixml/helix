package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(runProxyCmd)

	runProxyCmd.Flags().StringP("app-id", "a", "", "the app id to run the proxy for")
	runProxyCmd.Flags().StringP("api-key", "k", "", "the api key to use for the proxy")
	runProxyCmd.Flags().StringP("url", "u", "", "the url to use for the proxy")
}

func setup() {
	zerolog.TimeFieldFormat = time.RFC3339

	// setup logging, write into home directory under .helix/mcp.log, an append file
	// Create .helix directory if it doesn't exist
	helixDir := filepath.Join(os.Getenv("HOME"), ".helix")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return
	}

	var writer io.Writer

	logFile, err := os.OpenFile(filepath.Join(helixDir, "mcp.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		writer = io.Discard
	} else {
		writer = logFile
	}

	log.Logger = zerolog.New(writer).With().Timestamp().Logger()
}

var runProxyCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Helix mpc (model context protocol) proxy",
	Long:  `TODO`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		setup()

		cfg, err := config.LoadCliConfig()
		if err != nil {
			log.Error().Err(err).Msg("failed to load cli config")
			return err
		}

		url, err := cmd.Flags().GetString("url")
		if err != nil {
			log.Error().Err(err).Msg("failed to get url")
			return err
		}
		if url != "" {
			cfg.URL = url
		}

		apiKey, err := cmd.Flags().GetString("api-key")
		if err != nil {
			log.Error().Err(err).Msg("failed to get api key")
			return err
		}
		if apiKey != "" {
			cfg.APIKey = apiKey
		}

		helixAppID := os.Getenv("HELIX_APP_ID")

		appID, err := cmd.Flags().GetString("app-id")
		if err != nil {
			log.Error().Err(err).Msg("failed to get app id")
			return err
		}
		if appID != "" {
			helixAppID = appID
		}

		log.Trace().
			Str("app_id", helixAppID).
			Str("helix_url", cfg.URL).
			Msg("starting mcp proxy")

		if helixAppID == "" {
			log.Error().Msg("HELIX_APP_ID is not set")
			return fmt.Errorf("HELIX_APP_ID is not set")
		}

		apiClient, err := client.NewClient(cfg.URL, cfg.APIKey, cfg.TLSSkipVerify)
		if err != nil {
			log.Error().Err(err).Msg("failed to create api client")
			return err
		}

		srv := &ModelContextProtocolServer{
			apiClient: apiClient,
			appID:     helixAppID,
		}

		return srv.Start()
	},
}

type ModelContextProtocolServer struct {
	appID     string
	apiClient client.Client
}

func (mcps *ModelContextProtocolServer) Start() error {
	// Create a new MCP server
	s := server.NewMCPServer(
		"Helix ML",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	app, err := mcps.apiClient.GetApp(context.Background(), mcps.appID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get app")
		return err
	}

	// TODO: configure assistant
	mcpTools, err := mcps.getModelContextProtocolTools(&app.Config.Helix.Assistants[0])
	if err != nil {
		log.Error().
			Str("app_id", mcps.appID).
			Err(err).
			Msg("failed to get mcp tools for the assistant")
		return err
	}

	log.Info().
		Any("mcp_tools", mcpTools).
		Msg("adding tools")

	for _, mt := range mcpTools {
		s.AddTool(mt.tool, mt.toolHandler)
		s.AddPrompt(mt.prompt, mt.promptHandler)
	}

	// Start the server
	if err := server.ServeStdio(s); err != nil {
		log.Err(err).Msg("Server error")
	}

	return nil
}

type helixMCPTool struct {
	// Prompts: https://modelcontextprotocol.io/docs/concepts/prompts
	// Prompts enable servers to define reusable prompt templates and workflows that clients
	// can easily surface to users and LLMs. They provide a powerful way to standardize and
	//share common LLM interactions.
	prompt        mcp.Prompt
	promptHandler func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)

	// Tools: https://modelcontextprotocol.io/docs/concepts/roots
	// Roots are a concept in MCP that define the boundaries where servers can operate.
	// They provide a way for clients to inform servers about relevant resources and their locations
	tool        mcp.Tool
	toolHandler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// TODO: load zapier tools as well
func (mcps *ModelContextProtocolServer) getModelContextProtocolTools(app *types.AssistantConfig) ([]*helixMCPTool, error) {
	var mcpTools []*helixMCPTool

	for _, apiTool := range app.APIs {
		tool, err := store.ConvertAPIToTool(apiTool)
		if err != nil {
			log.Error().
				Err(err).
				Msg("failed to convert api tool to mcp tool")
			continue
		}

		// Each API tool has a list of actions, adding them separately
		for _, action := range tool.Config.API.Actions {
			parameters, err := tools.GetParametersFromSchema(tool.Config.API.Schema, action.Name)
			if err != nil {
				log.Error().
					Err(err).
					Str("tool", tool.Name).
					Str("action", action.Name).
					Msg("failed to get parameters from schema")
				continue
			}

			var (
				mcpParams []mcp.ToolOption // Tool parameters
			)

			mcpParams = append(mcpParams, mcp.WithDescription(action.Description))

			for _, param := range parameters {
				if param.Required {
					mcpParams = append(mcpParams, mcp.WithString(param.Name,
						mcp.Required(),
						mcp.Description(param.Description),
					))
				} else {
					mcpParams = append(mcpParams, mcp.WithString(param.Name,
						mcp.Description(param.Description),
					))
				}
			}

			mcpTool := mcp.NewTool(action.Name,
				mcpParams...,
			)

			log.Info().Any("tool", action).Msg("adding tool")

			mcpTools = append(mcpTools, &helixMCPTool{
				tool:        mcpTool,
				toolHandler: mcps.getAPIToolHandler(mcps.appID, tool, action.Name),
			})
		}
	}

	for _, zapier := range app.Zapier {
		mcpTool := mcp.NewTool(zapier.Name,
			mcp.WithDescription(zapier.Description),
		)

		mcpTools = append(mcpTools, &helixMCPTool{
			tool:        mcpTool,
			toolHandler: mcps.zapierToolHandler,
		})
	}

	log.Info().Msg("listing knowledges")

	knowledges, err := mcps.apiClient.ListKnowledge(context.Background(), &client.KnowledgeFilter{
		AppID: mcps.appID,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list knowledge")
		return nil, err
	}

	for _, knowledge := range knowledges {
		knowledgeDescription :=
			`Performs a search using the Helix knowledge base, ideal for finding information on a specific topic.
		`
		if knowledge.Description != "" {
			knowledgeDescription += fmt.Sprintf("This tool contains information on: %s", knowledge.Description)
		}
		mcpTool := mcp.NewTool(knowledge.Name,
			mcp.WithDescription(knowledgeDescription),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("For query use concise, main keywords as the engine is performing both semantic and full text search"),
			),
		)

		mcpTools = append(mcpTools, &helixMCPTool{
			tool:        mcpTool,
			toolHandler: mcps.getKnowledgeToolHandler(knowledge.ID),
		})
	}

	return mcpTools, nil
}

func (mcps *ModelContextProtocolServer) getAPIToolHandler(appID string, tool *types.Tool, action string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Info().
			Str("tool", tool.Name).
			Str("action", action).
			Msg("api tool handler")

		params := make(map[string]interface{})

		for k, v := range request.GetArguments() {
			params[k] = v
		}

		resp, err := mcps.apiClient.RunAPIAction(ctx, appID, action, params)
		if err != nil {
			log.Error().Err(err).Msg("failed to run api action")
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(resp.Response), nil
	}
}

func (mcps *ModelContextProtocolServer) zapierToolHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:unparam
	// TODO: implement zapier tool handler
	return mcp.NewToolResultText("Hello, World!"), nil
}

func (mcps *ModelContextProtocolServer) getKnowledgeToolHandler(knowledgeID string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		log.Info().Str("knowledge_id", knowledgeID).Str("query", query).Msg("searching knowledge")

		results, err := mcps.apiClient.SearchKnowledge(ctx, &client.KnowledgeSearchQuery{
			AppID:       mcps.appID,
			KnowledgeID: knowledgeID,
			Prompt:      query,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to search knowledge")
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(formatKnowledgeSearchResponse(results)), nil
	}
}

// formatKnowledgeSearchResponse formats the results into a text with just Source and Content fields. Each section is separated by an empty line
//
// Source: <URL>
// Content: <Content>
// ...
// Source: <URL>
// Content: <Content>

func formatKnowledgeSearchResponse(results []*types.KnowledgeSearchResult) string {
	if len(results) == 0 {
		return "No results found"
	}
	var buf bytes.Buffer
	for _, result := range results {
		for _, r := range result.Results {
			buf.WriteString(fmt.Sprintf("Source: %s\nContent: %s\n\n", r.Source, r.Content))
		}
	}

	return buf.String()
}
