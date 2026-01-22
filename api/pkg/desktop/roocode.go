package desktop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	socketio "github.com/doquangtan/socketio/v4"
)

// RooCodeBridge is a Socket.IO SERVER that the Roo Code VS Code extension connects to.
// It translates between Helix commands and Roo Code's bridge protocol.
//
// Architecture:
//   [Helix API] <--(WebSocket)--> [RooCodeBridge (this)] <--(Socket.IO)--> [Roo Code Extension]
//
// The Roo Code extension (running in VS Code inside the container) connects outbound
// to this server, which allows us to send commands and receive events.
//
// Protocol overview:
// - Extension channel: start_task, stop_task, resume_task (lifecycle)
// - Task channel: message, approve_ask, deny_ask (interaction)
type RooCodeBridge struct {
	server    *socketio.Server
	port      string
	sessionID string

	// Connected extension state
	extensionSocket socketio.Socket
	instanceID      string
	socketMu        sync.RWMutex

	// Current task state
	currentTaskID string
	taskMu        sync.RWMutex

	// Callbacks for translating events to Helix
	onAgentReady     func()
	onMessageAdded   func(content string, isComplete bool)
	onMessageUpdated func(content string)
	onError          func(err error)

	// HTTP server
	httpServer *http.Server
}

// RooCodeBridgeConfig contains configuration for the bridge
type RooCodeBridgeConfig struct {
	// Port is the Socket.IO server port (default: 9879)
	Port string

	// SessionID is the Helix session ID for logging
	SessionID string

	// Callbacks
	OnAgentReady     func()
	OnMessageAdded   func(content string, isComplete bool)
	OnMessageUpdated func(content string)
	OnError          func(err error)
}

// Roo Code protocol types
// These mirror the TypeScript types from packages/types/src/cloud.ts

// ExtensionBridgeCommand types
const (
	ExtCmdStartTask  = "start_task"
	ExtCmdStopTask   = "stop_task"
	ExtCmdResumeTask = "resume_task"
)

// TaskBridgeCommand types
const (
	TaskCmdMessage    = "message"
	TaskCmdApproveAsk = "approve_ask"
	TaskCmdDenyAsk    = "deny_ask"
)

// TaskBridgeEvent types
const (
	TaskEvtMessage          = "message"
	TaskEvtTaskModeSwitched = "task_mode_switched"
	TaskEvtTaskInteractive  = "task_interactive"
)

// ExtensionBridgeEvent types
const (
	ExtEvtTaskCreated     = "taskCreated"
	ExtEvtTaskStarted     = "taskStarted"
	ExtEvtTaskCompleted   = "taskCompleted"
	ExtEvtTaskAborted     = "taskAborted"
	ExtEvtTaskInteractive = "taskInteractive"
	ExtEvtTaskActive      = "taskActive"
)

// Socket event names (from Roo Code's cloud.ts)
const (
	SocketExtRegister        = "extension:register"
	SocketExtEvent           = "extension:event"
	SocketExtRelayedEvent    = "extension:relayed_event"
	SocketExtCommand         = "extension:command"
	SocketExtRelayedCommand  = "extension:relayed_command"
	SocketTaskJoin           = "task:join"
	SocketTaskLeave          = "task:leave"
	SocketTaskEvent          = "task:event"
	SocketTaskRelayedEvent   = "task:relayed_event"
	SocketTaskCommand        = "task:command"
	SocketTaskRelayedCommand = "task:relayed_command"
)

// ExtensionBridgeCommand represents a command to start/stop/resume tasks
type ExtensionBridgeCommand struct {
	Type       string                 `json:"type"`
	InstanceID string                 `json:"instanceId"`
	Payload    map[string]interface{} `json:"payload"`
	Timestamp  int64                  `json:"timestamp"`
}

