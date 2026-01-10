package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// Zed communication protocol constants
const (
	// Streams and queues for Zed communication
	ZedInstanceManagementStream = "zed_instance_management"
	ZedInstanceManagementQueue  = "zed_instance_queue"
	ZedThreadManagementStream   = "zed_thread_management"
	ZedThreadManagementQueue    = "zed_thread_queue"
	ZedEventStream              = "zed_events"
	ZedEventQueue               = "zed_event_queue"

	// Protocol versions
	ZedProtocolVersion = "v1.0"
)

// ZedProtocolMessage represents the base structure for all Zed protocol messages
type ZedProtocolMessage struct {
	Version   string                 `json:"version"`
	MessageID string                 `json:"message_id"`
	Type      ZedMessageType         `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Metadata  ZedMessageMetadata     `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
}

// ZedMessageType defines the types of Zed protocol messages
type ZedMessageType string

const (
	// Instance management messages
	ZedMessageTypeInstanceCreate  ZedMessageType = "instance_create"
	ZedMessageTypeInstanceCreated ZedMessageType = "instance_created"
	ZedMessageTypeInstanceStop    ZedMessageType = "instance_stop"
	ZedMessageTypeInstanceStopped ZedMessageType = "instance_stopped"
	ZedMessageTypeInstanceStatus  ZedMessageType = "instance_status"
	ZedMessageTypeInstanceError   ZedMessageType = "instance_error"

	// Thread management messages
	ZedMessageTypeThreadCreate  ZedMessageType = "thread_create"
	ZedMessageTypeThreadCreated ZedMessageType = "thread_created"
	ZedMessageTypeThreadStop    ZedMessageType = "thread_stop"
	ZedMessageTypeThreadStopped ZedMessageType = "thread_stopped"
	ZedMessageTypeThreadStatus  ZedMessageType = "thread_status"
	ZedMessageTypeThreadError   ZedMessageType = "thread_error"

	// Event messages
	ZedMessageTypeHeartbeat      ZedMessageType = "heartbeat"
	ZedMessageTypeActivityUpdate ZedMessageType = "activity_update"
	ZedMessageTypeProgressUpdate ZedMessageType = "progress_update"
	ZedMessageTypeCoordination   ZedMessageType = "coordination"

	// Legacy single-session messages
	ZedMessageTypeLegacyStart ZedMessageType = "legacy_start"
	ZedMessageTypeLegacyStop  ZedMessageType = "legacy_stop"
)

// ZedMessageMetadata contains contextual information about the message
type ZedMessageMetadata struct {
	SpecTaskID    string `json:"spec_task_id,omitempty"`
	WorkSessionID string `json:"work_session_id,omitempty"`
	InstanceID    string `json:"instance_id,omitempty"`
	ThreadID      string `json:"thread_id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"` // Helix session ID
	Priority      int    `json:"priority,omitempty"`
	TTL           int    `json:"ttl,omitempty"` // Time to live in seconds
	RetryCount    int    `json:"retry_count,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Instance management protocol structures

// ZedInstanceCreateRequest represents a request to create a new Zed instance
type ZedInstanceCreateRequest struct {
	InstanceID      string                 `json:"instance_id"`
	SpecTaskID      string                 `json:"spec_task_id"`
	UserID          string                 `json:"user_id"`
	ProjectPath     string                 `json:"project_path"`
	WorkDir         string                 `json:"work_dir"`
	WorkspaceConfig map[string]interface{} `json:"workspace_config"`
	Environment     []string               `json:"environment"`
	InitialThreads  []ZedThreadConfig      `json:"initial_threads,omitempty"`
}

// ZedInstanceCreateResponse represents the response to instance creation
type ZedInstanceCreateResponse struct {
	InstanceID    string    `json:"instance_id"`
	Status        string    `json:"status"`
	WebSocketURL  string    `json:"websocket_url,omitempty"`
	AuthToken     string    `json:"auth_token,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	EstimatedTime int       `json:"estimated_startup_time_seconds"`
}

