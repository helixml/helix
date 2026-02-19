package drm

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// Config configures the DRM manager.
type Config struct {
	DRMDevice  string // e.g., /dev/dri/card0
	SocketPath string // e.g., /run/helix-drm.sock
	QEMUAddr   string // e.g., 10.0.2.2:15937 (TCP to QEMU frame export server)
}

// LeaseInfo tracks an active DRM lease.
type LeaseInfo struct {
	ScanoutIdx  uint32
	ConnectorID uint32
	CrtcID      uint32
	LesseeID    uint32
	LeaseFD     int
	Width       uint32
	Height      uint32
}

// Manager manages DRM leases for container desktops.
type Manager struct {
	cfg    Config
	logger *slog.Logger

	drmFile *os.File

	mu          sync.Mutex
	available   []uint32             // free scanout indices (1-15)
	leases      map[uint32]*LeaseInfo // scanout_idx -> lease
	crtcIDs     []uint32
	connectorIDs []uint32
}

// New creates a DRM manager. Opens the DRM device and becomes master.
func New(cfg Config, logger *slog.Logger) (*Manager, error) {
	drmFile, err := openDRM(cfg.DRMDevice)
	if err != nil {
		return nil, fmt.Errorf("open DRM: %w", err)
	}

	crtcIDs, connectorIDs, err := getResources(drmFile)
	if err != nil {
		drmFile.Close()
		return nil, fmt.Errorf("get resources: %w", err)
	}

	logger.Info("DRM resources enumerated",
		"crtcs", len(crtcIDs),
		"connectors", len(connectorIDs))

	// Log connector and CRTC IDs for debugging
	for i, id := range connectorIDs {
		logger.Debug("connector", "index", i, "id", id)
	}
	for i, id := range crtcIDs {
		logger.Debug("crtc", "index", i, "id", id)
	}

	// Build available scanout pool (indices 1-15; 0 = VM console)
	maxScanouts := len(connectorIDs) - 1
	if maxScanouts > 15 {
		maxScanouts = 15
	}
	available := make([]uint32, 0, maxScanouts)
	for i := 1; i <= maxScanouts; i++ {
		available = append(available, uint32(i))
	}

	return &Manager{
		cfg:          cfg,
		logger:       logger,
		drmFile:      drmFile,
		available:    available,
		leases:       make(map[uint32]*LeaseInfo),
		crtcIDs:      crtcIDs,
		connectorIDs: connectorIDs,
	}, nil
}

// Run starts the Unix socket listener and serves lease requests.
func (m *Manager) Run(ctx context.Context) error {
	// Ensure parent directory exists (directory bind mounts survive socket
	// recreation across DRM manager restarts, unlike file bind mounts)
	if dir := filepath.Dir(m.cfg.SocketPath); dir != "." && dir != "/" {
		os.MkdirAll(dir, 0755)
	}
	os.Remove(m.cfg.SocketPath)

	ln, err := net.Listen("unix", m.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.cfg.SocketPath, err)
	}
	defer ln.Close()

	// Make socket world-accessible so containers can connect
	if err := os.Chmod(m.cfg.SocketPath, 0777); err != nil {
		m.logger.Warn("chmod socket failed", "err", err)
	}

	m.logger.Info("listening for lease requests",
		"socket", m.cfg.SocketPath,
		"available_scanouts", len(m.available))

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				m.logger.Error("accept error", "err", err)
				continue
			}
		}
		go m.handleClient(ctx, conn)
	}
}

// Wire protocol on the Unix socket (helix-drm.sock):
//
// Client -> Manager:
//   Request: { cmd uint8, width uint32, height uint32 }
//     cmd=1: request lease (allocate scanout, enable, create lease, send FD)
//     cmd=2: release lease (clean up, disable scanout)
//
// Manager -> Client:
//   Response: { status uint8, scanout_id uint32, connector_name [64]byte }
//     status=0: success (lease FD sent via SCM_RIGHTS)
//     status=1: error (connector_name contains error message)
//
// For release: client just closes the socket.

const (
	cmdRequestLease = 1
	cmdReleaseLease = 2
)

