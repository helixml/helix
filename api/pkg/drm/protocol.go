package drm

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Helix Frame Export Protocol constants.
// Must match helix-frame-export.h and gstvsockenc.h.
const (
	helixMsgMagic = 0x52465848 // 'HXFR' little-endian

	msgEnableScanout  = 0x20
	msgDisableScanout = 0x21
	msgScanoutResp    = 0x22
	msgSubscribe      = 0x30
	msgSubscribeResp  = 0x31
	msgFrameResponse  = 0x02
	msgError          = 0xFF
)

// helixMsgHeader is the common message header (12 bytes).
type helixMsgHeader struct {
	Magic       uint32
	MsgType     uint8
	Flags       uint8
	SessionID   uint16
	PayloadSize uint32
}

// enableScanoutPayload is sent after the header for ENABLE_SCANOUT.
type enableScanoutPayload struct {
	ScanoutID   uint32
	Width       uint32
	Height      uint32
	RefreshRate uint32
}

// disableScanoutPayload is sent after the header for DISABLE_SCANOUT.
type disableScanoutPayload struct {
	ScanoutID uint32
}

// scanoutRespPayload is received after the header for SCANOUT_RESP.
type scanoutRespPayload struct {
	ScanoutID uint32
	Success   uint32
	Connector [64]byte
}

func writeEnableScanout(w io.Writer, scanoutID, width, height uint32) error {
	payload := enableScanoutPayload{
		ScanoutID:   scanoutID,
		Width:       width,
		Height:      height,
		RefreshRate: 60,
	}
	hdr := helixMsgHeader{
		Magic:       helixMsgMagic,
		MsgType:     msgEnableScanout,
		PayloadSize: 16, // 4 uint32s
	}
	if err := binary.Write(w, binary.LittleEndian, hdr); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

func writeDisableScanout(w io.Writer, scanoutID uint32) error {
	payload := disableScanoutPayload{ScanoutID: scanoutID}
	hdr := helixMsgHeader{
		Magic:       helixMsgMagic,
		MsgType:     msgDisableScanout,
		PayloadSize: 4,
	}
	if err := binary.Write(w, binary.LittleEndian, hdr); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// WriteSubscribe sends a SUBSCRIBE message to QEMU to receive H.264 frames
// for a specific scanout.
func WriteSubscribe(w io.Writer, scanoutID uint32) error {
	hdr := helixMsgHeader{
		Magic:       helixMsgMagic,
		MsgType:     msgSubscribe,
		SessionID:   uint16(scanoutID),
		PayloadSize: 4,
	}
	if err := binary.Write(w, binary.LittleEndian, hdr); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, scanoutID); err != nil {
		return fmt.Errorf("write scanout_id: %w", err)
	}
	return nil
}

// ReadSubscribeResp reads the SUBSCRIBE_RESP from QEMU.
func ReadSubscribeResp(r io.Reader) (scanoutID uint32, success bool, err error) {
	var hdr helixMsgHeader
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return 0, false, fmt.Errorf("read header: %w", err)
	}
	if hdr.Magic != helixMsgMagic {
		return 0, false, fmt.Errorf("bad magic: 0x%x", hdr.Magic)
	}
	if hdr.MsgType != msgSubscribeResp {
		return 0, false, fmt.Errorf("unexpected msg_type: 0x%x", hdr.MsgType)
	}
	var payload struct {
		ScanoutID uint32
		Success   uint32
	}
	if err := binary.Read(r, binary.LittleEndian, &payload); err != nil {
		return 0, false, fmt.Errorf("read payload: %w", err)
	}
	return payload.ScanoutID, payload.Success != 0, nil
}

// FrameResponseHeader is the header for H.264 frame data from QEMU.
type FrameResponseHeader struct {
	Header    helixMsgHeader
	PTS       int64
	DTS       int64
	IsKeyframe uint8
	Reserved   [3]byte
	NALCount   uint32
}

// ReadFrameResponse reads one H.264 frame response from QEMU.
// Returns the scanout_id, H.264 NAL data, keyframe flag, and error.
func ReadFrameResponse(r io.Reader) (scanoutID uint32, nalData []byte, isKeyframe bool, err error) {
	var hdr helixMsgHeader
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return 0, nil, false, fmt.Errorf("read header: %w", err)
	}
	if hdr.Magic != helixMsgMagic {
		return 0, nil, false, fmt.Errorf("bad magic: 0x%x", hdr.Magic)
	}
	if hdr.MsgType != msgFrameResponse {
		// Skip non-frame messages
		if hdr.PayloadSize > 0 {
			skip := make([]byte, hdr.PayloadSize)
			io.ReadFull(r, skip)
		}
		return 0, nil, false, fmt.Errorf("unexpected msg_type: 0x%x", hdr.MsgType)
	}

	scanoutID = uint32(hdr.SessionID)

	// Read frame response fields (after header)
	var pts, dts int64
	var kf uint8
	var reserved [3]byte
	var nalCount uint32

	binary.Read(r, binary.LittleEndian, &pts)
	binary.Read(r, binary.LittleEndian, &dts)
	binary.Read(r, binary.LittleEndian, &kf)
	binary.Read(r, binary.LittleEndian, &reserved)
	if err := binary.Read(r, binary.LittleEndian, &nalCount); err != nil {
		return 0, nil, false, fmt.Errorf("read nal_count: %w", err)
	}

	// Read NAL data (nal_count=1, single blob)
	var nalSize uint32
	if err := binary.Read(r, binary.LittleEndian, &nalSize); err != nil {
		return 0, nil, false, fmt.Errorf("read nal_size: %w", err)
	}

	nalData = make([]byte, nalSize)
	if _, err := io.ReadFull(r, nalData); err != nil {
		return 0, nil, false, fmt.Errorf("read nal_data: %w", err)
	}

	return scanoutID, nalData, kf != 0, nil
}

func readScanoutResp(r io.Reader) (*scanoutRespPayload, error) {
	var hdr helixMsgHeader
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if hdr.Magic != helixMsgMagic {
		return nil, fmt.Errorf("bad magic: 0x%x (expected 0x%x)", hdr.Magic, helixMsgMagic)
	}
	if hdr.MsgType == msgError {
		// Read error message
		buf := make([]byte, hdr.PayloadSize)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("read error payload: %w", err)
		}
		return nil, fmt.Errorf("QEMU error: %s", string(buf))
	}
	if hdr.MsgType != msgScanoutResp {
		return nil, fmt.Errorf("unexpected msg_type: 0x%x (expected 0x%x)", hdr.MsgType, msgScanoutResp)
	}
	var resp scanoutRespPayload
	if err := binary.Read(r, binary.LittleEndian, &resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return &resp, nil
}
