package mcp

import (
	"context"
	"encoding/json"
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
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(runProxyCmd)
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
	RunE: func(_ *cobra.Command, _ []string) error {
		setup()

		cfg, err := config.LoadCliConfig()
		if err != nil {
			log.Error().Err(err).Msg("failed to load cli config")
			return err
		}

		helixAppID := os.Getenv("HELIX_APP_ID")

		cfg.APIKey = "hl-NColKMiDhBodAfm4UXZRHrQlU7HXgZiVd6Yg27gAJfo="
		cfg.URL = "http://localhost:8080"
		helixAppID = "app_01jfsqaq25xytqc1ep8hrt7php"

		log.Trace().
			Str("app_id", helixAppID).
			Str("helix_url", cfg.URL).
			Str("helix_api_key", cfg.APIKey).
			Msg("starting mcp proxy")

		if helixAppID == "" {
			log.Error().Msg("HELIX_APP_ID is not set")
			return fmt.Errorf("HELIX_APP_ID is not set")
		}

		apiClient, err := client.NewClient(cfg.URL, cfg.APIKey)
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

	log.Info().Any("mcpTools", mcpTools).Msg("adding tools")

	for _, mt := range mcpTools {
		s.AddTool(mt.tool, mt.handler)
	}

	// Add a calculator tool
	// calculatorTool := mcp.NewTool("calculate",
	// 	mcp.WithDescription("Perform basic arithmetic operations"),
	// mcp.WithString("operation",
	// 	mcp.Required(),
	// 	mcp.Description("The operation to perform (add, subtract, multiply, divide)"),
	// 	mcp.Enum("add", "subtract", "multiply", "divide"),
	// ),
	// 	mcp.WithNumber("x",
	// 		mcp.Required(),
	// 		mcp.Description("First number"),
	// 	),
	// 	mcp.WithNumber("y",
	// 		mcp.Required(),
	// 		mcp.Description("Second number"),
	// 	),
	// )

	// // Add the calculator handler
	// s.AddTool(calculatorTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	op := request.Params.Arguments["operation"].(string)
	// 	x := request.Params.Arguments["x"].(float64)
	// 	y := request.Params.Arguments["y"].(float64)

	// 	var result float64
	// 	switch op {
	// 	case "add":
	// 		result = x + y
	// 	case "subtract":
	// 		result = x - y
	// 	case "multiply":
	// 		result = x * y
	// 	case "divide":
	// 		if y == 0 {
	// 			return mcp.NewToolResultError("Cannot divide by zero"), nil
	// 		}
	// 		result = x / y
	// 	}

	// 	return mcp.NewToolResultText(fmt.Sprintf("%.2f", result)), nil
	// })

	// Start the server
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}

	return nil
}

type helixMCPTool struct {
	tool    mcp.Tool
	handler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// TODO: load gptscript, zapier tools as well
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
			mcpTool := mcp.NewTool(action.Name,
				mcp.WithDescription(action.Description),
			)

			log.Info().Any("tool", action).Msg("adding tool")

			mcpTools = append(mcpTools, &helixMCPTool{
				tool:    mcpTool,
				handler: mcps.apiToolHandler,
			})
		}
	}

	for _, gptScript := range app.GPTScripts {
		mcpTool := mcp.NewTool(gptScript.Name,
			mcp.WithDescription(gptScript.Description),
		)

		mcpTools = append(mcpTools, &helixMCPTool{
			tool:    mcpTool,
			handler: mcps.gptScriptToolHandler,
		})
	}

	for _, zapier := range app.Zapier {
		mcpTool := mcp.NewTool(zapier.Name,
			mcp.WithDescription(zapier.Description),
		)

		mcpTools = append(mcpTools, &helixMCPTool{
			tool:    mcpTool,
			handler: mcps.zapierToolHandler,
		})
	}

	knowledges, err := mcps.apiClient.ListKnowledge(context.Background(), &client.KnowledgeFilter{
		AppID: mcps.appID,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list knowledge")
		return nil, err
	}

	for _, knowledge := range knowledges {
		mcpTool := mcp.NewTool(knowledge.Name,
			mcp.WithDescription(fmt.Sprintf("This knowledge tool is for: %s", knowledge.Description)),
			mcp.WithString("prompt",
				mcp.Required(),
				mcp.Description("The prompt to search knowledge with, use concise, main keywords as the engine is performing both semantic and full text search"),
			),
		)

		mcpTools = append(mcpTools, &helixMCPTool{
			tool:    mcpTool,
			handler: mcps.getKnowledgeToolHandler(knowledge.ID),
		})
	}

	return mcpTools, nil
}

func (mcps *ModelContextProtocolServer) apiToolHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Hello, World!"), nil
}

func (mcps *ModelContextProtocolServer) gptScriptToolHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Hello, World!"), nil
}

func (mcps *ModelContextProtocolServer) zapierToolHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Hello, World!"), nil
}

func (mcps *ModelContextProtocolServer) getKnowledgeToolHandler(knowledgeID string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		prompt, ok := request.Params.Arguments["prompt"]
		if !ok {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		promptStr, ok := prompt.(string)
		if !ok {
			return mcp.NewToolResultError("prompt must be a string"), nil
		}

		results, err := mcps.apiClient.SearchKnowledge(ctx, &client.KnowledgeSearchQuery{
			AppID:       mcps.appID,
			KnowledgeID: knowledgeID,
			Prompt:      promptStr,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to search knowledge")
			return mcp.NewToolResultError(err.Error()), nil
		}

		resultsJSON, err := json.Marshal(results)
		if err != nil {
			log.Error().Err(err).Msg("failed to marshal knowledge search results")
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(string(resultsJSON)), nil
	}
}
