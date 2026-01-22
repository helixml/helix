//go:build cgo

package desktop

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// RooCodeIPC is a Unix domain socket client for the Roo Code IPC protocol.
// This allows direct local control of the Roo Code extension without going through
// the cloud bridge. It implements the same protocol as roo-cli and roo-ipc.
//
// Architecture:
//
//	[Helix API] <--(WebSocket)--> [RooCodeIPC (this)] <--(Unix Socket)--> [Roo Code Extension]
//
// The Roo Code extension listens on a Unix socket when started with ROO_CODE_IPC_SOCKET_PATH.
// We connect to this socket to send commands and receive events.
//
// Protocol overview (from @roo-code/types):
// - IpcMessageType.TaskCommand: Send commands (StartNewTask, SendMessage, CancelTask)
// - IpcMessageType.TaskEvent: Receive events (TaskStarted, Message, TaskCompleted)
// - IpcMessageType.Ack: Connection acknowledgment with clientId
type RooCodeIPC struct {
	socketPath string
	sessionID  string

	// Connection state
	conn     net.Conn
	clientID string
	connMu   sync.RWMutex

	// Current task state
	currentTaskID string
	taskMu        sync.RWMutex

	// Callbacks for translating events to Helix
	onAgentReady     func()
	onMessageAdded   func(content string, isComplete bool)
	onMessageUpdated func(content string)
	onError          func(err error)

	// Lifecycle
	done   chan struct{}
	closed bool
}

// RooCodeIPCConfig contains configuration for the IPC client
type RooCodeIPCConfig struct {
	// SocketPath is the Unix socket path (default: /tmp/roo-code.sock)
	SocketPath string

	// SessionID is the Helix session ID for logging
	SessionID string

	// Callbacks
	OnAgentReady     func()
	OnMessageAdded   func(content string, isComplete bool)
	OnMessageUpdated func(content string)
	OnError          func(err error)
}

// IPC Message Types (from @roo-code/types IpcMessageType)
const (
	IpcMsgConnect     = "Connect"
	IpcMsgDisconnect  = "Disconnect"
	IpcMsgAck         = "Ack"
	IpcMsgTaskCommand = "TaskCommand"
	IpcMsgTaskEvent   = "TaskEvent"
)

// IPC Origins
const (
	IpcOriginClient = "client"
	IpcOriginServer = "server"
)

// Task Command Names (from @roo-code/types TaskCommandName)
const (
	TaskCmdStartNewTask = "StartNewTask"
	TaskCmdSendMessage  = "SendMessage"
	TaskCmdCancelTask   = "CancelTask"
	TaskCmdCloseTask    = "CloseTask"
	TaskCmdResumeTask   = "ResumeTask"
)

// Roo Code Event Names (from @roo-code/types RooCodeEventName)
const (
	RooEvtTaskStarted   = "TaskStarted"
	RooEvtTaskCompleted = "TaskCompleted"
	RooEvtTaskAborted   = "TaskAborted"
	RooEvtMessage       = "Message"
)

// IpcMessage is the wire format for IPC messages
type IpcMessage struct {
	Type     string      `json:"type"`
	Origin   string      `json:"origin"`
	ClientID string      `json:"clientId,omitempty"`
	Data     interface{} `json:"data,omitempty"`
}

// AckData is the payload for Ack messages
type AckData struct {
	ClientID string `json:"clientId"`
	PID      int    `json:"pid"`
	PPID     int    `json:"ppid"`
}

// TaskCommand is the payload for TaskCommand messages
type TaskCommand struct {
	CommandName string      `json:"commandName"`
	Data        interface{} `json:"data"`
}

// StartNewTaskData is the data for StartNewTask commands
type StartNewTaskData struct {
	Configuration RooCodeConfiguration `json:"configuration"`
	Text          string               `json:"text"`
}

// RooCodeConfiguration contains settings for a Roo Code task
// These match the RooCodeSettings type from @roo-code/types
type RooCodeConfiguration struct {
	// API Provider settings - use environment variables from container
	APIProvider string `json:"apiProvider,omitempty"` // "openai-compatible" for our setup

	// Auto-approval settings for automated operation
	AutoApprovalEnabled   bool     `json:"autoApprovalEnabled"`
	AlwaysAllowReadOnly   bool     `json:"alwaysAllowReadOnly"`
	AlwaysAllowWrite      bool     `json:"alwaysAllowWrite"`
	AlwaysAllowBrowser    bool     `json:"alwaysAllowBrowser"`
	AlwaysApproveResubmit bool     `json:"alwaysApproveResubmit"`
	AlwaysAllowMcp        bool     `json:"alwaysAllowMcp"`
	AlwaysAllowModeSwitch bool     `json:"alwaysAllowModeSwitch"`
	AlwaysAllowSubtasks   bool     `json:"alwaysAllowSubtasks"`
	AlwaysAllowExecute    bool     `json:"alwaysAllowExecute"`
	AllowedCommands       []string `json:"allowedCommands"`

	// Feature toggles
	BrowserToolEnabled bool `json:"browserToolEnabled"`
	EnableCheckpoints  bool `json:"enableCheckpoints"`

	// Mode
	Mode string `json:"mode"` // "code", "architect", "ask", etc.
}

