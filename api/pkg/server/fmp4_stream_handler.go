package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// generateWebSocketKey generates a random WebSocket key for client handshake
func generateWebSocketKey() string {
	key := make([]byte, 16)
	rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}

// fMP4 streaming constants
const (
	fmp4Timescale = 90000 // 90kHz timescale (standard for video)
)

// fMP4StreamHandler handles /api/v1/external-agents/{sessionID}/video.mp4
// This endpoint streams video as fragmented MP4 for native <video> element playback.
// Unlike the WebSocket stream which requires WebCodecs for decoding, this allows
// Safari and other browsers to use their native video pipeline for PiP support.
//
// @Summary Stream video as fragmented MP4
// @Description Streams H.264 video from the desktop session as fragmented MP4.
// @Description This allows native video element playback with Picture-in-Picture support.
// @Tags external-agents
// @Produce video/mp4
// @Param sessionID path string true "Session ID"
// @Success 200 {file} video/mp4
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/video.mp4 [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) handleFMP4Stream(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for fMP4 stream")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID && !isAdmin(user) {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Msg("User does not have access to session for fMP4 stream")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse optional resolution query parameters (default 1280x720 for PiP)
	// Example: /video.mp4?width=1280&height=720
	width := 1280
	height := 720
	bitrate := 2000 // 2 Mbps default for PiP
	fps := 30

	if w := req.URL.Query().Get("width"); w != "" {
		if parsed, err := strconv.Atoi(w); err == nil && parsed > 0 && parsed <= 3840 {
			width = parsed
		}
	}
	if h := req.URL.Query().Get("height"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 && parsed <= 2160 {
			height = parsed
		}
	}
	if b := req.URL.Query().Get("bitrate"); b != "" {
		if parsed, err := strconv.Atoi(b); err == nil && parsed > 0 && parsed <= 50000 {
			bitrate = parsed
		}
	}

	// Get RevDial connection to desktop container
	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Int("width", width).
		Int("height", height).
		Int("bitrate", bitrate).
		Msg("Starting fMP4 video stream")

	// Connect to screenshot-server via RevDial
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	serverConn, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to connect to sandbox for fMP4 stream")
		http.Error(res, "Sandbox not connected", http.StatusServiceUnavailable)
		return
	}
	defer serverConn.Close()

	// Set up WebSocket connection to screenshot-server
	// Generate a simple WebSocket key
	wsKey := generateWebSocketKey()

	upgradeReq := fmt.Sprintf("GET /ws/stream HTTP/1.1\r\n"+
		"Host: localhost:9876\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", wsKey)

	if _, err := serverConn.Write([]byte(upgradeReq)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade for fMP4 stream")
		http.Error(res, "Failed to connect to video source", http.StatusBadGateway)
		return
	}

	// Read upgrade response
	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read WebSocket upgrade response for fMP4 stream")
		http.Error(res, "Video source connection failed", http.StatusBadGateway)
		return
	}
	defer upgradeResp.Body.Close()

	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		log.Error().Int("status", upgradeResp.StatusCode).Msg("Video source didn't accept WebSocket upgrade")
		http.Error(res, "Video source unavailable", http.StatusBadGateway)
		return
	}

	// Send WebSocket init message with resolution config
	// This tells the screenshot-server what resolution to encode at
	initMsg := map[string]interface{}{
		"type":    "init",
		"width":   width,
		"height":  height,
		"fps":     fps,
		"bitrate": bitrate,
	}
	initJSON, err := json.Marshal(initMsg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal init message")
		http.Error(res, "Internal error", http.StatusInternalServerError)
		return
	}

	// Write WebSocket text frame with init message
	if err := writeWebSocketFrame(serverConn, initJSON, true); err != nil {
		log.Error().Err(err).Msg("Failed to send init message for fMP4 stream")
		http.Error(res, "Failed to initialize video stream", http.StatusBadGateway)
		return
	}

	log.Debug().RawJSON("init", initJSON).Msg("Sent init message to screenshot-server")

	// Set response headers for streaming
	res.Header().Set("Content-Type", "video/mp4")
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.Header().Set("Transfer-Encoding", "chunked")
	// Important: Allow the video to start playing immediately
	res.Header().Set("X-Content-Type-Options", "nosniff")

	// Flush headers
	if flusher, ok := res.(http.Flusher); ok {
		flusher.Flush()
	}

	// Create fMP4 muxer state
	muxer := newFMP4Muxer(res)

	// Read WebSocket frames and convert to fMP4
	err = muxer.processWebSocketStream(serverReader, req.Context())
	if err != nil && err != io.EOF && err != context.Canceled {
		log.Error().Err(err).Str("session_id", sessionID).Msg("fMP4 stream ended with error")
	} else {
		log.Info().Str("session_id", sessionID).Msg("fMP4 stream ended normally")
	}
}

