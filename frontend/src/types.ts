export interface IUser {
  id: string,
  email: string,
  token: string,
}

export interface IBalanceTransferData {
  job_id?: string,
  stripe_payment_id?: string,
}

export interface IBalanceTransfer {
  id: string,
  created: number,
  owner: string,
  owner_type: string,
  payment_type: string,
  amount: number,
  data: IBalanceTransferData,
}

export type IOwnerType = 'user' | 'system' | 'org';

export interface IApiKey {
  owner: string;
  owner_type: string;
  key: string;
  name: string;
}

export interface IFileStoreBreadcrumb {
  path: string,
  title: string,
}

export interface IFileStoreItem {
  created: number;
  size: number;
  directory: boolean;
  name: string;
  path: string;
  url: string;
}

export interface IFileStoreFolder {
  name: string,
  readonly: boolean,
}

export interface IFileStoreConfig {
  user_prefix: string,
  folders: IFileStoreFolder[],
}

export type ISessionCreator = 'system' | 'user'

export const SESSION_CREATOR_SYSTEM: ISessionCreator = 'system'
export const SESSION_CREATOR_USER: ISessionCreator = 'user'

export type ISessionMode = 'inference' | 'finetune'

export const SESSION_MODE_INFERENCE: ISessionMode = 'inference'
export const SESSION_MODE_FINETUNE: ISessionMode = 'finetune'

export type ISessionType = 'text' | 'image'

export const SESSION_TYPE_TEXT: ISessionType = 'text'
export const SESSION_TYPE_IMAGE: ISessionType = 'image'

export type ISessionState = 'preparing' | 'ready' | 'error'

export const SESSION_STATE_PREPARING: ISessionState = 'preparing'
export const SESSION_STATE_READY: ISessionState = 'ready'
export const SESSION_STATE_ERROR: ISessionState = 'error'

export type IWebSocketEventType = 'session_update' | 'worker_task_response'
export const WEBSOCKET_EVENT_TYPE_SESSION_UPDATE: IWebSocketEventType = 'session_update'
export const WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE: IWebSocketEventType = 'worker_task_response'

export type IWorkerTaskResponseType = 'stream' | 'progress' | 'result'
export const WORKER_TASK_RESPONSE_TYPE_STREAM: IWorkerTaskResponseType = 'stream'
export const WORKER_TASK_RESPONSE_TYPE_PROGRESS: IWorkerTaskResponseType = 'progress'
export const WORKER_TASK_RESPONSE_TYPE_RESULT: IWorkerTaskResponseType = 'result'

export interface IWorkerTaskResponse {
  type: IWorkerTaskResponseType,
  session_id: string,
  owner: string,
  message?: string,
  progress?: number,
  status?: string,
  files?: string[],
  error?: string,
}

export interface IInteraction {
  id: string,
  created: number,
  creator: ISessionCreator,
  runner: string,
  message: string,
  progress: number,
  status: string,
  state: string,
  files: string[],
  finetune_file: string,
  finished: boolean,
  metadata: Record<string, string>,
  error: string,
}

export interface ISession {
  id: string,
  name: string,
  created: number,
  updated: number,
  mode: ISessionMode,
  type: ISessionType,
  model_name: string,
  state: ISessionState,
  error: string,
  finetune_file: string,
  interactions: IInteraction[],
  owner: string,
  owner_type: IOwnerType,
}

export interface IWebsocketEvent {
  type: IWebSocketEventType,
  session_id: string,
  owner: string,
  session?: ISession,
  worker_task_response?: IWorkerTaskResponse,
}

export interface IServerConfig {
  filestore_prefix: string,
}