// SendMessageData is the data for SendMessage commands
type SendMessageData struct {
	Text string `json:"text"`
}

// TaskEvent is the payload for TaskEvent messages
type TaskEvent struct {
	EventName string        `json:"eventName"`
	Payload   []interface{} `json:"payload"`
}

// NewRooCodeIPC creates a new IPC client for Roo Code
func NewRooCodeIPC(config RooCodeIPCConfig) (*RooCodeIPC, error) {
	if config.SocketPath == "" {
		config.SocketPath = "/tmp/roo-code.sock"
	}

	ipc := &RooCodeIPC{
		socketPath:       config.SocketPath,
		sessionID:        config.SessionID,
		onAgentReady:     config.OnAgentReady,
		onMessageAdded:   config.OnMessageAdded,
		onMessageUpdated: config.OnMessageUpdated,
		onError:          config.OnError,
		done:             make(chan struct{}),
	}

	return ipc, nil
}

// Start connects to the Roo Code IPC socket
func (c *RooCodeIPC) Start() error {
	log.Info().
		Str("session_id", c.sessionID).
		Str("socket_path", c.socketPath).
		Msg("[RooCodeIPC] Connecting to Roo Code IPC socket")

	// Connect to Unix socket
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to IPC socket: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	// Start read loop
	go c.readLoop()

	log.Info().
		Str("session_id", c.sessionID).
		Str("socket_path", c.socketPath).
		Msg("[RooCodeIPC] Connected to Roo Code IPC socket")

	return nil
}

// readLoop reads messages from the IPC socket
func (c *RooCodeIPC) readLoop() {
	decoder := json.NewDecoder(c.conn)

	for {
		select {
		case <-c.done:
			return
		default:
		}

		var msg IpcMessage
		if err := decoder.Decode(&msg); err != nil {
			if c.closed {
				return
			}
			log.Error().Err(err).Msg("[RooCodeIPC] Failed to decode message")
			if c.onError != nil {
				c.onError(err)
			}
			return
		}

		c.handleMessage(msg)
	}
}

// handleMessage processes an incoming IPC message
func (c *RooCodeIPC) handleMessage(msg IpcMessage) {
	// Only process server messages
	if msg.Origin != IpcOriginServer {
		return
	}

	log.Debug().
		Str("type", msg.Type).
		Str("session_id", c.sessionID).
		Msg("[RooCodeIPC] Received message")

	switch msg.Type {
	case IpcMsgAck:
		c.handleAck(msg)

	case IpcMsgTaskEvent:
		c.handleTaskEvent(msg)
	}
}

// handleAck processes an Ack message (connection confirmed)
func (c *RooCodeIPC) handleAck(msg IpcMessage) {
	// Parse ack data
	dataBytes, _ := json.Marshal(msg.Data)
	var ack AckData
	if err := json.Unmarshal(dataBytes, &ack); err != nil {
		log.Warn().Err(err).Msg("[RooCodeIPC] Failed to parse Ack data")
		return
	}

	c.connMu.Lock()
	c.clientID = ack.ClientID
	c.connMu.Unlock()

	log.Info().
		Str("client_id", ack.ClientID).
		Str("session_id", c.sessionID).
		Msg("[RooCodeIPC] Connection acknowledged")

	// Signal agent ready
	if c.onAgentReady != nil {
		c.onAgentReady()
	}
}

// handleTaskEvent processes a TaskEvent message
func (c *RooCodeIPC) handleTaskEvent(msg IpcMessage) {
	// Parse task event
	dataBytes, _ := json.Marshal(msg.Data)
	var event TaskEvent
	if err := json.Unmarshal(dataBytes, &event); err != nil {
		log.Warn().Err(err).Msg("[RooCodeIPC] Failed to parse TaskEvent")
		return
	}

	log.Debug().
		Str("event_name", event.EventName).
		Str("session_id", c.sessionID).
		Msg("[RooCodeIPC] Task event received")

	switch event.EventName {
	case RooEvtTaskStarted:
		// First payload element is the task ID
		if len(event.Payload) > 0 {
			if taskID, ok := event.Payload[0].(string); ok {
				c.taskMu.Lock()
				c.currentTaskID = taskID
				c.taskMu.Unlock()
				log.Info().Str("task_id", taskID).Msg("[RooCodeIPC] Task started")
			}
		}

	case RooEvtMessage:
		// Message payload format: [{taskId, message: {text, partial}}]
		if len(event.Payload) > 0 {
			if msgData, ok := event.Payload[0].(map[string]interface{}); ok {
				if msgObj, ok := msgData["message"].(map[string]interface{}); ok {
					text, _ := msgObj["text"].(string)
					partial, _ := msgObj["partial"].(bool)

					if !partial && c.onMessageAdded != nil {
						c.onMessageAdded(text, false)
					} else if partial && c.onMessageUpdated != nil {
						c.onMessageUpdated(text)
					}
				}
			}
		}

	case RooEvtTaskCompleted:
		log.Info().Str("session_id", c.sessionID).Msg("[RooCodeIPC] Task completed")
		if c.onMessageAdded != nil {
			c.onMessageAdded("", true) // Signal completion
		}

	case RooEvtTaskAborted:
		log.Warn().Str("session_id", c.sessionID).Msg("[RooCodeIPC] Task aborted")
		if c.onError != nil {
			c.onError(fmt.Errorf("task aborted"))
		}
	}
}

