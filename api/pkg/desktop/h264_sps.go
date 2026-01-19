// Package desktop provides H.264 SPS (Sequence Parameter Set) parsing and modification.
// This is used to set VUI parameters for zero-latency hardware decoding.
package desktop

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/bits"
)

// SPSVUIInfo contains parsed H.264 SPS VUI information relevant to decoder latency.
type SPSVUIInfo struct {
	ProfileIDC           uint8
	ConstraintSetFlags   uint8
	LevelIDC             uint8
	MaxNumRefFrames      uint
	Width                uint
	Height               uint

	// VUI parameters (if present)
	VUIParametersPresent     bool
	BitstreamRestrictionFlag bool
	MaxNumReorderFrames      uint
	MaxDecFrameBuffering     uint
}

// ParseSPS parses an H.264 SPS NAL unit using the mp4ff library.
// The spsData should include the NAL header byte.
func ParseSPS(spsData []byte) (*SPSVUIInfo, error) {
	if len(spsData) < 4 {
		return nil, fmt.Errorf("SPS data too short: %d bytes", len(spsData))
	}

	// Parse using mp4ff (requires NAL header byte)
	sps, err := avc.ParseSPSNALUnit(spsData, true)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SPS: %w", err)
	}

	info := &SPSVUIInfo{
		ProfileIDC:         uint8(sps.Profile),
		ConstraintSetFlags: uint8(sps.ProfileCompatibility),
		LevelIDC:           uint8(sps.Level),
		MaxNumRefFrames:    sps.NumRefFrames,
		Width:              sps.Width,
		Height:             sps.Height,
	}

	// Check for VUI parameters
	if sps.VUI != nil {
		info.VUIParametersPresent = true
		info.BitstreamRestrictionFlag = sps.VUI.BitstreamRestrictionFlag
		if sps.VUI.BitstreamRestrictionFlag {
			info.MaxNumReorderFrames = sps.VUI.MaxNumReorderFrames
			info.MaxDecFrameBuffering = sps.VUI.MaxDecFrameBuffering
		}
	}

	return info, nil
}

// GetSPSDebugString returns a debug string describing the SPS.
func GetSPSDebugString(spsData []byte) string {
	if len(spsData) < 4 {
		return fmt.Sprintf("SPS too short: %d bytes", len(spsData))
	}

	// Try parsing with mp4ff
	info, err := ParseSPS(spsData)
	if err != nil {
		// Fall back to basic info from raw bytes
		// Skip NAL header if present
		offset := 0
		if spsData[0]&0x1F == 7 { // NAL type SPS
			offset = 1
		}
		if offset+3 > len(spsData) {
			return fmt.Sprintf("SPS parse failed: %v", err)
		}
		profileIDC := spsData[offset]
		constraintFlags := spsData[offset+1]
		levelIDC := spsData[offset+2]
		constraintSet3 := (constraintFlags & 0x10) != 0

		return fmt.Sprintf("profile_idc=%d constraint_set3=%v level=%d.%d (VUI parse failed: %v)",
			profileIDC, constraintSet3, levelIDC/10, levelIDC%10, err)
	}

	constraintSet3 := (info.ConstraintSetFlags & 0x10) != 0

	s := fmt.Sprintf("profile_idc=%d constraint_set3=%v level=%d.%d max_num_ref_frames=%d resolution=%dx%d",
		info.ProfileIDC, constraintSet3, info.LevelIDC/10, info.LevelIDC%10,
		info.MaxNumRefFrames, info.Width, info.Height)

	if info.VUIParametersPresent {
		s += " VUI:present"
		if info.BitstreamRestrictionFlag {
			s += fmt.Sprintf(" bitstream_restriction:{max_num_reorder_frames=%d, max_dec_frame_buffering=%d}",
				info.MaxNumReorderFrames, info.MaxDecFrameBuffering)
		} else {
			s += " bitstream_restriction:absent"
		}
	} else {
		s += " VUI:absent"
	}

	return s
}

