package server

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// GuacamoleProtocol handles the Guacamole protocol for RDP proxy
type GuacamoleProtocol struct {
	frontendConn *websocket.Conn
	rdpConn      net.Conn
	sessionID    string
	userID       string
	rdpHost      string
	rdpPort      int
	username     string
	password     string
	width        int
	height       int
	connected    bool
}

// NewGuacamoleProtocol creates a new Guacamole protocol handler
func NewGuacamoleProtocol(frontendConn *websocket.Conn, sessionID, userID string) *GuacamoleProtocol {
	return &GuacamoleProtocol{
		frontendConn: frontendConn,
		sessionID:    sessionID,
		userID:       userID,
		width:        1920,
		height:       1080,
	}
}

// StartProxy begins the RDP proxy session
func (gp *GuacamoleProtocol) StartProxy(ctx context.Context, rdpHost string, rdpPort int, username, password string) error {
	gp.rdpHost = rdpHost
	gp.rdpPort = rdpPort
	gp.username = username
	gp.password = password

	log.Info().
		Str("session_id", gp.sessionID).
		Str("user_id", gp.userID).
		Str("rdp_host", rdpHost).
		Int("rdp_port", rdpPort).
		Msg("Starting Guacamole RDP proxy")

	// Handle incoming messages from frontend
	go gp.handleFrontendMessages(ctx)

	// Connect to RDP server and start proxy
	return gp.connectToRDP(ctx)
}

// connectToRDP establishes connection to the RDP server
func (gp *GuacamoleProtocol) connectToRDP(ctx context.Context) error {
	rdpAddr := fmt.Sprintf("%s:%d", gp.rdpHost, gp.rdpPort)

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", rdpAddr)
	if err != nil {
		gp.sendError("Failed to connect to RDP server: " + err.Error())
		return fmt.Errorf("failed to connect to RDP server %s: %w", rdpAddr, err)
	}

	gp.rdpConn = conn

	log.Info().
		Str("session_id", gp.sessionID).
		Str("rdp_addr", rdpAddr).
		Msg("Connected to RDP server")

	// Send ready signal to frontend
	gp.sendInstruction("ready")
	gp.connected = true

	// Start reading RDP data
	go gp.handleRDPData(ctx)

	return nil
}

// handleFrontendMessages processes incoming WebSocket messages from frontend
func (gp *GuacamoleProtocol) handleFrontendMessages(ctx context.Context) {
	defer func() {
		if gp.rdpConn != nil {
			gp.rdpConn.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messageType, data, err := gp.frontendConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("session_id", gp.sessionID).Msg("Frontend WebSocket error")
			}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			gp.handleGuacamoleInstruction(string(data))
		case websocket.BinaryMessage:
			// Binary data (e.g., clipboard, file transfer)
			gp.handleBinaryData(data)
		}
	}
}

// handleGuacamoleInstruction processes Guacamole protocol instructions
func (gp *GuacamoleProtocol) handleGuacamoleInstruction(instruction string) {
	// Parse instruction: "opcode,arg1,arg2,arg3;"
	instruction = strings.TrimSuffix(instruction, ";")
	parts := strings.Split(instruction, ",")

	if len(parts) == 0 {
		return
	}

	opcode := parts[0]
	args := parts[1:]

	log.Debug().
		Str("session_id", gp.sessionID).
		Str("opcode", opcode).
		Strs("args", args).
		Msg("Processing Guacamole instruction")

	switch opcode {
	case "select":
		// Protocol selection: select,rdp
		if len(args) > 0 && args[0] == "rdp" {
			log.Debug().Str("session_id", gp.sessionID).Msg("RDP protocol selected")
		}

	case "connect":
		// Connection request: connect,host,port,username,password,width,height,dpi
		if len(args) >= 7 {
			host := args[0]
			port, _ := strconv.Atoi(args[1])
			username := args[2]
			_ = args[3] // password - unused for now
			width, _ := strconv.Atoi(args[4])
			height, _ := strconv.Atoi(args[5])

			gp.width = width
			gp.height = height

			log.Info().
				Str("session_id", gp.sessionID).
				Str("host", host).
				Int("port", port).
				Str("username", username).
				Int("width", width).
				Int("height", height).
				Msg("Received RDP connection request")

			// Connection will be established by connectToRDP
		}

	case "size":
		// Display size change: size,width,height
		if len(args) >= 2 {
			width, _ := strconv.Atoi(args[0])
			height, _ := strconv.Atoi(args[1])
			gp.width = width
			gp.height = height

			log.Debug().
				Str("session_id", gp.sessionID).
				Int("width", width).
				Int("height", height).
				Msg("Display size changed")
		}

	case "mouse":
		// Mouse event: mouse,x,y,mask
		if len(args) >= 3 && gp.connected {
			gp.sendMouseToRDP(args)
		}

	case "key":
		// Key event: key,keysym,pressed
		if len(args) >= 2 && gp.connected {
			gp.sendKeyToRDP(args)
		}

	case "clipboard":
		// Clipboard data: clipboard,data
		if len(args) >= 1 && gp.connected {
			gp.sendClipboardToRDP(args[0])
		}

	case "disconnect":
		// Disconnect request
		log.Info().Str("session_id", gp.sessionID).Msg("Disconnect requested")
		if gp.rdpConn != nil {
			gp.rdpConn.Close()
		}

	default:
		log.Debug().
			Str("session_id", gp.sessionID).
			Str("opcode", opcode).
			Msg("Unhandled Guacamole instruction")
	}
}

