package gptscript

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// ZedSession represents an active Zed editor session
type ZedSession struct {
	SessionID    string
	PID          int
	DisplayNum   int
	RDPPort      int
	VNCPort      int
	WorkspaceDir string
	Cmd          *exec.Cmd
	StartTime    time.Time
	LastAccess   time.Time
	mu           sync.RWMutex
}

// ZedExecutor manages Zed editor instances
type ZedExecutor struct {
	sessions      map[string]*ZedSession
	mu            sync.RWMutex
	displayBase   int
	portBase      int
	vncBase       int
	workspaceBase string
}

// NewZedExecutor creates a new Zed executor
func NewZedExecutor(displayBase, portBase, vncBase int, workspaceBase string) *ZedExecutor {
	return &ZedExecutor{
		sessions:      make(map[string]*ZedSession),
		displayBase:   displayBase,
		portBase:      portBase,
		vncBase:       vncBase,
		workspaceBase: workspaceBase,
	}
}

// StartZedAgent starts a new Zed agent instance
func (ze *ZedExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	ze.mu.Lock()
	defer ze.mu.Unlock()

	// Check if session already exists
	if session, exists := ze.sessions[agent.SessionID]; exists {
		log.Info().Str("session_id", agent.SessionID).Msg("Zed session already exists")
		return &types.ZedAgentResponse{
			SessionID: agent.SessionID,
			RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
			Status:    "running",
			PID:       session.PID,
		}, nil
	}

	// Create new session
	displayNum := ze.displayBase + len(ze.sessions)
	rdpPort := ze.portBase + len(ze.sessions)
	vncPort := ze.vncBase + len(ze.sessions)

	workspaceDir := filepath.Join(ze.workspaceBase, agent.SessionID)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	session := &ZedSession{
		SessionID:    agent.SessionID,
		DisplayNum:   displayNum,
		RDPPort:      rdpPort,
		VNCPort:      vncPort,
		WorkspaceDir: workspaceDir,
		StartTime:    time.Now(),
		LastAccess:   time.Now(),
	}

	// Start X server
	if err := ze.startXServer(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to start X server: %w", err)
	}

	// Start VNC server
	if err := ze.startVNCServer(ctx, session); err != nil {
		ze.cleanup(session)
		return nil, fmt.Errorf("failed to start VNC server: %w", err)
	}

	// Start RDP server (x11vnc with RDP bridge or similar)
	if err := ze.startRDPServer(ctx, session); err != nil {
		ze.cleanup(session)
		return nil, fmt.Errorf("failed to start RDP server: %w", err)
	}

	// Start Zed editor
	if err := ze.startZedEditor(ctx, session, agent); err != nil {
		ze.cleanup(session)
		return nil, fmt.Errorf("failed to start Zed editor: %w", err)
	}

	// Store session
	ze.sessions[agent.SessionID] = session

	log.Info().
		Str("session_id", agent.SessionID).
		Int("pid", session.PID).
		Int("display", session.DisplayNum).
		Int("rdp_port", session.RDPPort).
		Int("vnc_port", session.VNCPort).
		Msg("Zed agent started successfully")

	return &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		Status:    "running",
		PID:       session.PID,
	}, nil
}

// StopZedAgent stops a Zed agent instance
func (ze *ZedExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	ze.mu.Lock()
	defer ze.mu.Unlock()

	session, exists := ze.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	ze.cleanup(session)
	delete(ze.sessions, sessionID)

	log.Info().Str("session_id", sessionID).Msg("Zed agent stopped")
	return nil
}

// GetSession returns information about a session
func (ze *ZedExecutor) GetSession(sessionID string) (*ZedSession, bool) {
	ze.mu.RLock()
	defer ze.mu.RUnlock()

	session, exists := ze.sessions[sessionID]
	if exists {
		session.mu.Lock()
		session.LastAccess = time.Now()
		session.mu.Unlock()
	}
	return session, exists
}

// ListSessions returns all active sessions
func (ze *ZedExecutor) ListSessions() []*ZedSession {
	ze.mu.RLock()
	defer ze.mu.RUnlock()

	sessions := make([]*ZedSession, 0, len(ze.sessions))
	for _, session := range ze.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// startXServer starts an X server for the session
func (ze *ZedExecutor) startXServer(ctx context.Context, session *ZedSession) error {
	displayStr := fmt.Sprintf(":%d", session.DisplayNum)

	// Start Xvfb (virtual framebuffer X server)
	cmd := exec.CommandContext(ctx, "Xvfb", displayStr,
		"-screen", "0", "1920x1080x24",
		"-ac", "+extension", "GLX", "+render", "-noreset")

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Xvfb: %w", err)
	}

	// Wait a moment for X server to start
	time.Sleep(2 * time.Second)

	log.Info().
		Str("session_id", session.SessionID).
		Str("display", displayStr).
		Msg("X server started")

	return nil
}