// CheckSPSNeedsModification checks if the SPS needs VUI modification for zero-latency decode.
// Returns true if modification would help, along with an explanation.
func CheckSPSNeedsModification(spsData []byte) (needsModification bool, reason string) {
	info, err := ParseSPS(spsData)
	if err != nil {
		return false, fmt.Sprintf("cannot parse SPS: %v", err)
	}

	// Check constraint_set3_flag (we already set this)
	constraintSet3 := (info.ConstraintSetFlags & 0x10) != 0
	if !constraintSet3 {
		return true, "constraint_set3_flag=0 (should be 1 for zero-latency)"
	}

	// Check VUI bitstream_restriction
	if !info.VUIParametersPresent {
		return true, "VUI not present (decoder may assume large DPB)"
	}

	if !info.BitstreamRestrictionFlag {
		return true, "bitstream_restriction not present (decoder may assume reordering needed)"
	}

	if info.MaxNumReorderFrames > 0 {
		return true, fmt.Sprintf("max_num_reorder_frames=%d (should be 0)", info.MaxNumReorderFrames)
	}

	// max_dec_frame_buffering should be exactly max_num_ref_frames (per WebRTC)
	expectedMaxDec := info.MaxNumRefFrames
	if expectedMaxDec == 0 {
		expectedMaxDec = 1
	}
	if info.MaxDecFrameBuffering != expectedMaxDec {
		return true, fmt.Sprintf("max_dec_frame_buffering=%d (should be %d)", info.MaxDecFrameBuffering, expectedMaxDec)
	}

	return false, "SPS already configured for zero-latency decode"
}

// ============================================================================
// VUI Modification Implementation
// ============================================================================
// We need to modify max_num_reorder_frames and max_dec_frame_buffering in the
// VUI bitstream_restriction section to enable zero-latency hardware decoding.
//
// Strategy:
// 1. Parse the SPS using mp4ff to get NrBytesBeforeVUI
// 2. Copy bytes before VUI verbatim (with NAL header modification for constraint_set3)
// 3. Rebuild VUI section using EBSPWriter with our modified values
// 4. EBSPWriter automatically handles emulation prevention bytes
//
// Reference: WebRTC sps_vui_rewriter.cc
// ============================================================================

// RewriteSPSForZeroLatency parses an SPS, modifies VUI for zero-latency, and returns the new SPS.
// If modification is not needed or fails, returns the original with constraint_set3_flag set.
//
// The input should be the raw SPS NAL unit (starting with NAL header byte).
// The output will also be a complete SPS NAL unit.
func RewriteSPSForZeroLatency(spsData []byte) ([]byte, bool) {
	if len(spsData) < 4 {
		return spsData, false
	}

	// Parse the SPS
	sps, err := avc.ParseSPSNALUnit(spsData, true)
	if err != nil {
		// Can't parse, just set constraint_set3_flag
		result := make([]byte, len(spsData))
		copy(result, spsData)
		if len(result) > 2 {
			result[2] |= 0x10 // Set constraint_set3_flag (byte 2 = constraint flags)
		}
		return result, false
	}

	// Check if modification is needed
	needsMod := false

	// Check constraint_set3_flag
	if (sps.ProfileCompatibility & 0x10) == 0 {
		needsMod = true
	}

	// Check VUI bitstream_restriction
	if sps.VUI == nil {
		needsMod = true
	} else if !sps.VUI.BitstreamRestrictionFlag {
		needsMod = true
	} else {
		// bitstream_restriction present - check values
		if sps.VUI.MaxNumReorderFrames != 0 {
			needsMod = true
		}
		// max_dec_frame_buffering should be exactly max_num_ref_frames (per WebRTC)
		expectedMaxDec := sps.NumRefFrames
		if expectedMaxDec == 0 {
			expectedMaxDec = 1
		}
		if sps.VUI.MaxDecFrameBuffering != expectedMaxDec {
			needsMod = true
		}
	}

	if !needsMod {
		// No modification needed, just return original with constraint_set3
		result := make([]byte, len(spsData))
		copy(result, spsData)
		if len(result) > 2 {
			result[2] |= 0x10
		}
		return result, false
	}

	// Rebuild the SPS with modified VUI
	// Strategy: Copy bytes up to VUI, then write new VUI using EBSPWriter
	return rebuildSPSWithVUI(spsData, sps)
}

