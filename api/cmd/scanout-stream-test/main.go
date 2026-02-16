// scanout-stream-test verifies the full scanout streaming pipeline:
// 1. Requests a DRM lease from helix-drm-manager
// 2. Connects to QEMU TCP:15937
// 3. Sends SUBSCRIBE for the allocated scanout
// 4. Waits for H.264 frames
//
// Usage: scanout-stream-test [--qemu-addr 10.0.2.2:15937] [--drm-socket /run/helix-drm.sock]
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
)

const (
	helixMsgMagic    = 0x52465848
	msgSubscribe     = 0x30
	msgSubscribeResp = 0x31
	msgFrameResponse = 0x02
)

type msgHeader struct {
	Magic       uint32
	MsgType     uint8
	Flags       uint8
	SessionID   uint16
	PayloadSize uint32
}

func main() {
	qemuAddr := "10.0.2.2:15937"
	drmSocket := "/run/helix-drm/drm.sock"

	if len(os.Args) > 1 {
		qemuAddr = os.Args[1]
	}

	fmt.Println("=== Scanout Stream Test ===")
	fmt.Printf("QEMU addr: %s\n", qemuAddr)
	fmt.Printf("DRM socket: %s\n", drmSocket)

	// Step 1: Request DRM lease
	fmt.Println("\n--- Step 1: Requesting DRM lease ---")
	client := drmmanager.NewClient(drmSocket)
	lease, err := client.RequestLease(1920, 1080)
	if err != nil {
		fmt.Printf("ERROR requesting lease: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Lease granted: scanout=%d, connector=%s, fd=%d\n",
		lease.ScanoutID, lease.ConnectorName, lease.LeaseFD)

	// Step 2: Connect to QEMU
	fmt.Println("\n--- Step 2: Connecting to QEMU ---")
	conn, err := net.DialTimeout("tcp", qemuAddr, 5*time.Second)
	if err != nil {
		fmt.Printf("ERROR connecting to QEMU: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("Connected to QEMU")

	// Step 3: Subscribe to scanout
	fmt.Printf("\n--- Step 3: Subscribing to scanout %d ---\n", lease.ScanoutID)
	hdr := msgHeader{
		Magic:       helixMsgMagic,
		MsgType:     msgSubscribe,
		SessionID:   uint16(lease.ScanoutID),
		PayloadSize: 4,
	}
	if err := binary.Write(conn, binary.LittleEndian, hdr); err != nil {
		fmt.Printf("ERROR writing subscribe header: %v\n", err)
		os.Exit(1)
	}
	if err := binary.Write(conn, binary.LittleEndian, lease.ScanoutID); err != nil {
		fmt.Printf("ERROR writing scanout_id: %v\n", err)
		os.Exit(1)
	}

	// Read subscribe response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var respHdr msgHeader
	if err := binary.Read(conn, binary.LittleEndian, &respHdr); err != nil {
		fmt.Printf("ERROR reading subscribe response: %v\n", err)
		os.Exit(1)
	}
	if respHdr.MsgType == msgSubscribeResp {
		var payload struct {
			ScanoutID uint32
			Success   uint32
		}
		binary.Read(conn, binary.LittleEndian, &payload)
		fmt.Printf("Subscribe response: scanout=%d, success=%d\n",
			payload.ScanoutID, payload.Success)
	} else {
		fmt.Printf("Unexpected response type: 0x%x\n", respHdr.MsgType)
		if respHdr.PayloadSize > 0 {
			skip := make([]byte, respHdr.PayloadSize)
			io.ReadFull(conn, skip)
		}
	}

	// Step 4: Wait for H.264 frames
	fmt.Println("\n--- Step 4: Waiting for H.264 frames ---")
	fmt.Println("(Frames will appear when something renders on the scanout's connector)")
	fmt.Println("(Try: modetest -M virtio_gpu -s 45:1920x1080 in another terminal)")

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	frameCount := 0
	totalBytes := 0
	startTime := time.Now()

	for {
		var fhdr msgHeader
		if err := binary.Read(conn, binary.LittleEndian, &fhdr); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("\nTimeout waiting for frames (60s)")
				break
			}
			fmt.Printf("\nRead error: %v\n", err)
			break
		}

		if fhdr.MsgType == msgFrameResponse {
			// Read frame data
			payload := make([]byte, fhdr.PayloadSize)
			if _, err := io.ReadFull(conn, payload); err != nil {
				fmt.Printf("Error reading frame payload: %v\n", err)
				break
			}
			frameCount++
			totalBytes += int(fhdr.PayloadSize)
			elapsed := time.Since(startTime).Seconds()
			fps := float64(frameCount) / elapsed
			fmt.Printf("\rFrame %d: %d bytes, scanout=%d, %.1f FPS, total=%d KB",
				frameCount, fhdr.PayloadSize, fhdr.SessionID, fps, totalBytes/1024)
		} else {
			// Skip other messages
			if fhdr.PayloadSize > 0 {
				skip := make([]byte, fhdr.PayloadSize)
				io.ReadFull(conn, skip)
			}
		}
	}

	fmt.Printf("\n\nResults: %d frames, %d KB total, %.1f FPS avg\n",
		frameCount, totalBytes/1024,
		float64(frameCount)/time.Since(startTime).Seconds())
}
