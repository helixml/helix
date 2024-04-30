// TODO: it sucks that there are constants defined in types
export type ISessionCreator = 'system' | 'user'
export const SESSION_CREATOR_SYSTEM: ISessionCreator = 'system'
export const SESSION_CREATOR_USER: ISessionCreator = 'user'

export type ISessionMode = 'inference' | 'finetune'
export const SESSION_MODE_INFERENCE: ISessionMode = 'inference'
export const SESSION_MODE_FINETUNE: ISessionMode = 'finetune'

export type ISessionType = 'text' | 'image'
export const SESSION_TYPE_TEXT: ISessionType = 'text'
export const SESSION_TYPE_IMAGE: ISessionType = 'image'

export type ISessionOriginType = 'user_created' | 'cloned'
export const SESSION_ORIGIN_TYPE_USER_CREATED: ISessionOriginType = 'user_created'
export const SESSION_ORIGIN_TYPE_CLONED: ISessionOriginType = 'cloned'

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

export type ICloneInteractionMode = 'just_data' | 'with_questions' | 'all'
export const CLONE_INTERACTION_MODE_JUST_DATA: ICloneInteractionMode = 'just_data'
export const CLONE_INTERACTION_MODE_WITH_QUESTIONS: ICloneInteractionMode = 'with_questions'
export const CLONE_INTERACTION_MODE_ALL: ICloneInteractionMode = 'all'

export type IModelName = 'mistralai/Mistral-7B-Instruct-v0.1' | 'stabilityai/stable-diffusion-xl-base-1.0' | 'mistral:7b-instruct' | 'mixtral:instruct' | 'llama3:instruct' | 'llama3:70b-instruct-q2_K'
export const MODEL_NAME_MISTRAL: IModelName = 'mistralai/Mistral-7B-Instruct-v0.1'
export const MODEL_NAME_SDXL: IModelName = 'stabilityai/stable-diffusion-xl-base-1.0'
export const MODEL_NAME_OLLAMA_MISTRAL: IModelName = 'mistral:7b-instruct'
export const MODEL_NAME_OLLAMA_LLAMA3_8B: IModelName = 'llama3:instruct'
export const MODEL_NAME_OLLAMA_LLAMA3_70B: IModelName = 'llama3:70b-instruct-q2_K'
export const MODEL_NAME_OLLAMA_MIXTRAL: IModelName = 'mixtral:instruct'

export type ITextDataPrepStage = '' | 'edit_files' | 'extract_text' | 'index_rag' | 'generate_questions' | 'edit_questions' | 'finetune' | 'complete'
export const TEXT_DATA_PREP_STAGE_NONE: ITextDataPrepStage = ''
export const TEXT_DATA_PREP_STAGE_EDIT_FILES: ITextDataPrepStage = 'edit_files'
export const TEXT_DATA_PREP_STAGE_EXTRACT_TEXT: ITextDataPrepStage = 'extract_text'
export const TEXT_DATA_PREP_STAGE_INDEX_RAG: ITextDataPrepStage = 'index_rag'
export const TEXT_DATA_PREP_STAGE_GENERATE_QUESTIONS: ITextDataPrepStage = 'generate_questions'
export const TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS: ITextDataPrepStage = 'edit_questions'
export const TEXT_DATA_PREP_STAGE_FINETUNE: ITextDataPrepStage = 'finetune'
export const TEXT_DATA_PREP_STAGE_COMPLETE: ITextDataPrepStage = 'complete'

export type IAppType = 'helix' | 'github'
export const APP_TYPE_HELIX: IAppType = 'helix'
export const APP_TYPE_GITHUB: IAppType = 'github'

export const TEXT_DATA_PREP_STAGES: ITextDataPrepStage[] = [
  TEXT_DATA_PREP_STAGE_EDIT_FILES,
  TEXT_DATA_PREP_STAGE_EXTRACT_TEXT,
  TEXT_DATA_PREP_STAGE_INDEX_RAG,
  TEXT_DATA_PREP_STAGE_GENERATE_QUESTIONS,
  TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS,
  TEXT_DATA_PREP_STAGE_FINETUNE,
  TEXT_DATA_PREP_STAGE_COMPLETE,
]

export const TEXT_DATA_PREP_DISPLAY_STAGES: ITextDataPrepStage[] = [
  TEXT_DATA_PREP_STAGE_EXTRACT_TEXT,
  TEXT_DATA_PREP_STAGE_INDEX_RAG,
  TEXT_DATA_PREP_STAGE_GENERATE_QUESTIONS,
  TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS,
  TEXT_DATA_PREP_STAGE_FINETUNE,
]

export const SESSION_PAGINATION_PAGE_LIMIT = 30

export interface IKeycloakUser {
  id: string,
  email: string,
  token: string,
  name: string,
}

export interface IUserConfig {
  stripe_subscription_active?: boolean,
  stripe_customer_id?: string,
  stripe_subscription_id?: string,
}

export type IOwnerType = 'user' | 'system' | 'org'

