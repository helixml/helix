import { 
  TypesUserAppAccessResponse,
  TypesStepInfo,
  TypesMessage,
  TypesAssistantCalculator,
  TypesToolCalculatorConfig,
  TypesAssistantBrowser,
  TypesToolBrowserConfig,
  TypesAssistantEmail,
  TypesToolEmailConfig,
  TypesAssistantWebSearch,
  TypesToolWebSearchConfig,
  TypesAssistantAzureDevOps,
  TypesTrigger,
  TypesSession,
  TypesAssistantMCP,
  TypesToolMCPClientConfig,
} from './api/api'

export type ISessionCreator = 'system' | 'user' | 'assistant'
// SYSTEM means the system prompt, NOT an assistant message (as it previously
// did). At time of writing, it's unused in the frontend because the frontend
// doesn't have system prompt support.
export const SESSION_CREATOR_SYSTEM: ISessionCreator = 'system'

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

export type IWebSocketEventType = 'session_update' | 'worker_task_response' | 'step_info'
export const WEBSOCKET_EVENT_TYPE_SESSION_UPDATE: IWebSocketEventType = 'session_update'
export const WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE: IWebSocketEventType = 'worker_task_response'
export const WEBSOCKET_EVENT_TYPE_STEP_INFO: IWebSocketEventType = 'step_info'
export type IWorkerTaskResponseType = 'stream' | 'progress' | 'result'
export const WORKER_TASK_RESPONSE_TYPE_STREAM: IWorkerTaskResponseType = 'stream'
export const WORKER_TASK_RESPONSE_TYPE_PROGRESS: IWorkerTaskResponseType = 'progress'
export const WORKER_TASK_RESPONSE_TYPE_RESULT: IWorkerTaskResponseType = 'result'

export type ICloneInteractionMode = 'just_data' | 'with_questions' | 'all'
export const CLONE_INTERACTION_MODE_JUST_DATA: ICloneInteractionMode = 'just_data'
export const CLONE_INTERACTION_MODE_WITH_QUESTIONS: ICloneInteractionMode = 'with_questions'
export const CLONE_INTERACTION_MODE_ALL: ICloneInteractionMode = 'all'

export type IModelName = string

export type ITextDataPrepStage = '' | 'edit_files' | 'extract_text' | 'index_rag' | 'generate_questions' | 'edit_questions' | 'finetune' | 'complete'
export const TEXT_DATA_PREP_STAGE_NONE: ITextDataPrepStage = ''
export const TEXT_DATA_PREP_STAGE_EDIT_FILES: ITextDataPrepStage = 'edit_files'
export const TEXT_DATA_PREP_STAGE_EXTRACT_TEXT: ITextDataPrepStage = 'extract_text'
export const TEXT_DATA_PREP_STAGE_INDEX_RAG: ITextDataPrepStage = 'index_rag'
export const TEXT_DATA_PREP_STAGE_GENERATE_QUESTIONS: ITextDataPrepStage = 'generate_questions'
export const TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS: ITextDataPrepStage = 'edit_questions'
export const TEXT_DATA_PREP_STAGE_FINETUNE: ITextDataPrepStage = 'finetune'
export const TEXT_DATA_PREP_STAGE_COMPLETE: ITextDataPrepStage = 'complete'

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

// Agent Types
export type IAgentType = 'helix_basic' | 'helix_agent' | 'zed_external'
export const AGENT_TYPE_HELIX_BASIC: IAgentType = 'helix_basic'
export const AGENT_TYPE_HELIX_AGENT: IAgentType = 'helix_agent'
export const AGENT_TYPE_ZED_EXTERNAL: IAgentType = 'zed_external'

export interface IExternalAgentConfig {
  workspace_dir?: string
  project_path?: string
  env_vars?: string[]
  auto_connect_rdp?: boolean
  // Video settings (Phase 3.5) - matches PDE display settings
  display_width?: number        // Streaming resolution width (default: 2560)
  display_height?: number       // Streaming resolution height (default: 1600)
  display_refresh_rate?: number // Streaming refresh rate (default: 60)
}

export interface IAgentTypeOption {
  value: IAgentType
  label: string
  description: string
  icon?: string
  disabled?: boolean
}

