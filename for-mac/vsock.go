package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation

#import <Foundation/Foundation.h>
#import <sys/socket.h>
#import <sys/un.h>

// vsock address family and port definitions
// On macOS, vsock is accessed via a UNIX socket created by the hypervisor
#define VMADDR_CID_ANY -1U
#define VMADDR_CID_HYPERVISOR 0
#define VMADDR_CID_LOCAL 1
#define VMADDR_CID_HOST 2

// For UTM/QEMU, vsock is exposed via a UNIX socket
// The path is typically: /tmp/utm-vsock-{vm-id}.sock or similar

*/
import "C"
import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// VsockMessage types for frame export protocol
const (
	VsockMsgFrameRequest  uint8 = 1 // Guest -> Host: Request to encode a frame (resource ID path)
	VsockMsgFrameResponse uint8 = 2 // Host -> Guest: Encoded NAL units
	VsockMsgKeyframeReq   uint8 = 3 // Guest -> Host: Request keyframe
	VsockMsgPing          uint8 = 4 // Keepalive
	VsockMsgPong          uint8 = 5 // Keepalive response
	VsockMsgRawFrame      uint8 = 6 // Guest -> Host: Raw frame data (fallback path)
)

// FrameRequest is sent from guest to host when a frame is ready to encode
// This is the zero-copy path using virtio-gpu resource IDs
type FrameRequest struct {
	ResourceID uint32 // virtio-gpu resource ID
	Width      uint32
	Height     uint32
	Format     uint32 // Pixel format (e.g., BGRA, NV12)
	PTS        int64  // Presentation timestamp (nanoseconds)
	Duration   int64  // Frame duration (nanoseconds)
}

// RawFrameRequest is sent from guest to host with actual pixel data
// This is the fallback path when resource ID → IOSurface mapping isn't available
type RawFrameRequest struct {
	Width    uint32 // Frame width
	Height   uint32 // Frame height
	Format   uint32 // Pixel format (BGRA=0x42475241)
	Stride   uint32 // Bytes per row
	PTS      int64  // Presentation timestamp (nanoseconds)
	Duration int64  // Frame duration (nanoseconds)
	// Pixel data follows (Width * Height * BytesPerPixel)
}

// FrameResponse is sent from host to guest with encoded data
type FrameResponse struct {
	PTS        int64  // Presentation timestamp
	IsKeyframe bool   // Whether this is a keyframe
	NALData    []byte // Encoded H.264 NAL units
}

// VsockServer handles vsock connections for frame export
type VsockServer struct {
	socketPath string
	listener   net.Listener
	encoder    *VideoToolboxEncoder
	clients    map[net.Conn]struct{}
	clientsMu  sync.RWMutex
	running    bool
	mu         sync.Mutex
	stopCh     chan struct{}

	// Callback for encoded frames to be sent back to guest
	onEncodedFrame func(response *FrameResponse)
}

// NewVsockServer creates a new vsock server
// socketPath is the UNIX socket path that UTM/QEMU exposes for vsock
func NewVsockServer(socketPath string, encoder *VideoToolboxEncoder) *VsockServer {
	return &VsockServer{
		socketPath: socketPath,
		encoder:    encoder,
		clients:    make(map[net.Conn]struct{}),
		stopCh:     make(chan struct{}),
	}
}

// Start starts the vsock server
func (v *VsockServer) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running {
		return fmt.Errorf("vsock server already running")
	}

	// For UTM/QEMU on macOS, vsock is typically accessed via a UNIX socket
	// The actual socket path depends on how UTM is configured
	listener, err := net.Listen("unix", v.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on vsock socket: %w", err)
	}

	v.listener = listener
	v.running = true

	go v.acceptLoop()

	log.Printf("Vsock server started on %s", v.socketPath)
	return nil
}

// Stop stops the vsock server
func (v *VsockServer) Stop() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.running {
		return nil
	}

	v.running = false
	close(v.stopCh)

	if v.listener != nil {
		v.listener.Close()
	}

	// Close all client connections
	v.clientsMu.Lock()
	for conn := range v.clients {
		conn.Close()
	}
	v.clients = make(map[net.Conn]struct{})
	v.clientsMu.Unlock()

	return nil
}

func (v *VsockServer) acceptLoop() {
	for {
		conn, err := v.listener.Accept()
		if err != nil {
			select {
			case <-v.stopCh:
				return
			default:
				log.Printf("Vsock accept error: %v", err)
				continue
			}
		}

		v.clientsMu.Lock()
		v.clients[conn] = struct{}{}
		v.clientsMu.Unlock()

		go v.handleConnection(conn)
	}
}