// startVNCServer starts a VNC server for the session
func (ze *ZedExecutor) startVNCServer(ctx context.Context, session *ZedSession) error {
	displayStr := fmt.Sprintf(":%d", session.DisplayNum)

	// Start x11vnc
	cmd := exec.CommandContext(ctx, "x11vnc",
		"-display", displayStr,
		"-rfbport", strconv.Itoa(session.VNCPort),
		"-forever",
		"-noxdamage",
		"-shared",
		"-passwd", "zedagent")

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start VNC server: %w", err)
	}

	log.Info().
		Str("session_id", session.SessionID).
		Int("vnc_port", session.VNCPort).
		Msg("VNC server started")

	return nil
}

// startRDPServer starts an RDP server for the session
func (ze *ZedExecutor) startRDPServer(ctx context.Context, session *ZedSession) error {
	// Use xrdp or a similar RDP server
	// For now, we'll use a simple approach with socat to bridge VNC to RDP-like protocol
	// In a real implementation, you'd want to use a proper RDP server like xrdp

	cmd := exec.CommandContext(ctx, "socat",
		fmt.Sprintf("TCP-LISTEN:%d,fork", session.RDPPort),
		fmt.Sprintf("TCP:localhost:%d", session.VNCPort))

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start RDP bridge: %w", err)
	}

	log.Info().
		Str("session_id", session.SessionID).
		Int("rdp_port", session.RDPPort).
		Msg("RDP bridge started")

	return nil
}

// startZedEditor starts the Zed editor
func (ze *ZedExecutor) startZedEditor(ctx context.Context, session *ZedSession, agent *types.ZedAgent) error {
	displayStr := fmt.Sprintf(":%d", session.DisplayNum)

	// Set up environment
	env := append(os.Environ(),
		"DISPLAY="+displayStr,
		"XAUTHORITY=/tmp/.X11-auth-"+session.SessionID,
	)

	// Add custom environment variables from agent config
	env = append(env, agent.Env...)

	// Determine working directory
	workDir := session.WorkspaceDir
	if agent.WorkDir != "" {
		workDir = agent.WorkDir
	}

	// Build Zed command
	args := []string{}
	if agent.ProjectPath != "" {
		// Create or ensure project directory exists
		projectDir := filepath.Join(workDir, agent.ProjectPath)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			return fmt.Errorf("failed to create project directory: %w", err)
		}
		args = append(args, projectDir)
	}

	// Start Zed editor
	zedBinary := "zed"
	if binary := os.Getenv("ZED_BINARY"); binary != "" {
		zedBinary = binary
	}

	cmd := exec.CommandContext(ctx, zedBinary, args...)
	cmd.Dir = workDir
	cmd.Env = env

	// Set process group to make cleanup easier
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Zed editor: %w", err)
	}

	session.Cmd = cmd
	session.PID = cmd.Process.Pid

	// Start a goroutine to wait for the process and handle cleanup
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Error().
				Err(err).
				Str("session_id", session.SessionID).
				Msg("Zed editor process exited with error")
		} else {
			log.Info().
				Str("session_id", session.SessionID).
				Msg("Zed editor process exited normally")
		}

		// Auto-cleanup when Zed exits
		ze.mu.Lock()
		if _, exists := ze.sessions[session.SessionID]; exists {
			ze.cleanup(session)
			delete(ze.sessions, session.SessionID)
		}
		ze.mu.Unlock()
	}()

	log.Info().
		Str("session_id", session.SessionID).
		Int("pid", session.PID).
		Str("work_dir", workDir).
		Msg("Zed editor started")

	return nil
}

// cleanup cleans up all resources for a session
func (ze *ZedExecutor) cleanup(session *ZedSession) {
	if session.Cmd != nil && session.Cmd.Process != nil {
		// Kill the process group to ensure all child processes are terminated
		pgid, err := syscall.Getpgid(session.PID)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)

			// Wait a bit, then force kill if necessary
			time.Sleep(2 * time.Second)
			syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			// Fallback to killing just the main process
			session.Cmd.Process.Kill()
		}
	}

	// Kill associated X server, VNC server, and RDP bridge processes
	// This is a simplified approach - in production you'd want more sophisticated process tracking
	displayStr := fmt.Sprintf(":%d", session.DisplayNum)

	// Kill Xvfb
	exec.Command("pkill", "-f", "Xvfb.*"+displayStr).Run()

	// Kill x11vnc
	exec.Command("pkill", "-f", "x11vnc.*"+strconv.Itoa(session.VNCPort)).Run()

	// Kill socat (RDP bridge)
	exec.Command("pkill", "-f", "socat.*"+strconv.Itoa(session.RDPPort)).Run()

	log.Info().
		Str("session_id", session.SessionID).
		Msg("Session cleanup completed")
}

// CleanupExpiredSessions removes sessions that have been inactive for too long
func (ze *ZedExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	ze.mu.Lock()
	defer ze.mu.Unlock()

	now := time.Now()
	expiredSessions := []string{}

	for sessionID, session := range ze.sessions {
		session.mu.RLock()
		if now.Sub(session.LastAccess) > timeout {
			expiredSessions = append(expiredSessions, sessionID)
		}
		session.mu.RUnlock()
	}

	for _, sessionID := range expiredSessions {
		session := ze.sessions[sessionID]
		ze.cleanup(session)
		delete(ze.sessions, sessionID)

		log.Info().
			Str("session_id", sessionID).
			Msg("Expired session cleaned up")
	}
}
