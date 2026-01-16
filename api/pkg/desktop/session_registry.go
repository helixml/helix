// Package desktop provides multi-player cursor and presence management.
package desktop

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// User colors for multi-player cursors (Figma-style palette)
var userColors = []string{
	"#F24822", // Red-orange
	"#FF7262", // Coral
	"#FFCD29", // Yellow
	"#14AE5C", // Green
	"#0D99FF", // Blue
	"#9747FF", // Purple
	"#FF6B6B", // Salmon
	"#4ECDC4", // Teal
	"#45B7D1", // Sky blue
	"#96CEB4", // Sage
	"#FFEAA7", // Cream yellow
	"#DDA0DD", // Plum
}

// ConnectedClient represents a client connected to a streaming session
type ConnectedClient struct {
	ID        uint32          // Unique client ID within session
	UserID    string          // Helix user ID (from auth)
	UserName  string          // Display name
	AvatarURL string          // Avatar URL (optional)
	Color     string          // Assigned color from palette
	Conn      *websocket.Conn // WebSocket connection
	LastX     int32           // Last known cursor X position
	LastY     int32           // Last known cursor Y position
	LastSeen  time.Time       // Last activity timestamp
	mu        sync.Mutex      // Protects writes to connection
}

// SessionRegistry manages connected clients for all sessions
type SessionRegistry struct {
	sessions sync.Map // map[sessionID]*SessionClients
	nextID   atomic.Uint32
}

// SessionClients holds all clients for a single session
type SessionClients struct {
	clients   sync.Map // map[clientID]*ConnectedClient
	colorIdx  int      // Next color index to assign
	colorLock sync.Mutex
}

// Global session registry
var globalRegistry = &SessionRegistry{}

// GetSessionRegistry returns the global session registry
func GetSessionRegistry() *SessionRegistry {
	return globalRegistry
}

// RegisterClient adds a new client to a session and returns assigned client ID
func (r *SessionRegistry) RegisterClient(sessionID string, userID, userName, avatarURL string, conn *websocket.Conn) *ConnectedClient {
	// Get or create session
	sessionI, _ := r.sessions.LoadOrStore(sessionID, &SessionClients{})
	session := sessionI.(*SessionClients)

	// Assign client ID and color
	clientID := r.nextID.Add(1)

	session.colorLock.Lock()
	color := userColors[session.colorIdx%len(userColors)]
	session.colorIdx++
	session.colorLock.Unlock()

	client := &ConnectedClient{
		ID:        clientID,
		UserID:    userID,
		UserName:  userName,
		AvatarURL: avatarURL,
		Color:     color,
		Conn:      conn,
		LastSeen:  time.Now(),
	}

	session.clients.Store(clientID, client)

	// Broadcast user joined to all other clients
	r.broadcastUserJoined(sessionID, client)

	// Send existing users to the new client
	r.sendExistingUsers(sessionID, client)

	return client
}

// UnregisterClient removes a client from a session
func (r *SessionRegistry) UnregisterClient(sessionID string, clientID uint32) {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return
	}
	session := sessionI.(*SessionClients)
	session.clients.Delete(clientID)

	// Broadcast user left to remaining clients
	r.broadcastUserLeft(sessionID, clientID)
}

// BroadcastCursorPosition sends cursor position to all other clients in the session
func (r *SessionRegistry) BroadcastCursorPosition(sessionID string, fromClientID uint32, x, y int32) {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return
	}
	session := sessionI.(*SessionClients)

	// Update sender's position
	if clientI, ok := session.clients.Load(fromClientID); ok {
		client := clientI.(*ConnectedClient)
		client.LastX = x
		client.LastY = y
		client.LastSeen = time.Now()
	}

	// Build RemoteCursor message (0x53)
	// Format: type(1) + userId(4) + x(4) + y(4) + colorLen(1) + color(N)
	var color string
	if clientI, ok := session.clients.Load(fromClientID); ok {
		color = clientI.(*ConnectedClient).Color
	}
	// Default color if client not found (e.g., clientID=0 from shared input server)
	if color == "" {
		color = "#0D99FF" // Default blue
	}

	colorBytes := []byte(color)
	msg := make([]byte, 1+4+4+4+1+len(colorBytes))
	msg[0] = StreamMsgRemoteCursor
	binary.LittleEndian.PutUint32(msg[1:5], fromClientID)
	binary.LittleEndian.PutUint32(msg[5:9], uint32(x))
	binary.LittleEndian.PutUint32(msg[9:13], uint32(y))
	msg[13] = byte(len(colorBytes))
	copy(msg[14:], colorBytes)

	// Broadcast to all OTHER clients
	session.clients.Range(func(key, value any) bool {
		client := value.(*ConnectedClient)
		if client.ID == fromClientID {
			return true // Skip sender
		}
		client.sendMessage(websocket.BinaryMessage, msg)
		return true
	})
}

