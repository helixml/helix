//go:build cgo

package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// AgentClient handles the WebSocket connection to Helix API for receiving commands
// and translates them to the appropriate agent backend (Zed, Roo Code, Cursor, or headless).
type AgentClient struct {
	apiURL       string
	sessionID    string
	token        string
	hostType     types.AgentHostType
	rooBridge    *RooCodeBridge
	cursorBridge *CursorBridge
	conn         *websocket.Conn
	sendChan     chan interface{}
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	reconnect    bool
	onReady      func() // Called when agent is ready
}

// AgentClientConfig contains configuration for the agent client
type AgentClientConfig struct {
	// APIURL is the Helix API URL (e.g., http://api:8080)
	APIURL string

	// SessionID is the Helix session ID
	SessionID string

	// Token is the user API token for authentication
	Token string

	// HostType determines which agent backend to use ("zed", "vscode", "headless")
	HostType string

	// RooCodeSocketURL is the Socket.IO port for Roo Code (only for vscode host type)
	RooCodeSocketURL string
}

// NewAgentClient creates a new agent client
func NewAgentClient(cfg AgentClientConfig) (*AgentClient, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Convert string to AgentHostType
	hostType := types.AgentHostType(cfg.HostType)
	if hostType == "" {
		hostType = types.AgentHostTypeZed
	}

	client := &AgentClient{
		apiURL:    cfg.APIURL,
		sessionID: cfg.SessionID,
		token:     cfg.Token,
		hostType:  hostType,
		sendChan:  make(chan interface{}, 100),
		ctx:       ctx,
		cancel:    cancel,
		reconnect: true,
	}

	// Initialize the appropriate agent backend
	switch hostType {
	case types.AgentHostTypeVSCode:
		// RooCodeBridge is a Socket.IO SERVER that Roo Code connects to
		// Port for the bridge (Roo Code API mock + Socket.IO server)
		bridgePort := cfg.RooCodeSocketURL
		if bridgePort == "" {
			bridgePort = "9879"
		}

		rooBridge, err := NewRooCodeBridge(RooCodeBridgeConfig{
			Port:      bridgePort,
			SessionID: cfg.SessionID,
			OnAgentReady: func() {
				client.sendAgentReady()
			},
			OnMessageAdded: func(content string, isComplete bool) {
				client.sendMessageAdded(content, isComplete)
			},
			OnMessageUpdated: func(content string) {
				// Not used yet - for streaming updates
			},
			OnError: func(err error) {
				log.Error().Err(err).Str("session_id", cfg.SessionID).Msg("[AgentClient] Roo Code error")
			},
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create Roo Code bridge: %w", err)
		}
		client.rooBridge = rooBridge

	case types.AgentHostTypeCursor:
		// CursorBridge uses subprocess-based communication with cursor-agent CLI
		cursorBridge, err := NewCursorBridge(CursorBridgeConfig{
			SessionID: cfg.SessionID,
			OnAgentReady: func() {
				client.sendAgentReady()
			},
			OnMessageAdded: func(content string, isComplete bool) {
				client.sendMessageAdded(content, isComplete)
			},
			OnMessageUpdated: func(content string) {
				// Not used yet - for streaming updates
			},
			OnError: func(err error) {
				log.Error().Err(err).Str("session_id", cfg.SessionID).Msg("[AgentClient] Cursor error")
			},
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create Cursor bridge: %w", err)
		}
		client.cursorBridge = cursorBridge

	case types.AgentHostTypeHeadless:
		// TODO: Implement headless ACP client
		log.Warn().Msg("[AgentClient] Headless mode not yet implemented")

	case types.AgentHostTypeZed:
		// Zed handles its own WebSocket connection via the Zed extension
		// This client is not used for Zed mode
		log.Info().Msg("[AgentClient] Zed mode - agent client not needed")
	}

	return client, nil
}

// Start begins the WebSocket connection and message handling
func (c *AgentClient) Start() error {
	if c.hostType == types.AgentHostTypeZed {
		// Zed mode doesn't need this client
		return nil
	}

	// Start RooCodeBridge server (if applicable)
	// The Roo Code extension will connect to this server
	if c.rooBridge != nil {
		if err := c.rooBridge.Start(); err != nil {
			return fmt.Errorf("failed to start Roo Code bridge: %w", err)
		}
	}

	// Start CursorBridge (if applicable)
	if c.cursorBridge != nil {
		if err := c.cursorBridge.Start(); err != nil {
			return fmt.Errorf("failed to start Cursor bridge: %w", err)
		}
	}

	// Connect to Helix API WebSocket
	if err := c.connect(); err != nil {
		return fmt.Errorf("failed to connect to Helix API: %w", err)
	}

	// Start message handlers
	go c.readLoop()
	go c.writeLoop()

	return nil
}

// connect establishes the WebSocket connection to Helix API
func (c *AgentClient) connect() error {
	// Build WebSocket URL
	apiParsed, err := url.Parse(c.apiURL)
	if err != nil {
		return fmt.Errorf("failed to parse API URL: %w", err)
	}

	wsScheme := "ws"
	if apiParsed.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/api/v1/external-agents/sync?session_id=%s",
		wsScheme, apiParsed.Host, c.sessionID)

	log.Info().
		Str("session_id", c.sessionID).
		Str("ws_url", wsURL).
		Msg("[AgentClient] Connecting to Helix API WebSocket")

	// Add auth header
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Info().
		Str("session_id", c.sessionID).
		Msg("[AgentClient] Connected to Helix API WebSocket")

	return nil
}

// readLoop handles incoming messages from Helix API
func (c *AgentClient) readLoop() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			time.Sleep(time.Second)
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Warn().Err(err).Str("session_id", c.sessionID).Msg("[AgentClient] WebSocket read error")
			if c.reconnect {
				time.Sleep(5 * time.Second)
				if err := c.connect(); err != nil {
					log.Error().Err(err).Msg("[AgentClient] Reconnect failed")
				}
			}
			continue
		}

		// Parse command
		var cmd types.ExternalAgentCommand
		if err := json.Unmarshal(message, &cmd); err != nil {
			log.Warn().Err(err).Str("raw", string(message)).Msg("[AgentClient] Failed to parse command")
			continue
		}

		c.handleCommand(cmd)
	}
}

