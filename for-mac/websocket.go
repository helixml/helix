package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message types for the video protocol
const (
	MsgTypeVideoFrame  = 0x01
	MsgTypeVideoBatch  = 0x03
	MsgTypeStreamInit  = 0x30
	MsgTypeStreamError = 0x31
	MsgTypePing        = 0x40
	MsgTypePong        = 0x41
)

// StreamInit contains stream initialization data
type StreamInit struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Framerate int    `json:"framerate"`
	Codec     string `json:"codec"`
}

// VideoServer handles WebSocket connections for video streaming
type VideoServer struct {
	port      int
	upgrader  websocket.Upgrader
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	broadcast chan []byte
	encoder   *VideoEncoder
	ctx       context.Context
	cancel    context.CancelFunc
	server    *http.Server
}

// NewVideoServer creates a new video server
func NewVideoServer(port int, encoder *VideoEncoder) *VideoServer {
	return &VideoServer{
		port:      port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for local development
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 65536,
		},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte, 100),
		encoder:   encoder,
	}
}

// Start starts the WebSocket server
func (vs *VideoServer) Start() error {
	vs.ctx, vs.cancel = context.WithCancel(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", vs.handleStream)
	mux.HandleFunc("/health", vs.handleHealth)

	vs.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", vs.port),
		Handler: mux,
	}

	// Start broadcast goroutine
	go vs.broadcastLoop()

	log.Printf("Video WebSocket server starting on port %d", vs.port)

	go func() {
		if err := vs.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the WebSocket server
func (vs *VideoServer) Stop() error {
	if vs.cancel != nil {
		vs.cancel()
	}

	// Close all client connections
	vs.clientsMu.Lock()
	for client := range vs.clients {
		client.Close()
	}
	vs.clients = make(map[*websocket.Conn]bool)
	vs.clientsMu.Unlock()

	if vs.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return vs.server.Shutdown(ctx)
	}

	return nil
}

// handleHealth handles health check requests
func (vs *VideoServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleStream handles WebSocket connections for video streaming
func (vs *VideoServer) handleStream(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from URL path
	// Expected format: /stream/{session_id}
	sessionID := r.URL.Path[len("/stream/"):]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	conn, err := vs.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	log.Printf("New video client connected for session: %s", sessionID)

	vs.clientsMu.Lock()
	vs.clients[conn] = true
	vs.clientsMu.Unlock()

	// Send stream init
	initMsg := vs.createStreamInitMessage()
	if err := conn.WriteMessage(websocket.BinaryMessage, initMsg); err != nil {
		log.Printf("Failed to send stream init: %v", err)
	}

	// Handle client messages (ping/pong, input events)
	go vs.handleClientMessages(conn, sessionID)

	// Keep connection alive
	<-vs.ctx.Done()

	vs.clientsMu.Lock()
	delete(vs.clients, conn)
	vs.clientsMu.Unlock()
	conn.Close()
}

// handleClientMessages handles incoming messages from a client
func (vs *VideoServer) handleClientMessages(conn *websocket.Conn, sessionID string) {
	defer func() {
		vs.clientsMu.Lock()
		delete(vs.clients, conn)
		vs.clientsMu.Unlock()
		conn.Close()
	}()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		if messageType == websocket.BinaryMessage && len(data) > 0 {
			msgType := data[0]
			switch msgType {
			case MsgTypePing:
				// Respond with pong
				pong := []byte{MsgTypePong}
				conn.WriteMessage(websocket.BinaryMessage, pong)
			}
		}
	}
}

// broadcastLoop broadcasts video frames to all connected clients
func (vs *VideoServer) broadcastLoop() {
	for {
		select {
		case <-vs.ctx.Done():
			return
		case frame := <-vs.broadcast:
			vs.clientsMu.RLock()
			for client := range vs.clients {
				err := client.WriteMessage(websocket.BinaryMessage, frame)
				if err != nil {
					log.Printf("Failed to send frame to client: %v", err)
				}
			}
			vs.clientsMu.RUnlock()
		}
	}
}

// BroadcastFrame sends a video frame to all connected clients
func (vs *VideoServer) BroadcastFrame(nalUnit []byte, timestamp uint64) {
	// Create frame message
	// Format: [type (1)] [timestamp (8)] [data (n)]
	msg := make([]byte, 9+len(nalUnit))
	msg[0] = MsgTypeVideoFrame
	binary.BigEndian.PutUint64(msg[1:9], timestamp)
	copy(msg[9:], nalUnit)

	select {
	case vs.broadcast <- msg:
	default:
		// Drop frame if buffer is full
		log.Println("Dropping frame - broadcast buffer full")
	}
}

// createStreamInitMessage creates a stream initialization message
func (vs *VideoServer) createStreamInitMessage() []byte {
	// Format: [type (1)] [width (2)] [height (2)] [fps (1)] [codec_len (1)] [codec (n)]
	codec := "h264"
	msg := make([]byte, 7+len(codec))
	msg[0] = MsgTypeStreamInit
	binary.BigEndian.PutUint16(msg[1:3], 1920)
	binary.BigEndian.PutUint16(msg[3:5], 1080)
	msg[5] = 60 // fps
	msg[6] = byte(len(codec))
	copy(msg[7:], codec)
	return msg
}

// ClientCount returns the number of connected clients
func (vs *VideoServer) ClientCount() int {
	vs.clientsMu.RLock()
	defer vs.clientsMu.RUnlock()
	return len(vs.clients)
}
