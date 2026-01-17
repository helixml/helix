/**
 * Video Codec Utilities
 *
 * Constants and functions for video codec handling in WebSocket streaming.
 */

export const WsVideoCodec = {
  H264: 0x01,
  H264High444: 0x02,
  H265: 0x10,
  H265Main10: 0x11,
  H265Rext8_444: 0x12,
  H265Rext10_444: 0x13,
  Av1Main8: 0x20,
  Av1Main10: 0x21,
  Av1High8_444: 0x22,
  Av1High10_444: 0x23,
} as const

export type WsVideoCodecType = typeof WsVideoCodec[keyof typeof WsVideoCodec]

// Map codec byte to WebCodecs codec string
export function codecToWebCodecsString(codec: number): string {
  switch (codec) {
    case WsVideoCodec.H264: return "avc1.4d0033"
    case WsVideoCodec.H264High444: return "avc1.640032"
    case WsVideoCodec.H265: return "hvc1.1.6.L120.90"
    case WsVideoCodec.H265Main10: return "hvc1.2.4.L120.90"
    case WsVideoCodec.H265Rext8_444: return "hvc1.4.10.L120.90"
    case WsVideoCodec.H265Rext10_444: return "hvc1.4.10.L120.90"
    case WsVideoCodec.Av1Main8: return "av01.0.08M.08"
    case WsVideoCodec.Av1Main10: return "av01.0.08M.10"
    case WsVideoCodec.Av1High8_444: return "av01.1.08H.08"
    case WsVideoCodec.Av1High10_444: return "av01.1.08H.10"
    default: return "avc1.4d0033" // Default to H264
  }
}

// Map codec byte to human-readable display name for stats UI
export function codecToDisplayName(codec: number | null): string {
  if (codec === null) return "Unknown"
  switch (codec) {
    case WsVideoCodec.H264: return "H.264"
    case WsVideoCodec.H264High444: return "H.264 High 4:4:4"
    case WsVideoCodec.H265: return "HEVC"
    case WsVideoCodec.H265Main10: return "HEVC Main10"
    case WsVideoCodec.H265Rext8_444: return "HEVC RExt 4:4:4"
    case WsVideoCodec.H265Rext10_444: return "HEVC RExt 10bit 4:4:4"
    case WsVideoCodec.Av1Main8: return "AV1"
    case WsVideoCodec.Av1Main10: return "AV1 10bit"
    case WsVideoCodec.Av1High8_444: return "AV1 High 4:4:4"
    case WsVideoCodec.Av1High10_444: return "AV1 High 10bit 4:4:4"
    default: return `Unknown (0x${codec.toString(16)})`
  }
}
