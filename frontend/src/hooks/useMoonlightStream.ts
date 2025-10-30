import { useEffect, useRef, useState, useCallback } from 'react';

/**
 * Stream capabilities from moonlight-web-stream
 */
export interface StreamCapabilities {
  video_codec_support?: {
    h264?: boolean;
    hevc?: boolean;
    av1?: boolean;
  };
  audio_codec_support?: {
    opus?: boolean;
  };
}

/**
 * Stream settings
 */
export interface StreamSettings {
  bitrate: number; // kbps
  packetSize: number; // bytes
  fps: number;
  width: number;
  height: number;
  videoSampleQueueSize: number;
  audioSampleQueueSize: number;
  playAudioLocal: boolean;
  videoCodecH264?: boolean;
  videoCodecHEVC?: boolean;
  videoCodecAV1?: boolean;
}

/**
 * Stream client messages (to server)
 */
type StreamClientMessage =
  | {
      AuthenticateAndInit: {
        credentials: string;
        host_id: number;
        app_id: number;
        bitrate: number;
        packet_size: number;
        fps: number;
        width: number;
        height: number;
        video_sample_queue_size: number;
        play_audio_local: boolean;
        audio_sample_queue_size: number;
        video_supported_formats: number;
        video_colorspace: string;
        video_color_range_full: boolean;
      };
    }
  | {
      Signaling: {
        type: 'offer' | 'answer';
        sdp: string;
      };
    }
  | {
      IceCandidate: {
        candidate: string;
        sdp_m_line_index: number;
        sdp_mid: string;
      };
    };

/**
 * Stream server messages (from server)
 */
interface StreamServerMessage {
  Info?: {
    app?: any;
  };
  Signaling?: {
    type: 'offer' | 'answer';
    sdp: string;
  };
  IceCandidate?: {
    candidate: string;
    sdp_m_line_index?: number;
    sdp_mid?: string;
  };
  StageStarting?: {
    stage: string;
  };
  StageComplete?: {
    stage: string;
  };
  StageFailed?: {
    stage: string;
    error_code: number;
  };
  ConnectionComplete?: {
    capabilities: StreamCapabilities;
  };
  ConnectionStatus?: {
    status: 'Connected' | 'Disconnected';
  };
  ConnectionTerminated?: {
    error_code: number;
  };
  HostNotPaired?: object;
  HostNotFound?: object;
  AppNotFound?: object;
  InternalServerError?: object;
}

export interface MoonlightStreamState {
  isConnecting: boolean;
  isConnected: boolean;
  error: string | null;
  mediaStream: MediaStream | null;
  capabilities: StreamCapabilities | null;
  currentStage: string | null;
}

/**
 * Custom hook for Moonlight streaming via moonlight-web-stream backend
 *
 * This hook manages the WebSocket connection to moonlight-web-stream's streamer service,
 * handles WebRTC signaling, and provides a MediaStream for video playback.
 *
 * Based on the Stream class from moonlight-web-stream project.
 */
