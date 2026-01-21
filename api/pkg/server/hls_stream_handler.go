package server

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// hlsSessionManager manages HLS muxers per session
type hlsSessionManager struct {
	mu      sync.RWMutex
	muxers  map[string]*hlsSession
	cleanup *time.Ticker
}

type hlsSession struct {
	muxer      *gohlslib.Muxer
	track      *gohlslib.Track
	cancelFunc context.CancelFunc
	lastAccess time.Time
	started    bool
	sps        []byte
	pps        []byte
	mu         sync.Mutex
}

var hlsManager = &hlsSessionManager{
	muxers: make(map[string]*hlsSession),
}

func init() {
	// Start cleanup goroutine
	hlsManager.cleanup = time.NewTicker(60 * time.Second)
	go func() {
		for range hlsManager.cleanup.C {
			hlsManager.cleanupStaleSessions()
		}
	}()
}

func (m *hlsSessionManager) cleanupStaleSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for sessionID, session := range m.muxers {
		if now.Sub(session.lastAccess) > 5*time.Minute {
			log.Info().Str("session_id", sessionID).Msg("Cleaning up stale HLS session")
			session.cancelFunc()
			// Only close the muxer if it was started - calling Close() on an
			// unstarted muxer causes a nil pointer dereference in gohlslib
			func() {
				session.mu.Lock()
				defer session.mu.Unlock()
				if session.started {
					session.muxer.Close()
				}
			}()
			delete(m.muxers, sessionID)
		}
	}
}

func (m *hlsSessionManager) getOrCreate(sessionID string, apiServer *HelixAPIServer, width, height, bitrate, fps int) (*hlsSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.muxers[sessionID]; ok {
		session.lastAccess = time.Now()
		return session, nil
	}

	// Create new HLS session
	ctx, cancel := context.WithCancel(context.Background())

	// Create HLS muxer with Low-Latency variant
	muxer := &gohlslib.Muxer{
		Variant:            gohlslib.MuxerVariantLowLatency,
		SegmentCount:       3,
		SegmentMinDuration: 500 * time.Millisecond, // 500ms segments for low latency
		PartMinDuration:    100 * time.Millisecond, // 100ms parts
	}

	session := &hlsSession{
		muxer:      muxer,
		cancelFunc: cancel,
		lastAccess: time.Now(),
	}

	m.muxers[sessionID] = session

	// Start background goroutine to feed video from WebSocket
	go m.feedVideoToMuxer(ctx, sessionID, session, apiServer, width, height, bitrate, fps)

	return session, nil
}

func (m *hlsSessionManager) feedVideoToMuxer(ctx context.Context, sessionID string, session *hlsSession, apiServer *HelixAPIServer, width, height, bitrate, fps int) {
	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Int("width", width).
		Int("height", height).
		Int("bitrate", bitrate).
		Int("fps", fps).
		Msg("Starting HLS video feed")

	for {
		select {
		case <-ctx.Done():
			log.Info().Str("session_id", sessionID).Msg("HLS video feed stopped")
			return
		default:
		}

		err := m.connectAndStream(ctx, sessionID, session, apiServer, runnerID, width, height, bitrate, fps)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Str("session_id", sessionID).Msg("HLS stream error, reconnecting...")
			time.Sleep(1 * time.Second)
		}
	}
}