func (v *VsockServer) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		v.clientsMu.Lock()
		delete(v.clients, conn)
		v.clientsMu.Unlock()
	}()

	log.Printf("Vsock client connected: %s", conn.RemoteAddr())

	for {
		// Read message header: type (1 byte) + length (4 bytes)
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			if err != io.EOF {
				log.Printf("Vsock read header error: %v", err)
			}
			return
		}

		msgType := header[0]
		msgLen := binary.LittleEndian.Uint32(header[1:5])

		// Read message body
		body := make([]byte, msgLen)
		if msgLen > 0 {
			if _, err := io.ReadFull(conn, body); err != nil {
				log.Printf("Vsock read body error: %v", err)
				return
			}
		}

		// Handle message
		switch msgType {
		case VsockMsgFrameRequest:
			v.handleFrameRequest(conn, body)
		case VsockMsgKeyframeReq:
			v.handleKeyframeRequest(conn)
		case VsockMsgPing:
			v.handlePing(conn)
		default:
			log.Printf("Unknown vsock message type: %d", msgType)
		}
	}
}

func (v *VsockServer) handleFrameRequest(conn net.Conn, body []byte) {
	if len(body) < 32 {
		log.Printf("Invalid frame request: too short")
		return
	}

	req := FrameRequest{
		ResourceID: binary.LittleEndian.Uint32(body[0:4]),
		Width:      binary.LittleEndian.Uint32(body[4:8]),
		Height:     binary.LittleEndian.Uint32(body[8:12]),
		Format:     binary.LittleEndian.Uint32(body[12:16]),
		PTS:        int64(binary.LittleEndian.Uint64(body[16:24])),
		Duration:   int64(binary.LittleEndian.Uint64(body[24:32])),
	}

	// Convert virtio-gpu resource ID to IOSurface via virglrenderer
	// This path: resource_id → virglrenderer → MTLTexture → IOSurface → VideoToolbox
	ioSurfaceID, err := ResourceToIOSurfaceID(req.ResourceID)
	if err != nil {
		log.Printf("Failed to get IOSurface for resource %d: %v", req.ResourceID, err)
		return
	}

	// Encode the frame using VideoToolbox with the IOSurface ID
	if err := v.encoder.EncodeIOSurface(ioSurfaceID, req.PTS, req.Duration); err != nil {
		log.Printf("Failed to encode frame: %v", err)
		return
	}

	// The encoded frame will be delivered via the encoder callback
	// which should call SendEncodedFrame
}

func (v *VsockServer) handleKeyframeRequest(conn net.Conn) {
	// Request the encoder to produce a keyframe on the next frame
	log.Printf("Keyframe requested")
	// TODO: Implement force keyframe in encoder
}

func (v *VsockServer) handlePing(conn net.Conn) {
	// Send pong response
	response := []byte{VsockMsgPong, 0, 0, 0, 0}
	conn.Write(response)
}

// SendEncodedFrame sends an encoded frame back to all connected guests
func (v *VsockServer) SendEncodedFrame(response *FrameResponse) error {
	// Build response message
	// Format: type (1) + length (4) + pts (8) + keyframe (1) + nal_len (4) + nal_data
	bodyLen := 8 + 1 + 4 + len(response.NALData)
	msg := make([]byte, 5+bodyLen)

	msg[0] = VsockMsgFrameResponse
	binary.LittleEndian.PutUint32(msg[1:5], uint32(bodyLen))
	binary.LittleEndian.PutUint64(msg[5:13], uint64(response.PTS))
	if response.IsKeyframe {
		msg[13] = 1
	}
	binary.LittleEndian.PutUint32(msg[14:18], uint32(len(response.NALData)))
	copy(msg[18:], response.NALData)

	// Send to all connected clients
	v.clientsMu.RLock()
	defer v.clientsMu.RUnlock()

	for conn := range v.clients {
		conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := conn.Write(msg); err != nil {
			log.Printf("Failed to send frame to client: %v", err)
		}
	}

	return nil
}

// ClientCount returns the number of connected clients
func (v *VsockServer) ClientCount() int {
	v.clientsMu.RLock()
	defer v.clientsMu.RUnlock()
	return len(v.clients)
}

// IsRunning returns whether the server is running
func (v *VsockServer) IsRunning() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.running
}

// VsockClient is used by the guest to communicate with the host
// This would run inside the VM, but we define the protocol here for reference
type VsockClient struct {
	conn    net.Conn
	running bool
	mu      sync.Mutex
}

// Protocol documentation for guest-side implementation:
//
// Frame Export Protocol over vsock:
//
// 1. Guest connects to host vsock port (e.g., port 5000)
//
// 2. Guest sends FrameRequest when PipeWire captures a frame:
//    - Header: type=1, length=32
//    - Body: resource_id (4), width (4), height (4), format (4), pts (8), duration (8)
//
// 3. Host encodes frame with VideoToolbox and sends FrameResponse:
//    - Header: type=2, length=variable
//    - Body: pts (8), is_keyframe (1), nal_len (4), nal_data (variable)
//
// 4. Guest forwards NAL data to WebSocket for browser streaming
//
// Resource ID Mapping:
// - Guest: DMA-BUF fd -> DRM_IOCTL_PRIME_FD_TO_HANDLE -> GEM handle
// - Guest: GEM handle -> virtio-gpu resource ID (kernel driver mapping)
// - Host: resource ID -> virgl_renderer_resource_get_info_ext() -> MTLTexture
// - Host: MTLTexture.iosurface -> IOSurface -> CVPixelBuffer -> VideoToolbox