export function useMoonlightStream(
  hostId: number,
  appId: number,
  settings: StreamSettings,
  credentials: string = 'helix'
) {
  const [state, setState] = useState<MoonlightStreamState>({
    isConnecting: false,
    isConnected: false,
    error: null,
    mediaStream: null,
    capabilities: null,
    currentStage: null,
  });

  const wsRef = useRef<WebSocket | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const mediaStreamRef = useRef<MediaStream>(new MediaStream());

  // Create supported video formats bitmask (from moonlight-web-stream)
  const createSupportedVideoFormatsBits = useCallback(() => {
    let bits = 0;
    if (settings.videoCodecH264) bits |= 1; // H264 = 1
    if (settings.videoCodecHEVC) bits |= 2; // HEVC = 2
    if (settings.videoCodecAV1) bits |= 4; // AV1 = 4
    return bits;
  }, [settings]);

  // Send message to WebSocket
  const sendWsMessage = useCallback((message: StreamClientMessage) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(message));
    }
  }, []);

  // Handle WebSocket messages
  const handleWsMessage = useCallback(async (event: MessageEvent) => {
    const message: StreamServerMessage = JSON.parse(event.data);

    // Handle different message types
    if (message.Info?.app) {
      console.log('Moonlight app info:', message.Info.app);
    } else if (message.Signaling) {
      // Server sent SDP offer/answer - handle WebRTC signaling
      if (!peerRef.current) {
        console.error('Received signaling but no peer connection exists');
        return;
      }

      const desc = new RTCSessionDescription({
        type: message.Signaling.type,
        sdp: message.Signaling.sdp,
      });

      await peerRef.current.setRemoteDescription(desc);

      // If we received an offer, create an answer
      if (message.Signaling.type === 'offer') {
        const answer = await peerRef.current.createAnswer();
        await peerRef.current.setLocalDescription(answer);

        sendWsMessage({
          Signaling: {
            type: 'answer',
            sdp: answer.sdp!,
          },
        });
      }
    } else if (message.IceCandidate) {
      // Server sent ICE candidate
      if (peerRef.current && message.IceCandidate.candidate) {
        await peerRef.current.addIceCandidate(
          new RTCIceCandidate({
            candidate: message.IceCandidate.candidate,
            sdpMLineIndex: message.IceCandidate.sdp_m_line_index,
            sdpMid: message.IceCandidate.sdp_mid,
          })
        );
      }
    } else if (message.StageStarting) {
      setState((prev) => ({ ...prev, currentStage: message.StageStarting!.stage }));
    } else if (message.StageComplete) {
      setState((prev) => ({ ...prev, currentStage: null }));
    } else if (message.StageFailed) {
      setState((prev) => ({
        ...prev,
        error: `Stage failed: ${message.StageFailed!.stage} (code: ${message.StageFailed!.error_code})`,
        isConnecting: false,
      }));
    } else if (message.ConnectionComplete) {
      setState((prev) => ({
        ...prev,
        isConnected: true,
        isConnecting: false,
        capabilities: message.ConnectionComplete!.capabilities,
      }));
    } else if (message.ConnectionStatus) {
      const connected = message.ConnectionStatus.status === 'Connected';
      setState((prev) => ({ ...prev, isConnected: connected }));
    } else if (message.ConnectionTerminated) {
      setState((prev) => ({
        ...prev,
        isConnected: false,
        error: `Connection terminated (code: ${message.ConnectionTerminated!.error_code})`,
      }));
    } else if (message.HostNotPaired) {
      setState((prev) => ({
        ...prev,
        error: 'Host is not paired - please pair with Wolf first',
        isConnecting: false,
      }));
    } else if (message.HostNotFound) {
      setState((prev) => ({
        ...prev,
        error: 'Host not found',
        isConnecting: false,
      }));
    } else if (message.AppNotFound) {
      setState((prev) => ({
        ...prev,
        error: 'App not found',
        isConnecting: false,
      }));
    } else if (message.InternalServerError) {
      setState((prev) => ({
        ...prev,
        error: 'Internal server error',
        isConnecting: false,
      }));
    }
  }, [sendWsMessage]);

  // Create WebRTC peer connection
  const createPeerConnection = useCallback(() => {
    const config: RTCConfiguration = {
      iceServers: [
        {
          urls: [
            'stun:stun.l.google.com:19302',
            'stun:stun1.l.google.com:19302',
            'stun:stun2.l.google.com:19302',
          ],
        },
      ],
    };

    const peer = new RTCPeerConnection(config);
    peerRef.current = peer;

    // Handle ICE candidates
    peer.onicecandidate = (event) => {
      if (event.candidate) {
        sendWsMessage({
          IceCandidate: {
            candidate: event.candidate.candidate,
            sdp_m_line_index: event.candidate.sdpMLineIndex || 0,
            sdp_mid: event.candidate.sdpMid || '',
          },
        });
      }
    };

    // Handle incoming media tracks
    peer.ontrack = (event) => {
      console.log('Received media track:', event.track.kind);

      // Add track to media stream
      mediaStreamRef.current.addTrack(event.track);

      setState((prev) => ({
        ...prev,
        mediaStream: mediaStreamRef.current,
      }));
    };

    // Handle connection state changes
    peer.onconnectionstatechange = () => {
      console.log('WebRTC connection state:', peer.connectionState);

      if (peer.connectionState === 'failed' || peer.connectionState === 'closed') {
        setState((prev) => ({
          ...prev,
          isConnected: false,
          error: `WebRTC connection ${peer.connectionState}`,
        }));
      }
    };

    return peer;
  }, [sendWsMessage]);

  // Connect to stream
  const connect = useCallback(() => {
    setState((prev) => ({ ...prev, isConnecting: true, error: null }));

    // Create WebSocket to moonlight-web-stream backend
    const wsUrl = `/moonlight/api/host/stream`;
    const ws = new WebSocket(
      `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//` +
        `${window.location.host}${wsUrl}`
    );
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('WebSocket connected to moonlight-web-stream');

      // Create WebRTC peer
      createPeerConnection();

      // Send authentication and init message
      sendWsMessage({
        AuthenticateAndInit: {
          credentials,
          host_id: hostId,
          app_id: appId,
          bitrate: settings.bitrate,
          packet_size: settings.packetSize,
          fps: settings.fps,
          width: settings.width,
          height: settings.height,
          video_sample_queue_size: settings.videoSampleQueueSize,
          play_audio_local: settings.playAudioLocal,
          audio_sample_queue_size: settings.audioSampleQueueSize,
          video_supported_formats: createSupportedVideoFormatsBits(),
          video_colorspace: 'Rec709',
          video_color_range_full: true,
        },
      });
    };

    ws.onmessage = handleWsMessage;

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      setState((prev) => ({
        ...prev,
        error: 'WebSocket connection failed',
        isConnecting: false,
      }));
    };

    ws.onclose = () => {
      console.log('WebSocket closed');
      setState((prev) => ({
        ...prev,
        isConnected: false,
        isConnecting: false,
      }));
    };
  }, [hostId, appId, settings, credentials, createPeerConnection, handleWsMessage, sendWsMessage, createSupportedVideoFormatsBits]);

  // Disconnect
  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    if (peerRef.current) {
      peerRef.current.close();
      peerRef.current = null;
    }

    setState({
      isConnecting: false,
      isConnected: false,
      error: null,
      mediaStream: null,
      capabilities: null,
      currentStage: null,
    });
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, [disconnect]);

  return {
    ...state,
    connect,
    disconnect,
  };
}
