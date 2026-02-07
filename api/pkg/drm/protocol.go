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
