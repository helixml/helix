import { Api } from "../api.js"
import { App, ConnectionStatus, RtcIceCandidate, StreamCapabilities, StreamClientMessage, StreamServerGeneralMessage, StreamServerMessage } from "../api_bindings.js"
import { StreamSettings } from "../component/settings_menu.js"
import { defaultStreamInputConfig, StreamInput } from "./input.js"
import { createSupportedVideoFormatsBits, VideoCodecSupport } from "./video.js"

export type InfoEvent = CustomEvent<
    { type: "app", app: App } |
    { type: "error", message: string } |
    { type: "stageStarting" | "stageComplete", stage: string } |
    { type: "stageFailed", stage: string, errorCode: number } |
    { type: "connectionComplete", capabilities: StreamCapabilities } |
    { type: "connectionStatus", status: ConnectionStatus } |
    { type: "connectionTerminated", errorCode: number } |
    { type: "addDebugLine", line: string }
>
export type InfoEventListener = (event: InfoEvent) => void

export function getStreamerSize(settings: StreamSettings, viewerScreenSize: [number, number]): [number, number] {
    let width, height
    if (settings.videoSize == "720p") {
        width = 1280
        height = 720
    } else if (settings.videoSize == "1080p") {
        width = 1920
        height = 1080
    } else if (settings.videoSize == "1440p") {
        width = 2560
        height = 1440
    } else if (settings.videoSize == "4k") {
        width = 3840
        height = 2160
    } else if (settings.videoSize == "custom") {
        width = settings.videoSizeCustom.width
        height = settings.videoSizeCustom.height
    } else { // native
        width = viewerScreenSize[0]
        height = viewerScreenSize[1]
    }
    return [width, height]
}

export class Stream {
    private api: Api
    private hostId: number
    private appId: number

    private settings: StreamSettings

    private eventTarget = new EventTarget()

    private mediaStream: MediaStream = new MediaStream()

    private ws: WebSocket

    private peer: RTCPeerConnection | null = null
    private input: StreamInput

    private streamerSize: [number, number]