type leaseRequest struct {
	Cmd    uint8
	Width  uint32
	Height uint32
}

type leaseResponse struct {
	Status        uint8
	ScanoutID     uint32
	ConnectorName [64]byte
}

func (m *Manager) handleClient(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		m.logger.Error("connection is not UnixConn")
		return
	}

	var req leaseRequest
	if err := binary.Read(conn, binary.LittleEndian, &req); err != nil {
		m.logger.Error("read request failed", "err", err)
		return
	}

	switch req.Cmd {
	case cmdRequestLease:
		scanoutIdx := m.handleLeaseRequest(ctx, unixConn, req.Width, req.Height)
		if scanoutIdx == 0 {
			return // lease request failed, nothing to track
		}

		// Block on the connection — when the client dies (SIGKILL, crash,
		// graceful shutdown), the kernel closes the socket and this read
		// returns. This is the liveness signal for automatic cleanup.
		buf := make([]byte, 1)
		conn.Read(buf) // blocks until connection closes

		m.logger.Info("lease client disconnected, releasing scanout", "scanout_idx", scanoutIdx)
		m.handleLeaseRelease(scanoutIdx)

	case cmdReleaseLease:
		// Client is releasing; scanout ID is in Width field (reused)
		m.handleLeaseRelease(req.Width)
	default:
		m.logger.Error("unknown command", "cmd", req.Cmd)
		m.sendError(conn, "unknown command")
	}
}

