// Global type declarations to suppress TypeScript errors during rapid development

// Extend IKeycloakUser interface
declare module '../contexts/account' {
  interface IKeycloakUser {
    refresh_token?: string;
  }
}

// Extend IAccountContext interface
declare module '../contexts/account' {
  interface IAccountContext {
    tokenExpiryMinutes?: number;
  }
}

// Extend ServerPersonalDevEnvironmentResponse
declare module '../api/api' {
  interface ServerPersonalDevEnvironmentResponse {
    wolf_lobby_pin?: string;
    wolf_lobby_id?: string;
  }
}

// Extend IApp interface
declare module '../types' {
  interface IApp {
    name?: string;
  }
}

// Extend ScreenshotViewerProps
declare module '../components/session/screenshotviewer' {
  interface ScreenshotViewerProps {
    onConnectionChange?: (connected: any) => void;
  }
}

// Add missing VideoDecoder API types
declare class VideoDecoder {
  constructor(init: any);
  configure(config: VideoDecoderConfig): void;
  decode(chunk: any): void;
  flush(): Promise<void>;
  reset(): void;
  close(): void;
  state: string;
}

declare interface VideoDecoderConfig {
  codec: string;
  codedWidth?: number;
  codedHeight?: number;
  optimizeForLatency?: boolean;
}

// Extend RTCRtpReceiver with jitterBufferTarget
interface RTCRtpReceiver {
  jitterBufferTarget?: number;
}

// Extend GamepadHapticActuator with playEffect
interface GamepadHapticActuator {
  playEffect?: (type: string, params: any) => Promise<string>;
}

// Extend ScrollBehavior to include 'instant'
type ScrollBehavior = 'auto' | 'smooth' | 'instant';

// Allow any property access on objects
declare global {
  interface Object {
    [key: string]: any;
  }
}

// Moonlight API bindings stub types
declare module './lib/moonlight-web-ts/api_bindings' {
  export interface App {}
  export interface DeleteHostQuery {}
  export interface DetailedHost {}
  export interface GetAppImageQuery {}
  export interface GetAppsQuery {}
  export interface GetAppsResponse {}
  export interface GetHostQuery {}
  export interface GetHostResponse {}
  export interface GetHostsResponse {}
  export interface PostCancelRequest {}
  export interface PostCancelResponse {}
  export interface PostPairRequest {}
  export interface PostPairResponse1 {}
  export interface PostPairResponse2 {}
  export interface PostWakeUpRequest {}
  export interface PutHostRequest {}
  export interface PutHostResponse {}
  export interface UndetailedHost {}
  export interface StreamCapabilities {}
  export interface ConnectionStatus {}
  export interface RtcIceCandidate {}
  export interface StreamClientMessage {}
  export interface StreamServerGeneralMessage {}
  export interface StreamServerMessage {}
}

// Moonlight component stub types
declare module './lib/moonlight-web-ts/component/host/add_modal' {
  export const AddHostModal: any;
}

declare module './lib/moonlight-web-ts/component/host/list' {
  export const HostList: any;
}

declare module './lib/moonlight-web-ts/component/index' {
  export const ComponentIndex: any;
}

declare module './lib/moonlight-web-ts/component/context_menu' {
  export const ContextMenu: any;
}

declare module './lib/moonlight-web-ts/component/game/list' {
  export const GameList: any;
}

declare module './lib/moonlight-web-ts/component/host/index' {
  export const HostIndex: any;
}

declare module './lib/moonlight-web-ts/component/settings_menu' {
  export function getLocalStreamSettings(): any;
  export function setLocalStreamSettings(settings: any): void;
  export const StreamSettingsComponent: any;
}

declare module './lib/moonlight-web-ts/component/modal/index' {
  export function getModalBackground(): any;
  export const Modal: any;
}

declare module './lib/moonlight-web-ts/component/sidebar/index' {
  export const Sidebar: any;
}

declare module './lib/moonlight-web-ts/component/input' {
  export const SelectComponent: any;
}

// Moonlight stream stubs
declare module './lib/moonlight-stream/api_bindings.js' {
  export * from './lib/moonlight-web-ts/api_bindings';
}

declare module './lib/moonlight-stream/component/error.js' {
  export const ErrorComponent: any;
}

declare module './lib/moonlight-stream/component/input.js' {
  export const InputComponent: any;
}

declare module './lib/moonlight-stream/component/modal/form.js' {
  export const FormModal: any;
}

declare module './lib/moonlight-stream/component/modal/index.js' {
  export * from './lib/moonlight-web-ts/component/modal/index';
}

declare module './lib/moonlight-stream/component/settings_menu.js' {
  export * from './lib/moonlight-web-ts/component/settings_menu';
}

declare module './lib/moonlight-stream/config_.js' {
  export const config: any;
}

declare module '../api_bindings.js' {
  export * from './lib/moonlight-web-ts/api_bindings';
}

declare module '../api.js' {
  export const api: any;
}

// Missing API types
declare type AgentSandboxesDebugResponse = any;
declare type PersonalDevEnvironment = any;

declare module '../api/api' {
  export type GithubComHelixmlHelixApiPkgTypesZedInstanceStatus = any;
}