// GetConnectedUsers returns all connected users in a session (including self)
func (r *SessionRegistry) GetConnectedUsers(sessionID string) []*ConnectedClient {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return nil
	}
	session := sessionI.(*SessionClients)

	var clients []*ConnectedClient
	session.clients.Range(func(key, value any) bool {
		clients = append(clients, value.(*ConnectedClient))
		return true
	})
	return clients
}

// broadcastUserJoined sends RemoteUser message to all clients
func (r *SessionRegistry) broadcastUserJoined(sessionID string, newClient *ConnectedClient) {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return
	}
	session := sessionI.(*SessionClients)

	// Build RemoteUser message (0x54)
	// Format: type(1) + action(1) + userId(4) + nameLen(1) + name(N) + colorLen(1) + color(N) + avatarLen(2) + avatar(N)
	nameBytes := []byte(newClient.UserName)
	colorBytes := []byte(newClient.Color)
	avatarBytes := []byte(newClient.AvatarURL)

	msgLen := 1 + 1 + 4 + 1 + len(nameBytes) + 1 + len(colorBytes) + 2 + len(avatarBytes)
	msg := make([]byte, msgLen)
	offset := 0

	msg[offset] = StreamMsgRemoteUser
	offset++
	msg[offset] = 0x01 // Action: joined
	offset++
	binary.LittleEndian.PutUint32(msg[offset:offset+4], newClient.ID)
	offset += 4
	msg[offset] = byte(len(nameBytes))
	offset++
	copy(msg[offset:], nameBytes)
	offset += len(nameBytes)
	msg[offset] = byte(len(colorBytes))
	offset++
	copy(msg[offset:], colorBytes)
	offset += len(colorBytes)
	binary.LittleEndian.PutUint16(msg[offset:offset+2], uint16(len(avatarBytes)))
	offset += 2
	copy(msg[offset:], avatarBytes)

	// Broadcast to all clients (including new client, so they know their own info)
	session.clients.Range(func(key, value any) bool {
		client := value.(*ConnectedClient)
		client.sendMessage(websocket.BinaryMessage, msg)
		return true
	})
}

// sendExistingUsers sends all existing users to a new client
func (r *SessionRegistry) sendExistingUsers(sessionID string, newClient *ConnectedClient) {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return
	}
	session := sessionI.(*SessionClients)

	session.clients.Range(func(key, value any) bool {
		existingClient := value.(*ConnectedClient)
		if existingClient.ID == newClient.ID {
			return true // Skip self (already sent in broadcastUserJoined)
		}

		// Build RemoteUser message for existing client
		nameBytes := []byte(existingClient.UserName)
		colorBytes := []byte(existingClient.Color)
		avatarBytes := []byte(existingClient.AvatarURL)

		msgLen := 1 + 1 + 4 + 1 + len(nameBytes) + 1 + len(colorBytes) + 2 + len(avatarBytes)
		msg := make([]byte, msgLen)
		offset := 0

		msg[offset] = StreamMsgRemoteUser
		offset++
		msg[offset] = 0x01 // Action: joined (existing user)
		offset++
		binary.LittleEndian.PutUint32(msg[offset:offset+4], existingClient.ID)
		offset += 4
		msg[offset] = byte(len(nameBytes))
		offset++
		copy(msg[offset:], nameBytes)
		offset += len(nameBytes)
		msg[offset] = byte(len(colorBytes))
		offset++
		copy(msg[offset:], colorBytes)
		offset += len(colorBytes)
		binary.LittleEndian.PutUint16(msg[offset:offset+2], uint16(len(avatarBytes)))
		offset += 2
		copy(msg[offset:], avatarBytes)

		newClient.sendMessage(websocket.BinaryMessage, msg)
		return true
	})
}

// broadcastUserLeft sends user left message to all clients
func (r *SessionRegistry) broadcastUserLeft(sessionID string, leftClientID uint32) {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return
	}
	session := sessionI.(*SessionClients)

	// Build RemoteUser message for leave (0x54)
	// Format: type(1) + action(1) + userId(4)
	msg := make([]byte, 6)
	msg[0] = StreamMsgRemoteUser
	msg[1] = 0x00 // Action: left
	binary.LittleEndian.PutUint32(msg[2:6], leftClientID)

	session.clients.Range(func(key, value any) bool {
		client := value.(*ConnectedClient)
		client.sendMessage(websocket.BinaryMessage, msg)
		return true
	})
}