// TaskBridgeCommand represents a command to interact with a running task
type TaskBridgeCommand struct {
	Type      string                 `json:"type"`
	TaskID    string                 `json:"taskId"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp int64                  `json:"timestamp"`
}

// TaskBridgeEvent represents an event from a running task
type TaskBridgeEvent struct {
	Type    string                 `json:"type"`
	TaskID  string                 `json:"taskId"`
	Action  string                 `json:"action,omitempty"`
	Mode    string                 `json:"mode,omitempty"`
	Message map[string]interface{} `json:"message,omitempty"`
}

// ExtensionBridgeEvent represents a lifecycle event from the extension
type ExtensionBridgeEvent struct {
	Type      string                 `json:"type"`
	Instance  map[string]interface{} `json:"instance,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

// NewRooCodeBridge creates a new Socket.IO server bridge
func NewRooCodeBridge(config RooCodeBridgeConfig) (*RooCodeBridge, error) {
	if config.Port == "" {
		config.Port = "9879"
	}

	bridge := &RooCodeBridge{
		port:             config.Port,
		sessionID:        config.SessionID,
		onAgentReady:     config.OnAgentReady,
		onMessageAdded:   config.OnMessageAdded,
		onMessageUpdated: config.OnMessageUpdated,
		onError:          config.OnError,
	}

	return bridge, nil
}

// Start starts the Socket.IO server
func (b *RooCodeBridge) Start() error {
	log.Info().
		Str("session_id", b.sessionID).
		Str("port", b.port).
		Msg("[RooCodeBridge] Starting Socket.IO server")

	// Create Socket.IO server
	server := socketio.NewServer(nil)
	b.server = server

	// Set up event handlers for the default namespace
	server.OnConnection(func(s socketio.Socket) {
		log.Info().
			Str("session_id", b.sessionID).
			Str("socket_id", s.Id()).
			Msg("[RooCodeBridge] Extension connected")

		b.setupSocketHandlers(s)
	})

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle("/socket.io/", server.ServeHandler(nil))

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Bridge config endpoint - Roo Code extension fetches this to get the Socket.IO URL
	// The extension uses ROO_CODE_API_URL env var to find this endpoint
	mux.HandleFunc("/api/extension/bridge/config", func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"socketBridgeUrl": fmt.Sprintf("http://localhost:%s", b.port),
			"enabled":         true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
		log.Debug().Str("session_id", b.sessionID).Msg("[RooCodeBridge] Served bridge config")
	})

	// Mock other Roo Code API endpoints that the extension might call
	mux.HandleFunc("/api/extension/user", func(w http.ResponseWriter, r *http.Request) {
		user := map[string]interface{}{
			"id":                     b.sessionID,
			"name":                   "Helix User",
			"extensionBridgeEnabled": true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	})

	b.httpServer = &http.Server{
		Addr:    ":" + b.port,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := b.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("[RooCodeBridge] Server error")
			if b.onError != nil {
				b.onError(err)
			}
		}
	}()

	log.Info().
		Str("session_id", b.sessionID).
		Str("port", b.port).
		Msg("[RooCodeBridge] Socket.IO server started")

	return nil
}

// setupSocketHandlers configures event handlers for a connected extension
func (b *RooCodeBridge) setupSocketHandlers(s socketio.Socket) {
	// Store the connected socket
	b.socketMu.Lock()
	b.extensionSocket = s
	b.socketMu.Unlock()

	// Extension registration
	s.On(SocketExtRegister, func(data string) {
		var registerData map[string]interface{}
		if err := json.Unmarshal([]byte(data), &registerData); err != nil {
			log.Warn().Err(err).Msg("[RooCodeBridge] Failed to parse register data")
			return
		}

		b.socketMu.Lock()
		if instanceID, ok := registerData["instanceId"].(string); ok {
			b.instanceID = instanceID
		}
		b.socketMu.Unlock()

		log.Info().
			Str("session_id", b.sessionID).
			Interface("register_data", registerData).
			Msg("[RooCodeBridge] Extension registered")

		// Send acknowledgment
		ackData, _ := json.Marshal(map[string]interface{}{
			"success":   true,
			"timestamp": time.Now().UnixMilli(),
		})
		s.Emit("extension:registered", string(ackData))
	})

	// Extension events (lifecycle)
	s.On(SocketExtEvent, func(data string) {
		var event ExtensionBridgeEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			log.Warn().Err(err).Msg("[RooCodeBridge] Failed to parse extension event")
			return
		}
		b.handleExtensionEvent(event)
	})

	// Task events (messages, state changes)
	s.On(SocketTaskEvent, func(data string) {
		var event TaskBridgeEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			log.Warn().Err(err).Msg("[RooCodeBridge] Failed to parse task event")
			return
		}
		b.handleTaskEvent(event)
	})

	// Task join (extension joining a task room)
	s.On(SocketTaskJoin, func(data string) {
		var joinData map[string]interface{}
		if err := json.Unmarshal([]byte(data), &joinData); err != nil {
			log.Warn().Err(err).Msg("[RooCodeBridge] Failed to parse join data")
			return
		}

		taskID, _ := joinData["taskId"].(string)
		log.Debug().
			Str("task_id", taskID).
			Str("session_id", b.sessionID).
			Msg("[RooCodeBridge] Extension joined task")

		// Send join response
		respData, _ := json.Marshal(map[string]interface{}{
			"success":   true,
			"taskId":    taskID,
			"timestamp": time.Now().UnixMilli(),
		})
		s.Emit("task:joined", string(respData))
	})

	// Handle disconnection
	s.OnDisconnect(func() {
		log.Warn().
			Str("session_id", b.sessionID).
			Msg("[RooCodeBridge] Extension disconnected")

		b.socketMu.Lock()
		b.extensionSocket = nil
		b.instanceID = ""
		b.socketMu.Unlock()
	})
}