// sendMessage sends a message to the IPC socket
func (c *RooCodeIPC) sendMessage(msg IpcMessage) error {
	c.connMu.RLock()
	conn := c.conn
	clientID := c.clientID
	c.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	// Set client ID if we have one
	if clientID != "" {
		msg.ClientID = clientID
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Add newline delimiter (node-ipc uses JSON lines)
	data = append(data, '\n')

	_, err = conn.Write(data)
	return err
}

// StartTask starts a new task with the given prompt
func (c *RooCodeIPC) StartTask(prompt string) error {
	// Create auto-approval configuration for automated operation
	config := RooCodeConfiguration{
		AutoApprovalEnabled:   true,
		AlwaysAllowReadOnly:   true,
		AlwaysAllowWrite:      true,
		AlwaysAllowBrowser:    true,
		AlwaysApproveResubmit: true,
		AlwaysAllowMcp:        true,
		AlwaysAllowModeSwitch: true,
		AlwaysAllowSubtasks:   true,
		AlwaysAllowExecute:    true,
		AllowedCommands:       []string{"*"},
		BrowserToolEnabled:    false,
		EnableCheckpoints:     false,
		Mode:                  "code",
	}

	msg := IpcMessage{
		Type:   IpcMsgTaskCommand,
		Origin: IpcOriginClient,
		Data: TaskCommand{
			CommandName: TaskCmdStartNewTask,
			Data: StartNewTaskData{
				Configuration: config,
				Text:          prompt,
			},
		},
	}

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	log.Info().
		Str("session_id", c.sessionID).
		Int("prompt_len", len(prompt)).
		Msg("[RooCodeIPC] Started new task")

	return nil
}

// SendMessage sends a message to the current task
func (c *RooCodeIPC) SendMessage(message string) error {
	c.taskMu.RLock()
	taskID := c.currentTaskID
	c.taskMu.RUnlock()

	if taskID == "" {
		// No active task, start a new one
		return c.StartTask(message)
	}

	msg := IpcMessage{
		Type:   IpcMsgTaskCommand,
		Origin: IpcOriginClient,
		Data: TaskCommand{
			CommandName: TaskCmdSendMessage,
			Data: SendMessageData{
				Text: message,
			},
		},
	}

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	log.Debug().
		Str("task_id", taskID).
		Str("session_id", c.sessionID).
		Int("message_len", len(message)).
		Msg("[RooCodeIPC] Sent message to task")

	return nil
}

// StopTask cancels the current task
func (c *RooCodeIPC) StopTask() error {
	c.taskMu.RLock()
	taskID := c.currentTaskID
	c.taskMu.RUnlock()

	if taskID == "" {
		return nil // No task to stop
	}

	msg := IpcMessage{
		Type:   IpcMsgTaskCommand,
		Origin: IpcOriginClient,
		Data: TaskCommand{
			CommandName: TaskCmdCancelTask,
			Data:        nil,
		},
	}

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	log.Info().
		Str("task_id", taskID).
		Str("session_id", c.sessionID).
		Msg("[RooCodeIPC] Cancelled task")

	return nil
}

// Close shuts down the IPC connection
func (c *RooCodeIPC) Close() error {
	c.closed = true
	close(c.done)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			log.Error().Err(err).Msg("[RooCodeIPC] Failed to close connection")
		}
		c.conn = nil
	}

	log.Info().
		Str("session_id", c.sessionID).
		Msg("[RooCodeIPC] Closed")

	return nil
}

// GetCurrentTaskID returns the current task ID (if any)
func (c *RooCodeIPC) GetCurrentTaskID() string {
	c.taskMu.RLock()
	defer c.taskMu.RUnlock()
	return c.currentTaskID
}

// IsConnected returns true if connected to the IPC socket
func (c *RooCodeIPC) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn != nil && c.clientID != ""
}

// WaitForSocket waits for the IPC socket to become available
func (c *RooCodeIPC) WaitForSocket(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to connect
		conn, err := net.DialTimeout("unix", c.socketPath, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		log.Debug().
			Str("socket_path", c.socketPath).
			Msg("[RooCodeIPC] Waiting for socket...")

		time.Sleep(time.Second)
	}

	return fmt.Errorf("timeout waiting for IPC socket: %s", c.socketPath)
}

// GenerateClientID creates a unique client ID
func GenerateClientID() string {
	return "helix-" + uuid.New().String()[:8]
}