    constructor(api: Api, hostId: number, appId: number, settings: StreamSettings, supportedVideoFormats: VideoCodecSupport, viewerScreenSize: [number, number]) {
        this.api = api
        this.hostId = hostId
        this.appId = appId

        this.settings = settings

        this.streamerSize = getStreamerSize(settings, viewerScreenSize)

        // Configure web socket
        this.ws = new WebSocket(`${api.host_url}/host/stream`)
        this.ws.addEventListener("error", this.onError.bind(this))
        this.ws.addEventListener("open", this.onWsOpen.bind(this))
        this.ws.addEventListener("close", this.onWsClose.bind(this))
        this.ws.addEventListener("message", this.onRawWsMessage.bind(this))

        const fps = this.settings.fps

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
        })

        // Stream Input
        const streamInputConfig = defaultStreamInputConfig()
        Object.assign(streamInputConfig, {
            mouseScrollMode: this.settings.mouseScrollMode,
            controllerConfig: this.settings.controllerConfig
        })
        this.input = new StreamInput(streamInputConfig)

        // Dispatch info for next frame so that listeners can be registers
        setTimeout(() => {
            this.debugLog("Requesting Stream with attributes: {")
            // Width, Height, Fps
            this.debugLog(`  Width ${this.streamerSize[0]}`)
            this.debugLog(`  Height ${this.streamerSize[1]}`)
            this.debugLog(`  Fps: ${fps}`)

            // Supported Video Formats
            const supportedVideoFormatsText = []
            for (const item in supportedVideoFormats) {
                if (supportedVideoFormats[item]) {
                    supportedVideoFormatsText.push(item)
                }
            }
            this.debugLog(`  Supported Video Formats: ${createPrettyList(supportedVideoFormatsText)}`)

            this.debugLog("}")
        })
    }

    private debugLog(message: string) {
        for (const line of message.split("\n")) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "addDebugLine", line }
            })

            this.eventTarget.dispatchEvent(event)
        }
    }

    private async createPeer(configuration: RTCConfiguration) {
        this.debugLog(`Creating Client Peer`)

        if (this.peer) {
            this.debugLog(`Cannot create Peer because a Peer already exists`)
            return
        }

        // Configure web rtc
        this.peer = new RTCPeerConnection(configuration)
        this.peer.addEventListener("error", this.onError.bind(this))

        this.peer.addEventListener("negotiationneeded", this.onNegotiationNeeded.bind(this))
        this.peer.addEventListener("icecandidate", this.onIceCandidate.bind(this))

        this.peer.addEventListener("track", this.onTrack.bind(this))
        this.peer.addEventListener("datachannel", this.onDataChannel.bind(this))

        this.peer.addEventListener("connectionstatechange", this.onConnectionStateChange.bind(this))
        this.peer.addEventListener("iceconnectionstatechange", this.onIceConnectionStateChange.bind(this))

        this.input.setPeer(this.peer)

        // Maybe we already received data
        if (this.remoteDescription) {
            await this.handleRemoteDescription(this.remoteDescription)
        } else {
            await this.onNegotiationNeeded()
        }
        await this.tryDequeueIceCandidates()
    }

    private async onMessage(message: StreamServerMessage | StreamServerGeneralMessage) {
        if (typeof message == "string") {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "error", message }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("StageStarting" in message) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "stageStarting", stage: message.StageStarting.stage }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("StageComplete" in message) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "stageComplete", stage: message.StageComplete.stage }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("StageFailed" in message) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "stageFailed", stage: message.StageFailed.stage, errorCode: message.StageFailed.error_code }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("ConnectionTerminated" in message) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "connectionTerminated", errorCode: message.ConnectionTerminated.error_code }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("ConnectionStatusUpdate" in message) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "connectionStatus", status: message.ConnectionStatusUpdate.status }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("UpdateApp" in message) {
            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "app", app: message.UpdateApp.app }
            })

            this.eventTarget.dispatchEvent(event)
        } else if ("ConnectionComplete" in message) {
            const capabilities = message.ConnectionComplete.capabilities
            const width = message.ConnectionComplete.width
            const height = message.ConnectionComplete.height

            const event: InfoEvent = new CustomEvent("stream-info", {
                detail: { type: "connectionComplete", capabilities }
            })

            this.eventTarget.dispatchEvent(event)

            this.input.onStreamStart(capabilities, [width, height])
        }
        // -- WebRTC Config
        else if ("WebRtcConfig" in message) {
            const iceServers = message.WebRtcConfig.ice_servers

            this.createPeer({
                iceServers: iceServers
            })

            this.debugLog(`Using WebRTC Ice Servers: ${createPrettyList(
                iceServers.map(server => server.urls).reduce((list, url) => list.concat(url), [])
            )}`)
        }
        // -- Signaling
        else if ("Signaling" in message) {
            if ("Description" in message.Signaling) {
                const descriptionRaw = message.Signaling.Description
                const description = {
                    type: descriptionRaw.ty as RTCSdpType,
                    sdp: descriptionRaw.sdp,
                }

                await this.handleRemoteDescription(description)
            } else if ("AddIceCandidate" in message.Signaling) {
                const candidateRaw = message.Signaling.AddIceCandidate;
                const candidate: RTCIceCandidateInit = {
                    candidate: candidateRaw.candidate,
                    sdpMid: candidateRaw.sdp_mid,
                    sdpMLineIndex: candidateRaw.sdp_mline_index,
                    usernameFragment: candidateRaw.username_fragment
                }

                await this.handleIceCandidate(candidate)
            }
        }
    }

    // -- Signaling
    private async onNegotiationNeeded() {
        if (!this.peer) {
            this.debugLog("OnNegotiationNeeded without a peer")
            return
        }

        await this.peer.setLocalDescription()

        await this.sendLocalDescription()
    }


    private remoteDescription: RTCSessionDescriptionInit | null = null
    private async handleRemoteDescription(description: RTCSessionDescriptionInit) {
        this.debugLog(`Received Remote Description of type ${description.type}`)

        this.remoteDescription = description
        if (!this.peer) {
            this.debugLog(`Saving Remote Description for Peer creation`)
            return
        }

        await this.peer.setRemoteDescription(description)

        if (description.type === "offer") {
            await this.peer.setLocalDescription()

            await this.sendLocalDescription()
        }

        await this.tryDequeueIceCandidates()
    }

    private iceCandidateQueue: Array<RTCIceCandidateInit> = []
    private async tryDequeueIceCandidates() {
        for (const candidate of this.iceCandidateQueue.splice(0)) {
            await this.handleIceCandidate(candidate)
        }
    }
    private async handleIceCandidate(candidate: RTCIceCandidateInit) {
        if (!this.peer || !this.remoteDescription) {
            this.debugLog(`Received Ice Candidate and queuing it: ${candidate.candidate}`)
            this.iceCandidateQueue.push(candidate)
            return
        }

        this.debugLog(`Adding Ice Candidate: ${candidate.candidate}`)

        this.peer.addIceCandidate(candidate)
    }

    private sendLocalDescription() {
        if (!this.peer) {
            this.debugLog("Send Local Description without a peer")
            return
        }

        const description = this.peer.localDescription as RTCSessionDescription;
        this.debugLog(`Sending Local Description of type ${description.type}`)

        this.sendWsMessage({
            Signaling: {
                Description: {
                    ty: description.type,
                    sdp: description.sdp
                }
            }
        })
    }
    private onIceCandidate(event: RTCPeerConnectionIceEvent) {
        const candidateJson = event.candidate?.toJSON()
        if (!candidateJson || !candidateJson?.candidate) {
            return;
        }
        this.debugLog(`Sending Ice Candidate: ${candidateJson.candidate}`)

        const candidate: RtcIceCandidate = {
            candidate: candidateJson?.candidate,
            sdp_mid: candidateJson?.sdpMid ?? null,
            sdp_mline_index: candidateJson?.sdpMLineIndex ?? null,
            username_fragment: candidateJson?.usernameFragment ?? null
        }

        this.sendWsMessage({
            Signaling: {
                AddIceCandidate: candidate
            }
        })
    }

    // -- Track and Data Channels
    private onTrack(event: RTCTrackEvent) {
        event.receiver.jitterBufferTarget = 0

        if ("playoutDelayHint" in event.receiver) {
            event.receiver.playoutDelayHint = 0
        } else {
            this.debugLog(`playoutDelayHint not supported in receiver: ${event.receiver.track.label}`)
        }

        const stream = event.streams[0]
        if (stream) {
            stream.getTracks().forEach(track => {
                this.debugLog(`Adding Media Track ${track.label}`)

                if (track.kind == "video" && "contentHint" in track) {
                    track.contentHint = "motion"
                }

                this.mediaStream.addTrack(track)
            })
        }
    }
    private onConnectionStateChange() {
        if (!this.peer) {
            this.debugLog("OnConnectionStateChange without a peer")
            return
        }
        this.debugLog(`Changing Peer State to ${this.peer.connectionState}`)

        if (this.peer.connectionState == "failed" || this.peer.connectionState == "disconnected" || this.peer.connectionState == "closed") {
            const customEvent: InfoEvent = new CustomEvent("stream-info", {
                detail: {
                    type: "error",
                    message: `Connection state is ${this.peer.connectionState}`
                }
            })

            this.eventTarget.dispatchEvent(customEvent)
        }
    }
    private onIceConnectionStateChange() {
        if (!this.peer) {
            this.debugLog("OnIceConnectionStateChange without a peer")
            return
        }
        this.debugLog(`Changing Peer Ice State to ${this.peer.iceConnectionState}`)
    }

    private onDataChannel(event: RTCDataChannelEvent) {
        this.debugLog(`Received Data Channel ${event.channel.label}`)

        if (event.channel.label == "general") {
            event.channel.addEventListener("message", this.onGeneralDataChannelMessage.bind(this))
        }
    }
    private async onGeneralDataChannelMessage(event: MessageEvent) {
        const data = event.data

        if (typeof data != "string") {
            return
        }

        let message = JSON.parse(data)
        await this.onMessage(message)
    }

    // -- Raw Web Socket stuff
    private wsSendBuffer: Array<string> = []

    private onWsOpen() {
        this.debugLog(`Web Socket Open`)

        for (const raw of this.wsSendBuffer.splice(0)) {
            this.ws.send(raw)
        }
    }
    private onWsClose() {
        this.debugLog(`Web Socket Closed`)
    }

    private sendWsMessage(message: StreamClientMessage) {
        const raw = JSON.stringify(message)
        if (this.ws.readyState == WebSocket.OPEN) {
            this.ws.send(raw)
        } else {
            this.wsSendBuffer.push(raw)
        }
    }

    private async onRawWsMessage(event: MessageEvent) {
        const data = event.data
        if (typeof data != "string") {
            return
        }

        let message = JSON.parse(data)
        await this.onMessage(message)
    }

    private onError(event: Event) {
        this.debugLog(`Web Socket or WebRtcPeer Error`)

        console.error("Stream Error", event)
    }

    // -- Class Api
    addInfoListener(listener: InfoEventListener) {
        this.eventTarget.addEventListener("stream-info", listener as EventListenerOrEventListenerObject)
    }
    removeInfoListener(listener: InfoEventListener) {
        this.eventTarget.removeEventListener("stream-info", listener as EventListenerOrEventListenerObject)
    }

    getMediaStream(): MediaStream {
        return this.mediaStream
    }

    getInput(): StreamInput {
        return this.input
    }

    getStreamerSize(): [number, number] {
        return this.streamerSize
    }
}

function createPrettyList(list: Array<string>): string {
    let isFirst = true
    let text = "["
    for (const item of list) {
        if (!isFirst) {
            text += ", "
        }
        isFirst = false

        text += item
    }
    text += "]"

    return text
}