// handleBinaryData processes binary data (clipboard, file transfer, etc.)
func (gp *GuacamoleProtocol) handleBinaryData(data []byte) {
	if !gp.connected {
		return
	}

	log.Debug().
		Str("session_id", gp.sessionID).
		Int("size", len(data)).
		Msg("Received binary data from frontend")

	// Forward binary data to RDP server
	if gp.rdpConn != nil {
		gp.rdpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := gp.rdpConn.Write(data)
		if err != nil {
			log.Error().Err(err).Str("session_id", gp.sessionID).Msg("Failed to write binary data to RDP")
		}
	}
}

// handleRDPData reads RDP data and converts to Guacamole protocol
func (gp *GuacamoleProtocol) handleRDPData(ctx context.Context) {
	defer func() {
		if gp.rdpConn != nil {
			gp.rdpConn.Close()
		}
	}()

	buffer := make([]byte, 8192)
	reader := bufio.NewReader(gp.rdpConn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		gp.rdpConn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := reader.Read(buffer)
		if err != nil {
			log.Error().Err(err).Str("session_id", gp.sessionID).Msg("RDP connection closed")
			gp.sendError("RDP connection lost")
			return
		}

		if n > 0 {
			// Convert RDP data to Guacamole protocol
			gp.processRDPData(buffer[:n])
		}
	}
}

// processRDPData converts RDP protocol data to Guacamole instructions
func (gp *GuacamoleProtocol) processRDPData(data []byte) {
	// This is a simplified RDP-to-Guacamole converter
	// In a full implementation, you would parse RDP packets and convert to appropriate Guacamole instructions

	log.Debug().
		Str("session_id", gp.sessionID).
		Int("size", len(data)).
		Msg("Processing RDP data")

	// For demo purposes, simulate screen updates
	// Real implementation would parse RDP bitmap updates, cursor changes, etc.

	// Check if this looks like RDP bitmap data (simplified detection)
	if len(data) > 100 && data[0] == 0x03 { // RDP update PDU
		// Convert to Guacamole PNG instruction
		gp.sendScreenUpdate(data)
	} else if len(data) < 50 { // Likely control data
		// Handle RDP control messages
		gp.handleRDPControlData(data)
	}
}

// sendScreenUpdate converts RDP bitmap data to Guacamole PNG instruction
func (gp *GuacamoleProtocol) sendScreenUpdate(rdpData []byte) {
	// In a real implementation, you would:
	// 1. Parse RDP bitmap update packets
	// 2. Extract bitmap data and coordinates
	// 3. Convert to PNG format
	// 4. Send as Guacamole png instruction

	// For now, send a placeholder rectangle
	x, y, width, height := 0, 0, 100, 100

	// Create a simple colored rectangle as PNG data (placeholder)
	pngData := gp.createPlaceholderPNG(width, height)
	base64Data := base64.StdEncoding.EncodeToString(pngData)

	gp.sendInstruction("png", "0", strconv.Itoa(x), strconv.Itoa(y), base64Data)
}

