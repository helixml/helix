//go:build ignore

// Test client for helix-drm-manager.
// Build: go build -o /tmp/drm-test-client test_client.go
// Usage: /tmp/drm-test-client [request|release <scanout_id>]
package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

const socketPath = "/run/helix-drm/drm.sock"

type leaseRequest struct {
	Cmd    uint8
	Width  uint32
	Height uint32
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: drm-test-client [request|release <scanout_id>]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "request":
		requestLease()
	case "release":
		if len(os.Args) < 3 {
			fmt.Println("Usage: drm-test-client release <scanout_id>")
			os.Exit(1)
		}
		id, _ := strconv.Atoi(os.Args[2])
		releaseLease(uint32(id))
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func requestLease() {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	unixConn := conn.(*net.UnixConn)

	req := leaseRequest{Cmd: 1, Width: 1920, Height: 1080}
	if err := binary.Write(unixConn, binary.LittleEndian, req); err != nil {
		fmt.Printf("Failed to write request: %v\n", err)
		os.Exit(1)
	}

	respBuf := make([]byte, 69)
	oob := make([]byte, unix.CmsgLen(4))
	n, oobn, _, _, err := unixConn.ReadMsgUnix(respBuf, oob)
	if err != nil {
		fmt.Printf("Failed to read response: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Response: %d bytes, %d oob bytes\n", n, oobn)

	status := respBuf[0]
	scanoutID := binary.LittleEndian.Uint32(respBuf[1:5])
	connName := string(respBuf[5:69])
	// Trim nulls
	for i, b := range respBuf[5:69] {
		if b == 0 {
			connName = string(respBuf[5 : 5+i])
			break
		}
	}

	fmt.Printf("Status: %d\n", status)
	fmt.Printf("Scanout ID: %d\n", scanoutID)
	fmt.Printf("Connector: %s\n", connName)

	if status != 0 {
		fmt.Printf("Error: %s\n", connName)
		os.Exit(1)
	}

	// Extract lease FD
	scms, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		fmt.Printf("Failed to parse control message: %v\n", err)
		os.Exit(1)
	}

	for _, scm := range scms {
		fds, err := unix.ParseUnixRights(&scm)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			fmt.Printf("Received lease FD: %d\n", fd)
			// Verify it's a valid DRM device
			var stat unix.Stat_t
			if err := unix.Fstat(fd, &stat); err != nil {
				fmt.Printf("  fstat failed: %v\n", err)
			} else {
				fmt.Printf("  Device: major=%d minor=%d\n", unix.Major(stat.Rdev), unix.Minor(stat.Rdev))
			}
			unix.Close(fd)
		}
	}
}

func releaseLease(scanoutID uint32) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	req := leaseRequest{Cmd: 2, Width: scanoutID}
	if err := binary.Write(conn, binary.LittleEndian, req); err != nil {
		fmt.Printf("Failed to write release request: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Released scanout %d\n", scanoutID)
}