// handleLeaseRequest processes a lease request and returns the allocated scanout index.
// Returns 0 if the request failed (error already sent to client).
func (m *Manager) handleLeaseRequest(ctx context.Context, conn *net.UnixConn, width, height uint32) uint32 {
	if width == 0 {
		width = 1920
	}
	if height == 0 {
		height = 1080
	}

	m.logger.Info("lease request received", "width", width, "height", height)

	// 1. Allocate scanout index
	scanoutIdx, err := m.allocateScanout()
	if err != nil {
		m.logger.Error("no scanouts available", "err", err)
		m.sendError(conn, err.Error())
		return 0
	}
	m.logger.Info("allocated scanout", "scanout_idx", scanoutIdx)

	// 2. Send HELIX_MSG_ENABLE_SCANOUT to QEMU
	if err := m.enableScanoutInQEMU(scanoutIdx, width, height); err != nil {
		m.logger.Error("enable scanout in QEMU failed", "err", err, "scanout_idx", scanoutIdx)
		m.releaseScanout(scanoutIdx)
		m.sendError(conn, fmt.Sprintf("QEMU enable failed: %v", err))
		return 0
	}
	m.logger.Info("scanout enabled in QEMU", "scanout_idx", scanoutIdx)

	// 3. Skip connector reprobe — writing to /sys/class/drm/card0-Virtual-N/status
	// calls drm_helper_probe_single_connector_modes which acquires mode_config.mutex.
	// If a gnome-shell is mid-atomic-commit waiting for a GPU fence, it holds
	// mode_config.mutex and this blocks indefinitely.  QEMU's enableScanout already
	// triggers dpy_set_ui_info → guest hotplug event, so the connector should
	// appear without an explicit reprobe.

	// 4. Wait briefly for connector to become connected
	time.Sleep(500 * time.Millisecond)

	// 5. Skip activateCrtc — it does DRM_IOCTL_MODE_SETCRTC on the master FD,
	// which acquires mode_config.mutex and deadlocks with running gnome-shells
	// doing atomic page flips on their lease FDs. Mutter should handle the
	// initial modeset itself via the lease FD now that DRM_CLIENT_CAP_UNIVERSAL_PLANES
	// is set on the master FD (see openDRM).
	connectorID := m.connectorIDForScanout(scanoutIdx)
	crtcID := m.crtcIDForScanout(scanoutIdx)
	primaryPlaneID, cursorPlaneID := m.planeIDsForScanout(scanoutIdx)

	// 6. Create DRM lease (connector + CRTC + planes)
	// DRM_CLIENT_CAP_UNIVERSAL_PLANES must be set on the master FD first
	// (done in openDRM), otherwise plane IDs don't exist and lease fails.
	m.logger.Info("creating DRM lease",
		"scanout_idx", scanoutIdx,
		"connector_id", connectorID,
		"crtc_id", crtcID,
		"primary_plane", primaryPlaneID,
		"cursor_plane", cursorPlaneID)

	objectIDs := []uint32{connectorID, crtcID, primaryPlaneID, cursorPlaneID}
	leaseFD, lesseeID, err := createLease(m.drmFile, objectIDs)
	if err != nil {
		m.logger.Error("create lease failed", "err", err)
		m.disableScanoutInQEMU(scanoutIdx)
		m.releaseScanout(scanoutIdx)
		m.sendError(conn, fmt.Sprintf("DRM lease failed: %v", err))
		return 0
	}

	m.logger.Info("DRM lease created",
		"lessee_id", lesseeID,
		"lease_fd", leaseFD,
		"scanout_idx", scanoutIdx)

	// Track the lease
	m.mu.Lock()
	m.leases[scanoutIdx] = &LeaseInfo{
		ScanoutIdx:  scanoutIdx,
		ConnectorID: connectorID,
		CrtcID:      crtcID,
		LesseeID:    lesseeID,
		LeaseFD:     leaseFD,
		Width:       width,
		Height:      height,
	}
	m.mu.Unlock()

	// 6. Send lease FD to client via SCM_RIGHTS
	connectorName := fmt.Sprintf("Virtual-%d", scanoutIdx+1)
	resp := leaseResponse{
		Status:    0,
		ScanoutID: scanoutIdx,
	}
	copy(resp.ConnectorName[:], connectorName)

	// Marshal response
	respBuf := make([]byte, 69) // 1 + 4 + 64
	respBuf[0] = resp.Status
	binary.LittleEndian.PutUint32(respBuf[1:5], resp.ScanoutID)
	copy(respBuf[5:], resp.ConnectorName[:])

	// Send response with lease FD via SCM_RIGHTS
	rights := unix.UnixRights(leaseFD)
	_, _, err = conn.WriteMsgUnix(respBuf, rights, nil)
	if err != nil {
		m.logger.Error("send lease FD failed", "err", err)
		// Clean up
		unix.Close(leaseFD)
		revokeLease(m.drmFile, lesseeID)
		m.disableScanoutInQEMU(scanoutIdx)
		m.releaseScanout(scanoutIdx)
		return 0
	}

	m.logger.Info("lease FD sent to client",
		"scanout_idx", scanoutIdx,
		"connector", connectorName,
		"lessee_id", lesseeID)

	// Close our copy of the lease FD (client has it now)
	unix.Close(leaseFD)

	return scanoutIdx
}

func (m *Manager) handleLeaseRelease(scanoutIdx uint32) {
	m.mu.Lock()
	lease, ok := m.leases[scanoutIdx]
	if !ok {
		m.mu.Unlock()
		m.logger.Warn("release request for unknown scanout", "scanout_idx", scanoutIdx)
		return
	}
	delete(m.leases, scanoutIdx)
	m.mu.Unlock()

	m.logger.Info("releasing lease", "scanout_idx", scanoutIdx, "lessee_id", lease.LesseeID)

	// Revoke the DRM lease
	if err := revokeLease(m.drmFile, lease.LesseeID); err != nil {
		m.logger.Warn("revoke lease failed", "err", err)
	}

	// Disable scanout in QEMU
	m.disableScanoutInQEMU(scanoutIdx)

	// Return scanout to pool
	m.releaseScanout(scanoutIdx)
}

func (m *Manager) allocateScanout() (uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.available) == 0 {
		return 0, fmt.Errorf("no scanouts available (all %d in use)", len(m.leases))
	}
	idx := m.available[0]
	m.available = m.available[1:]
	return idx, nil
}

func (m *Manager) releaseScanout(idx uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.available = append(m.available, idx)
}

