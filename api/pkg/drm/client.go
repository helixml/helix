package drm

import (
	"encoding/binary"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// Client connects to helix-drm-manager and requests DRM leases.
type Client struct {
	socketPath string
}

// NewClient creates a client for the DRM manager.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// LeaseResult contains the result of a successful lease request.
type LeaseResult struct {
	ScanoutID     uint32
	ConnectorName string
	LeaseFD       int // DRM lease file descriptor - caller must close when done

	// conn is the persistent connection to the DRM manager. Keeping it open
	// acts as a liveness signal — when the process dies (even SIGKILL), the
	// kernel closes the socket and the manager automatically releases the
	// scanout. Call Close() when the lease is no longer needed.
	conn net.Conn
}

// Close releases the lease by closing the liveness connection to the DRM manager.
// The manager detects the disconnect and automatically releases the scanout.
func (r *LeaseResult) Close() {
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
}

// RequestLease requests a DRM lease from the manager.
// Returns a LeaseResult with the lease FD on success.
// The caller owns the FD and must close it when done.
// The LeaseResult also holds an open connection to the manager as a liveness
// signal — call LeaseResult.Close() to release the scanout, or let the process
// exit (the kernel will close the connection automatically).
func (c *Client) RequestLease(width, height uint32) (*LeaseResult, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", c.socketPath, err)
	}

	unixConn := conn.(*net.UnixConn)

	// Send request
	req := leaseRequest{
		Cmd:    cmdRequestLease,
		Width:  width,
		Height: height,
	}
	if err := binary.Write(unixConn, binary.LittleEndian, req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read response with SCM_RIGHTS FD
	respBuf := make([]byte, 69)
	oob := make([]byte, unix.CmsgLen(4)) // space for one FD
	n, oobn, _, _, err := unixConn.ReadMsgUnix(respBuf, oob)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}
	if n < 69 {
		conn.Close()
		return nil, fmt.Errorf("short response: %d bytes", n)
	}

	status := respBuf[0]
	scanoutID := binary.LittleEndian.Uint32(respBuf[1:5])
	var connName [64]byte
	copy(connName[:], respBuf[5:69])

	// Trim null bytes from connector name
	connStr := ""
	for i, b := range connName {
		if b == 0 {
			connStr = string(connName[:i])
			break
		}
	}

	if status != 0 {
		conn.Close()
		return nil, fmt.Errorf("lease request failed: %s", connStr)
	}

	// Extract lease FD from SCM_RIGHTS
	scms, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse control message: %w", err)
	}

	var leaseFD int = -1
	for _, scm := range scms {
		fds, err := unix.ParseUnixRights(&scm)
		if err != nil {
			continue
		}
		if len(fds) > 0 {
			leaseFD = fds[0]
			// Close any extra FDs
			for _, fd := range fds[1:] {
				unix.Close(fd)
			}
			break
		}
	}

	if leaseFD < 0 {
		conn.Close()
		return nil, fmt.Errorf("no lease FD received via SCM_RIGHTS")
	}

	// Connection intentionally kept open — acts as liveness signal to the
	// manager. When this process dies, the kernel closes the socket and
	// the manager automatically releases the scanout.
	return &LeaseResult{
		ScanoutID:     scanoutID,
		ConnectorName: connStr,
		LeaseFD:       leaseFD,
		conn:          conn,
	}, nil
}

// ReleaseLease tells the manager to release a scanout.
func (c *Client) ReleaseLease(scanoutID uint32) error {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.socketPath, err)
	}
	defer conn.Close()

	req := leaseRequest{
		Cmd:   cmdReleaseLease,
		Width: scanoutID, // reuse Width field for scanout ID
	}
	if err := binary.Write(conn, binary.LittleEndian, req); err != nil {
		return fmt.Errorf("write release request: %w", err)
	}

	return nil
}