// handleCommand processes a command from Helix API
func (c *AgentClient) handleCommand(cmd types.ExternalAgentCommand) {
	log.Debug().
		Str("type", cmd.Type).
		Str("session_id", c.sessionID).
		Msg("[AgentClient] Received command")

	switch cmd.Type {
	case "chat_message":
		message, _ := cmd.Data["message"].(string)
		if message == "" {
			log.Warn().Msg("[AgentClient] chat_message with empty message")
			return
		}

		// Route to appropriate backend
		switch c.hostType {
		case types.AgentHostTypeVSCode:
			if c.rooBridge != nil {
				if err := c.rooBridge.SendMessage(message); err != nil {
					log.Error().Err(err).Msg("[AgentClient] Failed to send message to Roo Code")
				}
			}
		case types.AgentHostTypeCursor:
			if c.cursorBridge != nil {
				if err := c.cursorBridge.SendMessage(message); err != nil {
					log.Error().Err(err).Msg("[AgentClient] Failed to send message to Cursor")
				}
			}
		case types.AgentHostTypeHeadless:
			// TODO: Route to headless ACP client
			log.Warn().Msg("[AgentClient] Headless mode not yet implemented")
		}

	case "stop":
		if c.rooBridge != nil {
			_ = c.rooBridge.StopTask()
		}
		if c.cursorBridge != nil {
			_ = c.cursorBridge.StopTask()
		}

	default:
		log.Debug().Str("type", cmd.Type).Msg("[AgentClient] Unknown command type")
	}
}

// writeLoop handles outgoing messages to Helix API
func (c *AgentClient) writeLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-c.sendChan:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()

			if conn == nil {
				log.Warn().Msg("[AgentClient] Cannot send - no connection")
				continue
			}

			data, err := json.Marshal(msg)
			if err != nil {
				log.Error().Err(err).Msg("[AgentClient] Failed to marshal message")
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Error().Err(err).Msg("[AgentClient] Failed to send message")
			}
		}
	}
}

// sendAgentReady sends agent_ready event to Helix API
func (c *AgentClient) sendAgentReady() {
	syncMsg := types.SyncMessage{
		EventType: "agent_ready",
		SessionID: c.sessionID,
		Timestamp: time.Now(),
	}

	select {
	case c.sendChan <- syncMsg:
		log.Info().Str("session_id", c.sessionID).Msg("[AgentClient] Sent agent_ready")
	default:
		log.Warn().Msg("[AgentClient] Send channel full, dropping agent_ready")
	}

	// Also call the onReady callback if set
	if c.onReady != nil {
		c.onReady()
	}
}

// sendMessageAdded sends message_added event to Helix API
func (c *AgentClient) sendMessageAdded(content string, isComplete bool) {
	syncMsg := types.SyncMessage{
		EventType: "message_added",
		SessionID: c.sessionID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"content":  content,
			"complete": isComplete,
		},
	}

	select {
	case c.sendChan <- syncMsg:
		log.Debug().
			Str("session_id", c.sessionID).
			Bool("complete", isComplete).
			Int("content_len", len(content)).
			Msg("[AgentClient] Sent message_added")
	default:
		log.Warn().Msg("[AgentClient] Send channel full, dropping message_added")
	}
}

// Stop gracefully shuts down the agent client
func (c *AgentClient) Stop() error {
	c.reconnect = false
	c.cancel()

	// Close Roo Code bridge
	if c.rooBridge != nil {
		_ = c.rooBridge.Close()
	}

	// Close Cursor bridge
	if c.cursorBridge != nil {
		_ = c.cursorBridge.Close()
	}

	// Close WebSocket
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()

	log.Info().Str("session_id", c.sessionID).Msg("[AgentClient] Stopped")
	return nil
}