export const AGENT_TYPE_OPTIONS: IAgentTypeOption[] = [
  {
    value: AGENT_TYPE_HELIX_BASIC,
    label: 'Basic Helix Agent',
    description: 'Simple conversational AI (no multi-turn, useful for RAG)',
    icon: 'chat',
  },
  {
    value: AGENT_TYPE_HELIX_AGENT,
    label: 'Multi-Turn Helix Agent',
    description: 'Advanced conversational AI with multi-turn tool use and reasoning',
    icon: 'auto_awesome',
  },
  {
    value: AGENT_TYPE_ZED_EXTERNAL,
    label: 'Zed External Agent', 
    description: 'Full development environment with code editing via RDP',
    icon: 'code',
  }
]



export interface IKeycloakUser {
  id: string,
  email: string,
  token: string,
  name: string,
}

export interface IUserConfig {
  // Removed stripe subscription fields - now come from wallet
}

export interface IHelixModel {
  id: string;
  type: string;
  name: string;
  description: string;
  hide?: boolean;
}


export type IOwnerType = 'user' | 'system' | 'org'

export type IApiKeyType = 'api' | 'app'

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

// export interface IInteraction {
//   id: string,
//   created: string,
//   updated: string,
//   scheduled: string,
//   completed: string,
//   creator: ISessionCreator,
//   mode: ISessionMode,
//   runner: string,
//   message: string,
//   content: TypesMessageContent,
//   display_message: string,
//   progress: number,
//   files: string[],
//   finished: boolean,
//   metadata: Record<string, string>,
//   state: IInteractionState,
//   status: string,
//   error: string,
//   lora_dir: string,
//   data_prep_chunks: Record<string, IDataPrepChunk[]>,
//   data_prep_stage: ITextDataPrepStage,
//   data_prep_limited: boolean,
//   data_prep_limit: number,
// }

// export interface ISessionOrigin {
//   type: ISessionOriginType,
//   cloned_session_id?: string,
//   cloned_interaction_id?: string,
// }

// export interface ISessionConfig {
//   original_mode: ISessionMode,
//   origin: ISessionOrigin,
//   avatar: string,
//   priority: boolean,
//   document_ids: Record<string, string>,
//   document_group_id: string,
//   session_rag_results: ISessionRAGResult[],
//   manually_review_questions: boolean,
//   system_prompt: string,
//   helix_version: string,
//   eval_run_id: string,
//   eval_user_score: string,
//   eval_user_reason: string,
//   eval_manual_score: string,
//   eval_manual_reason: string,
//   eval_automatic_score: string,
//   eval_automatic_reason: string,
//   eval_original_user_prompts: string[],
//   rag_source_data_entity_id: string,
// }

// export interface ISession {
//   id: string,
//   name: string,
//   created: string,
//   updated: string,
//   parent_session: string,
//   parent_app: string,
//   config: ISessionConfig,
//   mode: ISessionMode,
//   type: ISessionType,
//   model_name: string,
//   provider: string,
//   lora_dir: string,
//   interactions: IInteraction[],
//   owner: string,
//   owner_type: IOwnerType,
// }

// export interface IBotForm {
//   name: string,
// }

// export interface IBotConfig {
// }

// export interface IBot {
//   id: string,
//   name: string,
//   created: string,
//   updated: string,
//   owner: string,
//   owner_type: IOwnerType,
//   config: IBotConfig,
// }

export interface IWebsocketEvent {
  type: IWebSocketEventType,
  session_id: string,
  owner: string,
  session?: TypesSession,
  worker_task_response?: IWorkerTaskResponse,
  step_info?: TypesStepInfo,
}