func (m *hlsSessionManager) connectAndStream(ctx context.Context, sessionID string, session *hlsSession, apiServer *HelixAPIServer, runnerID string, width, height, bitrate, fps int) error {
	// Connect to desktop container via RevDial
	connCtx, connCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connCancel()

	serverConn, err := apiServer.connman.Dial(connCtx, runnerID)
	if err != nil {
		return fmt.Errorf("failed to connect to sandbox: %w", err)
	}
	defer serverConn.Close()

	// WebSocket upgrade
	wsKey := generateWebSocketKey()
	upgradeReq := fmt.Sprintf("GET /ws/stream HTTP/1.1\r\n"+
		"Host: localhost:9876\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", wsKey)

	if _, err := serverConn.Write([]byte(upgradeReq)); err != nil {
		return fmt.Errorf("failed to send WebSocket upgrade: %w", err)
	}

	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		return fmt.Errorf("failed to read WebSocket upgrade response: %w", err)
	}
	defer upgradeResp.Body.Close()

	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("WebSocket upgrade failed with status %d", upgradeResp.StatusCode)
	}

	// Send init message
	gopSize := fps
	if gopSize < 30 {
		gopSize = 30
	}
	initMsg := map[string]interface{}{
		"type":     "init",
		"width":    width,
		"height":   height,
		"fps":      fps,
		"bitrate":  bitrate,
		"gop_size": gopSize,
	}
	initJSON, _ := json.Marshal(initMsg)
	if err := writeWebSocketFrame(serverConn, initJSON, true); err != nil {
		return fmt.Errorf("failed to send init message: %w", err)
	}

	log.Debug().RawJSON("init", initJSON).Msg("HLS: Sent init message to screenshot-server")

	// Read and process WebSocket frames
	var frameNum uint32
	baseTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frameData, err := readWebSocketFrame(serverReader)
		if err != nil {
			return err
		}

		if len(frameData) < 15 {
			continue
		}

		msgType := frameData[0]
		if msgType != 0x01 {
			continue
		}

		// Parse video frame header
		isKeyframe := frameData[2] != 0
		pts := binary.BigEndian.Uint64(frameData[3:11])
		// width and height are at [11:13] and [13:15] but we don't need them
		nalData := frameData[15:]

		if len(nalData) == 0 {
			continue
		}

		// Parse NAL units
		nalus := avc.ExtractNalusFromByteStream(nalData)
		if len(nalus) == 0 {
			continue
		}

		// Extract SPS/PPS and initialize muxer if needed
		session.mu.Lock()
		for _, nalu := range nalus {
			if len(nalu) == 0 {
				continue
			}
			nalType := nalu[0] & 0x1F
			switch nalType {
			case 7: // SPS
				if session.sps == nil {
					session.sps = make([]byte, len(nalu))
					copy(session.sps, nalu)
				}
			case 8: // PPS
				if session.pps == nil {
					session.pps = make([]byte, len(nalu))
					copy(session.pps, nalu)
				}
			}
		}

		// Start muxer once we have SPS/PPS
		if !session.started && session.sps != nil && session.pps != nil {
			// Create H264 track
			session.track = &gohlslib.Track{
				Codec: &codecs.H264{
					SPS: session.sps,
					PPS: session.pps,
				},
			}
			session.muxer.Tracks = []*gohlslib.Track{session.track}

			if err := session.muxer.Start(); err != nil {
				session.mu.Unlock()
				return fmt.Errorf("failed to start HLS muxer: %w", err)
			}
			session.started = true
			log.Info().Str("session_id", sessionID).Msg("HLS muxer started")
		}

		if !session.started {
			session.mu.Unlock()
			continue
		}
		session.mu.Unlock()

		// Filter NAL units (exclude SPS/PPS)
		var frameNALUs [][]byte
		for _, nalu := range nalus {
			if len(nalu) == 0 {
				continue
			}
			nalType := nalu[0] & 0x1F
			if nalType != 7 && nalType != 8 {
				frameNALUs = append(frameNALUs, nalu)
			}
		}

		if len(frameNALUs) == 0 {
			continue
		}

		frameNum++

		// Calculate PTS in 90kHz clock (standard for H264 in HLS)
		// pts from WebSocket is in microseconds, convert to 90kHz ticks
		// 90000 ticks/second = 90 ticks/millisecond = 0.09 ticks/microsecond
		pts90k := int64((pts - uint64(baseTime.UnixMicro())) * 90000 / 1000000)
		if pts90k < 0 {
			pts90k = int64(frameNum) * 90000 / int64(fps)
		}

		// Write H264 access unit to muxer
		err = session.muxer.WriteH264(session.track, time.Now(), pts90k, frameNALUs)
		if err != nil {
			log.Error().Err(err).Uint32("frame", frameNum).Msg("Failed to write H264 to HLS muxer")
			continue
		}

		if frameNum <= 3 || frameNum%60 == 0 {
			log.Debug().
				Uint32("frame", frameNum).
				Bool("keyframe", isKeyframe).
				Int("nalus", len(frameNALUs)).
				Msg("HLS frame written")
		}
	}
}

// handleHLSStream handles /api/v1/external-agents/{sessionID}/stream.m3u8 and /stream/* paths
// This endpoint serves HLS manifest and segments for iOS Safari PiP support.
//
// @Summary Stream video as HLS
// @Description Streams H.264 video from the desktop session as Low-Latency HLS.
// @Description This allows iOS Safari video playback with Picture-in-Picture support.
// @Tags external-agents
// @Produce application/vnd.apple.mpegurl
// @Param sessionID path string true "Session ID"
// @Success 200 {file} application/vnd.apple.mpegurl
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/stream.m3u8 [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) handleHLSStream(res http.ResponseWriter, req *http.Request) {
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
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for HLS stream")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	if session.Owner != user.ID && !isAdmin(user) {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Msg("User does not have access to session for HLS stream")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse resolution parameters
	width := 1280
	height := 720
	bitrate := 2000
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
	if f := req.URL.Query().Get("fps"); f != "" {
		if parsed, err := strconv.Atoi(f); err == nil && parsed > 0 && parsed <= 120 {
			fps = parsed
		}
	}

	// Get or create HLS session
	hlsSession, err := hlsManager.getOrCreate(sessionID, apiServer, width, height, bitrate, fps)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to create HLS session")
		http.Error(res, "Failed to start HLS stream", http.StatusServiceUnavailable)
		return
	}

	// Wait for muxer to be ready (up to 5 seconds)
	for i := 0; i < 50; i++ {
		hlsSession.mu.Lock()
		started := hlsSession.started
		hlsSession.mu.Unlock()
		if started {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	hlsSession.mu.Lock()
	if !hlsSession.started {
		hlsSession.mu.Unlock()
		log.Error().Str("session_id", sessionID).Msg("HLS muxer not ready after timeout")
		http.Error(res, "HLS stream not ready", http.StatusServiceUnavailable)
		return
	}
	hlsSession.mu.Unlock()

	// Extract the path suffix (e.g., "stream.m3u8" or "stream/segment.mp4")
	path := req.URL.Path
	// Find the part after sessionID
	parts := strings.Split(path, sessionID+"/")
	if len(parts) < 2 {
		http.Error(res, "Invalid path", http.StatusBadRequest)
		return
	}

	// Modify the request path for the muxer
	hlsPath := "/" + parts[1]
	req.URL.Path = hlsPath

	log.Debug().
		Str("session_id", sessionID).
		Str("original_path", path).
		Str("hls_path", hlsPath).
		Msg("Serving HLS request")

	// Let the muxer handle the request
	hlsSession.muxer.Handle(res, req)
}

// handleHLSSegment handles HLS segment requests
func (apiServer *HelixAPIServer) handleHLSSegment(res http.ResponseWriter, req *http.Request) {
	// This uses the same handler as handleHLSStream
	apiServer.handleHLSStream(res, req)
}