export type IApiKeyType = 'api' | 'github' | 'app'

export interface IApiKey {
  owner: string,
  owner_type: string,
  key: string,
  name: string,
  app_id: string,
  type: IApiKeyType,
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

export interface IInteractionMessage {
  role: string,
  content: string,
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
  display_message: string,
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
  data_prep_limited: boolean,
  data_prep_limit: number,
}

export interface ISessionOrigin {
  type: ISessionOriginType,
  cloned_session_id?: string,
  cloned_interaction_id?: string,
}

export interface ISessionConfig {
  original_mode: ISessionMode,
  origin: ISessionOrigin,
  shared?: boolean,
  avatar: string,
  priority: boolean,
  document_ids: Record<string, string>,
  document_group_id: string,
  manually_review_questions: boolean,
  system_prompt: string,
  helix_version: string,
  eval_run_id: string,
  eval_user_score: string,
  eval_user_reason: string,
  eval_manual_score: string,
  eval_manual_reason: string,
  eval_automatic_score: string,
  eval_automatic_reason: string,
  eval_original_user_prompts: string[],
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
  stripe_enabled: boolean,
  sentry_dsn_frontend: string,
  google_analytics_frontend: string,
  eval_user_id: string,
  tools_enabled: boolean,
  apps_enabled: boolean,
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

export interface ICounter {
  count: number,
}

export interface ISessionsList {
  sessions: ISessionSummary[],
  counter: ICounter,
}

export interface IPaginationState {
  total: number,
  limit: number,
  offset: number,
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

export const BUTTON_STATES: IButtonStates = {
  addUrlColor: 'primary',
  addUrlLabel: 'Add URL',
  addTextColor: 'primary',
  addTextLabel: 'Add Text',
  uploadFilesColor: 'primary',
  uploadFilesLabel: 'Or Choose Files',
}

// these are kept in local storage so we know what to do once we are logged in
export interface IShareSessionInstructions {
  cloneMode?: ICloneInteractionMode,
  cloneInteractionID?: string,
  inferencePrompt?: string,
  addDocumentsMode?: boolean,
}

export type IToolType = 'api' | 'gptscript'

export interface IToolApiAction {
  name: string,
  description: string,
  method: string,
  path: string,
}

export interface IToolApiConfig {
  url: string,
  schema: string,
  actions: IToolApiAction[],
  headers: Record<string, string>,
  query: Record<string, string>,
}

export interface IToolGptScriptConfig {
  script?: string,
  script_url?: string, // If script lives on a remote server, specify the URL
}

export interface IToolConfig {
  api?: IToolApiConfig,
  gptscript?: IToolGptScriptConfig,
}

export interface ITool {
  id: string,
  created: string,
  updated: string,
  owner: string,
  owner_type: IOwnerType,
  name: string,
  description: string,
  global: boolean,
  tool_type: IToolType,
  config: IToolConfig,
}

export interface IKeyPair {
	type: string,
  privateKey: string,
  publicKey: string,
}

export interface IAppHelixConfigGptScript {
  source?: string,
  name?: string,
  file_path?: string,
  content?: string,
  input?: string,
  env?: string[],
}

export interface IAppHelixConfigGptScripts {
  files?: string[],
  scripts?: IAppHelixConfigGptScript[],
}

export interface IAppHelixConfig {
  name?: string,
  description?: string,
  avatar?: string,
  system_prompt?: string,
  active_tools?: string[],
  secrets?: Record<string, string>,
  allowed_domains?: string[],
  gptscript?: IAppHelixConfigGptScripts,
}

export interface IAppGithubConfig {
  repo: string,
  hash: string,
  key_pair?: IKeyPair,
}

export interface IAppConfig {
  helix?: IAppHelixConfig,
  github?: IAppGithubConfig,
}

export interface IApp {
  id: string,
  created: Date,
  updated: Date,
  owner: string,
  owner_type: IOwnerType,
  name: string,
  description: string,
  app_type: IAppType,
  config: IAppConfig,
}

export interface IAppUpdate {
  name: string,
  description: string,
  secrets: Record<string, string>,
  allowed_domains: string[],
}

export interface IGithubStatus {
  has_token: boolean,
  redirect_url: string,
}

export interface IGithubRepo {
  full_name: string,
}

export interface IGptScriptRequest {
  file_path: string,
  input: string,
}

export interface IGptScriptResponse {
  output: string,
  error: string,
}

export type IRagDistanceFunction = 'l2' | 'inner_product' | 'cosine'
export interface ICreateSessionConfig {
  activeToolIDs: string[],
  finetuneEnabled: boolean,
  ragEnabled: boolean,
  ragDistanceFunction: IRagDistanceFunction, 
  ragThreshold: number,
  ragResultsCount: number,
  ragChunkSize: number,
  ragChunkOverflow: number,
}

export interface IHelixModel {
  id: string,
  title: string,
  description: string,
}

export interface IUploadFile {
  // used for the file drawer - e.g. show the actual URL or a preview of the text
  label: string,
  file: File,
}