// handleRDPControlData processes RDP control messages
func (gp *GuacamoleProtocol) handleRDPControlData(data []byte) {
	// Handle RDP control messages like cursor updates, clipboard, etc.
	log.Debug().
		Str("session_id", gp.sessionID).
		Int("size", len(data)).
		Msg("Processing RDP control data")

	// Example: cursor position update
	if len(data) >= 8 {
		// Parse cursor data (simplified)
		x := int(data[0]) | int(data[1])<<8
		y := int(data[2]) | int(data[3])<<8

		gp.sendInstruction("cursor", strconv.Itoa(x), strconv.Itoa(y))
	}
}

// sendMouseToRDP forwards mouse events to RDP server
func (gp *GuacamoleProtocol) sendMouseToRDP(args []string) {
	if len(args) < 3 || gp.rdpConn == nil {
		return
	}

	x, _ := strconv.Atoi(args[0])
	y, _ := strconv.Atoi(args[1])
	mask, _ := strconv.Atoi(args[2])

	log.Debug().
		Str("session_id", gp.sessionID).
		Int("x", x).
		Int("y", y).
		Int("mask", mask).
		Msg("Sending mouse event to RDP")

	// Create RDP mouse event packet
	// This is a simplified RDP packet structure
	mousePacket := []byte{
		0x03, 0x00, 0x00, 0x10, // RDP header
		0x02, 0xF0, // Mouse event
		byte(x & 0xFF), byte((x >> 8) & 0xFF),
		byte(y & 0xFF), byte((y >> 8) & 0xFF),
		byte(mask & 0xFF), 0x00,
	}

	gp.rdpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := gp.rdpConn.Write(mousePacket)
	if err != nil {
		log.Error().Err(err).Str("session_id", gp.sessionID).Msg("Failed to send mouse event to RDP")
	}
}

// sendKeyToRDP forwards keyboard events to RDP server
func (gp *GuacamoleProtocol) sendKeyToRDP(args []string) {
	if len(args) < 2 || gp.rdpConn == nil {
		return
	}

	keysym, _ := strconv.Atoi(args[0])
	pressed, _ := strconv.Atoi(args[1])

	log.Debug().
		Str("session_id", gp.sessionID).
		Int("keysym", keysym).
		Int("pressed", pressed).
		Msg("Sending key event to RDP")

	// Create RDP keyboard event packet
	keyPacket := []byte{
		0x03, 0x00, 0x00, 0x0C, // RDP header
		0x01, 0xF0, // Keyboard event
		byte(pressed & 0xFF), // Key state
		0x00,
		byte(keysym & 0xFF), byte((keysym >> 8) & 0xFF),
		0x00, 0x00,
	}

	gp.rdpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := gp.rdpConn.Write(keyPacket)
	if err != nil {
		log.Error().Err(err).Str("session_id", gp.sessionID).Msg("Failed to send key event to RDP")
	}
}

// sendClipboardToRDP forwards clipboard data to RDP server
func (gp *GuacamoleProtocol) sendClipboardToRDP(base64Data string) {
	if gp.rdpConn == nil {
		return
	}

	clipboardData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		log.Error().Err(err).Str("session_id", gp.sessionID).Msg("Failed to decode clipboard data")
		return
	}

	log.Debug().
		Str("session_id", gp.sessionID).
		Int("size", len(clipboardData)).
		Msg("Sending clipboard data to RDP")

	// Create RDP clipboard packet
	clipboardPacket := make([]byte, 8+len(clipboardData))
	clipboardPacket[0] = 0x03 // RDP header
	clipboardPacket[1] = 0x00
	clipboardPacket[2] = byte((8 + len(clipboardData)) & 0xFF)
	clipboardPacket[3] = byte(((8 + len(clipboardData)) >> 8) & 0xFF)
	clipboardPacket[4] = 0x05 // Clipboard event
	clipboardPacket[5] = 0xF0
	clipboardPacket[6] = byte(len(clipboardData) & 0xFF)
	clipboardPacket[7] = byte((len(clipboardData) >> 8) & 0xFF)
	copy(clipboardPacket[8:], clipboardData)

	gp.rdpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = gp.rdpConn.Write(clipboardPacket)
	if err != nil {
		log.Error().Err(err).Str("session_id", gp.sessionID).Msg("Failed to send clipboard to RDP")
	}
}

