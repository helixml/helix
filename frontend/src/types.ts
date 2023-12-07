import {
  IWorkerTaskResponseType,
  ISessionCreator,
  ISessionMode,
  IInteractionState,
  ITextDataPrepStage,
  IWebSocketEventType,
  ISessionType,
} from './constants'

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

export interface IDataPrepChunk {
  index: number,
  question_count: number,
  error: string,
}

export interface IDataPrepStats {
  total_files: number,
  total_chunks: number,
  total_questions: number,
  converted: number,
  errors: number,
}

export interface IDataPrepChunkWithFilename extends IDataPrepChunk {
  filename: string,
}

export interface IInteraction {
  id: string,
  created: string,
  updated: string,
  scheduled: string,
  completed: string,
  creator: ISessionCreator,
  mode: ISessionMode,
  runner: string,
  message: string,
  progress: number,
  files: string[],
  finished: boolean,
  metadata: Record<string, string>,
  state: IInteractionState,
  status: string,
  error: string,
  lora_dir: string,
  data_prep_chunks: Record<string, IDataPrepChunk[]>,
  data_prep_stage: ITextDataPrepStage,
}

export interface ISessionConfig {
  original_mode: ISessionMode,
}

export interface ISession {
  id: string,
  name: string,
  created: string,
  updated: string,
  parent_session: string,
  parent_bot: string,
  child_bot: string,
  config: ISessionConfig,
  mode: ISessionMode,
  type: ISessionType,
  model_name: string,
  lora_dir: string,
  interactions: IInteraction[],
  owner: string,
  owner_type: IOwnerType,
}

export interface IBotForm {
  name: string,
}

export interface IBotConfig {

}

export interface IBot {
  id: string,
  name: string,
  created: string,
  updated: string,
  owner: string,
  owner_type: IOwnerType,
  config: IBotConfig,
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

export interface IModelInstanceState {
  id: string,
  model_name: string,
  mode: ISessionMode,
  lora_dir: string,
  initial_session_id: string,
  current_session?: ISessionSummary | null,
  job_history: ISessionSummary[],
  timeout: number,
  last_activity: number,
  stale: boolean,
  memory: number,
}

export interface IRunnerState {
  id: string,
  created: string,
  total_memory: number,
  free_memory: number,
  labels: Record<string, string>,
  model_instances: IModelInstanceState[],
  scheduling_decisions: string[],
}

export interface ISessionFilterModel {
  mode: ISessionMode,
  model_name?: string,
  lora_dir?: string,
}
export interface ISessionFilter {
  mode?: ISessionMode | "",
  type?: ISessionType | "",
  model_name?: string,
  lora_dir?: string,
  memory?: number,
  reject?: ISessionFilterModel[],
  older?: string,
}

export interface  IGlobalSchedulingDecision {
  created: string,
  runner_id: string,
  session_id: string,
  interaction_id: string,
  filter: ISessionFilter,
  mode: ISessionMode,
  model_name: string,
}

export interface IDashboardData {
  session_queue: ISessionSummary[],
  runners: IRunnerState[],
  global_scheduling_decisions: IGlobalSchedulingDecision[],
}

export interface ISessionSummary {
  created: string,
  updated: string,
  scheduled: string,
  completed: string,
  session_id: string,
  name: string,
  interaction_id: string,
  model_name: string,
  mode: ISessionMode,
  type: ISessionType,
  owner: string,
  lora_dir?: string,
  summary: string,
}

export interface ISessionMetaUpdate {
  id: string,
  name: string,
  owner?: string,
  owner_type?: string,
}


export interface ISerlializedFile {
  filename: string
  content: string
  mimeType: string
}

export interface ISerializedPage {
  files: ISerlializedFile[],
  labels: Record<string, string>,
  fineTuneStep: number,
  manualTextFileCounter: number,
  inputValue: string,
}

export type IButtonStateColor = 'primary' | 'secondary'
export interface IButtonStates {
  addTextColor: IButtonStateColor,
  addTextLabel: string,
  addUrlColor: IButtonStateColor,
  addUrlLabel: string,
  uploadFilesColor: IButtonStateColor,
  uploadFilesLabel: string,
}