// handleExtensionEvent processes lifecycle events from Roo Code
func (b *RooCodeBridge) handleExtensionEvent(event ExtensionBridgeEvent) {
	log.Debug().
		Str("type", event.Type).
		Str("session_id", b.sessionID).
		Msg("[RooCodeBridge] Extension event received")

	switch event.Type {
	case ExtEvtTaskCreated:
		// Extract task ID from instance
		if taskID, ok := b.extractTaskID(event.Instance); ok {
			b.taskMu.Lock()
			b.currentTaskID = taskID
			b.taskMu.Unlock()
			log.Info().Str("task_id", taskID).Msg("[RooCodeBridge] Task created")
		}

	case ExtEvtTaskInteractive, ExtEvtTaskActive:
		// Agent is ready for input
		log.Info().Str("session_id", b.sessionID).Msg("[RooCodeBridge] Agent ready")
		if b.onAgentReady != nil {
			b.onAgentReady()
		}

	case ExtEvtTaskCompleted:
		log.Info().Str("session_id", b.sessionID).Msg("[RooCodeBridge] Task completed")
		if b.onMessageAdded != nil {
			b.onMessageAdded("", true) // Signal completion
		}

	case ExtEvtTaskAborted:
		log.Warn().Str("session_id", b.sessionID).Msg("[RooCodeBridge] Task aborted")
		if b.onError != nil {
			b.onError(fmt.Errorf("task aborted"))
		}
	}
}

// handleTaskEvent processes messages and state changes from a running task
func (b *RooCodeBridge) handleTaskEvent(event TaskBridgeEvent) {
	log.Debug().
		Str("type", event.Type).
		Str("task_id", event.TaskID).
		Str("action", event.Action).
		Str("session_id", b.sessionID).
		Msg("[RooCodeBridge] Task event received")

	switch event.Type {
	case TaskEvtMessage:
		// Extract message content
		if content, ok := b.extractMessageContent(event.Message); ok {
			// Check if this is an "ask" that needs approval
			if event.Action == "ask" {
				// Auto-approve tool calls for automated operation
				b.approveAsk(event.TaskID, "")
			}

			if b.onMessageAdded != nil {
				b.onMessageAdded(content, false)
			}
		}

	case TaskEvtTaskInteractive:
		// Task is waiting for input
		if b.onAgentReady != nil {
			b.onAgentReady()
		}

	case TaskEvtTaskModeSwitched:
		log.Debug().
			Str("mode", event.Mode).
			Str("session_id", b.sessionID).
			Msg("[RooCodeBridge] Task mode switched")
	}
}

// extractTaskID gets the task ID from an instance object
func (b *RooCodeBridge) extractTaskID(instance map[string]interface{}) (string, bool) {
	if instance == nil {
		return "", false
	}
	if taskID, ok := instance["taskId"].(string); ok {
		return taskID, true
	}
	if task, ok := instance["task"].(map[string]interface{}); ok {
		if taskID, ok := task["id"].(string); ok {
			return taskID, true
		}
	}
	return "", false
}

// extractMessageContent gets the text content from a message object
func (b *RooCodeBridge) extractMessageContent(message map[string]interface{}) (string, bool) {
	if message == nil {
		return "", false
	}
	// Roo Code messages have various formats; try common patterns
	if text, ok := message["text"].(string); ok {
		return text, true
	}
	if content, ok := message["content"].(string); ok {
		return content, true
	}
	// For structured messages, serialize to JSON
	if len(message) > 0 {
		data, _ := json.Marshal(message)
		return string(data), true
	}
	return "", false
}