// fMP4Muxer handles conversion of H.264 NAL units to fragmented MP4
type fMP4Muxer struct {
	w             io.Writer
	flusher       http.Flusher
	mu            sync.Mutex
	initialized   bool
	sps           []byte
	pps           []byte
	width         uint32
	height        uint32
	frameNum      uint32
	baseTime      uint64
	lastTimestamp uint64
}

func newFMP4Muxer(w io.Writer) *fMP4Muxer {
	muxer := &fMP4Muxer{
		w: w,
	}
	if flusher, ok := w.(http.Flusher); ok {
		muxer.flusher = flusher
	}
	return muxer
}

// processWebSocketStream reads WebSocket frames and converts to fMP4
func (m *fMP4Muxer) processWebSocketStream(r *bufio.Reader, ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read WebSocket frame header
		frameData, err := readWebSocketFrame(r)
		if err != nil {
			return err
		}

		if len(frameData) < 1 {
			continue
		}

		// Check message type - video frames are type 0x01
		msgType := frameData[0]
		if msgType != 0x01 {
			// Not a video frame, skip
			continue
		}

		// Parse video frame header
		// Format: type(1) + codec(1) + keyframe(1) + width(2) + height(2) + pts(8) + data
		if len(frameData) < 15 {
			continue
		}

		// codec := frameData[1]
		isKeyframe := frameData[2] != 0
		width := binary.LittleEndian.Uint16(frameData[3:5])
		height := binary.LittleEndian.Uint16(frameData[5:7])
		pts := binary.LittleEndian.Uint64(frameData[7:15])
		nalData := frameData[15:]

		if len(nalData) == 0 {
			continue
		}

		// Process NAL units
		err = m.processNALUnits(nalData, isKeyframe, uint32(width), uint32(height), pts)
		if err != nil {
			log.Error().Err(err).Msg("Failed to process NAL units")
			continue
		}
	}
}

// processNALUnits handles H.264 NAL units and outputs fMP4 fragments
func (m *fMP4Muxer) processNALUnits(data []byte, isKeyframe bool, width, height uint32, pts uint64) error {
	// Parse NAL units (Annex B format with start codes)
	nalus := avc.ExtractNalusFromByteStream(data)
	if len(nalus) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Extract SPS/PPS for initialization
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		nalType := nalu[0] & 0x1F
		switch nalType {
		case 7: // SPS
			m.sps = make([]byte, len(nalu))
			copy(m.sps, nalu)
			m.width = width
			m.height = height
		case 8: // PPS
			m.pps = make([]byte, len(nalu))
			copy(m.pps, nalu)
		}
	}

	// If we have SPS/PPS and haven't initialized, write init segment
	if !m.initialized && m.sps != nil && m.pps != nil {
		if err := m.writeInitSegment(); err != nil {
			return fmt.Errorf("failed to write init segment: %w", err)
		}
		m.initialized = true
		m.baseTime = pts
	}

	if !m.initialized {
		// Wait for SPS/PPS
		return nil
	}

	// Filter out SPS/PPS from the actual frame data (they're in the init segment)
	var frameNALUs [][]byte
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		nalType := nalu[0] & 0x1F
		if nalType != 7 && nalType != 8 { // Not SPS or PPS
			frameNALUs = append(frameNALUs, nalu)
		}
	}

	if len(frameNALUs) == 0 {
		return nil
	}

	// Write media segment
	return m.writeMediaSegment(frameNALUs, isKeyframe, pts)
}

// writeInitSegment writes the fMP4 initialization segment (ftyp + moov)
func (m *fMP4Muxer) writeInitSegment() error {
	// Parse SPS for actual dimensions
	spsInfo, err := avc.ParseSPSNALUnit(m.sps, true)
	if err != nil {
		// Use provided dimensions
		log.Warn().Err(err).Msg("Failed to parse SPS, using provided dimensions")
	} else {
		m.width = uint32(spsInfo.Width)
		m.height = uint32(spsInfo.Height)
	}

	// Create init segment
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(fmp4Timescale, "video", "und")

	// Set up AVC decoder configuration
	trak := init.Moov.Trak
	stsd := trak.Mdia.Minf.Stbl.Stsd

	// Create avcC box with SPS/PPS
	avcC, err := mp4.CreateAvcC([][]byte{m.sps}, [][]byte{m.pps}, true)
	if err != nil {
		return fmt.Errorf("failed to create avcC: %w", err)
	}

	// Create visual sample entry
	avcx := mp4.CreateVisualSampleEntryBox("avc1", uint16(m.width), uint16(m.height), avcC)
	stsd.AddChild(avcx)

	// Encode and write init segment
	var initBuf bytes.Buffer
	if err := init.Encode(&initBuf); err != nil {
		return fmt.Errorf("failed to encode init segment: %w", err)
	}

	if _, err := m.w.Write(initBuf.Bytes()); err != nil {
		return fmt.Errorf("failed to write init segment: %w", err)
	}

	if m.flusher != nil {
		m.flusher.Flush()
	}

	log.Info().
		Uint32("width", m.width).
		Uint32("height", m.height).
		Msg("fMP4 init segment written")

	return nil
}