// sendMessage safely sends a message to a client's WebSocket
func (c *ConnectedClient) sendMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(messageType, data)
}

// AgentAction represents the type of action the agent is performing
type AgentAction byte

const (
	AgentActionIdle      AgentAction = 0
	AgentActionMoving    AgentAction = 1
	AgentActionClicking  AgentAction = 2
	AgentActionTyping    AgentAction = 3
	AgentActionScrolling AgentAction = 4
	AgentActionDragging  AgentAction = 5
)

// TouchEventType represents the type of touch event
type TouchEventType byte

const (
	TouchEventStart  TouchEventType = 0
	TouchEventMove   TouchEventType = 1
	TouchEventEnd    TouchEventType = 2
	TouchEventCancel TouchEventType = 3
)

// BroadcastTouchEvent sends touch event to all other clients in the session
func (r *SessionRegistry) BroadcastTouchEvent(sessionID string, fromClientID uint32, touchID uint32, eventType TouchEventType, x, y int32, pressure float32) {
	sessionI, ok := r.sessions.Load(sessionID)
	if !ok {
		return
	}
	session := sessionI.(*SessionClients)

	// Get sender's color for the touch indicator
	var color string
	if clientI, ok := session.clients.Load(fromClientID); ok {
		color = clientI.(*ConnectedClient).Color
	}
	// Default color if client not found (e.g., clientID=0 from shared input server)
	if color == "" {
		color = "#0D99FF" // Default blue
	}

	// Build RemoteTouch message (0x56)
	// Format: type(1) + userId(4) + touchId(4) + eventType(1) + x(4) + y(4) + pressure(4) + colorLen(1) + color(N)
	colorBytes := []byte(color)
	msg := make([]byte, 1+4+4+1+4+4+4+1+len(colorBytes))
	offset := 0

	msg[offset] = StreamMsgRemoteTouch
	offset++
	binary.LittleEndian.PutUint32(msg[offset:offset+4], fromClientID)
	offset += 4
	binary.LittleEndian.PutUint32(msg[offset:offset+4], touchID)
	offset += 4
	msg[offset] = byte(eventType)
	offset++
	binary.LittleEndian.PutUint32(msg[offset:offset+4], uint32(x))
	offset += 4
	binary.LittleEndian.PutUint32(msg[offset:offset+4], uint32(y))
	offset += 4
	// Encode pressure as fixed-point (multiply by 65535 to fit in uint32)
	binary.LittleEndian.PutUint32(msg[offset:offset+4], uint32(pressure*65535))
	offset += 4
	msg[offset] = byte(len(colorBytes))
	offset++
	copy(msg[offset:], colorBytes)

	// Broadcast to all OTHER clients
	session.clients.Range(func(key, value any) bool {
		client := value.(*ConnectedClient)
		if client.ID == fromClientID {
			return true // Skip sender
		}
		client.sendMessage(websocket.BinaryMessage, msg)
		return true
	})
}

// BroadcastAgentCursor sends agent cursor position to all clients in all sessions
// This is called from the MCP server when the agent performs mouse/keyboard actions
func (r *SessionRegistry) BroadcastAgentCursor(x, y int32, action AgentAction, visible bool) {
	// Build AgentCursor message (0x55)
	// Format: type(1) + agentId(4) + x(2) + y(2) + action(1) + visible(1)
	msg := make([]byte, 11)
	msg[0] = StreamMsgAgentCursor
	binary.LittleEndian.PutUint32(msg[1:5], 1) // Agent ID = 1 (single agent per session)
	binary.LittleEndian.PutUint16(msg[5:7], uint16(x))
	binary.LittleEndian.PutUint16(msg[7:9], uint16(y))
	msg[9] = byte(action)
	if visible {
		msg[10] = 1
	} else {
		msg[10] = 0
	}

	// Broadcast to all sessions (typically just one per desktop container)
	r.sessions.Range(func(sessionID, sessionI any) bool {
		session := sessionI.(*SessionClients)
		session.clients.Range(func(key, value any) bool {
			client := value.(*ConnectedClient)
			client.sendMessage(websocket.BinaryMessage, msg)
			return true
		})
		return true
	})
}