export interface IServerConfig {
  filestore_prefix: string,
  stripe_enabled: boolean,
  sentry_dsn_frontend: string,
  google_analytics_frontend: string,
  eval_user_id: string,
  tools_enabled: boolean,
  apps_enabled: boolean,
  version?: string,
  latest_version?: string,
  deployment_id?: string,
  license?: {
    valid: boolean,
    organization: string,
    valid_until: string,
    features: {
      users: boolean,
    },
    limits: {
      users: number,
      machines: number,
    },
  },
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
  status?: string,
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

export interface IGlobalSchedulingDecision {
  created: string,
  runner_id: string,
  session_id: string,
  interaction_id: string,
  filter: ISessionFilter,
  mode: ISessionMode,
  model_name: string,
}

export interface ISlot {
  id: string,
  runtime: string,
  model: string,
  version: string,
  active: boolean,
  ready: boolean,
  status: string,
}

export interface LLMInferenceRequest {
  RequestID: string,
  CreatedAt: string,
  Priority: boolean,
  OwnerID: string,
  SessionID: string,
  InteractionID: string,
  Request: {
    model: string,
    messages: IInteractionMessage[],
    stream: boolean,
  }
}

export interface ISlotAttributesWorkload {
  Session: TypesSession,
  LLMInferenceRequest: LLMInferenceRequest,
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
  mode: string,
  type: string,
  owner: string,
  lora_dir?: string,
  summary: string,
  app_id?: string,
}

export interface ISessionMetaUpdate {
  id: string,
  name: string,
  owner?: string,
  owner_type?: string,
}

export interface IUploadFile {
  // used for the file drawer - e.g. show the actual URL or a preview of the text
  drawerLabel: string,
  file: File,
}

export interface ISerlializedFile {
  filename: string
  content: string
  mimeType: string
}

export interface ISerializedPage {
  files: ISerlializedFile[],
  drawerLabels: Record<string, string>,
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

export type IToolType = 'api' | 'gptscript' | 'zapier' | 'web_search' | 'calculator' | 'email' | 'browser' | 'mcp'

export interface IToolApiAction {
  name: string,
  description: string,
  method: string,
  path: string,
}

export interface IToolApiConfig {
  url: string;
  schema?: string;
  actions?: IToolApiAction[];
  headers?: Record<string, string>;
  query?: Record<string, string>;
  request_prep_template?: string;
  response_success_template?: string;
  response_error_template?: string;
  model?: string;
  oauth_provider?: string;
  oauth_scopes?: string[];
}

export interface IToolGptScriptConfig {
  script?: string,
  script_url?: string, // If script lives on a remote server, specify the URL
}

export interface IToolZapierConfig {
  api_key?: string,
  model?: string,
  max_iterations?: number,
}

export interface IToolConfig {
  api?: IToolApiConfig,
  gptscript?: IToolGptScriptConfig,
  zapier?: IToolZapierConfig,
  helix?: IAppHelixConfig,
  browser?: TypesToolBrowserConfig,
  calculator?: TypesToolCalculatorConfig,
  email?: TypesToolEmailConfig,
  web_search?: TypesToolWebSearchConfig,
  mcp?: TypesToolMCPClientConfig,
}

export interface ITool {
  id: string,
  created: string,
  updated: string,
  owner: string,
  owner_type: IOwnerType,
  name: string,
  description: string,
  tool_type: IToolType,
  global: boolean,
  config: IToolConfig,
}

export interface IKeyPair {
  type: string,
  private_key: string,
  public_key: string,
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

export interface IAssistantApi {
  name: string,
  description: string,
  schema: string,
  url: string,
  headers?: Record<string, string>,
  query?: Record<string, string>,
  request_prep_template?: string,
  response_success_template?: string,
  response_error_template?: string,
  system_prompt?: string,
  oauth_provider?: string,
  oauth_scopes?: string[],
  skip_unknown_keys?: boolean,
  transform_output?: boolean,
}

export interface IAssistantGPTScript {
  name: string,
  description: string,
  file?: string,
  content: string,
}

export interface IAssistantZapier {
  name: string,
  description: string,
  api_key?: string,
  model?: string,
  max_iterations?: number,
}

export interface IAssistantConfig {
  id?: string;
  name?: string;
  description?: string;
  avatar?: string;
  image?: string;
  provider?: string;
  model?: string;
  conversation_starters?: string[];
  agent_mode?: boolean;
  agent_type?: IAgentType;
  memory?: boolean;
  max_iterations?: number;
  reasoning_model?: string;
  reasoning_model_provider?: string;
  reasoning_model_effort?: string;
  generation_model?: string;
  generation_model_provider?: string;
  small_reasoning_model?: string;
  small_reasoning_model_provider?: string;
  small_reasoning_model_effort?: string;
  small_generation_model?: string;
  small_generation_model_provider?: string;
  /**
   * ContextLimit - the number of messages to include in the context for the AI assistant.
   * When set to 1, the AI assistant will only see and remember the most recent message.
   */
  context_limit?: number;
  /**
   * How much to penalize new tokens based on their frequency in the text so far.
   * Increases the model's likelihood to talk about new topics
   * 0 - balanced
   * 2 - less repetitive
   */
  frequency_penalty?: number;  
  /** The maximum number of tokens to generate before stopping. */
  max_tokens?: number;
  /**
   * How much to penalize new tokens based on whether they appear in the text so far.
   * Increases the model's likelihood to talk about new topics
   * 0 - balanced
   * 2 - open minded
   */  
  presence_penalty?: number;
  /** Controls effort on reasoning for reasoning models. It can be set to "low", "medium", or "high". */
  reasoning_effort?: string;
  /**
   * Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
   * 0.01 - precise
   * 1 - neutral
   * 2 - creative
   */
  temperature?: number;
  /**
   * An alternative to sampling with temperature, called nucleus sampling,
   * where the model considers the results of the tokens with top_p probability mass.
   * So 0.1 means only the tokens comprising the top 10% probability mass are considered.
   * 0 - balanced
   * 2 - more creative
   */
  top_p?: number;
  