// rebuildSPSWithVUI rebuilds an SPS NAL unit with modified VUI parameters.
// It copies the pre-VUI portion and rewrites VUI with zero-latency settings.
func rebuildSPSWithVUI(originalData []byte, sps *avc.SPS) ([]byte, bool) {
	// We need to rebuild the entire SPS because the VUI is at the end
	// and changing its size affects everything

	var buf bytes.Buffer
	w := bits.NewEBSPWriter(&buf)

	// NAL header
	w.Write(0x67, 8) // nal_ref_idc=3, nal_unit_type=7 (SPS)

	// profile_idc
	w.Write(uint(sps.Profile), 8)

	// constraint_set_flags with constraint_set3=1 for zero-latency
	constraintFlags := sps.ProfileCompatibility | 0x10 // Set constraint_set3_flag
	w.Write(uint(constraintFlags), 8)

	// level_idc
	w.Write(uint(sps.Level), 8)

	// seq_parameter_set_id
	w.WriteExpGolomb(uint(sps.ParameterID))

	// Profile-specific fields
	switch sps.Profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		w.WriteExpGolomb(uint(sps.ChromaFormatIDC))
		if sps.ChromaFormatIDC == 3 {
			writeFlag(w, sps.SeparateColourPlaneFlag)
		}
		w.WriteExpGolomb(sps.BitDepthLumaMinus8)
		w.WriteExpGolomb(sps.BitDepthChromaMinus8)
		writeFlag(w, sps.QPPrimeYZeroTransformBypassFlag)
		writeFlag(w, sps.SeqScalingMatrixPresentFlag)
		if sps.SeqScalingMatrixPresentFlag {
			// Scaling lists - we need to write these if present
			// This is complex, so for now we only handle cases without scaling lists
			// In practice, real-time encoders rarely use custom scaling lists
			return nil, false
		}
	}

	// log2_max_frame_num_minus4
	w.WriteExpGolomb(sps.Log2MaxFrameNumMinus4)

	// pic_order_cnt_type
	w.WriteExpGolomb(sps.PicOrderCntType)
	switch sps.PicOrderCntType {
	case 0:
		w.WriteExpGolomb(sps.Log2MaxPicOrderCntLsbMinus4)
	case 1:
		writeFlag(w, sps.DeltaPicOrderAlwaysZeroFlag)
		w.WriteExpGolomb(sps.OffsetForNonRefPic)
		w.WriteExpGolomb(sps.OffsetForTopToBottomField)
		w.WriteExpGolomb(uint(len(sps.RefFramesInPicOrderCntCycle)))
		for _, offset := range sps.RefFramesInPicOrderCntCycle {
			w.WriteExpGolomb(offset)
		}
	}

	// max_num_ref_frames
	w.WriteExpGolomb(sps.NumRefFrames)

	// gaps_in_frame_num_value_allowed_flag
	writeFlag(w, sps.GapsInFrameNumValueAllowedFlag)

	// Calculate pic_width/height_in_mbs_minus1 from resolution
	// sps.Width and sps.Height are already crop-adjusted, so we need to add back the cropping
	// to get the original macroblock dimensions
	var cropUnitX, cropUnitY uint = 1, 1
	var frameMbsOnly uint = 0
	if sps.FrameMbsOnlyFlag {
		frameMbsOnly = 1
	}
	switch sps.ChromaFormatIDC {
	case 0:
		cropUnitX, cropUnitY = 1, 2-frameMbsOnly
	case 1:
		cropUnitX, cropUnitY = 2, 2*(2-frameMbsOnly)
	case 2:
		cropUnitX, cropUnitY = 2, 1*(2-frameMbsOnly)
	case 3:
		cropUnitX, cropUnitY = 1, 1*(2-frameMbsOnly)
	}

	// Add back cropped pixels to get full macroblock dimensions
	fullWidth := sps.Width
	fullHeight := sps.Height
	if sps.FrameCroppingFlag {
		fullWidth += (sps.FrameCropLeftOffset + sps.FrameCropRightOffset) * cropUnitX
		fullHeight += (sps.FrameCropTopOffset + sps.FrameCropBottomOffset) * cropUnitY
	}

	picWidthInMbsMinus1 := (fullWidth / 16) - 1
	picHeightInMapUnitsMinus1 := (fullHeight / 16) - 1
	if !sps.FrameMbsOnlyFlag {
		// Interlaced: use map units instead of macroblocks
		picHeightInMapUnitsMinus1 = (fullHeight / 32) - 1
	}

	w.WriteExpGolomb(picWidthInMbsMinus1)
	w.WriteExpGolomb(picHeightInMapUnitsMinus1)

	// frame_mbs_only_flag
	writeFlag(w, sps.FrameMbsOnlyFlag)
	if !sps.FrameMbsOnlyFlag {
		writeFlag(w, sps.MbAdaptiveFrameFieldFlag)
	}

	// direct_8x8_inference_flag
	writeFlag(w, sps.Direct8x8InferenceFlag)

	// frame_cropping
	writeFlag(w, sps.FrameCroppingFlag)
	if sps.FrameCroppingFlag {
		w.WriteExpGolomb(sps.FrameCropLeftOffset)
		w.WriteExpGolomb(sps.FrameCropRightOffset)
		w.WriteExpGolomb(sps.FrameCropTopOffset)
		w.WriteExpGolomb(sps.FrameCropBottomOffset)
	}

	// vui_parameters_present_flag - always true since we're adding VUI
	w.Write(1, 1)

	// Write VUI with zero-latency parameters
	writeVUIForZeroLatency(w, sps)

	// rbsp_trailing_bits
	w.WriteRbspTrailingBits()

	if w.AccError() != nil {
		// Encoding failed, return original with constraint_set3
		result := make([]byte, len(originalData))
		copy(result, originalData)
		if len(result) > 2 {
			result[2] |= 0x10
		}
		return result, false
	}

	return buf.Bytes(), true
}

