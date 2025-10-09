var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
import { defaultStreamInputConfig, StreamInput } from "./input.js";
import { createSupportedVideoFormatsBits } from "./video.js";
export function getStreamerSize(settings, viewerScreenSize) {
    let width, height;
    if (settings.videoSize == "720p") {
        width = 1280;
        height = 720;
    }
    else if (settings.videoSize == "1080p") {
        width = 1920;
        height = 1080;
    }
    else if (settings.videoSize == "1440p") {
        width = 2560;
        height = 1440;
    }
    else if (settings.videoSize == "4k") {
        width = 3840;
        height = 2160;
    }
    else if (settings.videoSize == "custom") {
        width = settings.videoSizeCustom.width;
        height = settings.videoSizeCustom.height;
    }
    else { // native
        width = viewerScreenSize[0];
        height = viewerScreenSize[1];
    }
    return [width, height];
}
export class Stream {
    constructor(api, hostId, appId, settings, supportedVideoFormats, viewerScreenSize) {
        this.eventTarget = new EventTarget();
        this.mediaStream = new MediaStream();
        this.peer = null;
        this.remoteDescription = null;
        this.iceCandidateQueue = [];
        // -- Raw Web Socket stuff
        this.wsSendBuffer = [];
        this.api = api;
        this.hostId = hostId;
        this.appId = appId;
        this.settings = settings;
        this.streamerSize = getStreamerSize(settings, viewerScreenSize);
        // Configure web socket
        this.ws = new WebSocket(`${api.host_url}/host/stream`);
        this.ws.addEventListener("error", this.onError.bind(this));
        this.ws.addEventListener("open", this.onWsOpen.bind(this));
        this.ws.addEventListener("close", this.onWsClose.bind(this));
        this.ws.addEventListener("message", this.onRawWsMessage.bind(this));
        const fps = this.settings.fps;
        this.sendWsMessage({
            AuthenticateAndInit: {
                credentials: this.api.credentials,
                host_id: this.hostId,
                app_id: this.appId,
                bitrate: this.settings.bitrate,
                packet_size: this.settings.packetSize,
                fps,
                width: this.streamerSize[0],
                height: this.streamerSize[1],
                video_sample_queue_size: this.settings.videoSampleQueueSize,
                play_audio_local: this.settings.playAudioLocal,
                audio_sample_queue_size: this.settings.audioSampleQueueSize,
                video_supported_formats: createSupportedVideoFormatsBits(supportedVideoFormats),
                video_colorspace: "Rec709", // TODO <---
                video_color_range_full: true, // TODO <---
            }
        });
        // Stream Input
        const streamInputConfig = defaultStreamInputConfig();
        Object.assign(streamInputConfig, {
            mouseScrollMode: this.settings.mouseScrollMode,
            controllerConfig: this.settings.controllerConfig
        });
        this.input = new StreamInput(streamInputConfig);
        // Dispatch info for next frame so that listeners can be registers
        setTimeout(() => {
            this.debugLog("Requesting Stream with attributes: {");
            // Width, Height, Fps
            this.debugLog(`  Width ${this.streamerSize[0]}`);
            this.debugLog(`  Height ${this.streamerSize[1]}`);
            this.debugLog(`  Fps: ${fps}`);
            // Supported Video Formats
            const supportedVideoFormatsText = [];
            for (const item in supportedVideoFormats) {
                if (supportedVideoFormats[item]) {
                    supportedVideoFormatsText.push(item);
                }
            }
            this.debugLog(`  Supported Video Formats: ${createPrettyList(supportedVideoFormatsText)}`);
            this.debugLog("}");
        });
    }
    debugLog(message) {
        for (const line of message.split("\n")) {
            const event = new CustomEvent("stream-info", {
                detail: { type: "addDebugLine", line }
            });
            this.eventTarget.dispatchEvent(event);
        }
    }
    createPeer(configuration) {
        return __awaiter(this, void 0, void 0, function* () {
            this.debugLog(`Creating Client Peer`);
            if (this.peer) {
                this.debugLog(`Cannot create Peer because a Peer already exists`);
                return;
            }
            // Configure web rtc
            this.peer = new RTCPeerConnection(configuration);
            this.peer.addEventListener("error", this.onError.bind(this));
            this.peer.addEventListener("negotiationneeded", this.onNegotiationNeeded.bind(this));
            this.peer.addEventListener("icecandidate", this.onIceCandidate.bind(this));
            this.peer.addEventListener("track", this.onTrack.bind(this));
            this.peer.addEventListener("datachannel", this.onDataChannel.bind(this));
            this.peer.addEventListener("connectionstatechange", this.onConnectionStateChange.bind(this));
            this.peer.addEventListener("iceconnectionstatechange", this.onIceConnectionStateChange.bind(this));
            this.input.setPeer(this.peer);
            // Maybe we already received data
            if (this.remoteDescription) {
                yield this.handleRemoteDescription(this.remoteDescription);
            }
            else {
                yield this.onNegotiationNeeded();
            }
            yield this.tryDequeueIceCandidates();
        });
    }
    onMessage(message) {
        return __awaiter(this, void 0, void 0, function* () {
            if (typeof message == "string") {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "error", message }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("StageStarting" in message) {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "stageStarting", stage: message.StageStarting.stage }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("StageComplete" in message) {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "stageComplete", stage: message.StageComplete.stage }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("StageFailed" in message) {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "stageFailed", stage: message.StageFailed.stage, errorCode: message.StageFailed.error_code }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("ConnectionTerminated" in message) {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "connectionTerminated", errorCode: message.ConnectionTerminated.error_code }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("ConnectionStatusUpdate" in message) {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "connectionStatus", status: message.ConnectionStatusUpdate.status }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("UpdateApp" in message) {
                const event = new CustomEvent("stream-info", {
                    detail: { type: "app", app: message.UpdateApp.app }
                });
                this.eventTarget.dispatchEvent(event);
            }
            else if ("ConnectionComplete" in message) {
                const capabilities = message.ConnectionComplete.capabilities;
                const width = message.ConnectionComplete.width;
                const height = message.ConnectionComplete.height;
                const event = new CustomEvent("stream-info", {
                    detail: { type: "connectionComplete", capabilities }
                });
                this.eventTarget.dispatchEvent(event);
                this.input.onStreamStart(capabilities, [width, height]);
            }
            // -- WebRTC Config
            else if ("WebRtcConfig" in message) {
                const iceServers = message.WebRtcConfig.ice_servers;
                this.createPeer({
                    iceServers: iceServers
                });
                this.debugLog(`Using WebRTC Ice Servers: ${createPrettyList(iceServers.map(server => server.urls).reduce((list, url) => list.concat(url), []))}`);
            }
            // -- Signaling
            else if ("Signaling" in message) {
                if ("Description" in message.Signaling) {
                    const descriptionRaw = message.Signaling.Description;
                    const description = {
                        type: descriptionRaw.ty,
                        sdp: descriptionRaw.sdp,
                    };
                    yield this.handleRemoteDescription(description);
                }
                else if ("AddIceCandidate" in message.Signaling) {
                    const candidateRaw = message.Signaling.AddIceCandidate;
                    const candidate = {
                        candidate: candidateRaw.candidate,
                        sdpMid: candidateRaw.sdp_mid,
                        sdpMLineIndex: candidateRaw.sdp_mline_index,
                        usernameFragment: candidateRaw.username_fragment
                    };
                    yield this.handleIceCandidate(candidate);
                }
            }
        });
    }
    // -- Signaling
    onNegotiationNeeded() {
        return __awaiter(this, void 0, void 0, function* () {
            if (!this.peer) {
                this.debugLog("OnNegotiationNeeded without a peer");
                return;
            }
            yield this.peer.setLocalDescription();
            yield this.sendLocalDescription();
        });
    }
    handleRemoteDescription(description) {
        return __awaiter(this, void 0, void 0, function* () {
            this.debugLog(`Received Remote Description of type ${description.type}`);
            this.remoteDescription = description;
            if (!this.peer) {
                this.debugLog(`Saving Remote Description for Peer creation`);
                return;
            }
            yield this.peer.setRemoteDescription(description);
            if (description.type === "offer") {
                yield this.peer.setLocalDescription();
                yield this.sendLocalDescription();
            }
            yield this.tryDequeueIceCandidates();
        });
    }
    tryDequeueIceCandidates() {
        return __awaiter(this, void 0, void 0, function* () {
            for (const candidate of this.iceCandidateQueue.splice(0)) {
                yield this.handleIceCandidate(candidate);
            }
        });
    }
    handleIceCandidate(candidate) {
        return __awaiter(this, void 0, void 0, function* () {
            if (!this.peer || !this.remoteDescription) {
                this.debugLog(`Received Ice Candidate and queuing it: ${candidate.candidate}`);
                this.iceCandidateQueue.push(candidate);
                return;
            }
            this.debugLog(`Adding Ice Candidate: ${candidate.candidate}`);
            this.peer.addIceCandidate(candidate);
        });
    }
    sendLocalDescription() {
        if (!this.peer) {
            this.debugLog("Send Local Description without a peer");
            return;
        }
        const description = this.peer.localDescription;
        this.debugLog(`Sending Local Description of type ${description.type}`);
        this.sendWsMessage({
            Signaling: {
                Description: {
                    ty: description.type,
                    sdp: description.sdp
                }
            }
        });
    }
    onIceCandidate(event) {
        var _a, _b, _c, _d;
        const candidateJson = (_a = event.candidate) === null || _a === void 0 ? void 0 : _a.toJSON();
        if (!candidateJson || !(candidateJson === null || candidateJson === void 0 ? void 0 : candidateJson.candidate)) {
            return;
        }
        this.debugLog(`Sending Ice Candidate: ${candidateJson.candidate}`);
        const candidate = {
            candidate: candidateJson === null || candidateJson === void 0 ? void 0 : candidateJson.candidate,
            sdp_mid: (_b = candidateJson === null || candidateJson === void 0 ? void 0 : candidateJson.sdpMid) !== null && _b !== void 0 ? _b : null,
            sdp_mline_index: (_c = candidateJson === null || candidateJson === void 0 ? void 0 : candidateJson.sdpMLineIndex) !== null && _c !== void 0 ? _c : null,
            username_fragment: (_d = candidateJson === null || candidateJson === void 0 ? void 0 : candidateJson.usernameFragment) !== null && _d !== void 0 ? _d : null
        };
        this.sendWsMessage({
            Signaling: {
                AddIceCandidate: candidate
            }
        });
    }
    // -- Track and Data Channels
    onTrack(event) {
        event.receiver.jitterBufferTarget = 0;
        if ("playoutDelayHint" in event.receiver) {
            event.receiver.playoutDelayHint = 0;
        }
        else {
            this.debugLog(`playoutDelayHint not supported in receiver: ${event.receiver.track.label}`);
        }
        const stream = event.streams[0];
        if (stream) {
            stream.getTracks().forEach(track => {
                this.debugLog(`Adding Media Track ${track.label}`);
                if (track.kind == "video" && "contentHint" in track) {
                    track.contentHint = "motion";
                }
                this.mediaStream.addTrack(track);
            });
        }
    }
    onConnectionStateChange() {
        if (!this.peer) {
            this.debugLog("OnConnectionStateChange without a peer");
            return;
        }
        this.debugLog(`Changing Peer State to ${this.peer.connectionState}`);
        if (this.peer.connectionState == "failed" || this.peer.connectionState == "disconnected" || this.peer.connectionState == "closed") {
            const customEvent = new CustomEvent("stream-info", {
                detail: {
                    type: "error",
                    message: `Connection state is ${this.peer.connectionState}`
                }
            });
            this.eventTarget.dispatchEvent(customEvent);
        }
    }
    onIceConnectionStateChange() {
        if (!this.peer) {
            this.debugLog("OnIceConnectionStateChange without a peer");
            return;
        }
        this.debugLog(`Changing Peer Ice State to ${this.peer.iceConnectionState}`);
    }
    onDataChannel(event) {
        this.debugLog(`Received Data Channel ${event.channel.label}`);
        if (event.channel.label == "general") {
            event.channel.addEventListener("message", this.onGeneralDataChannelMessage.bind(this));
        }
    }
    onGeneralDataChannelMessage(event) {
        return __awaiter(this, void 0, void 0, function* () {
            const data = event.data;
            if (typeof data != "string") {
                return;
            }
            let message = JSON.parse(data);
            yield this.onMessage(message);
        });
    }
    onWsOpen() {
        this.debugLog(`Web Socket Open`);
        for (const raw of this.wsSendBuffer.splice(0)) {
            this.ws.send(raw);
        }
    }
    onWsClose() {
        this.debugLog(`Web Socket Closed`);
    }
    sendWsMessage(message) {
        const raw = JSON.stringify(message);
        if (this.ws.readyState == WebSocket.OPEN) {
            this.ws.send(raw);
        }
        else {
            this.wsSendBuffer.push(raw);
        }
    }
    onRawWsMessage(event) {
        return __awaiter(this, void 0, void 0, function* () {
            const data = event.data;
            if (typeof data != "string") {
                return;
            }
            let message = JSON.parse(data);
            yield this.onMessage(message);
        });
    }
    onError(event) {
        this.debugLog(`Web Socket or WebRtcPeer Error`);
        console.error("Stream Error", event);
    }
    // -- Class Api
    addInfoListener(listener) {
        this.eventTarget.addEventListener("stream-info", listener);
    }
    removeInfoListener(listener) {
        this.eventTarget.removeEventListener("stream-info", listener);
    }
    getMediaStream() {
        return this.mediaStream;
    }
    getInput() {
        return this.input;
    }
    getStreamerSize() {
        return this.streamerSize;
    }
}
function createPrettyList(list) {
    let isFirst = true;
    let text = "[";
    for (const item of list) {
        if (!isFirst) {
            text += ", ";
        }
        isFirst = false;
        text += item;
    }
    text += "]";
    return text;
}