// sendInstruction sends a Guacamole instruction to the frontend
func (gp *GuacamoleProtocol) sendInstruction(opcode string, args ...string) {
	instruction := opcode
	if len(args) > 0 {
		instruction += "," + strings.Join(args, ",")
	}
	instruction += ";"

	gp.frontendConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := gp.frontendConn.WriteMessage(websocket.TextMessage, []byte(instruction))
	if err != nil {
		log.Error().Err(err).
			Str("session_id", gp.sessionID).
			Str("instruction", instruction).
			Msg("Failed to send Guacamole instruction")
	}
}

// sendError sends an error instruction to the frontend
func (gp *GuacamoleProtocol) sendError(message string) {
	gp.sendInstruction("error", message)
}

// createPlaceholderPNG creates a simple PNG for demonstration
func (gp *GuacamoleProtocol) createPlaceholderPNG(width, height int) []byte {
	// This is a minimal PNG implementation for demonstration
	// In a real implementation, you would use a proper PNG library

	// Create a simple colored rectangle
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	}

	// IHDR chunk for a simple RGB image
	ihdr := []byte{
		0x00, 0x00, 0x00, 0x0D, // Chunk length
		0x49, 0x48, 0x44, 0x52, // "IHDR"
		byte(width >> 24), byte(width >> 16), byte(width >> 8), byte(width),
		byte(height >> 24), byte(height >> 16), byte(height >> 8), byte(height),
		0x08, 0x02, 0x00, 0x00, 0x00, // 8-bit RGB, no compression, no filter, no interlace
		0x9D, 0x19, 0x34, 0x6D, // CRC (calculated for this specific IHDR)
	}

	// Simple IDAT chunk with solid color
	idat := []byte{
		0x00, 0x00, 0x00, 0x0C, // Chunk length
		0x49, 0x44, 0x41, 0x54, // "IDAT"
		0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, // Compressed data
		0x0D, 0x0A, 0x2D, 0xB4, // CRC
	}

	// IEND chunk
	iend := []byte{
		0x00, 0x00, 0x00, 0x00, // Chunk length
		0x49, 0x45, 0x4E, 0x44, // "IEND"
		0xAE, 0x42, 0x60, 0x82, // CRC
	}

	// Combine all chunks
	png := make([]byte, 0, len(pngHeader)+len(ihdr)+len(idat)+len(iend))
	png = append(png, pngHeader...)
	png = append(png, ihdr...)
	png = append(png, idat...)
	png = append(png, iend...)

	return png
}

// parseRDPPacket parses an RDP packet and extracts relevant information
func (gp *GuacamoleProtocol) parseRDPPacket(data []byte) (packetType byte, payload []byte) {
	if len(data) < 4 {
		return 0, nil
	}

	// Basic RDP packet structure: [version][pduType][length][data...]
	packetType = data[1]

	if len(data) > 4 {
		payload = data[4:]
	}

	return packetType, payload
}

// convertRDPBitmapToGuacamole converts RDP bitmap data to Guacamole PNG instruction
func (gp *GuacamoleProtocol) convertRDPBitmapToGuacamole(bitmapData []byte, x, y, width, height int) {
	// In a real implementation, this would:
	// 1. Parse RDP bitmap format (color depth, compression, etc.)
	// 2. Convert bitmap data to PNG format
	// 3. Encode as base64
	// 4. Send as Guacamole png instruction

	// For now, create a placeholder PNG
	pngData := gp.createPlaceholderPNG(width, height)
	base64Data := base64.StdEncoding.EncodeToString(pngData)

	gp.sendInstruction("png", "0", strconv.Itoa(x), strconv.Itoa(y), base64Data)
}

// Close cleanly closes the protocol handler
func (gp *GuacamoleProtocol) Close() {
	log.Info().Str("session_id", gp.sessionID).Msg("Closing Guacamole protocol handler")

	if gp.rdpConn != nil {
		gp.rdpConn.Close()
		gp.rdpConn = nil
	}

	if gp.frontendConn != nil {
		gp.frontendConn.Close()
	}

	gp.connected = false
}

// GetConnectionInfo returns information about the current connection
func (gp *GuacamoleProtocol) GetConnectionInfo() map[string]interface{} {
	return map[string]interface{}{
		"session_id": gp.sessionID,
		"user_id":    gp.userID,
		"connected":  gp.connected,
		"rdp_host":   gp.rdpHost,
		"rdp_port":   gp.rdpPort,
		"width":      gp.width,
		"height":     gp.height,
		"protocol":   "guacamole-over-websocket",
	}
}
