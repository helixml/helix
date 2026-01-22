//go:build cgo

// desktop-bridge is the Helix guest agent that runs inside desktop containers.
// It provides:
// 1. Screenshot capture server (HTTP API for screenshots)
// 2. Desktop MCP server (Model Context Protocol for AI tool integration)
// 3. RevDial client (reverse proxy for API communication through NAT)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/helixml/helix/api/pkg/desktop"
	"github.com/helixml/helix/api/pkg/revdial"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	logger.Info("starting desktop-bridge (Helix guest agent)")

	cfg := desktop.Config{
		HTTPPort:      os.Getenv("SCREENSHOT_PORT"),
		XDGRuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
		SessionID:     os.Getenv("HELIX_SESSION_ID"),
	}

	// Apply defaults
	if cfg.HTTPPort == "" {
		cfg.HTTPPort = "9876"
	}
	if cfg.XDGRuntimeDir == "" {
		cfg.XDGRuntimeDir = "/tmp/sockets"
	}

	// Desktop MCP server config (screenshot, clipboard, input, window management)
	// Note: settings-sync-daemon uses 9877, so MCP uses 9878
	desktopMCPPort := os.Getenv("DESKTOP_MCP_PORT")
	if desktopMCPPort == "" {
		desktopMCPPort = "9878" // Desktop MCP on 9878, settings-sync uses 9877
	}
	mcpEnabled := os.Getenv("MCP_ENABLED") != "false" // Enabled by default

	// RevDial configuration (for API communication through NAT)
	revdialEnabled := os.Getenv("REVDIAL_ENABLED") != "false" // Enabled by default
	apiURL := os.Getenv("HELIX_API_BASE_URL")                 // e.g., http://api:8080
	sessionID := os.Getenv("HELIX_SESSION_ID")                // Session ID for runner ID prefix
	runnerID := ""
	if sessionID != "" {
		runnerID = "desktop-" + sessionID // Match standalone revdial-client format
	}
	runnerToken := os.Getenv("USER_API_TOKEN") // User's API token for auth

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// Start screenshot server
	server := desktop.NewServer(cfg, logger)
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("starting screenshot server", "port", cfg.HTTPPort)
		if err := server.Run(ctx); err != nil && err != context.Canceled {
			logger.Error("screenshot server error", "err", err)
		}
	}()

	// Start Desktop MCP server if enabled (port 9878)
	if mcpEnabled {
		mcpCfg := desktop.MCPConfig{
			Port:          desktopMCPPort,
			ScreenshotURL: fmt.Sprintf("http://localhost:%s/screenshot", cfg.HTTPPort),
		}
		mcpServer := desktop.NewMCPServer(mcpCfg, logger)

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("starting Desktop MCP server", "port", desktopMCPPort)
			if err := mcpServer.Run(ctx, desktopMCPPort); err != nil && err != context.Canceled {
				logger.Error("Desktop MCP server error", "err", err)
			}
		}()
	}

	// Start RevDial client if enabled and configured
	// This allows the API to reach this desktop container through NAT/firewalls
	if revdialEnabled && apiURL != "" && runnerID != "" && runnerToken != "" {
		logger.Info("starting RevDial client",
			"api_url", apiURL,
			"runner_id", runnerID,
			"local_addr", fmt.Sprintf("localhost:%s", cfg.HTTPPort))

		revdialClient := revdial.NewClient(&revdial.ClientConfig{
			ServerURL:          apiURL,
			RunnerID:           runnerID,
			RunnerToken:        runnerToken,
			LocalAddr:          fmt.Sprintf("localhost:%s", cfg.HTTPPort),
			InsecureSkipVerify: true, // TODO: make configurable for enterprise CAs
		})

		wg.Add(1)
		go func() {
			defer wg.Done()
			revdialClient.Start(ctx)
			<-ctx.Done()
			revdialClient.Stop()
			logger.Info("RevDial client stopped")
		}()
	} else if revdialEnabled {
		logger.Warn("RevDial client disabled: missing HELIX_API_URL, HELIX_SESSION_ID, or USER_API_TOKEN")
	}

	// Start AgentClient for non-Zed agent host types (e.g., VS Code + Roo Code)
	// This handles the WebSocket connection to Helix API and translation to the agent
	agentHostType := os.Getenv("HELIX_AGENT_HOST_TYPE")
	if agentHostType == "vscode" && apiURL != "" && sessionID != "" && runnerToken != "" {
		logger.Info("starting AgentClient for VS Code + Roo Code mode",
			"session_id", sessionID,
			"api_url", apiURL)

		agentClient, err := desktop.NewAgentClient(desktop.AgentClientConfig{
			APIURL:           apiURL,
			SessionID:        sessionID,
			Token:            runnerToken,
			HostType:         "vscode",
			RooCodeSocketURL: "9879", // Port for RooCodeBridge Socket.IO server
		})
		if err != nil {
			logger.Error("failed to create AgentClient", "err", err)
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := agentClient.Start(); err != nil {
					logger.Error("AgentClient error", "err", err)
				}
				<-ctx.Done()
				agentClient.Stop()
				logger.Info("AgentClient stopped")
			}()
		}
	}

	// Wait for all services to finish
	wg.Wait()
	logger.Info("desktop-bridge shutdown complete")
}
