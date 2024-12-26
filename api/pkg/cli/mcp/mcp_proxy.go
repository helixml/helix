package mcp

import (
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
		log.Fatal().Err(err).Msg("failed to create .helix directory")
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
		// setup()

		cfg, err := config.LoadCliConfig()
		if err != nil {
			log.Error().Err(err).Msg("failed to load cli config")
			return err
		}

		helixAppID := os.Getenv("HELIX_APP_ID")

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
		"Calculator Demo",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Add a calculator tool
	calculatorTool := mcp.NewTool("calculate",
		mcp.WithDescription("Perform basic arithmetic operations"),
		mcp.WithString("operation",
			mcp.Required(),
			mcp.Description("The operation to perform (add, subtract, multiply, divide)"),
			mcp.Enum("add", "subtract", "multiply", "divide"),
		),
		mcp.WithNumber("x",
			mcp.Required(),
			mcp.Description("First number"),
		),
		mcp.WithNumber("y",
			mcp.Required(),
			mcp.Description("Second number"),
		),
	)

	// Add the calculator handler
	s.AddTool(calculatorTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		op := request.Params.Arguments["operation"].(string)
		x := request.Params.Arguments["x"].(float64)
		y := request.Params.Arguments["y"].(float64)

		var result float64
		switch op {
		case "add":
			result = x + y
		case "subtract":
			result = x - y
		case "multiply":
			result = x * y
		case "divide":
			if y == 0 {
				return mcp.NewToolResultError("Cannot divide by zero"), nil
			}
			result = x / y
		}

		return mcp.NewToolResultText(fmt.Sprintf("%.2f", result)), nil
	})

	// Start the server
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}

	return nil
}

const (
	testAppID  = "app_01jfsqaq25xytqc1ep8hrt7php"
	testUserID = "191517eb-4dd3-4e71-8370-40d419f1c854"
)

type helixMCPTool struct {
	appID   string
	tool    *mcp.Tool
	handler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// TODO: load gptscript, zapier tools as well
func (mcps *ModelContextProtocolServer) getModelContextProtocolTools(app *types.AssistantConfig) ([]*helixMCPTool, error) {
	var mcpTools []*helixMCPTool

	for _, tool := range app.Tools {
		switch tool.ToolType {
		case types.ToolTypeAPI:
			// Each API tool has a list of actions, adding them separately
			for _, action := range tool.Config.API.Actions {
				mcpTool := mcp.NewTool(action.Name,
					mcp.WithDescription(action.Description),
				)

				mcpTools = append(mcpTools, &helixMCPTool{
					appID:   app.ID,
					tool:    &mcpTool,
					handler: mcps.helloHandler,
				})
			}
		case types.ToolTypeGPTScript:
			mcpTool := mcp.NewTool(tool.Name,
				mcp.WithDescription(tool.Description),
			)

			mcpTools = append(mcpTools, &helixMCPTool{
				appID:   app.ID,
				tool:    &mcpTool,
				handler: mcps.helloHandler,
			})
		case types.ToolTypeZapier:
			mcpTool := mcp.NewTool(tool.Name,
				mcp.WithDescription(tool.Description),
			)
			mcpTools = append(mcpTools, &helixMCPTool{
				appID:   app.ID,
				tool:    &mcpTool,
				handler: mcps.helloHandler,
			})
		}

	}

	return mcpTools, nil
}

func (mcps *ModelContextProtocolServer) helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Hello, World!"), nil
}
