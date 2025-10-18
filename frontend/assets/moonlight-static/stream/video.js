var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { StreamSupportedVideoFormats } from "../api_bindings.js";
const CAPABILITIES_CODECS = [
    // H264
    { key: "H264", mimeType: "video/H264", fmtpLine: ["packetization-mode=1", "profile-level-id=42e01f"] },
    { key: "H264_HIGH8_444", mimeType: "video/H264", fmtpLine: ["packetization-mode=1", "profile-level-id=640032"] },
    // H265
    // TODO: check level id in check function
    { key: "H265", mimeType: "video/H265", fmtpLine: [] }, // <-- Safari H265 fmtpLine is empty (for some dumb reason)
    { key: "H265_MAIN10", mimeType: "video/H265", fmtpLine: ["profile-id=2", "tier-flag=0", "tx-mode=SRST"] },
    { key: "H265_REXT8_444", mimeType: "video/H265", fmtpLine: ["profile-id=4", "tier-flag=0", "tx-mode=SRST"] },
    { key: "H265_REXT10_444", mimeType: "video/H265", fmtpLine: ["profile-id=5", "tier-flag=0", "tx-mode=SRST"] },
    // Av1
    { key: "AV1_MAIN8", mimeType: "video/AV1", fmtpLine: [] }, // <-- Safari AV1 fmtpLine is empty
    { key: "AV1_MAIN10", mimeType: "video/AV1", fmtpLine: [] }, // <-- Safari AV1 fmtpLine is empty
    { key: "AV1_HIGH8", mimeType: "video/AV1", fmtpLine: ["profile=1"] },
    { key: "AV1_HIGH10", mimeType: "video/AV1", fmtpLine: ["profile=1"] },
];
const VIDEO_DECODER_CODECS = [
    { key: "H264_HIGH8_444", codec: "avc1.4d400c", colorSpace: { primaries: "bt709", matrix: "bt709", transfer: "bt709", fullRange: true } },
    // TODO? No major browser currently supports WebRTC h265, but it might support h265 video without webrtc so we don't check that
    // { key: "H265", codec: "hvc1.1.6.L93.B0" },
    // { key: "H265_MAIN10", codec: "hvc1.2.4.L120.90" },
    // { key: "H265_REXT8_444", codec: "hvc1.6.6.L93.90", colorSpace: { primaries: "bt709", matrix: "bt709", transfer: "bt709", fullRange: true } },
    // { key: "H265_REXT10_444", codec: "hvc1.6.10.L120.90", colorSpace: { primaries: "bt709", matrix: "bt709", transfer: "bt709", fullRange: true } },
    // TODO: Av1
    // { key: "AV1_MAIN8", codec: "av01.0.04M.08" },
    // { key: "AV1_MAIN10", codec: "av01.0.04M.10" },
    // { key: "AV1_HIGH8_444", codec: "av01.0.08M.08", colorSpace: { primaries: "bt709", matrix: "bt709", transfer: "bt709", fullRange: true } },
    // { key: "AV1_HIGH10_444", codec: "av01.0.08M.10", colorSpace: { primaries: "bt709", matrix: "bt709", transfer: "bt709", fullRange: true } }
];
export function getStandardVideoFormats() {
    return {
        H264: true, // assumed universal
        H264_HIGH8_444: false,
        H265: false,
        H265_MAIN10: false,
        H265_REXT8_444: false,
        H265_REXT10_444: false,
        AV1_MAIN8: false,
        AV1_MAIN10: false,
        AV1_HIGH8_444: false,
        AV1_HIGH10_444: false
    };
}
export function getSupportedVideoFormats() {
    return __awaiter(this, void 0, void 0, function* () {
        var _a;
        let support = getStandardVideoFormats();
        let capabilities = RTCRtpReceiver.getCapabilities("video");
        if ("getCapabilities" in RTCRtpReceiver && typeof RTCRtpReceiver.getCapabilities == "function" && (capabilities = RTCRtpReceiver.getCapabilities("video"))) {
            for (const capCodec of capabilities.codecs) {
                for (const codec of CAPABILITIES_CODECS) {
                    let compatible = true;
                    if (capCodec.mimeType.toLowerCase() != codec.mimeType.toLowerCase()) {
                        compatible = false;
                    }
                    for (const fmtpLineAttrib of codec.fmtpLine) {
                        if (!((_a = capCodec.sdpFmtpLine) === null || _a === void 0 ? void 0 : _a.includes(fmtpLineAttrib))) {
                            compatible = false;
                        }
                    }
                    if (compatible) {
                        support[codec.key] = true;
                    }
                }
            }
        }
        else if ("VideoDecoder" in window && window.isSecureContext) {
            for (const codec of VIDEO_DECODER_CODECS) {
                try {
                    const result = yield VideoDecoder.isConfigSupported(codec);
                    support[codec.key] = result.supported || support[codec.key];
                }
                catch (e) {
                    support[codec.key] = false;
                }
            }
        }
        else if ("MediaSource" in window) {
            for (const codec of VIDEO_DECODER_CODECS) {
                const supported = MediaSource.isTypeSupported(`video/mp4; codecs="${codec.codec}"`);
                support[codec.key] = supported || support[codec.key];
            }
        }
        else {
            const mediaElement = document.createElement("video");
            for (const codec of VIDEO_DECODER_CODECS) {
                const supported = mediaElement.canPlayType(`video/mp4; codecs="${codec.codec}"`);
                support[codec.key] = supported == "probably" || support[codec.key];
            }
        }
        return support;
    });
}
export function createSupportedVideoFormatsBits(support) {
    let mask = 0;
    if (support.H264) {
        mask |= StreamSupportedVideoFormats.H264;
    }
    if (support.H264_HIGH8_444) {
        mask |= StreamSupportedVideoFormats.H264_HIGH8_444;
    }
    if (support.H265) {
        mask |= StreamSupportedVideoFormats.H265;
    }
    if (support.H265_MAIN10) {
        mask |= StreamSupportedVideoFormats.H265_MAIN10;
    }
    if (support.H265_REXT8_444) {
        mask |= StreamSupportedVideoFormats.H265_REXT8_444;
    }
    if (support.H265_REXT10_444) {
        mask |= StreamSupportedVideoFormats.H265_REXT10_444;
    }
    if (support.AV1_MAIN8) {
        mask |= StreamSupportedVideoFormats.AV1_MAIN8;
    }
    if (support.AV1_MAIN10) {
        mask |= StreamSupportedVideoFormats.AV1_MAIN10;
    }
    if (support.AV1_HIGH8_444) {
        mask |= StreamSupportedVideoFormats.AV1_HIGH8_444;
    }
    if (support.AV1_HIGH10_444) {
        mask |= StreamSupportedVideoFormats.AV1_HIGH10_444;
    }
    return mask;
}