// writeFlag writes a boolean flag as a single bit
func writeFlag(w *bits.EBSPWriter, flag bool) {
	if flag {
		w.Write(1, 1)
	} else {
		w.Write(0, 1)
	}
}

// writeVUIForZeroLatency writes VUI parameters optimized for zero-latency decode.
// Key settings: max_num_reorder_frames=0, max_dec_frame_buffering=max(1, num_ref_frames)
func writeVUIForZeroLatency(w *bits.EBSPWriter, sps *avc.SPS) {
	vui := sps.VUI

	// If original had no VUI, create minimal one with just bitstream_restriction
	if vui == nil {
		vui = &avc.VUIParameters{}
	}

	// aspect_ratio_info_present_flag
	hasAspectRatio := vui.SampleAspectRatioWidth > 0 && vui.SampleAspectRatioHeight > 0
	writeFlag(w, hasAspectRatio)
	if hasAspectRatio {
		// For simplicity, use Extended_SAR (255) to write explicit values
		w.Write(255, 8) // aspect_ratio_idc = Extended_SAR
		w.Write(vui.SampleAspectRatioWidth, 16)
		w.Write(vui.SampleAspectRatioHeight, 16)
	}

	// overscan_info_present_flag
	writeFlag(w, vui.OverscanInfoPresentFlag)
	if vui.OverscanInfoPresentFlag {
		writeFlag(w, vui.OverscanAppropriateFlag)
	}

	// video_signal_type_present_flag
	writeFlag(w, vui.VideoSignalTypePresentFlag)
	if vui.VideoSignalTypePresentFlag {
		w.Write(vui.VideoFormat, 3)
		writeFlag(w, vui.VideoFullRangeFlag)
		writeFlag(w, vui.ColourDescriptionFlag)
		if vui.ColourDescriptionFlag {
			w.Write(vui.ColourPrimaries, 8)
			w.Write(vui.TransferCharacteristics, 8)
			w.Write(vui.MatrixCoefficients, 8)
		}
	}

	// chroma_loc_info_present_flag
	writeFlag(w, vui.ChromaLocInfoPresentFlag)
	if vui.ChromaLocInfoPresentFlag {
		w.WriteExpGolomb(vui.ChromaSampleLocTypeTopField)
		w.WriteExpGolomb(vui.ChromaSampleLocTypeBottomField)
	}

	// timing_info_present_flag
	writeFlag(w, vui.TimingInfoPresentFlag)
	if vui.TimingInfoPresentFlag {
		w.Write(vui.NumUnitsInTick, 32)
		w.Write(vui.TimeScale, 32)
		writeFlag(w, vui.FixedFrameRateFlag)
	}

	// nal_hrd_parameters_present_flag
	writeFlag(w, vui.NalHrdParametersPresentFlag)
	if vui.NalHrdParametersPresentFlag {
		writeHrdParameters(w, vui.NalHrdParameters)
	}

	// vcl_hrd_parameters_present_flag
	writeFlag(w, vui.VclHrdParametersPresentFlag)
	if vui.VclHrdParametersPresentFlag {
		writeHrdParameters(w, vui.VclHrdParameters)
	}

	// low_delay_hrd_flag (only if HRD present)
	if vui.NalHrdParametersPresentFlag || vui.VclHrdParametersPresentFlag {
		writeFlag(w, vui.LowDelayHrdFlag)
	}

	// pic_struct_present_flag
	writeFlag(w, vui.PicStructPresentFlag)

	// bitstream_restriction_flag - always true for zero-latency
	w.Write(1, 1)

	// bitstream_restriction fields - optimized for zero-latency
	writeFlag(w, true) // motion_vectors_over_pic_boundaries_flag
	w.WriteExpGolomb(2)  // max_bytes_per_pic_denom (default: 2)
	w.WriteExpGolomb(1)  // max_bits_per_mb_denom (default: 1)
	w.WriteExpGolomb(16) // log2_max_mv_length_horizontal (default: 16)
	w.WriteExpGolomb(16) // log2_max_mv_length_vertical (default: 16)

	// CRITICAL: These are the key fields for zero-latency decode
	// Reference: WebRTC sps_vui_rewriter.cc line 400
	// https://webrtc.googlesource.com/src/+/refs/heads/main/common_video/h264/sps_vui_rewriter.cc#400
	w.WriteExpGolomb(0) // max_num_reorder_frames = 0 (no frame reordering needed)

	// max_dec_frame_buffering = max_num_ref_frames (per WebRTC and H.264 spec)
	// The key insight is that max_num_reorder_frames=0 is what eliminates buffering.
	// max_dec_frame_buffering just needs to be >= max_num_ref_frames per spec.
	maxDecBuf := sps.NumRefFrames
	if maxDecBuf == 0 {
		maxDecBuf = 1 // At least 1 for valid decode
	}
	w.WriteExpGolomb(maxDecBuf)
}

// writeHrdParameters writes HRD (Hypothetical Reference Decoder) parameters
func writeHrdParameters(w *bits.EBSPWriter, hrd *avc.HrdParameters) {
	if hrd == nil {
		return
	}

	w.WriteExpGolomb(hrd.CpbCountMinus1)
	w.Write(hrd.BitRateScale, 4)
	w.Write(hrd.CpbSizeScale, 4)

	for i := uint(0); i <= hrd.CpbCountMinus1; i++ {
		if i < uint(len(hrd.CpbEntries)) {
			entry := hrd.CpbEntries[i]
			w.WriteExpGolomb(entry.BitRateValueMinus1)
			w.WriteExpGolomb(entry.CpbSizeValueMinus1)
			writeFlag(w, entry.CbrFlag)
		}
	}

	w.Write(hrd.InitialCpbRemovalDelayLengthMinus1, 5)
	w.Write(hrd.CpbRemovalDelayLengthMinus1, 5)
	w.Write(hrd.DpbOutputDelayLengthMinus1, 5)
	w.Write(hrd.TimeOffsetLength, 5)
}
