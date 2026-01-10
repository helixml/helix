// Package desktop provides H.264 RTP depacketization.
// This matches the approach used in moonlight-web-stream's WebRTC mode
// where RTP provides reliable frame boundaries via marker bits.
package desktop

import (
	"encoding/binary"
	"fmt"
)

// H.264 NAL unit types (RFC 6184)
const (
	nalTypeSingleNAL = 1  // Single NAL unit packet (types 1-23)
	nalTypeSTAPA     = 24 // Single-time aggregation packet type A
	nalTypeFUA       = 28 // Fragmentation unit type A
)

// RTPHeader represents a parsed RTP header (12 bytes fixed + optional extensions)
type RTPHeader struct {
	Version        uint8
	Padding        bool
	Extension      bool
	CSRCCount      uint8
	Marker         bool // End of Access Unit for H.264
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
}

// ParseRTPHeader parses an RTP header from the first 12 bytes
func ParseRTPHeader(data []byte) (RTPHeader, int, error) {
	if len(data) < 12 {
		return RTPHeader{}, 0, fmt.Errorf("RTP packet too short: %d bytes", len(data))
	}

	h := RTPHeader{
		Version:        (data[0] >> 6) & 0x03,
		Padding:        (data[0]>>5)&0x01 == 1,
		Extension:      (data[0]>>4)&0x01 == 1,
		CSRCCount:      data[0] & 0x0F,
		Marker:         (data[1]>>7)&0x01 == 1,
		PayloadType:    data[1] & 0x7F,
		SequenceNumber: binary.BigEndian.Uint16(data[2:4]),
		Timestamp:      binary.BigEndian.Uint32(data[4:8]),
		SSRC:           binary.BigEndian.Uint32(data[8:12]),
	}

	if h.Version != 2 {
		return RTPHeader{}, 0, fmt.Errorf("invalid RTP version: %d", h.Version)
	}

	// Calculate header length including CSRC and extension
	headerLen := 12 + int(h.CSRCCount)*4

	if h.Extension {
		if len(data) < headerLen+4 {
			return RTPHeader{}, 0, fmt.Errorf("RTP extension header truncated")
		}
		// Extension header: 2 bytes profile-specific, 2 bytes length (in 32-bit words)
		extLen := int(binary.BigEndian.Uint16(data[headerLen+2:headerLen+4])) * 4
		headerLen += 4 + extLen
	}

	if len(data) < headerLen {
		return RTPHeader{}, 0, fmt.Errorf("RTP packet truncated: need %d, have %d", headerLen, len(data))
	}

	return h, headerLen, nil
}

// H264Depacketizer reassembles H.264 NAL units from RTP packets.
// Handles Single NAL, STAP-A (aggregation), and FU-A (fragmentation).
type H264Depacketizer struct {
	// Current Access Unit being assembled
	accessUnit []byte
	isKeyframe bool

	// FU-A fragmentation state
	fuaBuffer []byte
	fuaActive bool

	// For logging
	lastSeq uint16
}

// NewH264Depacketizer creates a new depacketizer
func NewH264Depacketizer() *H264Depacketizer {
	return &H264Depacketizer{
		accessUnit: make([]byte, 0, 512*1024), // Pre-allocate for 4K
		fuaBuffer:  make([]byte, 0, 256*1024),
	}
}

// annexBStartCode is the 4-byte start code for Annex B format
var annexBStartCode = []byte{0x00, 0x00, 0x00, 0x01}

