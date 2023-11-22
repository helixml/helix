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

export type IInteractionState = 'waiting' | 'editing' | 'complete' | 'error'

export const INTERACTION_STATE_WAITING: IInteractionState = 'waiting'
export const INTERACTION_STATE_EDITING: IInteractionState = 'editing'
export const INTERACTION_STATE_COMPLETE: IInteractionState = 'complete'
export const INTERACTION_STATE_ERROR: IInteractionState = 'error'

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
  created: string,
  scheduled: string,
  completed: string,
  creator: ISessionCreator,
  runner: string,
  message: string,
  progress: number,
  status: string,
  state: IInteractionState,
  files: string[],
  lora_dir: string,
  finished: boolean,
  metadata: Record<string, string>,
  error: string,
}

export interface ISession {
  id: string,
  name: string,
  created: string,
  updated: string,
  parent_session: string,
  mode: ISessionMode,
  type: ISessionType,
  model_name: string,
  lora_dir: string,
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

export interface IConversation {
  from: string,
  value: string,
}

export interface IConversations {
  conversations: IConversation[],
}

export interface IQuestionAnswer {
  id: string,
  question: string,
  answer: string,
}

export interface IModelInstanceJob {
  created: number,
  session_id: string,
  interaction_id: string,
}

export interface IModelInstanceState {
  id: string,
  model_name: string,
  mode: ISessionMode,
  lora_dir: string,
  initial_session_id: string,
  current_session?: ISession,
  job_history: IModelInstanceJob[],
}

export interface IRunnerState {
  id: string,
  created: number,
  total_memory: number,
  free_memory: number,
  labels: Record<string, string>,
  model_instances: IModelInstanceState[],
}

export interface IDashboardData {
  session_queue: ISession[],
  runners: IRunnerState[],
}