// StartTask sends a command to start a new task with the given prompt
func (b *RooCodeBridge) StartTask(prompt string) error {
	b.socketMu.RLock()
	socket := b.extensionSocket
	instanceID := b.instanceID
	b.socketMu.RUnlock()

	if socket == nil {
		return fmt.Errorf("no extension connected")
	}

	cmd := ExtensionBridgeCommand{
		Type:       ExtCmdStartTask,
		InstanceID: instanceID,
		Payload: map[string]interface{}{
			"text": prompt,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	socket.Emit(SocketExtRelayedCommand, string(data))

	log.Info().
		Str("session_id", b.sessionID).
		Int("prompt_len", len(prompt)).
		Msg("[RooCodeBridge] Started new task")

	return nil
}

// SendMessage sends a message to the current task
func (b *RooCodeBridge) SendMessage(message string) error {
	b.taskMu.RLock()
	taskID := b.currentTaskID
	b.taskMu.RUnlock()

	if taskID == "" {
		// No active task, start a new one
		return b.StartTask(message)
	}

	b.socketMu.RLock()
	socket := b.extensionSocket
	b.socketMu.RUnlock()

	if socket == nil {
		return fmt.Errorf("no extension connected")
	}

	cmd := TaskBridgeCommand{
		Type:   TaskCmdMessage,
		TaskID: taskID,
		Payload: map[string]interface{}{
			"text": message,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	socket.Emit(SocketTaskRelayedCommand, string(data))

	log.Debug().
		Str("task_id", taskID).
		Str("session_id", b.sessionID).
		Int("message_len", len(message)).
		Msg("[RooCodeBridge] Sent message to task")

	return nil
}

// approveAsk sends an approval for a tool call ask
func (b *RooCodeBridge) approveAsk(taskID string, response string) {
	b.socketMu.RLock()
	socket := b.extensionSocket
	b.socketMu.RUnlock()

	if socket == nil {
		log.Warn().Msg("[RooCodeBridge] Cannot approve ask - no extension connected")
		return
	}

	cmd := TaskBridgeCommand{
		Type:   TaskCmdApproveAsk,
		TaskID: taskID,
		Payload: map[string]interface{}{
			"text": response,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	data, _ := json.Marshal(cmd)
	socket.Emit(SocketTaskRelayedCommand, string(data))

	log.Debug().
		Str("task_id", taskID).
		Str("session_id", b.sessionID).
		Msg("[RooCodeBridge] Auto-approved ask")
}

// StopTask stops the current task
func (b *RooCodeBridge) StopTask() error {
	b.taskMu.RLock()
	taskID := b.currentTaskID
	b.taskMu.RUnlock()

	if taskID == "" {
		return nil // No task to stop
	}

	b.socketMu.RLock()
	socket := b.extensionSocket
	instanceID := b.instanceID
	b.socketMu.RUnlock()

	if socket == nil {
		return fmt.Errorf("no extension connected")
	}

	cmd := ExtensionBridgeCommand{
		Type:       ExtCmdStopTask,
		InstanceID: instanceID,
		Payload: map[string]interface{}{
			"taskId": taskID,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	socket.Emit(SocketExtRelayedCommand, string(data))

	log.Info().
		Str("task_id", taskID).
		Str("session_id", b.sessionID).
		Msg("[RooCodeBridge] Stopped task")

	return nil
}

// Close shuts down the Socket.IO server
func (b *RooCodeBridge) Close() error {
	if b.httpServer != nil {
		if err := b.httpServer.Close(); err != nil {
			log.Error().Err(err).Msg("[RooCodeBridge] Failed to close HTTP server")
		}
	}

	if b.server != nil {
		b.server.Close()
	}

	log.Info().
		Str("session_id", b.sessionID).
		Msg("[RooCodeBridge] Closed")

	return nil
}

// GetCurrentTaskID returns the current task ID (if any)
func (b *RooCodeBridge) GetCurrentTaskID() string {
	b.taskMu.RLock()
	defer b.taskMu.RUnlock()
	return b.currentTaskID
}

// IsConnected returns true if an extension is connected
func (b *RooCodeBridge) IsConnected() bool {
	b.socketMu.RLock()
	defer b.socketMu.RUnlock()
	return b.extensionSocket != nil
}