  type?: ISessionType;
  system_prompt?: string;
  rag_source_id?: string;
  lora_id?: string;
  is_actionable_template?: string;
  is_actionable_history_length?: number;
  apis?: IAssistantApi[];
  mcps?: TypesAssistantMCP[];
  gptscripts?: IAssistantGPTScript[];
  zapier?: IAssistantZapier[];
  browser?: TypesAssistantBrowser;
  web_search?: TypesAssistantWebSearch;
  calculator?: TypesAssistantCalculator;
  email?: TypesAssistantEmail;
  azure_devops?: TypesAssistantAzureDevOps;
  tools?: ITool[];
  knowledge?: IKnowledgeSource[];
  tests?: ITest[];
}

export interface IKnowledgeProgress {
  step: string;
  progress: number;
  elapsed_seconds: number;
  message?: string;
  started_at?: Date;
}

export interface IKnowledgeSource {
  id: string;
  name: string;
  version: string;
  description?: string;
  rag_settings: {
    results_count: number;
    chunk_size: number;
    chunk_overflow: number;
    enable_vision: boolean;
  };
  state: string;
  message?: string;
  progress?: IKnowledgeProgress;
  crawled_sources?: ICrawledSources;
  source: {
    helix_drive?: {
      path: string;
    };
    s3?: {
      bucket: string;
      path: string;
    };
    gcs?: {
      bucket: string;
      path: string;
    };
    filestore?: {
      path: string;
    };
    web?: {
      urls?: string[];
      excludes?: string[];
      auth?: {
        username: string;
        password: string;
      };
      crawler?: {
        firecrawl?: {
          api_key: string;
          api_url: string;
        };
        enabled: boolean;
        max_depth?: number;
        max_pages?: number;
        user_agent?: string;
        readability?: boolean;
      };
    };
    text?: string;
  };
  refresh_enabled?: boolean;
  refresh_schedule?: string;
}

export interface ICrawledURL {
  url: string;
  status_code: number;
  message: string;
  duration_ms: number;
}

export interface ICrawledSources {
  urls: ICrawledURL[];
}

export interface IKnowledgeSearchResult {
  knowledge: IKnowledgeSource;
  results: ISessionRAGResult[];
  duration_ms: number;
}

export interface ISessionRAGResult {
  content: string;
  source: string;
  document_id: string;
  document_group_id: string;
  metadata?: Record<string, string>;
  // Add any other properties that your API returns
}

export interface IAppHelixConfig {
  name: string;
  description: string;
  avatar?: string;
  image?: string;
  assistants?: IAssistantConfig[];
  triggers?: TypesTrigger[];
  external_url: string;
  default_agent_type?: IAgentType;
  external_agent_config?: IExternalAgentConfig;
  // Add any other properties that might be part of the helix config
}

export interface IAppConfig {
  helix: IAppHelixConfig;  
  secrets: Record<string, string>;
  allowed_domains: string[];
}

export interface IApp {
  id: string,
  organization_id?: string,
  config: IAppConfig;
  global: boolean;
  created: Date;
  updated: Date;
  owner: string;
  owner_type: IOwnerType;
  user?: IUser;
}

export interface IAppUpdate {
  id: string;
  config: {
    helix: IAppHelixConfig;
    secrets: Record<string, string>;
    allowed_domains: string[];
  };
  global: boolean;
  owner: string;
  owner_type: IOwnerType;
}

export interface IAppFlatState {
  name?: string
  description?: string
  avatar?: string
  image?: string
  global?: boolean
  secrets?: Record<string, string>
  allowedDomains?: string[]
  system_prompt?: string
  provider?: string
  model?: string
  agent_mode?: boolean
  memory?: boolean
  max_iterations?: number
  reasoning_model?: string
  reasoning_model_provider?: string
  reasoning_model_effort?: string
  generation_model?: string
  generation_model_provider?: string
  small_reasoning_model?: string
  small_reasoning_model_provider?: string
  small_reasoning_model_effort?: string
  small_generation_model?: string
  small_generation_model_provider?: string
  context_limit?: number
  frequency_penalty?: number
  max_tokens?: number
  presence_penalty?: number
  reasoning_effort?: string
  temperature?: number
  top_p?: number
  