// ProcessPacket processes an RTP packet and returns a complete Access Unit
// when the marker bit indicates end of frame.
// Returns (accessUnit, isKeyframe, complete) where complete is true when a full AU is ready.
func (d *H264Depacketizer) ProcessPacket(packet []byte) ([]byte, bool, bool, error) {
	header, headerLen, err := ParseRTPHeader(packet)
	if err != nil {
		return nil, false, false, err
	}

	payload := packet[headerLen:]
	if len(payload) == 0 {
		return nil, false, false, nil
	}

	// Check for sequence number gaps (packet loss)
	if d.lastSeq != 0 && header.SequenceNumber != d.lastSeq+1 {
		// Gap detected - might need to request keyframe
		// For now, just note it and continue
	}
	d.lastSeq = header.SequenceNumber

	// Parse H.264 NAL unit type from first byte of payload
	nalHeader := payload[0]
	nalType := nalHeader & 0x1F

	switch {
	case nalType >= 1 && nalType <= 23:
		// Single NAL unit packet - payload is the NAL unit directly
		d.appendNALUnit(payload)

	case nalType == nalTypeSTAPA:
		// STAP-A: Multiple NAL units aggregated
		if err := d.processSTAPA(payload); err != nil {
			return nil, false, false, err
		}

	case nalType == nalTypeFUA:
		// FU-A: Fragmented NAL unit
		if err := d.processFUA(payload); err != nil {
			return nil, false, false, err
		}

	default:
		// Unknown or unsupported type (STAP-B, MTAP, FU-B)
		return nil, false, false, fmt.Errorf("unsupported NAL type: %d", nalType)
	}

	// If marker bit is set, Access Unit is complete
	if header.Marker {
		au := make([]byte, len(d.accessUnit))
		copy(au, d.accessUnit)
		isKey := d.isKeyframe

		// Reset for next Access Unit
		d.accessUnit = d.accessUnit[:0]
		d.isKeyframe = false

		return au, isKey, true, nil
	}

	return nil, false, false, nil
}

// appendNALUnit appends a NAL unit with Annex B start code to the Access Unit
func (d *H264Depacketizer) appendNALUnit(nal []byte) {
	if len(nal) == 0 {
		return
	}

	// Add Annex B start code
	d.accessUnit = append(d.accessUnit, annexBStartCode...)
	d.accessUnit = append(d.accessUnit, nal...)

	// Check NAL type for keyframe detection
	nalType := nal[0] & 0x1F
	if nalType == 5 { // IDR slice
		d.isKeyframe = true
	}
}

// processSTAPA handles Single-Time Aggregation Packet type A (RFC 6184 Section 5.7.1)
// Format: STAP-A header (1 byte) + [NAL size (2 bytes) + NAL data]...
func (d *H264Depacketizer) processSTAPA(payload []byte) error {
	if len(payload) < 3 {
		return fmt.Errorf("STAP-A packet too short")
	}

	// Skip STAP-A header byte
	offset := 1

	for offset < len(payload) {
		if offset+2 > len(payload) {
			return fmt.Errorf("STAP-A truncated at NAL size")
		}

		nalSize := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
		offset += 2

		if offset+nalSize > len(payload) {
			return fmt.Errorf("STAP-A truncated at NAL data: need %d, have %d", nalSize, len(payload)-offset)
		}

		d.appendNALUnit(payload[offset : offset+nalSize])
		offset += nalSize
	}

	return nil
}

// processFUA handles Fragmentation Unit type A (RFC 6184 Section 5.8)
// Format: FU indicator (1 byte) + FU header (1 byte) + FU payload
// FU header: S(1) | E(1) | R(1) | Type(5)
func (d *H264Depacketizer) processFUA(payload []byte) error {
	if len(payload) < 2 {
		return fmt.Errorf("FU-A packet too short")
	}

	fuIndicator := payload[0]
	fuHeader := payload[1]

	startBit := (fuHeader >> 7) & 0x01
	endBit := (fuHeader >> 6) & 0x01
	nalType := fuHeader & 0x1F

	if startBit == 1 {
		// Start of fragmented NAL unit
		// Reconstruct NAL header: NRI from FU indicator + Type from FU header
		nalHeader := (fuIndicator & 0xE0) | nalType
		d.fuaBuffer = d.fuaBuffer[:0]
		d.fuaBuffer = append(d.fuaBuffer, nalHeader)
		d.fuaActive = true
	}

	if !d.fuaActive {
		// Got middle/end fragment without start - discard
		return nil
	}

	// Append fragment payload (skip FU indicator and FU header)
	d.fuaBuffer = append(d.fuaBuffer, payload[2:]...)

	if endBit == 1 {
		// End of fragmented NAL unit - append complete NAL to Access Unit
		d.appendNALUnit(d.fuaBuffer)
		d.fuaBuffer = d.fuaBuffer[:0]
		d.fuaActive = false
	}

	return nil
}

// Reset clears all state for a new stream
func (d *H264Depacketizer) Reset() {
	d.accessUnit = d.accessUnit[:0]
	d.isKeyframe = false
	d.fuaBuffer = d.fuaBuffer[:0]
	d.fuaActive = false
	d.lastSeq = 0
}