// writeMediaSegment writes an fMP4 media segment (moof + mdat)
func (m *fMP4Muxer) writeMediaSegment(nalus [][]byte, isKeyframe bool, pts uint64) error {
	m.frameNum++

	// Calculate decode time (relative to base)
	decodeTime := pts - m.baseTime

	// Calculate sample duration (assume 30fps = 3000 ticks at 90kHz)
	var sampleDur uint32 = 3000
	if m.lastTimestamp > 0 && pts > m.lastTimestamp {
		sampleDur = uint32((pts - m.lastTimestamp) * fmp4Timescale / 1000000) // Convert from microseconds
		if sampleDur == 0 {
			sampleDur = 3000
		}
	}
	m.lastTimestamp = pts

	// Build sample data in AVCC format (length-prefixed NALUs)
	var sampleData []byte
	for _, nalu := range nalus {
		// 4-byte length prefix
		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(nalu)))
		sampleData = append(sampleData, lenBuf...)
		sampleData = append(sampleData, nalu...)
	}

	// Create fragment
	frag, err := mp4.CreateFragment(m.frameNum, 1) // trackID = 1
	if err != nil {
		return fmt.Errorf("failed to create fragment: %w", err)
	}

	// Create full sample
	sample := mp4.FullSample{
		Sample: mp4.Sample{
			Flags:                 mp4.SyncSampleFlags, // Will be overwritten if not keyframe
			Dur:                   sampleDur,
			Size:                  uint32(len(sampleData)),
			CompositionTimeOffset: 0,
		},
		DecodeTime: decodeTime,
		Data:       sampleData,
	}

	if !isKeyframe {
		sample.Sample.Flags = mp4.NonSyncSampleFlags
	}

	// Add sample to fragment
	frag.AddFullSample(sample)

	// Encode fragment
	var fragBuf bytes.Buffer
	if err := frag.Encode(&fragBuf); err != nil {
		return fmt.Errorf("failed to encode fragment: %w", err)
	}

	// Write fragment
	if _, err := m.w.Write(fragBuf.Bytes()); err != nil {
		return fmt.Errorf("failed to write fragment: %w", err)
	}

	if m.flusher != nil {
		m.flusher.Flush()
	}

	return nil
}

// readWebSocketFrame reads a single WebSocket frame
func readWebSocketFrame(r *bufio.Reader) ([]byte, error) {
	// Read first 2 bytes of WebSocket frame header
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// fin := (header[0] & 0x80) != 0
	// opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	payloadLen := uint64(header[1] & 0x7F)

	// Extended payload length
	if payloadLen == 126 {
		extLen := make([]byte, 2)
		if _, err := io.ReadFull(r, extLen); err != nil {
			return nil, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(extLen))
	} else if payloadLen == 127 {
		extLen := make([]byte, 8)
		if _, err := io.ReadFull(r, extLen); err != nil {
			return nil, err
		}
		payloadLen = binary.BigEndian.Uint64(extLen)
	}

	// Read masking key if present
	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(r, maskKey); err != nil {
			return nil, err
		}
	}

	// Read payload
	if payloadLen > 10*1024*1024 { // 10MB limit
		return nil, fmt.Errorf("payload too large: %d", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	// Unmask if needed
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return payload, nil
}

// writeWebSocketFrame writes a WebSocket frame to the connection
// isText determines if it's a text frame (opcode 1) or binary frame (opcode 2)
func writeWebSocketFrame(w io.Writer, payload []byte, isText bool) error {
	var opcode byte = 0x02 // Binary frame
	if isText {
		opcode = 0x01 // Text frame
	}

	payloadLen := len(payload)

	// Build frame header
	var header []byte
	if payloadLen < 126 {
		header = []byte{0x80 | opcode, byte(payloadLen)}
	} else if payloadLen < 65536 {
		header = make([]byte, 4)
		header[0] = 0x80 | opcode
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:4], uint16(payloadLen))
	} else {
		header = make([]byte, 10)
		header[0] = 0x80 | opcode
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:10], uint64(payloadLen))
	}

	// Write header
	if _, err := w.Write(header); err != nil {
		return err
	}

	// Write payload
	if _, err := w.Write(payload); err != nil {
		return err
	}

	return nil
}