// ZedInstanceStatusUpdate represents status updates from Zed instances
type ZedInstanceStatusUpdate struct {
	InstanceID     string                 `json:"instance_id"`
	Status         string                 `json:"status"`
	ThreadCount    int                    `json:"thread_count"`
	ActiveThreads  int                    `json:"active_threads"`
	LastActivity   time.Time              `json:"last_activity"`
	ResourceUsage  ZedResourceUsage       `json:"resource_usage"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	AdditionalInfo map[string]interface{} `json:"additional_info,omitempty"`
}

// Thread management protocol structures

// ZedThreadConfig represents configuration for creating a Zed thread
type ZedThreadConfig struct {
	ThreadID                string                 `json:"thread_id"`
	WorkSessionID           string                 `json:"work_session_id"`
	Name                    string                 `json:"name"`
	Description             string                 `json:"description"`
	ImplementationTaskTitle string                 `json:"implementation_task_title"`
	ImplementationTaskIndex int                    `json:"implementation_task_index"`
	AgentConfig             map[string]interface{} `json:"agent_config,omitempty"`
	Environment             []string               `json:"environment,omitempty"`
	InitialPrompt           string                 `json:"initial_prompt,omitempty"`
}

// ZedThreadCreateRequest represents a request to create a thread within an instance
type ZedThreadCreateRequest struct {
	InstanceID string          `json:"instance_id"`
	Thread     ZedThreadConfig `json:"thread"`
}

// ZedThreadCreateResponse represents the response to thread creation
type ZedThreadCreateResponse struct {
	InstanceID string    `json:"instance_id"`
	ThreadID   string    `json:"thread_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ZedThreadStatusUpdate represents status updates from Zed threads
type ZedThreadStatusUpdate struct {
	InstanceID    string                 `json:"instance_id"`
	ThreadID      string                 `json:"thread_id"`
	WorkSessionID string                 `json:"work_session_id"`
	Status        string                 `json:"status"`
	Progress      float64                `json:"progress,omitempty"`
	LastActivity  time.Time              `json:"last_activity"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Output        string                 `json:"output,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// Event protocol structures

// ZedHeartbeatMessage represents periodic heartbeat from Zed instances
type ZedHeartbeatMessage struct {
	InstanceID       string            `json:"instance_id"`
	Status           string            `json:"status"`
	ThreadStatuses   map[string]string `json:"thread_statuses"`
	ResourceUsage    ZedResourceUsage  `json:"resource_usage"`
	LastActivity     time.Time         `json:"last_activity"`
	UptimeSeconds    int               `json:"uptime_seconds"`
	PendingCommands  int               `json:"pending_commands"`
	QueuedOperations []string          `json:"queued_operations,omitempty"`
}

// ZedActivityUpdate represents activity updates from Zed threads
type ZedActivityUpdate struct {
	InstanceID   string                 `json:"instance_id"`
	ThreadID     string                 `json:"thread_id"`
	ActivityType ZedActivityType        `json:"activity_type"`
	Description  string                 `json:"description"`
	Progress     float64                `json:"progress,omitempty"`
	FilesChanged []string               `json:"files_changed,omitempty"`
	LinesAdded   int                    `json:"lines_added,omitempty"`
	LinesRemoved int                    `json:"lines_removed,omitempty"`
	TestsRun     int                    `json:"tests_run,omitempty"`
	TestsPassed  int                    `json:"tests_passed,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
}

// ZedCoordinationMessage represents coordination between threads
type ZedCoordinationMessage struct {
	InstanceID       string                 `json:"instance_id"`
	FromThreadID     string                 `json:"from_thread_id"`
	ToThreadID       string                 `json:"to_thread_id,omitempty"` // Empty means broadcast
	CoordinationType ZedCoordinationType    `json:"coordination_type"`
	Message          string                 `json:"message"`
	Data             map[string]interface{} `json:"data,omitempty"`
	Timestamp        time.Time              `json:"timestamp"`
}

// Enums and supporting types

// ZedActivityType represents types of activities in Zed threads
type ZedActivityType string

const (
	ZedActivityTypeCodeEdit    ZedActivityType = "code_edit"
	ZedActivityTypeFileCreate  ZedActivityType = "file_create"
	ZedActivityTypeFileDelete  ZedActivityType = "file_delete"
	ZedActivityTypeTestRun     ZedActivityType = "test_run"
	ZedActivityTypeBuildRun    ZedActivityType = "build_run"
	ZedActivityTypeDebugStart  ZedActivityType = "debug_start"
	ZedActivityTypeGitCommit   ZedActivityType = "git_commit"
	ZedActivityTypeGitPush     ZedActivityType = "git_push"
	ZedActivityTypeTerminalCmd ZedActivityType = "terminal_cmd"
	ZedActivityTypeThinking    ZedActivityType = "thinking"
	ZedActivityTypeCompletion  ZedActivityType = "completion"
)

// ZedCoordinationType represents types of coordination between threads
type ZedCoordinationType string

const (
	ZedCoordinationTypeHandoff      ZedCoordinationType = "handoff"
	ZedCoordinationTypeBlocking     ZedCoordinationType = "blocking"
	ZedCoordinationTypeNotification ZedCoordinationType = "notification"
	ZedCoordinationTypeRequest      ZedCoordinationType = "request"
	ZedCoordinationTypeResponse     ZedCoordinationType = "response"
	ZedCoordinationTypeBroadcast    ZedCoordinationType = "broadcast"
)

// ZedResourceUsage represents resource usage statistics
type ZedResourceUsage struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryMB      int     `json:"memory_mb"`
	DiskMB        int     `json:"disk_mb"`
	NetworkKbps   int     `json:"network_kbps"`
	ThreadCount   int     `json:"thread_count"`
	ProcessCount  int     `json:"process_count"`
	OpenFiles     int     `json:"open_files"`
	UptimeSeconds int     `json:"uptime_seconds"`
}

// Protocol helper functions

// NewZedProtocolMessage creates a new protocol message with standard metadata
func NewZedProtocolMessage(msgType ZedMessageType, data map[string]interface{}, metadata ZedMessageMetadata) *ZedProtocolMessage {
	return &ZedProtocolMessage{
		Version:   ZedProtocolVersion,
		MessageID: generateMessageID(),
		Type:      msgType,
		Data:      data,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}
}

// CreateInstanceCreateMessage creates a message to request instance creation
func CreateInstanceCreateMessage(req *ZedInstanceCreateRequest) *ZedProtocolMessage {
	data := map[string]interface{}{
		"instance_id":      req.InstanceID,
		"spec_task_id":     req.SpecTaskID,
		"user_id":          req.UserID,
		"project_path":     req.ProjectPath,
		"work_dir":         req.WorkDir,
		"workspace_config": req.WorkspaceConfig,
		"environment":      req.Environment,
		"initial_threads":  req.InitialThreads,
	}

	metadata := ZedMessageMetadata{
		SpecTaskID: req.SpecTaskID,
		InstanceID: req.InstanceID,
		UserID:     req.UserID,
		Priority:   1,
		TTL:        300, // 5 minutes
	}

	return NewZedProtocolMessage(ZedMessageTypeInstanceCreate, data, metadata)
}

// CreateThreadCreateMessage creates a message to request thread creation
func CreateThreadCreateMessage(req *ZedThreadCreateRequest) *ZedProtocolMessage {
	data := map[string]interface{}{
		"instance_id": req.InstanceID,
		"thread":      req.Thread,
	}

	metadata := ZedMessageMetadata{
		InstanceID:    req.InstanceID,
		ThreadID:      req.Thread.ThreadID,
		WorkSessionID: req.Thread.WorkSessionID,
		Priority:      2,
		TTL:           180, // 3 minutes
	}

	return NewZedProtocolMessage(ZedMessageTypeThreadCreate, data, metadata)
}

// CreateHeartbeatMessage creates a heartbeat message from Zed instance
func CreateHeartbeatMessage(heartbeat *ZedHeartbeatMessage) *ZedProtocolMessage {
	data := map[string]interface{}{
		"instance_id":       heartbeat.InstanceID,
		"status":            heartbeat.Status,
		"thread_statuses":   heartbeat.ThreadStatuses,
		"resource_usage":    heartbeat.ResourceUsage,
		"last_activity":     heartbeat.LastActivity,
		"uptime_seconds":    heartbeat.UptimeSeconds,
		"pending_commands":  heartbeat.PendingCommands,
		"queued_operations": heartbeat.QueuedOperations,
	}

	metadata := ZedMessageMetadata{
		InstanceID: heartbeat.InstanceID,
		Priority:   3,  // Lower priority for heartbeats
		TTL:        60, // 1 minute
	}

	return NewZedProtocolMessage(ZedMessageTypeHeartbeat, data, metadata)
}

// CreateActivityUpdateMessage creates an activity update message from Zed thread
func CreateActivityUpdateMessage(update *ZedActivityUpdate) *ZedProtocolMessage {
	data := map[string]interface{}{
		"instance_id":   update.InstanceID,
		"thread_id":     update.ThreadID,
		"activity_type": update.ActivityType,
		"description":   update.Description,
		"progress":      update.Progress,
		"files_changed": update.FilesChanged,
		"lines_added":   update.LinesAdded,
		"lines_removed": update.LinesRemoved,
		"tests_run":     update.TestsRun,
		"tests_passed":  update.TestsPassed,
		"metadata":      update.Metadata,
	}

	metadata := ZedMessageMetadata{
		InstanceID: update.InstanceID,
		ThreadID:   update.ThreadID,
		Priority:   2,
		TTL:        120, // 2 minutes
	}

	return NewZedProtocolMessage(ZedMessageTypeActivityUpdate, data, metadata)
}

// CreateCoordinationMessage creates a coordination message between threads
func CreateCoordinationMessage(coord *ZedCoordinationMessage) *ZedProtocolMessage {
	data := map[string]interface{}{
		"instance_id":       coord.InstanceID,
		"from_thread_id":    coord.FromThreadID,
		"to_thread_id":      coord.ToThreadID,
		"coordination_type": coord.CoordinationType,
		"message":           coord.Message,
		"data":              coord.Data,
	}

	metadata := ZedMessageMetadata{
		InstanceID: coord.InstanceID,
		ThreadID:   coord.FromThreadID,
		Priority:   1, // High priority for coordination
		TTL:        180,
	}

	return NewZedProtocolMessage(ZedMessageTypeCoordination, data, metadata)
}

// Protocol parsing and validation functions

// ParseZedProtocolMessage parses a raw message into a ZedProtocolMessage
func ParseZedProtocolMessage(rawData []byte) (*ZedProtocolMessage, error) {
	var msg ZedProtocolMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse Zed protocol message: %w", err)
	}

	// Validate message
	if err := validateZedProtocolMessage(&msg); err != nil {
		return nil, fmt.Errorf("invalid Zed protocol message: %w", err)
	}

	return &msg, nil
}

// validateZedProtocolMessage validates a Zed protocol message
func validateZedProtocolMessage(msg *ZedProtocolMessage) error {
	if msg.Version == "" {
		return fmt.Errorf("version is required")
	}
	if msg.MessageID == "" {
		return fmt.Errorf("message_id is required")
	}
	if msg.Type == "" {
		return fmt.Errorf("type is required")
	}
	if msg.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	// Validate metadata based on message type
	switch msg.Type {
	case ZedMessageTypeInstanceCreate, ZedMessageTypeInstanceCreated:
		if msg.Metadata.InstanceID == "" {
			return fmt.Errorf("instance_id required for instance messages")
		}
	case ZedMessageTypeThreadCreate, ZedMessageTypeThreadCreated, ZedMessageTypeThreadStatus:
		if msg.Metadata.InstanceID == "" {
			return fmt.Errorf("instance_id required for thread messages")
		}
		if msg.Metadata.ThreadID == "" {
			return fmt.Errorf("thread_id required for thread messages")
		}
	}

	return nil
}

// Protocol conversion functions

// ConvertZedAgentToInstanceRequest converts a ZedAgent to instance creation request
func ConvertZedAgentToInstanceRequest(agent *types.DesktopAgent) *ZedInstanceCreateRequest {
	req := &ZedInstanceCreateRequest{
		InstanceID:      agent.InstanceID,
		SpecTaskID:      agent.SessionID, // SessionID contains SpecTask ID for instances
		UserID:          agent.UserID,
		ProjectPath:     agent.ProjectPath,
		WorkDir:         agent.WorkDir,
		Environment:     agent.Env,
		WorkspaceConfig: make(map[string]interface{}),
	}

	// If a specific thread is requested, add it as initial thread
	if agent.ThreadID != "" {
		req.InitialThreads = []ZedThreadConfig{
			{
				ThreadID:      agent.ThreadID,
				WorkSessionID: agent.SessionID, // For initial thread, SessionID is work session ID
				Name:          "Initial Implementation",
				Description:   agent.Input,
			},
		}
	}

	return req
}

// ConvertInstanceResponseToZedAgentResponse converts instance response to ZedAgent response
func ConvertInstanceResponseToZedAgentResponse(resp *ZedInstanceCreateResponse, sessionID string) *types.DesktopAgentResponse {
	return &types.DesktopAgentResponse{
		SessionID:    sessionID,
		WebSocketURL: resp.WebSocketURL,
		AuthToken:    resp.AuthToken,
		Status:       resp.Status,
	}
}

// Message serialization helpers

// SerializeZedMessage serializes a ZedProtocolMessage for transmission
func SerializeZedMessage(msg *ZedProtocolMessage) ([]byte, error) {
	return json.Marshal(msg)
}

// CreateMessageHeaders creates pub/sub headers for a Zed message
func CreateMessageHeaders(msg *ZedProtocolMessage) map[string]string {
	headers := map[string]string{
		"protocol_version": msg.Version,
		"message_type":     string(msg.Type),
		"message_id":       msg.MessageID,
		"timestamp":        msg.Timestamp.Format(time.RFC3339),
	}

	// Add metadata to headers for routing
	if msg.Metadata.SpecTaskID != "" {
		headers["spec_task_id"] = msg.Metadata.SpecTaskID
	}
	if msg.Metadata.InstanceID != "" {
		headers["instance_id"] = msg.Metadata.InstanceID
	}
	if msg.Metadata.ThreadID != "" {
		headers["thread_id"] = msg.Metadata.ThreadID
	}
	if msg.Metadata.UserID != "" {
		headers["user_id"] = msg.Metadata.UserID
	}
	if msg.Metadata.Priority > 0 {
		headers["priority"] = fmt.Sprintf("%d", msg.Metadata.Priority)
	}

	return headers
}

// Utility functions

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("zed_msg_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// GetStreamForMessageType returns the appropriate stream for a message type
func GetStreamForMessageType(msgType ZedMessageType) string {
	switch msgType {
	case ZedMessageTypeInstanceCreate, ZedMessageTypeInstanceStop, ZedMessageTypeInstanceStatus:
		return ZedInstanceManagementStream
	case ZedMessageTypeThreadCreate, ZedMessageTypeThreadStop, ZedMessageTypeThreadStatus:
		return ZedThreadManagementStream
	case ZedMessageTypeHeartbeat, ZedMessageTypeActivityUpdate, ZedMessageTypeProgressUpdate, ZedMessageTypeCoordination:
		return ZedEventStream
	default:
		return ZedEventStream // Default to event stream
	}
}

// GetQueueForMessageType returns the appropriate queue for a message type
func GetQueueForMessageType(msgType ZedMessageType) string {
	switch msgType {
	case ZedMessageTypeInstanceCreate, ZedMessageTypeInstanceStop, ZedMessageTypeInstanceStatus:
		return ZedInstanceManagementQueue
	case ZedMessageTypeThreadCreate, ZedMessageTypeThreadStop, ZedMessageTypeThreadStatus:
		return ZedThreadManagementQueue
	case ZedMessageTypeHeartbeat, ZedMessageTypeActivityUpdate, ZedMessageTypeProgressUpdate, ZedMessageTypeCoordination:
		return ZedEventQueue
	default:
		return ZedEventQueue // Default to event queue
	}
}

// Protocol client interface for services

// ZedProtocolClient provides a high-level interface for Zed protocol communication
type ZedProtocolClient struct {
	pubsub PubSub
}

// NewZedProtocolClient creates a new Zed protocol client
func NewZedProtocolClient(pubsub PubSub) *ZedProtocolClient {
	return &ZedProtocolClient{
		pubsub: pubsub,
	}
}

// SendInstanceCreateRequest sends a request to create a Zed instance
func (c *ZedProtocolClient) SendInstanceCreateRequest(ctx context.Context, req *ZedInstanceCreateRequest) error {
	msg := CreateInstanceCreateMessage(req)
	data, err := SerializeZedMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	headers := CreateMessageHeaders(msg)
	stream := GetStreamForMessageType(msg.Type)
	queue := GetQueueForMessageType(msg.Type)

	_, err = c.pubsub.StreamRequest(ctx, stream, queue, data, headers, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send instance create request: %w", err)
	}

	return nil
}

// SendThreadCreateRequest sends a request to create a Zed thread
func (c *ZedProtocolClient) SendThreadCreateRequest(ctx context.Context, req *ZedThreadCreateRequest) error {
	msg := CreateThreadCreateMessage(req)
	data, err := SerializeZedMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	headers := CreateMessageHeaders(msg)
	stream := GetStreamForMessageType(msg.Type)
	queue := GetQueueForMessageType(msg.Type)

	_, err = c.pubsub.StreamRequest(ctx, stream, queue, data, headers, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send thread create request: %w", err)
	}

	return nil
}

// SendHeartbeat sends a heartbeat message from a Zed instance
func (c *ZedProtocolClient) SendHeartbeat(ctx context.Context, heartbeat *ZedHeartbeatMessage) error {
	msg := CreateHeartbeatMessage(heartbeat)
	data, err := SerializeZedMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize heartbeat: %w", err)
	}

	stream := GetStreamForMessageType(msg.Type)

	return c.pubsub.Publish(ctx, stream, data)
}

// SendActivityUpdate sends an activity update from a Zed thread
func (c *ZedProtocolClient) SendActivityUpdate(ctx context.Context, update *ZedActivityUpdate) error {
	msg := CreateActivityUpdateMessage(update)
	data, err := SerializeZedMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize activity update: %w", err)
	}

	stream := GetStreamForMessageType(msg.Type)

	return c.pubsub.Publish(ctx, stream, data)
}

// SendCoordinationMessage sends a coordination message between threads
func (c *ZedProtocolClient) SendCoordinationMessage(ctx context.Context, coord *ZedCoordinationMessage) error {
	msg := CreateCoordinationMessage(coord)
	data, err := SerializeZedMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize coordination message: %w", err)
	}

	stream := GetStreamForMessageType(msg.Type)

	return c.pubsub.Publish(ctx, stream, data)
}