// connectorIDForScanout returns the DRM connector ID for a scanout index.
// Uses the actual enumerated connector IDs from DRM.
func (m *Manager) connectorIDForScanout(scanoutIdx uint32) uint32 {
	idx := int(scanoutIdx)
	if idx < len(m.connectorIDs) {
		return m.connectorIDs[idx]
	}
	// Fallback: use the pattern from design doc
	return 38 + scanoutIdx*7
}

// crtcIDForScanout returns the DRM CRTC ID for a scanout index.
// Uses the actual enumerated CRTC IDs from DRM.
func (m *Manager) crtcIDForScanout(scanoutIdx uint32) uint32 {
	idx := int(scanoutIdx)
	if idx < len(m.crtcIDs) {
		return m.crtcIDs[idx]
	}
	// Fallback: use the pattern from design doc
	return 37 + scanoutIdx*7
}

// planeIDsForScanout returns the primary and cursor plane IDs for a scanout.
// virtio-gpu creates 2 planes per CRTC: primary (type=1) and cursor (type=2).
// Pattern: primary = base + scanout * 7, cursor = base + 1 + scanout * 7
func (m *Manager) planeIDsForScanout(scanoutIdx uint32) (primaryPlaneID, cursorPlaneID uint32) {
	// The plane IDs follow the pattern: 33 + N*7 (primary), 34 + N*7 (cursor)
	// This matches the connector/CRTC pattern but offset by -4
	primaryPlaneID = 33 + scanoutIdx*7
	cursorPlaneID = 34 + scanoutIdx*7
	return
}

func (m *Manager) enableScanoutInQEMU(scanoutIdx, width, height uint32) error {
	conn, err := net.DialTimeout("tcp", m.cfg.QEMUAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to QEMU %s: %w", m.cfg.QEMUAddr, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := writeEnableScanout(conn, scanoutIdx, width, height); err != nil {
		return fmt.Errorf("write enable scanout: %w", err)
	}

	resp, err := readScanoutResp(conn)
	if err != nil {
		return fmt.Errorf("read scanout response: %w", err)
	}

	if resp.Success == 0 {
		return fmt.Errorf("QEMU refused to enable scanout %d", scanoutIdx)
	}

	connName := string(resp.Connector[:])
	// Trim null bytes
	for i, b := range resp.Connector {
		if b == 0 {
			connName = string(resp.Connector[:i])
			break
		}
	}
	m.logger.Info("QEMU confirmed scanout enabled", "scanout_idx", scanoutIdx, "connector", connName)
	return nil
}

func (m *Manager) disableScanoutInQEMU(scanoutIdx uint32) {
	conn, err := net.DialTimeout("tcp", m.cfg.QEMUAddr, 5*time.Second)
	if err != nil {
		m.logger.Warn("connect to QEMU for disable failed", "err", err)
		return
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := writeDisableScanout(conn, scanoutIdx); err != nil {
		m.logger.Warn("write disable scanout failed", "err", err)
		return
	}

	resp, err := readScanoutResp(conn)
	if err != nil {
		m.logger.Warn("read disable response failed", "err", err)
		return
	}

	m.logger.Info("QEMU confirmed scanout disabled",
		"scanout_idx", scanoutIdx,
		"success", resp.Success)
}

func (m *Manager) sendError(conn net.Conn, msg string) {
	resp := leaseResponse{Status: 1}
	copy(resp.ConnectorName[:], msg)
	respBuf := make([]byte, 69)
	respBuf[0] = resp.Status
	binary.LittleEndian.PutUint32(respBuf[1:5], resp.ScanoutID)
	copy(respBuf[5:], resp.ConnectorName[:])
	conn.Write(respBuf)
}

// Close cleans up the DRM manager.
func (m *Manager) Close() error {
	m.mu.Lock()
	leases := make([]*LeaseInfo, 0, len(m.leases))
	for _, l := range m.leases {
		leases = append(leases, l)
	}
	m.mu.Unlock()

	for _, l := range leases {
		m.handleLeaseRelease(l.ScanoutIdx)
	}

	if err := dropMaster(m.drmFile); err != nil {
		m.logger.Warn("drop master failed", "err", err)
	}
	return m.drmFile.Close()
}