  knowledge?: IKnowledgeSource[] // Added knowledge parameter
  is_actionable_template?: string;
  is_actionable_history_length?: number;
  apiTools?: IAssistantApi[]
  zapierTools?: IAssistantZapier[]
  gptscriptTools?: IAssistantGPTScript[]
  mcpTools?: TypesAssistantMCP[]
  browserTool?: TypesAssistantBrowser
  webSearchTool?: TypesAssistantWebSearch
  calculatorTool?: TypesAssistantCalculator
  emailTool?: TypesAssistantEmail
  azureDevOpsTool?: TypesAssistantAzureDevOps
  conversation_starters?: string[];
  triggers?: TypesTrigger[];
  tests?: ITest[];
  default_agent_type?: IAgentType;
  external_agent_config?: IExternalAgentConfig;

  tools?: ITool[]
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
  ragDisableChunking: boolean,
  agentType: IAgentType,
  externalAgentConfig?: IExternalAgentConfig,
}

export interface IHelixModel {
  id: string,
  title: string,
  description: string,
}

export interface IRouterNavigateFunction {
  (name: string, params?: Record<string, any>): void,
}

export interface IFeatureAction {
  title: string,
  color: 'primary' | 'secondary',
  variant: 'text' | 'contained' | 'outlined',
  handler: (navigate: IRouterNavigateFunction) => void,
  id?: string;
}

export interface IFeature {
  title: string,
  description: string,
  image?: string,
  icon?: React.ReactNode,
  disabled?: boolean,
  actions: IFeatureAction[],
}

export type IRequiredApiParameter = 'query' | 'header'

// TODO: use TypesSkillDefinition instead
export interface IAgentSkill {
  name: string;
  description: string;
  icon?: React.ReactNode;
  systemPrompt: string; // Will be used to configure the skill when it's running
  apiSkill: {
    schema: string;        // Schema of the API to be used, only applicable for API tools
    url: string;           // URL of the API to be used, only applicable for API tools
    headers?: Record<string, string>; // Headers to set for HTTP requests
    query?: Record<string, string>;   // Query parameters to set for HTTP requests (will override whatever AI model sets)
    oauth_provider?: string;
    oauth_scopes?: string[];
    skip_unknown_keys?: boolean;
    transform_output?: boolean;
    requiredParameters: Array<{
      name: string;                // Name of the parameter
      description: string;         // Description of the parameter
      type: IRequiredApiParameter; // Type of the parameter (query, header)
      required: boolean;           // Whether the parameter is required      
    }>;
  }
  configurable: boolean; // Whether the skill can be configured by the user    
}

export interface ISessionLearnRequestRAGSettings {
  distance_function: string,
  threshold: number,
  results_count: number,
  chunk_size: number,
  chunk_overflow: number,
  disable_chunking: boolean,
}

export type IMessageContentPartText = {
  type: 'text';
  text: string;
};

export type IMessageContentPartImage = {
  type: 'image_url';
  image_url: {
    url: string; // base64 data URI
    // detail?: 'low' | 'high' | 'auto'; // Optional: for image detail control
  };
};

export type IMessageContentPart = IMessageContentPartText | IMessageContentPartImage;

export interface ISessionLearnRequest {
  type: ISessionType,
  data_entity_id: string,
  rag_enabled: boolean,
  text_finetune_enabled: boolean,
  rag_settings: ISessionLearnRequestRAGSettings,
}


export interface ISessionChatRequest {
  regenerate?: boolean,
  app_id?: string,
  organization_id?: string,
  assistant_id?: string,
  session_id?: string,
  interaction_id?: string,
  stream?: boolean,
  legacy?: boolean,
  type?: ISessionType,
  lora_dir?: string,
  system?: string,
  messages?: TypesMessage[],
  agent_type?: IAgentType,
  external_agent_config?: IExternalAgentConfig,
  tools?: string[],
  provider?: string,
  model?: string,
  rag_source_id?: string,
  lora_id?: string,
}

export interface IDataEntity {
  id: string,
  // TODO: the rest
}

export interface IPageBreadcrumb {
  title: string,
  routeName?: string,
  params?: Record<string, any>,
}

// Add this interface near the top of the file, with other interfaces
export interface IApiOptions {
  snackbar?: boolean;
  errorCapture?: (error: any) => void;
  signal?: AbortSignal;
}

export interface ICreateSecret {
  name: string,
  value: string,
}

export interface ISecret {
  id: string,
  name: string,
  created: string,
  updated: string,
}

export type IProviderEndpointType = 'global' | 'user'

export interface IProviderEndpoint {
  id: string
  created: string
  updated: string
  name: string
  description: string
  models?: string[]
  available_models?: string[] 
  endpoint_type: IProviderEndpointType
  owner: string
  owner_type: IOwnerType
  base_url: string
  api_key: string
  api_key_file?: string
  default: boolean
  billing_enabled?: boolean
  headers?: Record<string, string>
}

// Resource type for access grants
export enum Resource {
  Application = "application",
  Dataset = "dataset",
  ProviderEndpoint = "provider_endpoint",
  Knowledge = "knowledge",
}

// User type used in access grants
export interface IUser {
  id: string;
  created_at: string;
  updated_at: string;
  token_type?: string;
  email?: string;
  username?: string;
  full_name?: string;
  avatar?: string;
}

// Role type used in access grants
export interface IRole {
  id: string;
  created_at: string;
  updated_at: string;
  organization_id: string;
  name: string;
  description: string;
}

// Access grant for apps to users or teams
export interface IAccessGrant {
  id: string;
  created_at: string;
  updated_at: string;
  resource_type: Resource;
  resource_id: string;
  organization_id: string;
  team_id?: string;
  user_id?: string;
  user?: IUser;
  roles?: IRole[];
}

// Request to create a new access grant
export interface CreateAccessGrantRequest {
  user_reference?: string; // User ID or email
  team_id?: string;        // Team ID
  roles: string[];         // Role names
}

export interface IUserAppAccessState {
  loading: boolean
  error: string | null
  access: TypesUserAppAccessResponse | null
  refresh: () => Promise<void>
  isAdmin: boolean
  canWrite: boolean
  canRead: boolean
}

// Test interfaces
export interface ITestStep {
  prompt?: string;
  expected_output?: string;
}

export interface ITest {
  name?: string;
  steps?: ITestStep[];
}

// Model substitution information returned from app creation
export interface IModelSubstitution {
  assistant_name: string
  original_provider: string
  original_model: string
  new_provider: string
  new_model: string
  reason: string
}

// Extended app creation response that includes substitution info
export interface IAppCreateResponse {
  id: string,
  organization_id?: string,
  config: IAppConfig;
  global: boolean;
  created: Date;
  updated: Date;
  owner: string;
  owner_type: IOwnerType;
  model_substitutions?: IModelSubstitution[]
}

export interface IWallet {
  id?: string;
  balance?: number;
  created_at?: string;
  updated_at?: string;
  stripe_customer_id?: string;
  stripe_subscription_id?: string;
  subscription_status?: string;
  subscription_current_period_start?: number;
  subscription_current_period_end?: number;
  subscription_created?: number;
  user_id?: string;
  org_id?: string;
}
