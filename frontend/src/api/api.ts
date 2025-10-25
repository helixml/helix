/* eslint-disable */
/* tslint:disable */
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

export interface ControllerMemoryEstimationResponse {
  cached?: boolean;
  error?: string;
  estimate?: MemoryMemoryEstimate;
  model_id?: string;
}

export interface FilestoreConfig {
  folders?: FilestoreFolder[];
  /**
   * this will be the virtual path from the storage instance
   * to the users root directory
   * we use this to strip the full paths in the frontend so we can deal with only relative paths
   */
  user_prefix?: string;
}

export interface FilestoreFolder {
  name?: string;
  readonly?: boolean;
}

export interface FilestoreItem {
  /** timestamp */
  created?: number;
  /** is this thing a folder or not? */
  directory?: boolean;
  /** the filename */
  name?: string;
  /** the relative path to the file from the base path of the storage instance */
  path?: string;
  /** bytes */
  size?: number;
  /** the URL that can be used to load the object directly */
  url?: string;
}

export interface GithubComHelixmlHelixApiPkgTypesConfig {
  rules?: TypesRule[];
}

export interface GormDeletedAt {
  time?: string;
  /** Valid is true if Time is not NULL */
  valid?: boolean;
}

export interface McpMeta {
  /**
   * AdditionalFields are any fields present in the Meta that are not
   * otherwise defined in the protocol.
   */
  additionalFields?: Record<string, any>;
  /**
   * If specified, the caller is requesting out-of-band progress
   * notifications for this request (as represented by
   * notifications/progress). The value of this parameter is an
   * opaque token that will be attached to any subsequent
   * notifications. The receiver is not obligated to provide these
   * notifications.
   */
  progressToken?: any;
}

export interface McpTool {
  /** Meta is a metadata object that is reserved by MCP for storing additional information. */
  _meta?: McpMeta;
  /** Optional properties describing tool behavior */
  annotations?: McpToolAnnotation;
  /** A human-readable description of the tool. */
  description?: string;
  /** A JSON Schema object defining the expected parameters for the tool. */
  inputSchema?: McpToolInputSchema;
  /** The name of the tool. */
  name?: string;
}

export interface McpToolAnnotation {
  /** If true, the tool may perform destructive updates */
  destructiveHint?: boolean;
  /** If true, repeated calls with same args have no additional effect */
  idempotentHint?: boolean;
  /** If true, tool interacts with external entities */
  openWorldHint?: boolean;
  /** If true, the tool does not modify its environment */
  readOnlyHint?: boolean;
  /** Human-readable title for the tool */
  title?: string;
}

export interface McpToolInputSchema {
  $defs?: Record<string, any>;
  properties?: Record<string, any>;
  required?: string[];
  type?: string;
}

export interface MemoryEstimateOptions {
  /** Advanced options */
  flash_attention?: boolean;
  /** "f16", "q8_0", "q4_0" */
  kv_cache_type?: string;
  /** Batch size */
  num_batch?: number;
  /** Context size */
  num_ctx?: number;
  /**
   * ⚠️  CRITICAL CONFUSION WARNING ⚠️
   * NumGPU is NOT the number of GPUs in your hardware configuration!
   * NumGPU is the number of MODEL LAYERS to offload to GPU (-1 for auto-detect all that fit)
   *
   * Examples:
   * - NumGPU = -1: Auto-detect max layers that fit (RECOMMENDED - gives full model memory)
   * - NumGPU = 1:  Only offload 1 layer to GPU (gives tiny memory estimate)
   * - NumGPU = 0:  CPU only (no GPU layers)
   *
   * To estimate for different GPU hardware configs (1 GPU vs 4 GPUs),
   * you pass different GPU configuration arrays to the estimation function,
   * NOT different NumGPU values!
   */
  num_gpu?: number;
  /** Number of parallel sequences */
  num_parallel?: number;
}

export interface MemoryGPUInfo {
  compute?: string;
  dependency_path?: string[];
  driver_major?: number;
  driver_minor?: number;
  env_workarounds?: string[][];
  free_memory?: number;
  id?: string;
  index?: number;
  /** "cuda", "rocm", "metal", "cpu" */
  library?: string;
  minimum_memory?: number;
  name?: string;
  total_memory?: number;
  unreliable_free_memory?: boolean;
  /** Additional fields for compatibility with Ollama's estimation */
  variant?: string;
}

export interface MemoryMemoryEstimate {
  /** Metadata */
  architecture?: string;
  estimated_at?: string;
  /** Whether all layers fit on GPU */
  fully_loaded?: boolean;
  /** Memory allocation per GPU */
  gpu_sizes?: number[];
  gpus?: MemoryGPUInfo[];
  /** Graph memory requirement */
  graph?: number;
  /** Graph computation memory */
  graph_mem?: number;
  /** Breakdown for analysis */
  kv_cache?: number;
  /** Core results */
  layers?: number;
  model_path?: string;
  /** Configuration used for estimation */
  options?: MemoryEstimateOptions;
  /** Projector weights (for multimodal) */
  projectors?: number;
  /** Whether CPU fallback is needed */
  requires_fallback?: boolean;
  /** Layers per GPU for tensor parallel */
  tensor_split?: number[];
  /** Total memory requirement */
  total_size?: number;
  /** Total VRAM usage */
  vram_size?: number;
  /** Model weights memory */
  weights?: number;
}

export interface OpenaiChatCompletionChoice {
  content_filter_results?: OpenaiContentFilterResults;
  /**
   * FinishReason
   * stop: API returned complete message,
   * or a message terminated by one of the stop sequences provided via the stop parameter
   * length: Incomplete model output due to max_tokens parameter or token limit
   * function_call: The model decided to call a function
   * content_filter: Omitted content due to a flag from our content filters
   * null: API response still in progress or incomplete
   */
  finish_reason?: OpenaiFinishReason;
  index?: number;
  logprobs?: OpenaiLogProbs;
  message?: OpenaiChatCompletionMessage;
}

export interface OpenaiChatCompletionMessage {
  content?: string;
  function_call?: OpenaiFunctionCall;
  multiContent?: OpenaiChatMessagePart[];
  /**
   * This property isn't in the official documentation, but it's in
   * the documentation for the official library for python:
   * - https://github.com/openai/openai-python/blob/main/chatml.md
   * - https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
   */
  name?: string;
  refusal?: string;
  role?: string;
  /** For Role=tool prompts this should be set to the ID given in the assistant's prior request to call a tool. */
  tool_call_id?: string;
  /** For Role=assistant prompts this may be set to the tool calls generated by the model, such as function calls. */
  tool_calls?: OpenaiToolCall[];
}

export interface OpenaiChatCompletionRequest {
  frequency_penalty?: number;
  /** Deprecated: use ToolChoice instead. */
  function_call?: any;
  /** Deprecated: use Tools instead. */
  functions?: OpenaiFunctionDefinition[];
  /**
   * LogitBias is must be a token id string (specified by their token ID in the tokenizer), not a word string.
   * incorrect: `"logit_bias":{"You": 6}`, correct: `"logit_bias":{"1639": 6}`
   * refs: https://platform.openai.com/docs/api-reference/chat/create#chat/create-logit_bias
   */
  logit_bias?: Record<string, number>;
  /**
   * LogProbs indicates whether to return log probabilities of the output tokens or not.
   * If true, returns the log probabilities of each output token returned in the content of message.
   * This option is currently not available on the gpt-4-vision-preview model.
   */
  logprobs?: boolean;
  /**
   * MaxCompletionTokens An upper bound for the number of tokens that can be generated for a completion,
   * including visible output tokens and reasoning tokens https://platform.openai.com/docs/guides/reasoning
   */
  max_completion_tokens?: number;
  /**
   * MaxTokens The maximum number of tokens that can be generated in the chat completion.
   * This value can be used to control costs for text generated via API.
   * This value is now deprecated in favor of max_completion_tokens, and is not compatible with o1 series models.
   * refs: https://platform.openai.com/docs/api-reference/chat/create#chat-create-max_tokens
   */
  max_tokens?: number;
  messages?: OpenaiChatCompletionMessage[];
  /** Metadata to store with the completion. */
  metadata?: Record<string, string>;
  model?: string;
  n?: number;
  /** Disable the default behavior of parallel tool calls by setting it: false. */
  parallel_tool_calls?: any;
  presence_penalty?: number;
  /** Controls effort on reasoning for reasoning models. It can be set to "low", "medium", or "high". */
  reasoning_effort?: string;
  response_format?: OpenaiChatCompletionResponseFormat;
  seed?: number;
  stop?: string[];
  /**
   * Store can be set to true to store the output of this completion request for use in distillations and evals.
   * https://platform.openai.com/docs/api-reference/chat/create#chat-create-store
   */
  store?: boolean;
  stream?: boolean;
  /** Options for streaming response. Only set this when you set stream: true. */
  stream_options?: OpenaiStreamOptions;
  temperature?: number;
  /** This can be either a string or an ToolChoice object. */
  tool_choice?: any;
  tools?: OpenaiTool[];
  /**
   * TopLogProbs is an integer between 0 and 5 specifying the number of most likely tokens to return at each
   * token position, each with an associated log probability.
   * logprobs must be set to true if this parameter is used.
   */
  top_logprobs?: number;
  top_p?: number;
  user?: string;
}

export interface OpenaiChatCompletionResponse {
  choices?: OpenaiChatCompletionChoice[];
  created?: number;
  id?: string;
  model?: string;
  object?: string;
  prompt_filter_results?: OpenaiPromptFilterResult[];
  system_fingerprint?: string;
  usage?: OpenaiUsage;
}

export interface OpenaiChatCompletionResponseFormat {
  json_schema?: OpenaiChatCompletionResponseFormatJSONSchema;
  type?: OpenaiChatCompletionResponseFormatType;
}

export interface OpenaiChatCompletionResponseFormatJSONSchema {
  description?: string;
  name?: string;
  schema?: any;
  strict?: boolean;
}

export enum OpenaiChatCompletionResponseFormatType {
  ChatCompletionResponseFormatTypeJSONObject = "json_object",
  ChatCompletionResponseFormatTypeJSONSchema = "json_schema",
  ChatCompletionResponseFormatTypeText = "text",
}

export interface OpenaiChatMessageImageURL {
  detail?: OpenaiImageURLDetail;
  url?: string;
}

export interface OpenaiChatMessagePart {
  image_url?: OpenaiChatMessageImageURL;
  text?: string;
  type?: OpenaiChatMessagePartType;
}

export enum OpenaiChatMessagePartType {
  ChatMessagePartTypeText = "text",
  ChatMessagePartTypeImageURL = "image_url",
}

export interface OpenaiCompletionTokensDetails {
  audio_tokens?: number;
  reasoning_tokens?: number;
}

export interface OpenaiContentFilterResults {
  hate?: OpenaiHate;
  jailbreak?: OpenaiJailBreak;
  profanity?: OpenaiProfanity;
  self_harm?: OpenaiSelfHarm;
  sexual?: OpenaiSexual;
  violence?: OpenaiViolence;
}

export enum OpenaiFinishReason {
  FinishReasonStop = "stop",
  FinishReasonLength = "length",
  FinishReasonFunctionCall = "function_call",
  FinishReasonToolCalls = "tool_calls",
  FinishReasonContentFilter = "content_filter",
  FinishReasonNull = "null",
}

export interface OpenaiFunctionCall {
  /** call function with arguments in JSON format */
  arguments?: string;
  name?: string;
}

export interface OpenaiFunctionDefinition {
  description?: string;
  name?: string;
  /**
   * Parameters is an object describing the function.
   * You can pass json.RawMessage to describe the schema,
   * or you can pass in a struct which serializes to the proper JSON schema.
   * The jsonschema package is provided for convenience, but you should
   * consider another specialized library if you require more complex schemas.
   */
  parameters?: any;
  strict?: boolean;
}

export interface OpenaiHate {
  filtered?: boolean;
  severity?: string;
}

export enum OpenaiImageURLDetail {
  ImageURLDetailHigh = "high",
  ImageURLDetailLow = "low",
  ImageURLDetailAuto = "auto",
}

export interface OpenaiJailBreak {
  detected?: boolean;
  filtered?: boolean;
}

export interface OpenaiLogProb {
  /** Omitting the field if it is null */
  bytes?: number[];
  logprob?: number;
  token?: string;
  /**
   * TopLogProbs is a list of the most likely tokens and their log probability, at this token position.
   * In rare cases, there may be fewer than the number of requested top_logprobs returned.
   */
  top_logprobs?: OpenaiTopLogProbs[];
}

export interface OpenaiLogProbs {
  /** Content is a list of message content tokens with log probability information. */
  content?: OpenaiLogProb[];
}

export interface OpenaiProfanity {
  detected?: boolean;
  filtered?: boolean;
}

export interface OpenaiPromptFilterResult {
  content_filter_results?: OpenaiContentFilterResults;
  index?: number;
}

export interface OpenaiPromptTokensDetails {
  audio_tokens?: number;
  cached_tokens?: number;
}

export interface OpenaiSelfHarm {
  filtered?: boolean;
  severity?: string;
}

export interface OpenaiSexual {
  filtered?: boolean;
  severity?: string;
}

export interface OpenaiStreamOptions {
  /**
   * If set, an additional chunk will be streamed before the data: [DONE] message.
   * The usage field on this chunk shows the token usage statistics for the entire request,
   * and the choices field will always be an empty array.
   * All other chunks will also include a usage field, but with a null value.
   */
  include_usage?: boolean;
}

export interface OpenaiTool {
  function?: OpenaiFunctionDefinition;
  type?: OpenaiToolType;
}

export interface OpenaiToolCall {
  function?: OpenaiFunctionCall;
  id?: string;
  /** Index is not nil only in chat completion chunk object */
  index?: number;
  type?: OpenaiToolType;
}

export enum OpenaiToolType {
  ToolTypeFunction = "function",
}

export interface OpenaiTopLogProbs {
  bytes?: number[];
  logprob?: number;
  token?: string;
}

export interface OpenaiUsage {
  completion_tokens?: number;
  completion_tokens_details?: OpenaiCompletionTokensDetails;
  prompt_tokens?: number;
  prompt_tokens_details?: OpenaiPromptTokensDetails;
  total_tokens?: number;
}

export interface OpenaiViolence {
  filtered?: boolean;
  severity?: string;
}

export interface ServerAgentProgressItem {
  agent_id?: string;
  current_task?: ServerTaskItemDTO;
  last_update?: string;
  phase?: string;
  task_id?: string;
  task_name?: string;
  tasks_after?: ServerTaskItemDTO[];
  tasks_before?: ServerTaskItemDTO[];
}

export interface ServerAgentSandboxesDebugResponse {
  /** Apps mode */
  apps?: ServerWolfAppInfo[];
  /** Lobbies mode */
  lobbies?: ServerWolfLobbyInfo[];
  memory?: ServerWolfSystemMemory;
  /** NEW: moonlight-web client connections */
  moonlight_clients?: ServerMoonlightClientInfo[];
  sessions?: ServerWolfSessionInfo[];
  /** Current Wolf mode ("apps" or "lobbies") */
  wolf_mode?: string;
}

export interface ServerAppCreateResponse {
  config?: TypesAppConfig;
  created?: string;
  global?: boolean;
  id?: string;
  model_substitutions?: ServerModelSubstitution[];
  organization_id?: string;
  /** uuid of user ID */
  owner?: string;
  /** e.g. user, system, org */
  owner_type?: TypesOwnerType;
  updated?: string;
  /** Owner user struct, populated by the server for organization views */
  user?: TypesUser;
}

export interface ServerApprovalWithHandoffRequest {
  approved?: boolean;
  changes?: string[];
  comments?: string;
  create_pull_request?: boolean;
  handoff_config?: ServicesDocumentHandoffConfig;
  project_path?: string;
}

export interface ServerCloneCommandResponse {
  clone_command?: string;
  clone_url?: string;
  repository_id?: string;
  target_dir?: string;
}

export interface ServerCombinedApprovalHandoffResult {
  approval?: TypesSpecApprovalResponse;
  handoff_result?: ServicesHandoffResult;
  message?: string;
  next_steps?: string[];
  spec_task?: TypesSpecTask;
  success?: boolean;
}

export interface ServerCoordinationLogResponse {
  active_sessions?: number;
  completed_sessions?: number;
  events?: ServicesCoordinationEvent[];
  events_by_type?: Record<string, number>;
  filtered_events?: number;
  last_activity?: string;
  spec_task_id?: string;
  total_events?: number;
}

export interface ServerCreatePersonalDevEnvironmentRequest {
  app_id?: string;
  description?: string;
  /** Default: 120 */
  display_fps?: number;
  /** Default: 1640 (iPad Pro) */
  display_height?: number;
  /** Display configuration for the streaming session */
  display_width?: number;
  environment_name?: string;
}

export interface ServerCreateSampleRepositoryRequest {
  description?: string;
  /** Enable Kodit code intelligence indexing */
  kodit_indexing?: boolean;
  name?: string;
  owner_id?: string;
  sample_type?: string;
}

export interface ServerCreateSpecTaskFromDemoRequest {
  demo_repo: string;
  priority?: string;
  prompt: string;
  type?: string;
}

export interface ServerCreateSpecTaskRepositoryRequest {
  description?: string;
  name?: string;
  owner_id?: string;
  project_id?: string;
  spec_task_id?: string;
  template_files?: Record<string, string>;
}

export interface ServerCreateTopUpRequest {
  amount?: number;
  org_id?: string;
}

export interface ServerDesignDocsResponse {
  current_task_index?: number;
  design_markdown?: string;
  progress_markdown?: string;
  task_id?: string;
}

export interface ServerDesignDocsShareLinkResponse {
  expires_at?: string;
  share_url?: string;
  token?: string;
}

export interface ServerForkSampleProjectRequest {
  description?: string;
  private?: boolean;
  project_name?: string;
  sample_project_id?: string;
}

export interface ServerForkSampleProjectResponse {
  github_repo_url?: string;
  message?: string;
  project_id?: string;
}

export interface ServerForkSimpleProjectRequest {
  description?: string;
  project_name?: string;
  sample_project_id?: string;
}

export interface ServerForkSimpleProjectResponse {
  github_repo_url?: string;
  message?: string;
  project_id?: string;
  tasks_created?: number;
}

export interface ServerInitializeSampleRepositoriesRequest {
  owner_id?: string;
  /** If empty, creates all samples */
  sample_types?: string[];
}

export interface ServerInitializeSampleRepositoriesResponse {
  created_count?: number;
  created_repositories?: ServicesGitRepository[];
  errors?: string[];
  success?: boolean;
}

export interface ServerLicenseKeyRequest {
  license_key?: string;
}

export interface ServerLiveAgentFleetProgressResponse {
  agents?: ServerAgentProgressItem[];
  timestamp?: string;
}

export interface ServerLogsSummary {
  active_instances?: number;
  error_retention_hours?: number;
  instances_with_errors?: number;
  max_lines_per_buffer?: number;
  recent_errors?: number;
  slots?: ServerSlotLogSummary[];
}

export interface ServerModelSubstitution {
  assistant_name?: string;
  new_model?: string;
  new_provider?: string;
  original_model?: string;
  original_provider?: string;
  reason?: string;
}

export interface ServerMoonlightClientInfo {
  /** Unique Moonlight client ID (null for browser clients) */
  client_unique_id?: string;
  /** Is a WebRTC client currently connected? */
  has_websocket?: boolean;
  /** "create", "keepalive", "join" */
  mode?: string;
  session_id?: string;
}

export interface ServerPersonalDevEnvironmentResponse {
  /** Helix App ID for configuration (MCP servers, tools, etc.) */
  appID?: string;
  /** MCP servers enabled */
  configured_tools?: string[];
  /** Container information for direct network access */
  container_name?: string;
  createdAt?: string;
  /** Connected data sources */
  data_sources?: string[];
  description?: string;
  /** Streaming framerate */
  display_fps?: number;
  /** Streaming resolution height */
  display_height?: number;
  /** Display configuration for streaming */
  display_width?: number;
  /** User-friendly name */
  environment_name?: string;
  instanceID?: string;
  /** "spec_task", "personal_dev", "shared_workspace" */
  instanceType?: string;
  /** Personal dev environment specific */
  is_personal_env?: boolean;
  lastActivity?: string;
  projectPath?: string;
  /** Optional - null for personal dev environments */
  specTaskID?: string;
  status?: string;
  stream_url?: string;
  threadCount?: number;
  /** Always required */
  userID?: string;
  /** VNC port inside container (5901) */
  vnc_port?: number;
  /** Wolf's numeric session ID for API calls */
  wolf_session_id?: string;
}

export interface ServerPhaseProgress {
  agent?: string;
  completed_at?: string;
  revision_count?: number;
  session_id?: string;
  started_at?: string;
  status?: string;
}

export interface ServerSampleProject {
  /** "web", "api", "mobile", "data", "ai" */
  category?: string;
  default_branch?: string;
  demo_url?: string;
  description?: string;
  /** "beginner", "intermediate", "advanced" */
  difficulty?: string;
  github_repo?: string;
  id?: string;
  name?: string;
  readme_url?: string;
  sample_tasks?: ServerSampleProjectTask[];
  technologies?: string[];
}

export interface ServerSampleProjectTask {
  acceptance_criteria?: string[];
  description?: string;
  estimated_hours?: number;
  files_to_modify?: string[];
  labels?: string[];
  /** "low", "medium", "high", "critical" */
  priority?: string;
  /** "backlog", "ready", "in_progress", "review", "done" */
  status?: string;
  technical_notes?: string;
  title?: string;
  /** "feature", "bug", "task", "epic" */
  type?: string;
}

export interface ServerSampleTaskPrompt {
  /** Any specific constraints or requirements */
  constraints?: string;
  /** Additional context about the codebase */
  context?: string;
  /** Tags for organization */
  labels?: string[];
  /** "low", "medium", "high", "critical" */
  priority?: string;
  /** Natural language request */
  prompt?: string;
}

export interface ServerSampleType {
  description?: string;
  id?: string;
  name?: string;
  tech_stack?: string[];
}

export interface ServerSampleTypesResponse {
  count?: number;
  sample_types?: ServerSampleType[];
}

export interface ServerSessionHistoryEntry {
  activity_type?: string;
  content?: string;
  files_affected?: string[];
  timestamp?: string;
}

export interface ServerSessionHistoryResponse {
  activity_type?: string;
  entries?: ServerSessionHistoryEntry[];
  git_branch?: string;
  last_commit?: string;
  limit?: number;
  spec_task_id?: string;
  total_entries?: number;
  work_session_id?: string;
}

export interface ServerSessionWolfAppStateResponse {
  /** Unique Moonlight client ID for this agent */
  client_unique_id?: string;
  /** Is a browser client currently connected? */
  has_websocket?: boolean;
  session_id?: string;
  /** "absent", "running", "resumable" */
  state?: string;
  wolf_app_id?: string;
}

export interface ServerSimpleSampleProject {
  category?: string;
  default_branch?: string;
  demo_url?: string;
  description?: string;
  difficulty?: string;
  github_repo?: string;
  id?: string;
  name?: string;
  readme_url?: string;
  task_prompts?: ServerSampleTaskPrompt[];
  technologies?: string[];
}

export interface ServerSlotLogSummary {
  has_logs?: boolean;
  id?: string;
  model?: string;
  runner_id?: string;
}

export interface ServerSpecDocumentContentResponse {
  content?: string;
  content_type?: string;
  document_name?: string;
  filename?: string;
  last_modified?: string;
  size?: number;
  spec_task_id?: string;
}

export interface ServerSpecTaskExternalAgentStatusResponse {
  exists?: boolean;
  external_agent_id?: string;
  helix_session_ids?: string[];
  idle_minutes?: number;
  session_count?: number;
  status?: string;
  warning_threshold?: boolean;
  will_terminate_in?: number;
  wolf_app_id?: string;
  workspace_dir?: string;
}

export interface ServerTaskItemDTO {
  description?: string;
  index?: number;
  status?: string;
}

export interface ServerTaskProgressResponse {
  created_at?: string;
  implementation?: ServerPhaseProgress;
  specification?: ServerPhaseProgress;
  status?: string;
  task_id?: string;
  updated_at?: string;
}

export interface ServerTaskSpecsResponse {
  implementation_plan?: string;
  original_prompt?: string;
  requirements_spec?: string;
  spec_approved_at?: string;
  spec_approved_by?: string;
  spec_revision_count?: number;
  status?: string;
  task_id?: string;
  technical_design?: string;
}

export interface ServerWolfAppInfo {
  id?: string;
  title?: string;
}

export interface ServerWolfAppMemory {
  app_id?: string;
  app_name?: string;
  client_count?: number;
  memory_bytes?: number;
  resolution?: string;
}

export interface ServerWolfClientConnection {
  /** apps mode: connected app */
  app_id?: string;
  client_ip?: string;
  /** lobbies mode: connected lobby */
  lobby_id?: string;
  memory_bytes?: number;
  resolution?: string;
  /** Wolf returns this as string (Moonlight protocol requirement) */
  session_id?: string;
}

export interface ServerWolfLobbyInfo {
  id?: string;
  multi_user?: boolean;
  name?: string;
  pin?: number[];
  started_by_profile_id?: string;
  stop_when_everyone_leaves?: boolean;
}

export interface ServerWolfLobbyMemory {
  client_count?: string;
  lobby_id?: string;
  lobby_name?: string;
  memory_bytes?: number;
  resolution?: string;
}

export interface ServerWolfSessionInfo {
  app_id?: string;
  client_ip?: string;
  display_mode?: {
    av1_supported?: boolean;
    height?: number;
    hevc_supported?: boolean;
    refresh_rate?: number;
    width?: number;
  };
  /** Exposed as session_id for frontend */
  session_id?: string;
}

export interface ServerWolfSystemMemory {
  /** Apps mode */
  apps?: ServerWolfAppMemory[];
  clients?: ServerWolfClientConnection[];
  gstreamer_buffer_bytes?: number;
  /** Lobbies mode */
  lobbies?: ServerWolfLobbyMemory[];
  process_rss_bytes?: number;
  success?: boolean;
  total_memory_bytes?: number;
}

export interface ServicesCoordinationEvent {
  acknowledged?: boolean;
  acknowledged_at?: string;
  data?: Record<string, any>;
  event_type?: ServicesCoordinationEventType;
  from_session_id?: string;
  id?: string;
  message?: string;
  response?: string;
  timestamp?: string;
  /** Empty for broadcast */
  to_session_id?: string;
}

export enum ServicesCoordinationEventType {
  CoordinationEventTypeHandoff = "handoff",
  CoordinationEventTypeBlocking = "blocking",
  CoordinationEventTypeNotification = "notification",
  CoordinationEventTypeRequest = "request",
  CoordinationEventTypeResponse = "response",
  CoordinationEventTypeBroadcast = "broadcast",
  CoordinationEventTypeCompletion = "completion",
  CoordinationEventTypeSpawn = "spawn",
}

export interface ServicesCreateTaskRequest {
  priority?: string;
  project_id?: string;
  prompt?: string;
  type?: string;
  user_id?: string;
}

export interface ServicesDocumentHandoffConfig {
  auto_merge_approved_specs?: boolean;
  commit_frequency_minutes?: number;
  custom_git_hooks?: Record<string, string>;
  enable_git_integration?: boolean;
  enable_pull_requests?: boolean;
  enable_session_recording?: boolean;
  notification_webhooks?: string[];
  require_code_review?: boolean;
  spec_reviewers?: string[];
}

export interface ServicesDocumentHandoffStatus {
  current_phase?: string;
  documents_generated?: boolean;
  git_integration_status?: string;
  last_commit_hash?: string;
  last_commit_time?: string;
  session_recording_active?: boolean;
  spec_task_id?: string;
}

export interface ServicesGitRepository {
  branches?: string[];
  clone_url?: string;
  created_at?: string;
  default_branch?: string;
  description?: string;
  id?: string;
  last_activity?: string;
  local_path?: string;
  metadata?: Record<string, any>;
  name?: string;
  owner_id?: string;
  project_id?: string;
  repo_type?: ServicesGitRepositoryType;
  spec_task_id?: string;
  status?: ServicesGitRepositoryStatus;
  updated_at?: string;
}

export interface ServicesGitRepositoryCreateRequest {
  default_branch?: string;
  description?: string;
  initial_files?: Record<string, string>;
  metadata?: Record<string, any>;
  name?: string;
  owner_id?: string;
  project_id?: string;
  repo_type?: ServicesGitRepositoryType;
  spec_task_id?: string;
}

export enum ServicesGitRepositoryStatus {
  GitRepositoryStatusActive = "active",
  GitRepositoryStatusArchived = "archived",
  GitRepositoryStatusDeleted = "deleted",
}

export enum ServicesGitRepositoryType {
  GitRepositoryTypeProject = "project",
  GitRepositoryTypeSpecTask = "spec_task",
  GitRepositoryTypeSample = "sample",
  GitRepositoryTypeTemplate = "template",
}

export interface ServicesHandoffResult {
  branch_name?: string;
  estimated_completion?: string;
  files_committed?: string[];
  git_commit_hash?: string;
  handoff_timestamp?: string;
  message?: string;
  metadata?: Record<string, any>;
  next_actions?: string[];
  notifications_sent?: string[];
  /** "spec_commit", "implementation_start", "progress_update", "completion" */
  phase?: string;
  pull_request_url?: string;
  spec_task_id?: string;
  success?: boolean;
  warnings?: string[];
  work_sessions_created?: string[];
  zed_instance_id?: string;
}

export interface ServicesSampleProjectCode {
  description?: string;
  /** filepath -> content */
  files?: Record<string, string>;
  github_repo?: string;
  gitignore?: string;
  id?: string;
  language?: string;
  name?: string;
  readme_url?: string;
  technologies?: string[];
}

export interface ServicesSessionHistoryRecord {
  /** "conversation", "code_change", "decision", "coordination" */
  activity_type?: string;
  code_changes?: Record<string, any>;
  content?: string;
  coordination_data?: Record<string, any>;
  files_affected?: string[];
  helix_session_id?: string;
  metadata?: Record<string, any>;
  timestamp?: string;
  work_session_id?: string;
  zed_thread_id?: string;
}

export interface ServicesSpecDocumentConfig {
  branch_name?: string;
  commit_message?: string;
  create_pull_request?: boolean;
  custom_metadata?: Record<string, string>;
  generate_task_board?: boolean;
  include_timestamps?: boolean;
  overwrite_existing?: boolean;
  project_path?: string;
  reviewers_needed?: string[];
  spec_task_id?: string;
}

export interface ServicesSpecDocumentResult {
  branch_name?: string;
  commit_hash?: string;
  files_created?: string[];
  /** filename -> content */
  generated_files?: Record<string, string>;
  message?: string;
  metadata?: Record<string, any>;
  pull_request_url?: string;
  spec_task_id?: string;
  success?: boolean;
  warnings?: string[];
}

export interface ServicesZedSessionCreationResult {
  /** "spawned", "planned", "ad_hoc" */
  creation_method?: string;
  helix_session?: TypesSession;
  message?: string;
  parent_work_session?: TypesSpecTaskWorkSession;
  spec_task?: TypesSpecTask;
  success?: boolean;
  warnings?: string[];
  work_session?: TypesSpecTaskWorkSession;
  zed_thread?: TypesSpecTaskZedThread;
}

export interface ServicesZedThreadCreationContext {
  agent_configuration?: Record<string, any>;
  environment_variables?: string[];
  estimated_duration_hours?: number;
  expected_work_type?: string;
  initial_prompt?: string;
  parent_zed_thread_id?: string;
  project_path?: string;
  related_implementation_task?: number;
  spawn_reason?: string;
  spec_task_id?: string;
  thread_description?: string;
  thread_name?: string;
  user_id?: string;
  zed_instance_id?: string;
  zed_thread_id?: string;
}

export interface SqlNullString {
  string?: string;
  /** Valid is true if String is not NULL */
  valid?: boolean;
}

export enum StripeSubscriptionStatus {
  SubscriptionStatusActive = "active",
  SubscriptionStatusCanceled = "canceled",
  SubscriptionStatusIncomplete = "incomplete",
  SubscriptionStatusIncompleteExpired = "incomplete_expired",
  SubscriptionStatusPastDue = "past_due",
  SubscriptionStatusPaused = "paused",
  SubscriptionStatusTrialing = "trialing",
  SubscriptionStatusUnpaid = "unpaid",
}

export interface SystemHTTPError {
  message?: string;
  statusCode?: number;
}

export interface TypesAPIError {
  code?: any;
  message?: string;
  param?: string;
  type?: string;
}

export enum TypesAPIKeyType {
  APIkeytypeNone = "",
  APIkeytypeAPI = "api",
  APIkeytypeApp = "app",
}

export interface TypesAccessGrant {
  created_at?: string;
  id?: string;
  /** If granted to an organization */
  organization_id?: string;
  /** App ID, Knowledge ID, etc */
  resource_id?: string;
  roles?: TypesRole[];
  /** If granted to a team */
  team_id?: string;
  updated_at?: string;
  /** Populated by the server if UserID is set */
  user?: TypesUser;
  /** If granted to a user */
  user_id?: string;
}

export enum TypesAction {
  ActionGet = "Get",
  ActionList = "List",
  ActionDelete = "Delete",
  ActionUpdate = "Update",
  ActionCreate = "Create",
  ActionUseAction = "UseAction",
}

export interface TypesAddOrganizationMemberRequest {
  role?: TypesOrganizationRole;
  /** Either user ID or user email */
  user_reference?: string;
}

export interface TypesAddTeamMemberRequest {
  /** Either user ID or user email */
  user_reference?: string;
}

export interface TypesAgentDashboardSummary {
  active_help_requests?: TypesHelpRequest[];
  active_sessions?: TypesAgentSessionStatus[];
  global_allocation_decisions?: TypesGlobalAllocationDecision[];
  last_updated?: string;
  pending_reviews?: TypesJobCompletion[];
  pending_work?: TypesAgentWorkItem[];
  queue?: TypesWorkloadSummary[];
  recent_completions?: TypesJobCompletion[];
  runners?: TypesDashboardRunner[];
  running_work?: TypesAgentWorkItem[];
  scheduling_decisions?: TypesSchedulingDecision[];
  sessions_needing_help?: TypesAgentSessionStatus[];
  work_queue_stats?: TypesAgentWorkQueueStats;
}

export interface TypesAgentFleetSummary {
  active_help_requests?: TypesHelpRequest[];
  active_sessions?: TypesAgentSessionStatus[];
  external_agent_runners?: TypesExternalAgentConnection[];
  last_updated?: string;
  pending_reviews?: TypesJobCompletion[];
  pending_work?: TypesAgentWorkItem[];
  recent_completions?: TypesJobCompletion[];
  running_work?: TypesAgentWorkItem[];
  sessions_needing_help?: TypesAgentSessionStatus[];
  work_queue_stats?: TypesAgentWorkQueueStats;
}

export interface TypesAgentSessionStatus {
  agent_type?: string;
  app_id?: string;
  completed_at?: string;
  configuration?: number[];
  container_id?: string;
  created_at?: string;
  /** What the agent is currently doing */
  current_task?: string;
  current_work_item?: string;
  health_checked_at?: string;
  /** "healthy", "unhealthy", "unknown" */
  health_status?: string;
  id?: string;
  last_activity?: string;
  metadata?: number[];
  metrics?: number[];
  organization_id?: string;
  process_id?: number;
  rdp_port?: number;
  session_id?: string;
  state?: number[];
  /** "starting", "active", "waiting_for_help", "paused", "completed", "pending_review", "failed" */
  status?: string;
  updated_at?: string;
  user_id?: string;
  workspace_dir?: string;
}

export interface TypesAgentSessionsResponse {
  page?: number;
  page_size?: number;
  sessions?: TypesAgentSessionStatus[];
  total?: number;
}

export enum TypesAgentType {
  AgentTypeHelixBasic = "helix_basic",
  AgentTypeHelixAgent = "helix_agent",
  AgentTypeZedExternal = "zed_external",
}

export interface TypesAgentWorkConfig {
  /** Agent environment and setup */
  environment?: Record<string, string>;
  /** Source integration settings */
  github?: TypesGitHubWorkConfig;
  /** Custom instructions or context */
  instructions?: string;
  manual?: TypesManualWorkConfig;
  /** Skills to enable for this work type */
  skills?: string[];
  webhook?: TypesWebhookWorkConfig;
  working_dir?: string;
}

export interface TypesAgentWorkItem {
  /** Required agent type */
  agent_type?: string;
  app_id?: string;
  assigned_session_id?: string;
  completed_at?: string;
  /** Agent configuration */
  config?: number[];
  created_at?: string;
  deadline_at?: string;
  description?: string;
  id?: string;
  /** Labels/tags for filtering */
  labels?: number[];
  last_error?: string;
  max_retries?: number;
  metadata?: number[];
  name?: string;
  organization_id?: string;
  /** Lower = higher priority */
  priority?: number;
  retry_count?: number;
  /** When to start this work */
  scheduled_for?: string;
  /** "github", "manual", "webhook", etc. */
  source?: string;
  /** External ID from source system */
  source_id?: string;
  /** URL to source (e.g., GitHub issue URL) */
  source_url?: string;
  started_at?: string;
  /** "pending", "assigned", "in_progress", "completed", "failed", "cancelled" */
  status?: string;
  /** Links to TriggerConfiguration */
  trigger_config_id?: string;
  updated_at?: string;
  user_id?: string;
  /** Work-specific data */
  work_data?: number[];
}

export interface TypesAgentWorkItemCreateRequest {
  agent_type?: string;
  config?: Record<string, any>;
  deadline_at?: string;
  description?: string;
  labels?: string[];
  max_retries?: number;
  name?: string;
  priority?: number;
  scheduled_for?: string;
  source?: string;
  source_id?: string;
  source_url?: string;
  work_data?: Record<string, any>;
}

export interface TypesAgentWorkItemUpdateRequest {
  config?: Record<string, any>;
  description?: string;
  labels?: string[];
  name?: string;
  priority?: number;
  status?: string;
  work_data?: Record<string, any>;
}

export interface TypesAgentWorkItemsResponse {
  items?: TypesAgentWorkItem[];
  page?: number;
  page_size?: number;
  total?: number;
}

export interface TypesAgentWorkQueueStats {
  active_sessions?: number;
  average_wait_time_minutes?: number;
  by_agent_type?: Record<string, number>;
  by_priority?: Record<string, number>;
  by_source?: Record<string, number>;
  oldest_pending?: string;
  total_completed?: number;
  total_failed?: number;
  total_pending?: number;
  total_running?: number;
}

export interface TypesAgentWorkQueueTrigger {
  /** "zed", "helix", etc. */
  agent_type?: string;
  /** Auto-assign to available agents */
  auto_assign?: boolean;
  enabled?: boolean;
  /** Maximum retry attempts */
  max_retries?: number;
  /** Lower numbers = higher priority */
  priority?: number;
  /** Timeout in minutes */
  timeout_mins?: number;
  /** Work item configuration */
  work_config?: TypesAgentWorkConfig;
}

export interface TypesAggregatedUsageMetric {
  completion_cost?: number;
  completion_tokens?: number;
  /** ID    string    `json:"id" gorm:"primaryKey"` */
  date?: string;
  latency_ms?: number;
  prompt_cost?: number;
  prompt_tokens?: number;
  request_size_bytes?: number;
  response_size_bytes?: number;
  /** Total cost of the call (prompt and completion tokens) */
  total_cost?: number;
  total_requests?: number;
  total_tokens?: number;
}

export interface TypesAllocationPlanView {
  cost?: number;
  /** Slot IDs */
  evictions_needed?: string[];
  gpu_count?: number;
  gpus?: number[];
  id?: string;
  is_multi_gpu?: boolean;
  is_valid?: boolean;
  memory_per_gpu?: number;
  requires_eviction?: boolean;
  /** GPU index -> total memory */
  runner_capacity?: Record<string, number>;
  runner_id?: string;
  /** GPU index -> allocated memory */
  runner_memory_state?: Record<string, number>;
  runtime?: TypesRuntime;
  tensor_parallel_size?: number;
  total_memory_required?: number;
  validation_error?: string;
}

export interface TypesApiKey {
  app_id?: SqlNullString;
  created?: string;
  key?: string;
  name?: string;
  owner?: string;
  owner_type?: TypesOwnerType;
  type?: TypesAPIKeyType;
}

export interface TypesApp {
  config?: TypesAppConfig;
  created?: string;
  global?: boolean;
  id?: string;
  organization_id?: string;
  /** uuid of user ID */
  owner?: string;
  /** e.g. user, system, org */
  owner_type?: TypesOwnerType;
  updated?: string;
  /** Owner user struct, populated by the server for organization views */
  user?: TypesUser;
}

export interface TypesAppConfig {
  allowed_domains?: string[];
  helix?: TypesAppHelixConfig;
  secrets?: Record<string, string>;
}

export interface TypesAppHelixConfig {
  assistants?: TypesAssistantConfig[];
  avatar?: string;
  avatar_content_type?: string;
  /** Agent configuration */
  default_agent_type?: TypesAgentType;
  description?: string;
  external_agent_config?: TypesExternalAgentConfig;
  external_agent_enabled?: boolean;
  external_url?: string;
  image?: string;
  name?: string;
  triggers?: TypesTrigger[];
}

export interface TypesAssistantAPI {
  description?: string;
  headers?: Record<string, string>;
  name?: string;
  /** OAuth configuration */
  oauth_provider?: string;
  /** Required OAuth scopes for this API */
  oauth_scopes?: string[];
  path_params?: Record<string, string>;
  query?: Record<string, string>;
  request_prep_template?: string;
  response_error_template?: string;
  response_success_template?: string;
  schema?: string;
  /**
   * if true, unknown keys in the response body will be removed before
   * returning to the agent for interpretation
   */
  skip_unknown_keys?: boolean;
  system_prompt?: string;
  /**
   * Transform JSON into readable text to reduce the
   * size of the response body
   */
  transform_output?: boolean;
  url?: string;
}

export interface TypesAssistantAzureDevOps {
  enabled?: boolean;
  organization_url?: string;
  personal_access_token?: string;
}

export interface TypesAssistantBrowser {
  enabled?: boolean;
  /** If true, the browser will return the HTML as markdown */
  markdown_post_processing?: boolean;
  /** If true, the browser will process the output of the tool call before returning it to the top loop. Useful for skills that return structured data such as Browser, */
  process_output?: boolean;
}

export interface TypesAssistantCalculator {
  enabled?: boolean;
}

export interface TypesAssistantConfig {
  /** AgentMode triggers the use of the agent loop (deprecated - use AgentType instead) */
  agent_mode?: boolean;
  /** AgentType specifies the type of agent to use */
  agent_type?: TypesAgentType;
  apis?: TypesAssistantAPI[];
  avatar?: string;
  azure_devops?: TypesAssistantAzureDevOps;
  browser?: TypesAssistantBrowser;
  calculator?: TypesAssistantCalculator;
  /**
   * ContextLimit - the number of messages to include in the context for the AI assistant.
   * When set to 1, the AI assistant will only see and remember the most recent message.
   */
  context_limit?: number;
  /**
   * ConversationStarters is a list of messages that will be presented to the user
   * when a new session is about to be launched. Use this to showcase the capabilities of the assistant.
   */
  conversation_starters?: string[];
  description?: string;
  email?: TypesAssistantEmail;
  /**
   * How much to penalize new tokens based on their frequency in the text so far.
   * Increases the model's likelihood to talk about new topics
   * 0 - balanced
   * 2 - less repetitive
   */
  frequency_penalty?: number;
  generation_model?: string;
  generation_model_provider?: string;
  id?: string;
  image?: string;
  /** Defaults to 4 */
  is_actionable_history_length?: number;
  is_actionable_template?: string;
  knowledge?: TypesAssistantKnowledge[];
  lora_id?: string;
  max_iterations?: number;
  /** The maximum number of tokens to generate before stopping. */
  max_tokens?: number;
  mcps?: TypesAssistantMCP[];
  /** Enable/disable user based memory for the agent */
  memory?: boolean;
  model?: string;
  name?: string;
  /**
   * How much to penalize new tokens based on whether they appear in the text so far.
   * Increases the model's likelihood to talk about new topics
   * 0 - balanced
   * 2 - open minded
   */
  presence_penalty?: number;
  provider?: string;
  rag_source_id?: string;
  /** Controls effort on reasoning for reasoning models. It can be set to "low", "medium", or "high". */
  reasoning_effort?: string;
  reasoning_model?: string;
  reasoning_model_effort?: string;
  reasoning_model_provider?: string;
  small_generation_model?: string;
  small_generation_model_provider?: string;
  small_reasoning_model?: string;
  small_reasoning_model_effort?: string;
  small_reasoning_model_provider?: string;
  system_prompt?: string;
  /**
   * Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
   * 0.01 - precise
   * 1 - neutral
   * 2 - creative
   */
  temperature?: number;
  tests?: {
    name?: string;
    steps?: TypesTestStep[];
  }[];
  tools?: TypesTool[];
  /**
   * An alternative to sampling with temperature, called nucleus sampling,
   * where the model considers the results of the tokens with top_p probability mass.
   * So 0.1 means only the tokens comprising the top 10% probability mass are considered.
   * 0 - balanced
   * 2 - more creative
   */
  top_p?: number;
  web_search?: TypesAssistantWebSearch;
  zapier?: TypesAssistantZapier[];
}

export interface TypesAssistantEmail {
  enabled?: boolean;
  template_example?: string;
}

export interface TypesAssistantKnowledge {
  /**
   * Description of the knowledge, will be used in the prompt
   * to explain the knowledge to the assistant
   */
  description?: string;
  /** Name of the knowledge, will be unique within the Helix app */
  name?: string;
  /**
   * RAGSettings defines the settings for the RAG system, how
   * chunking is configured and where the index/query service is
   * hosted.
   */
  rag_settings?: TypesRAGSettings;
  /**
   * RefreshEnabled defines if the knowledge should be refreshed periodically
   * or on events. For example a Google Drive knowledge can be refreshed
   * every 24 hours.
   */
  refresh_enabled?: boolean;
  /**
   * RefreshSchedule defines the schedule for refreshing the knowledge.
   * It can be specified in cron format or as a duration for example '@every 2h'
   * or 'every 5m' or '0 0 * * *' for daily at midnight.
   */
  refresh_schedule?: string;
  /**
   * Source defines where the raw data is fetched from. It can be
   * directly uploaded files, S3, GCS, Google Drive, Gmail, etc.
   */
  source?: TypesKnowledgeSource;
}

export interface TypesAssistantMCP {
  description?: string;
  headers?: Record<string, string>;
  name?: string;
  /** The name of the OAuth provider to use for authentication */
  oauth_provider?: string;
  /** Required OAuth scopes for this API */
  oauth_scopes?: string[];
  tools?: McpTool[];
  url?: string;
}

export interface TypesAssistantWebSearch {
  enabled?: boolean;
  max_results?: number;
}

export interface TypesAssistantZapier {
  api_key?: string;
  description?: string;
  max_iterations?: number;
  model?: string;
  name?: string;
}

export interface TypesAuthenticatedResponse {
  authenticated?: boolean;
}

export interface TypesAzureDevOpsTrigger {
  enabled?: boolean;
}

export interface TypesChatCompletionMessage {
  content?: string;
  multiContent?: TypesChatMessagePart[];
  name?: string;
  role?: string;
  /** For Role=tool prompts this should be set to the ID given in the assistant's prior request to call a tool. */
  tool_call_id?: string;
  /** For Role=assistant prompts this may be set to the tool calls generated by the model, such as function calls. */
  tool_calls?: OpenaiToolCall[];
}

export interface TypesChatMessageImageURL {
  detail?: TypesImageURLDetail;
  url?: string;
}

export interface TypesChatMessagePart {
  image_url?: TypesChatMessageImageURL;
  text?: string;
  type?: TypesChatMessagePartType;
}

export enum TypesChatMessagePartType {
  ChatMessagePartTypeText = "text",
  ChatMessagePartTypeImageURL = "image_url",
}

export interface TypesChoice {
  delta?: TypesOpenAIMessage;
  finish_reason?: string;
  index?: number;
  message?: TypesOpenAIMessage;
  text?: string;
}

export interface TypesContextMenuAction {
  /** Forms the grouping in the UI */
  action_label?: string;
  /** The label that will be shown in the UI */
  label?: string;
  /** The value written to the text area when the action is selected */
  value?: string;
}

export interface TypesContextMenuResponse {
  data?: TypesContextMenuAction[];
}

export interface TypesCrawledSources {
  urls?: TypesCrawledURL[];
}

export interface TypesCrawledURL {
  document_id?: string;
  duration_ms?: number;
  message?: string;
  status_code?: number;
  url?: string;
}

export interface TypesCreateAccessGrantRequest {
  /** Role names */
  roles?: string[];
  /** Team ID */
  team_id?: string;
  /** User ID or email */
  user_reference?: string;
}

export interface TypesCreateTeamRequest {
  name?: string;
  organization_id?: string;
}

export interface TypesCrispTrigger {
  enabled?: boolean;
  /** Token identifier */
  identifier?: string;
  /** Optional */
  nickname?: string;
  token?: string;
}

export interface TypesCronTrigger {
  enabled?: boolean;
  input?: string;
  schedule?: string;
}

export interface TypesDashboardData {
  global_allocation_decisions?: TypesGlobalAllocationDecision[];
  queue?: TypesWorkloadSummary[];
  runners?: TypesDashboardRunner[];
  scheduling_decisions?: TypesSchedulingDecision[];
}

export interface TypesDashboardRunner {
  allocated_memory?: number;
  created?: string;
  free_memory?: number;
  /** Number of GPUs detected */
  gpu_count?: number;
  /** GPU memory stabilization statistics */
  gpu_memory_stats?: TypesGPUMemoryStats;
  /** Per-GPU memory status */
  gpus?: TypesGPUStatus[];
  id?: string;
  labels?: Record<string, string>;
  memory_string?: string;
  models?: TypesRunnerModelStatus[];
  /** Process tracking and cleanup statistics */
  process_stats?: any;
  slots?: TypesRunnerSlot[];
  total_memory?: number;
  updated?: string;
  used_memory?: number;
  version?: string;
}

export interface TypesDiscordTrigger {
  server_name?: string;
}

export interface TypesDynamicModelInfo {
  created?: string;
  id?: string;
  model_info?: TypesModelInfo;
  /** Model name */
  name?: string;
  /** helix, openai, etc. (Helix internal information) */
  provider?: string;
  updated?: string;
}

export enum TypesEffect {
  EffectAllow = "allow",
  EffectDeny = "deny",
}

export interface TypesExternalAgentConfig {
  /** Whether to auto-connect RDP viewer */
  auto_connect_rdp?: boolean;
  /** Streaming resolution height (default: 1600) */
  display_height?: number;
  /** Streaming refresh rate (default: 60) */
  display_refresh_rate?: number;
  /** Video settings for streaming (Phase 3.5) - matches PDE display settings */
  display_width?: number;
  /** Environment variables in KEY=VALUE format */
  env_vars?: string[];
  /** Relative path for the project directory */
  project_path?: string;
  /** Custom working directory */
  workspace_dir?: string;
}

export interface TypesExternalAgentConnection {
  connected_at?: string;
  last_ping?: string;
  session_id?: string;
  status?: string;
}

export enum TypesFeedback {
  FeedbackLike = "like",
  FeedbackDislike = "dislike",
}

export interface TypesFeedbackRequest {
  feedback?: TypesFeedback;
  feedback_message?: string;
}

export interface TypesFirecrawl {
  api_key?: string;
  api_url?: string;
}

export interface TypesFlexibleEmbeddingRequest {
  dimensions?: number;
  encoding_format?: string;
  /** Can be string, []string, [][]int, etc. */
  input?: any;
  /** For Chat Embeddings API format */
  messages?: TypesChatCompletionMessage[];
  model?: string;
}

export interface TypesFlexibleEmbeddingResponse {
  data?: {
    embedding?: number[];
    index?: number;
    object?: string;
  }[];
  model?: string;
  object?: string;
  usage?: {
    prompt_tokens?: number;
    total_tokens?: number;
  };
}

export interface TypesFrontendLicenseInfo {
  features?: {
    users?: boolean;
  };
  limits?: {
    machines?: number;
    users?: number;
  };
  organization?: string;
  valid?: boolean;
  valid_until?: string;
}

export interface TypesGPUMemoryDataPoint {
  /** Actual free memory (from nvidia-smi) */
  actual_free_mb?: number;
  /** Total GPU memory */
  actual_total_mb?: number;
  /** Actual memory used (from nvidia-smi) */
  actual_used_mb?: number;
  /** Memory allocated by Helix scheduler */
  allocated_mb?: number;
  gpu_index?: number;
  timestamp?: string;
}

export interface TypesGPUMemoryReading {
  delta_mb?: number;
  is_stable?: boolean;
  memory_mb?: number;
  poll_number?: number;
  stable_count?: number;
}

export interface TypesGPUMemoryStabilizationEvent {
  /** "startup" or "deletion" */
  context?: string;
  error_message?: string;
  memory_delta_threshold_mb?: number;
  memory_readings?: TypesGPUMemoryReading[];
  poll_interval_ms?: number;
  polls_taken?: number;
  required_stable_polls?: number;
  runtime?: string;
  slot_id?: string;
  stabilized_memory_mb?: number;
  success?: boolean;
  timeout_seconds?: number;
  timestamp?: string;
  total_wait_seconds?: number;
}

export interface TypesGPUMemoryStats {
  average_wait_time_seconds?: number;
  failed_stabilizations?: number;
  last_stabilization?: string;
  max_wait_time_seconds?: number;
  /** Last 10 minutes of memory data */
  memory_time_series?: TypesGPUMemoryDataPoint[];
  min_wait_time_seconds?: number;
  /** Last 20 events */
  recent_events?: TypesGPUMemoryStabilizationEvent[];
  /** Last 10 minutes of scheduling events */
  scheduling_events?: TypesSchedulingEvent[];
  successful_stabilizations?: number;
  total_stabilizations?: number;
}

export interface TypesGPUState {
  /** Slot IDs using this GPU */
  active_slots?: string[];
  allocated_memory?: number;
  free_memory?: number;
  index?: number;
  total_memory?: number;
  /** 0.0 - 1.0 */
  utilization?: number;
}

export interface TypesGPUStatus {
  /** CUDA version */
  cuda_version?: string;
  /** NVIDIA driver version */
  driver_version?: string;
  /** Free memory in bytes */
  free_memory?: number;
  /** GPU index (0, 1, 2, etc.) */
  index?: number;
  /** GPU model name (e.g., "NVIDIA H100 PCIe", "NVIDIA GeForce RTX 4090") */
  model_name?: string;
  /** Total memory in bytes */
  total_memory?: number;
  /** Used memory in bytes */
  used_memory?: number;
}

export interface TypesGitHubWorkConfig {
  access_token?: string;
  /** Comment when starting work */
  auto_comment?: boolean;
  enabled?: boolean;
  /** "issue", "pull_request" */
  issue_types?: string[];
  /** Filter by labels */
  labels?: string[];
  repo_name?: string;
  repo_owner?: string;
}

export interface TypesGlobalAllocationDecision {
  after_state?: Record<string, TypesRunnerStateView>;
  /** Global state snapshots */
  before_state?: Record<string, TypesRunnerStateView>;
  /** All plans considered */
  considered_plans?: TypesAllocationPlanView[];
  created?: string;
  error_message?: string;
  execution_time_ms?: number;
  id?: string;
  model_name?: string;
  /** How optimal the final decision was */
  optimization_score?: number;
  /** Timing information */
  planning_time_ms?: number;
  reason?: string;
  runtime?: TypesRuntime;
  selected_plan?: TypesAllocationPlanView;
  session_id?: string;
  /** Decision outcome */
  success?: boolean;
  total_plans_generated?: number;
  /** Decision metadata */
  total_runners_evaluated?: number;
  total_time_ms?: number;
  workload_id?: string;
}

export interface TypesHelpRequest {
  app_id?: string;
  /** What the agent has already tried */
  attempted_solutions?: string;
  /** Brief context about the current task */
  context?: string;
  created_at?: string;
  /** "decision", "expertise", "clarification", "review", "guidance", "stuck", "other" */
  help_type?: string;
  id?: string;
  interaction_id?: string;
  /** Additional metadata as JSON */
  metadata?: number[];
  /** The resolution or guidance provided */
  resolution?: string;
  resolved_at?: string;
  /** UserID of the human who resolved this */
  resolved_by?: string;
  session_id?: string;
  /** Specific description of what help is needed */
  specific_need?: string;
  /** "pending", "in_progress", "resolved", "cancelled" */
  status?: string;
  /** Potential approaches the agent suggests */
  suggested_approaches?: string;
  updated_at?: string;
  /** "low", "medium", "high", "critical" */
  urgency?: string;
  user_id?: string;
}

export interface TypesHelpRequestsListResponse {
  help_requests?: TypesHelpRequest[];
  page?: number;
  page_size?: number;
  total?: number;
}

export enum TypesImageURLDetail {
  ImageURLDetailHigh = "high",
  ImageURLDetailLow = "low",
  ImageURLDetailAuto = "auto",
}

export interface TypesInteraction {
  app_id?: string;
  completed?: string;
  created?: string;
  /** if this is defined, the UI will always display it instead of the message (so we can augment the internal prompt with RAG context) */
  display_message?: string;
  /** How long the interaction took to complete in milliseconds */
  duration_ms?: number;
  error?: string;
  feedback?: TypesFeedback;
  feedback_message?: string;
  /**
   * GenerationID, starts at 0, increments for each regeneration (when user retries a message, anywhere from the past)
   * it is used to keep a timeline when querying the database for messages or viewing previous generations
   */
  generation_id?: number;
  id?: string;
  mode?: TypesSessionMode;
  /** User prompt (text) */
  prompt_message?: string;
  /** User prompt (multi-part) */
  prompt_message_content?: TypesMessageContent;
  rag_results?: TypesSessionRAGResult[];
  /** e.g. json */
  response_format?: TypesResponseFormat;
  /** e.g. json */
  response_format_response?: string;
  /**
   * TODO: add the full multi-part response content
   * ResponseMessageContent MessageContent `json:"response_message_content"` // LLM response
   */
  response_message?: string;
  /** the ID of the runner that processed this interaction */
  runner?: string;
  scheduled?: string;
  session_id?: string;
  state?: TypesInteractionState;
  status?: string;
  /** System prompt for the interaction (copy of the session's system prompt that was used to create this interaction) */
  system_prompt?: string;
  /** For Role=tool prompts this should be set to the ID given in the assistant's prior request to call a tool. */
  tool_call_id?: string;
  /** For Role=assistant prompts this may be set to the tool calls generated by the model, such as function calls. */
  tool_calls?: OpenaiToolCall[];
  /** Model function calling, not to be mistaken with Helix tools */
  tools?: OpenaiTool[];
  /** Session (default), slack, crisp, etc */
  trigger?: string;
  updated?: string;
  usage?: TypesUsage;
  user_id?: string;
}

export enum TypesInteractionState {
  InteractionStateNone = "",
  InteractionStateWaiting = "waiting",
  InteractionStateEditing = "editing",
  InteractionStateComplete = "complete",
  InteractionStateError = "error",
}

export interface TypesItem {
  b64_json?: string;
  embedding?: number[];
  index?: number;
  object?: string;
  /** Images */
  url?: string;
}

export interface TypesJobCompletion {
  app_id?: string;
  /** "fully_completed", "milestone_reached", etc. */
  completion_status?: string;
  /** "high", "medium", "low" */
  confidence?: string;
  created_at?: string;
  deliverables?: string;
  files_created?: string;
  id?: string;
  interaction_id?: string;
  limitations?: string;
  metadata?: number[];
  next_steps?: string;
  review_needed?: boolean;
  review_notes?: string;
  /** "approval", "feedback", "validation", etc. */
  review_type?: string;
  reviewed_at?: string;
  reviewed_by?: string;
  session_id?: string;
  /** "pending_review", "approved", "needs_changes", "archived" */
  status?: string;
  summary?: string;
  time_spent?: string;
  updated_at?: string;
  user_id?: string;
  work_item_id?: string;
}

export interface TypesKnowledge {
  /** AppID through which the knowledge was created */
  app_id?: string;
  /** URLs crawled in the last run (should match last knowledge version) */
  crawled_sources?: TypesCrawledSources;
  created?: string;
  /**
   * Description of the knowledge, will be used in the prompt
   * to explain the knowledge to the assistant
   */
  description?: string;
  id?: string;
  /** Set if something wrong happens */
  message?: string;
  name?: string;
  /** Populated by the cron job controller */
  next_run?: string;
  /** User ID */
  owner?: string;
  /** e.g. user, system, org */
  owner_type?: TypesOwnerType;
  /** Ephemeral state from knowledge controller */
  progress?: TypesKnowledgeProgress;
  rag_settings?: TypesRAGSettings;
  /**
   * RefreshEnabled defines if the knowledge should be refreshed periodically
   * or on events. For example a Google Drive knowledge can be refreshed
   * every 24 hours.
   */
  refresh_enabled?: boolean;
  /**
   * RefreshSchedule defines the schedule for refreshing the knowledge.
   * It can be specified in cron format or as a duration for example '@every 2h'
   * or 'every 5m' or '0 0 * * *' for daily at midnight.
   */
  refresh_schedule?: string;
  /** Size of the knowledge in bytes */
  size?: number;
  /**
   * Source defines where the raw data is fetched from. It can be
   * directly uploaded files, S3, GCS, Google Drive, Gmail, etc.
   */
  source?: TypesKnowledgeSource;
  state?: TypesKnowledgeState;
  updated?: string;
  /**
   * Version of the knowledge, will be used to separate different versions
   * of the same knowledge when updating it. Format is
   * YYYY-MM-DD-HH-MM-SS.
   */
  version?: string;
  versions?: TypesKnowledgeVersion[];
}

export interface TypesKnowledgeProgress {
  elapsed_seconds?: number;
  message?: string;
  progress?: number;
  started_at?: string;
  step?: string;
}

export interface TypesKnowledgeSearchResult {
  duration_ms?: number;
  knowledge?: TypesKnowledge;
  results?: TypesSessionRAGResult[];
}

export interface TypesKnowledgeSource {
  filestore?: TypesKnowledgeSourceHelixFilestore;
  gcs?: TypesKnowledgeSourceGCS;
  s3?: TypesKnowledgeSourceS3;
  text?: string;
  web?: TypesKnowledgeSourceWeb;
}

export interface TypesKnowledgeSourceGCS {
  bucket?: string;
  path?: string;
}

export interface TypesKnowledgeSourceHelixFilestore {
  path?: string;
  seed_zip_url?: string;
}

export interface TypesKnowledgeSourceS3 {
  bucket?: string;
  path?: string;
}

export interface TypesKnowledgeSourceWeb {
  auth?: TypesKnowledgeSourceWebAuth;
  /** Additional options for the crawler */
  crawler?: TypesWebsiteCrawler;
  excludes?: string[];
  urls?: string[];
}

export interface TypesKnowledgeSourceWebAuth {
  password?: string;
  username?: string;
}

export enum TypesKnowledgeState {
  KnowledgeStatePreparing = "preparing",
  KnowledgeStatePending = "pending",
  KnowledgeStateIndexing = "indexing",
  KnowledgeStateReady = "ready",
  KnowledgeStateError = "error",
}

export interface TypesKnowledgeVersion {
  crawled_sources?: TypesCrawledSources;
  created?: string;
  /** Model used to embed the knowledge */
  embeddings_model?: string;
  id?: string;
  knowledge_id?: string;
  /** Set if something wrong happens */
  message?: string;
  provider?: string;
  size?: number;
  state?: TypesKnowledgeState;
  updated?: string;
  version?: string;
}

export interface TypesLLMCall {
  app_id?: string;
  completion_cost?: number;
  completion_tokens?: number;
  created?: string;
  duration_ms?: number;
  error?: string;
  id?: string;
  interaction_id?: string;
  model?: string;
  organization_id?: string;
  original_request?: number[];
  prompt_cost?: number;
  prompt_tokens?: number;
  provider?: string;
  request?: number[];
  response?: number[];
  session_id?: string;
  step?: TypesLLMCallStep;
  stream?: boolean;
  /** Total cost of the call (prompt and completion tokens) */
  total_cost?: number;
  total_tokens?: number;
  updated?: string;
  user_id?: string;
}

export enum TypesLLMCallStep {
  LLMCallStepDefault = "default",
  LLMCallStepIsActionable = "is_actionable",
  LLMCallStepPrepareAPIRequest = "prepare_api_request",
  LLMCallStepInterpretResponse = "interpret_response",
  LLMCallStepGenerateTitle = "generate_title",
  LLMCallStepSummarizeConversation = "summarize_conversation",
}

export interface TypesLoginRequest {
  redirect_uri?: string;
}

export interface TypesManualWorkConfig {
  allow_anonymous?: boolean;
  default_priority?: number;
  enabled?: boolean;
}

export interface TypesMemory {
  app_id?: string;
  contents?: string;
  created?: string;
  id?: string;
  updated?: string;
  user_id?: string;
}

export interface TypesMessage {
  content?: TypesMessageContent;
  created_at?: string;
  /** Interaction ID */
  id?: string;
  role?: string;
  state?: TypesInteractionState;
  updated_at?: string;
}

export interface TypesMessageContent {
  /** text, image_url, multimodal_text */
  content_type?: TypesMessageContentType;
  /**
   * Parts is a list of strings or objects. For example for text, it's a list of strings, for
   * multi-modal it can be an object:
   * "parts": [
   * 		{
   * 				"content_type": "image_asset_pointer",
   * 				"asset_pointer": "file-service://file-28uHss2LgJ8HUEEVAnXa70Tg",
   * 				"size_bytes": 185427,
   * 				"width": 2048,
   * 				"height": 1020,
   * 				"fovea": null,
   * 				"metadata": null
   * 		},
   * 		"what is in the image?"
   * ]
   */
  parts?: any[];
}

export enum TypesMessageContentType {
  MessageContentTypeText = "text",
}

export enum TypesModality {
  ModalityText = "text",
  ModalityImage = "image",
  ModalityFile = "file",
}

export interface TypesModel {
  allocated_gpu_count?: number;
  /** EXPORTED ALLOCATION FIELDS: Set by NewModelForGPUAllocation based on scheduler's GPU allocation decision */
  allocated_memory?: number;
  allocated_per_gpu_memory?: number[];
  allocated_specific_gpus?: number[];
  /** Safety flag */
  allocation_configured?: boolean;
  /** Whether to automatically pull the model if missing in the runner */
  auto_pull?: boolean;
  /** max concurrent requests per slot (0 = use global default) */
  concurrency?: number;
  context_length?: number;
  created?: string;
  description?: string;
  enabled?: boolean;
  hide?: boolean;
  /** for example 'phi3.5:3.8b-mini-instruct-q8_0' */
  id?: string;
  /** DATABASE FIELD: Admin-configured memory for VLLM models, MUST be 0 for Ollama models */
  memory?: number;
  name?: string;
  /** Whether to prewarm this model to fill free GPU memory on runners */
  prewarm?: boolean;
  runtime?: TypesRuntime;
  /** Runtime-specific arguments (e.g., VLLM command line args) */
  runtime_args?: Record<string, any>;
  /** Order for sorting models in UI (lower numbers appear first) */
  sort_order?: number;
  type?: TypesModelType;
  updated?: string;
  /** User modification tracking - system defaults are automatically updated if this is false */
  user_modified?: boolean;
}

export interface TypesModelInfo {
  author?: string;
  context_length?: number;
  description?: string;
  input_modalities?: TypesModality[];
  max_completion_tokens?: number;
  name?: string;
  output_modalities?: TypesModality[];
  pricing?: TypesPricing;
  provider_model_id?: string;
  provider_slug?: string;
  slug?: string;
  supported_parameters?: string[];
  supports_reasoning?: boolean;
}

export enum TypesModelType {
  ModelTypeChat = "chat",
  ModelTypeImage = "image",
  ModelTypeEmbed = "embed",
}

export interface TypesOAuthConnection {
  /** OAuth token fields */
  access_token?: string;
  created_at?: string;
  deleted_at?: GormDeletedAt;
  expires_at?: string;
  id?: string;
  metadata?: string;
  profile?: TypesOAuthUserInfo;
  /** Provider is a reference to the OAuth provider */
  provider?: TypesOAuthProvider;
  provider_id?: string;
  provider_user_email?: string;
  /** User details from the provider */
  provider_user_id?: string;
  provider_username?: string;
  refresh_token?: string;
  scopes?: string[];
  updated_at?: string;
  user_id?: string;
}

export interface TypesOAuthConnectionTestResult {
  message?: string;
  /** Returned from the provider itself */
  provider_details?: Record<string, any>;
  success?: boolean;
}

export interface TypesOAuthProvider {
  /** OAuth 2.0 fields */
  auth_url?: string;
  callback_url?: string;
  /** Common fields for all providers */
  client_id?: string;
  client_secret?: string;
  created_at?: string;
  /** Who created/owns this provider */
  creator_id?: string;
  creator_type?: TypesOwnerType;
  deleted_at?: GormDeletedAt;
  description?: string;
  discovery_url?: string;
  enabled?: boolean;
  id?: string;
  name?: string;
  /** Misc configuration */
  scopes?: string[];
  token_url?: string;
  type?: TypesOAuthProviderType;
  updated_at?: string;
  user_info_url?: string;
}

export enum TypesOAuthProviderType {
  OAuthProviderTypeUnknown = "",
  OAuthProviderTypeAtlassian = "atlassian",
  OAuthProviderTypeGoogle = "google",
  OAuthProviderTypeMicrosoft = "microsoft",
  OAuthProviderTypeGitHub = "github",
  OAuthProviderTypeSlack = "slack",
  OAuthProviderTypeLinkedIn = "linkedin",
  OAuthProviderTypeHubSpot = "hubspot",
  OAuthProviderTypeCustom = "custom",
}

export interface TypesOAuthUserInfo {
  avatar_url?: string;
  display_name?: string;
  email?: string;
  id?: string;
  name?: string;
  /** Raw JSON response from provider */
  raw?: string;
}

export interface TypesOpenAIMessage {
  /** The message content */
  content?: string;
  /** The message role */
  role?: string;
  /** For Role=tool prompts this should be set to the ID given in the assistant's prior request to call a tool. */
  tool_call_id?: string;
  /** For Role=assistant prompts this may be set to the tool calls generated by the model, such as function calls. */
  tool_calls?: OpenaiToolCall[];
}

export interface TypesOpenAIModel {
  context_length?: number;
  created?: number;
  description?: string;
  enabled?: boolean;
  hide?: boolean;
  id?: string;
  model_info?: TypesModelInfo;
  name?: string;
  object?: string;
  owned_by?: string;
  parent?: string;
  permission?: TypesOpenAIPermission[];
  root?: string;
  type?: string;
}

export interface TypesOpenAIModelsList {
  data?: TypesOpenAIModel[];
}

export interface TypesOpenAIPermission {
  allow_create_engine?: boolean;
  allow_fine_tuning?: boolean;
  allow_logprobs?: boolean;
  allow_sampling?: boolean;
  allow_search_indices?: boolean;
  allow_view?: boolean;
  created?: number;
  group?: any;
  id?: string;
  is_blocking?: boolean;
  object?: string;
  organization?: string;
}

export interface TypesOpenAIResponse {
  choices?: TypesChoice[];
  created?: number;
  data?: TypesItem[];
  id?: string;
  model?: string;
  object?: string;
  usage?: TypesOpenAIUsage;
}

export interface TypesOpenAIUsage {
  completion_tokens?: number;
  prompt_tokens?: number;
  total_tokens?: number;
}

export interface TypesOrganization {
  created_at?: string;
  deleted_at?: GormDeletedAt;
  display_name?: string;
  id?: string;
  /** Memberships in the organization */
  memberships?: TypesOrganizationMembership[];
  name?: string;
  /** Who created the org */
  owner?: string;
  /** Roles in the organization */
  roles?: TypesRole[];
  /** Teams in the organization */
  teams?: TypesTeam[];
  updated_at?: string;
}

export interface TypesOrganizationMembership {
  created_at?: string;
  organization_id?: string;
  /** Role - the role of the user in the organization (owner or member) */
  role?: TypesOrganizationRole;
  updated_at?: string;
  user?: TypesUser;
  /** composite key */
  user_id?: string;
}

export enum TypesOrganizationRole {
  OrganizationRoleOwner = "owner",
  OrganizationRoleMember = "member",
}

export enum TypesOwnerType {
  OwnerTypeUser = "user",
  OwnerTypeRunner = "runner",
  OwnerTypeSystem = "system",
  OwnerTypeSocket = "socket",
  OwnerTypeOrg = "org",
}

export interface TypesPaginatedInteractions {
  interactions?: TypesInteraction[];
  page?: number;
  pageSize?: number;
  totalCount?: number;
  totalPages?: number;
}

export interface TypesPaginatedLLMCalls {
  calls?: TypesLLMCall[];
  page?: number;
  pageSize?: number;
  totalCount?: number;
  totalPages?: number;
}

export interface TypesPaginatedSessionsList {
  page?: number;
  pageSize?: number;
  sessions?: TypesSessionSummary[];
  totalCount?: number;
  totalPages?: number;
}

export interface TypesPaginatedUsersList {
  page?: number;
  pageSize?: number;
  totalCount?: number;
  totalPages?: number;
  users?: TypesUser[];
}

export interface TypesPricing {
  audio?: string;
  completion?: string;
  image?: string;
  internal_reasoning?: string;
  prompt?: string;
  request?: string;
  web_search?: string;
}

export enum TypesProvider {
  ProviderOpenAI = "openai",
  ProviderTogetherAI = "togetherai",
  ProviderAnthropic = "anthropic",
  ProviderHelix = "helix",
  ProviderVLLM = "vllm",
}

export interface TypesProviderEndpoint {
  api_key?: string;
  /** Must be mounted to the container */
  api_key_file?: string;
  available_models?: TypesOpenAIModel[];
  base_url?: string;
  billing_enabled?: boolean;
  created?: string;
  /** Set from environment variable */
  default?: boolean;
  description?: string;
  /** global, user (TODO: orgs, teams) */
  endpoint_type?: TypesProviderEndpointType;
  error?: string;
  /** If for example anthropic expects x-api-key and anthropic-version */
  headers?: Record<string, string>;
  id?: string;
  /** Optional */
  models?: string[];
  name?: string;
  owner?: string;
  /** user, system, org */
  owner_type?: TypesOwnerType;
  /** If we can't fetch models */
  status?: TypesProviderEndpointStatus;
  updated?: string;
}

export enum TypesProviderEndpointStatus {
  ProviderEndpointStatusOK = "ok",
  ProviderEndpointStatusError = "error",
  ProviderEndpointStatusLoading = "loading",
  ProviderEndpointStatusDisabled = "disabled",
}

export enum TypesProviderEndpointType {
  ProviderEndpointTypeGlobal = "global",
  ProviderEndpointTypeUser = "user",
  ProviderEndpointTypeOrg = "org",
  ProviderEndpointTypeTeam = "team",
}

export interface TypesRAGSettings {
  /** the amount of overlap between chunks - will default to 32 bytes */
  chunk_overflow?: number;
  /** the size of each text chunk - will default to 2000 bytes */
  chunk_size?: number;
  /** the URL of the delete endpoint (defaults to Helix RAG_DELETE_URL env var) */
  delete_url?: string;
  /** if true, we will not chunk the text and send the entire file to the RAG indexing endpoint */
  disable_chunking?: boolean;
  /** if true, we will not download the file and send the URL to the RAG indexing endpoint */
  disable_downloading?: boolean;
  /** this is one of l2, inner_product or cosine - will default to cosine */
  distance_function?: string;
  /** if true, we will use the vision pipeline -- Future - might want to specify different pipelines */
  enable_vision?: boolean;
  /** RAG endpoint configuration if used with a custom RAG service */
  index_url?: string;
  /** the prompt template to use for the RAG query */
  prompt_template?: string;
  /** the URL of the query endpoint (defaults to Helix RAG_QUERY_URL env var) */
  query_url?: string;
  /** this is the max number of results to return - will default to 3 */
  results_count?: number;
  /** Markdown if empty or 'text' */
  text_splitter?: TypesTextSplitterType;
  /** this is the threshold for a "good" answer - will default to 0.2 */
  threshold?: number;
  typesense?: {
    api_key?: string;
    collection?: string;
    url?: string;
  };
}

export enum TypesResource {
  ResourceTeam = "Team",
  ResourceOrganization = "Organization",
  ResourceRole = "Role",
  ResourceMembership = "Membership",
  ResourceMembershipRoleBinding = "MembershipRoleBinding",
  ResourceApplication = "Application",
  ResourceAccessGrants = "AccessGrants",
  ResourceKnowledge = "Knowledge",
  ResourceUser = "User",
  ResourceAny = "*",
  ResourceTypeDataset = "Dataset",
}

export interface TypesResponseFormat {
  schema?: OpenaiChatCompletionResponseFormatJSONSchema;
  type?: TypesResponseFormatType;
}

export enum TypesResponseFormatType {
  ResponseFormatTypeJSONObject = "json_object",
  ResponseFormatTypeText = "text",
}

export interface TypesRole {
  config?: GithubComHelixmlHelixApiPkgTypesConfig;
  created_at?: string;
  description?: string;
  id?: string;
  name?: string;
  organization_id?: string;
  updated_at?: string;
}

export interface TypesRule {
  actions?: TypesAction[];
  effect?: TypesEffect;
  resource?: TypesResource[];
}

export interface TypesRunAPIActionRequest {
  action?: string;
  parameters?: Record<string, any>;
}

export interface TypesRunAPIActionResponse {
  error?: string;
  /** Raw response from the API */
  response?: string;
}

export interface TypesRunnerModelStatus {
  download_in_progress?: boolean;
  download_percent?: number;
  error?: string;
  /** Memory requirement in bytes */
  memory?: number;
  model_id?: string;
  runtime?: TypesRuntime;
}

export interface TypesRunnerSlot {
  active?: boolean;
  active_requests?: number;
  command_line?: string;
  context_length?: number;
  created?: string;
  gpu_allocation_data?: Record<string, any>;
  gpu_index?: number;
  gpu_indices?: number[];
  id?: string;
  max_concurrency?: number;
  memory_estimation_meta?: Record<string, any>;
  model?: string;
  model_memory_requirement?: number;
  ready?: boolean;
  runner_id?: string;
  runtime?: TypesRuntime;
  runtime_args?: Record<string, any>;
  status?: string;
  tensor_parallel_size?: number;
  updated?: string;
  version?: string;
  workload_data?: Record<string, any>;
}

export interface TypesRunnerStateView {
  active_slots?: number;
  /** GPU index -> state */
  gpu_states?: Record<string, TypesGPUState>;
  is_connected?: boolean;
  runner_id?: string;
  total_slots?: number;
  warm_slots?: number;
}

export enum TypesRuntime {
  RuntimeOllama = "ollama",
  RuntimeDiffusers = "diffusers",
  RuntimeAxolotl = "axolotl",
  RuntimeVLLM = "vllm",
}

export interface TypesSSHKeyCreateRequest {
  key_name: string;
  private_key: string;
  public_key: string;
}

export interface TypesSSHKeyGenerateRequest {
  key_name: string;
  /** "ed25519" or "rsa", defaults to ed25519 */
  key_type?: string;
}

export interface TypesSSHKeyGenerateResponse {
  id?: string;
  key_name?: string;
  /** Only returned once during generation */
  private_key?: string;
  public_key?: string;
}

export interface TypesSSHKeyResponse {
  created?: string;
  id?: string;
  key_name?: string;
  last_used?: string;
  public_key?: string;
}

export interface TypesSchedulingDecision {
  available_runners?: string[];
  created?: string;
  decision_type?: TypesSchedulingDecisionType;
  id?: string;
  memory_available?: number;
  memory_required?: number;
  mode?: TypesSessionMode;
  model_name?: string;
  processing_time_ms?: number;
  queue_position?: number;
  reason?: string;
  repeat_count?: number;
  runner_id?: string;
  session_id?: string;
  slot_id?: string;
  success?: boolean;
  total_slot_count?: number;
  warm_slot_count?: number;
  workload_id?: string;
}

export enum TypesSchedulingDecisionType {
  SchedulingDecisionTypeQueued = "queued",
  SchedulingDecisionTypeReuseWarmSlot = "reuse_warm_slot",
  SchedulingDecisionTypeCreateNewSlot = "create_new_slot",
  SchedulingDecisionTypeEvictStaleSlot = "evict_stale_slot",
  SchedulingDecisionTypeRejected = "rejected",
  SchedulingDecisionTypeError = "error",
  SchedulingDecisionTypeUnschedulable = "unschedulable",
}

export interface TypesSchedulingEvent {
  description?: string;
  /** "slot_created", "slot_deleted", "eviction", "stabilization_start", "stabilization_end" */
  event_type?: string;
  gpu_indices?: number[];
  memory_mb?: number;
  model_name?: string;
  runtime?: string;
  slot_id?: string;
  timestamp?: string;
}

export interface TypesSecret {
  /** optional, if set, the secret will be available to the specified app */
  app_id?: string;
  created?: string;
  id?: string;
  name?: string;
  owner?: string;
  ownerType?: TypesOwnerType;
  updated?: string;
  value?: number[];
}

export interface TypesServerConfigForFrontend {
  apps_enabled?: boolean;
  /** Charging for usage */
  billing_enabled?: boolean;
  deployment_id?: string;
  disable_llm_call_logging?: boolean;
  eval_user_id?: string;
  /**
   * used to prepend onto raw filestore paths to download files
   * the filestore path will have the user info in it - i.e.
   * it's a low level filestore path
   * if we are using an object storage thing - then this URL
   * can be the prefix to the bucket
   */
  filestore_prefix?: string;
  google_analytics_frontend?: string;
  latest_version?: string;
  license?: TypesFrontendLicenseInfo;
  /** "single" or "multi" - determines streaming architecture */
  moonlight_web_mode?: string;
  organizations_create_enabled_for_non_admins?: boolean;
  rudderstack_data_plane_url?: string;
  rudderstack_write_key?: string;
  sentry_dsn_frontend?: string;
  /** Stripe top-ups enabled */
  stripe_enabled?: boolean;
  tools_enabled?: boolean;
  version?: string;
}

export interface TypesSession {
  /** named config for backward compat */
  config?: TypesSessionMetadata;
  created?: string;
  /** Current generation ID */
  generation_id?: number;
  id?: string;
  /**
   * for now we just whack the entire history of the interaction in here, json
   * style
   */
  interactions?: TypesInteraction[];
  /**
   * if type == finetune, we record a filestore path to e.g. lora file here
   * currently the only place you can do inference on a finetune is within the
   * session where the finetune was generated
   */
  lora_dir?: string;
  /** e.g. inference, finetune */
  mode?: TypesSessionMode;
  model_name?: string;
  /**
   * name that goes in the UI - ideally autogenerated by AI but for now can be
   * named manually
   */
  name?: string;
  /** the organization this session belongs to, if any */
  organization_id?: string;
  /** uuid of owner entity */
  owner?: string;
  /** e.g. user, system, org */
  owner_type?: TypesOwnerType;
  /**
   * the app this session was spawned from
   * TODO: rename to AppID
   */
  parent_app?: string;
  parent_session?: string;
  /**
   * huggingface model name e.g. mistralai/Mistral-7B-Instruct-v0.1 or
   * stabilityai/stable-diffusion-xl-base-1.0
   */
  provider?: string;
  trigger?: string;
  /** e.g. text, image */
  type?: TypesSessionType;
  updated?: string;
}

export interface TypesSessionChatRequest {
  /** Agent type: "helix" or "zed_external" */
  agent_type?: string;
  /** Assign the session settings from the specified app */
  app_id?: string;
  /** Which assistant are we speaking to? */
  assistant_id?: string;
  /** Configuration for external agents */
  external_agent_config?: TypesExternalAgentConfig;
  /** If empty, we will start a new interaction */
  interaction_id?: string;
  lora_dir?: string;
  /** Initial messages */
  messages?: TypesMessage[];
  /** The model to use */
  model?: string;
  /** The organization this session belongs to, if any */
  organization_id?: string;
  /** The provider to use */
  provider?: TypesProvider;
  /** If true, we will regenerate the response for the last message */
  regenerate?: boolean;
  /** If empty, we will start a new session */
  session_id?: string;
  /** If true, we will stream the response */
  stream?: boolean;
  /** System message, only applicable when starting a new session */
  system?: string;
  /** Available tools to use in the session */
  tools?: string[];
  /** e.g. text, image */
  type?: TypesSessionType;
}

export interface TypesSessionMetadata {
  active_tools?: string[];
  /** Agent type: "helix" or "zed_external" */
  agent_type?: string;
  /** Streaming resolution height (default: 1600) */
  agent_video_height?: number;
  /** Streaming refresh rate (default: 60) */
  agent_video_refresh_rate?: number;
  /** Video settings for external agent sessions (Phase 3.5) */
  agent_video_width?: number;
  /** Passing through user defined app params */
  app_query_params?: Record<string, string>;
  /** which assistant are we talking to? */
  assistant_id?: string;
  avatar?: string;
  document_group_id?: string;
  document_ids?: Record<string, string>;
  eval_automatic_reason?: string;
  eval_automatic_score?: string;
  eval_manual_reason?: string;
  eval_manual_score?: string;
  eval_original_user_prompts?: string[];
  /**
   * Evals are cool. Scores are strings of floats so we can distinguish ""
   * (not rated) from "0.0"
   */
  eval_run_id?: string;
  eval_user_reason?: string;
  eval_user_score?: string;
  /** Configuration for external agents */
  external_agent_config?: TypesExternalAgentConfig;
  /** NEW: External agent ID for this session */
  external_agent_id?: string;
  /** NEW: External agent status (running, stopped, terminated_idle) */
  external_agent_status?: string;
  helix_version?: string;
  /** Index of implementation task this session handles */
  implementation_task_index?: number;
  manually_review_questions?: boolean;
  /** NEW: SpecTask phase (planning, implementation) */
  phase?: string;
  priority?: boolean;
  /**
   * these settings control which features of a session we want to use
   * even if we have a Lora file and RAG indexed prepared
   * we might choose to not use them (this will help our eval framework know what works the best)
   * we well as activate RAG - we also get to control some properties, e.g. which distance function to use,
   * and what the threshold for a "good" answer is
   */
  rag_enabled?: boolean;
  rag_settings?: TypesRAGSettings;
  session_rag_results?: TypesSessionRAGResult[];
  /** "planning", "implementation", "coordination" */
  session_role?: string;
  /** Multi-session SpecTask context */
  spec_task_id?: string;
  stream?: boolean;
  system_prompt?: string;
  /** without any user input, this will default to true */
  text_finetune_enabled?: boolean;
  /** when we do fine tuning or RAG, we need to know which data entity we used */
  uploaded_data_entity_id?: string;
  /** Wolf lobby ID for streaming */
  wolf_lobby_id?: string;
  /** PIN for Wolf lobby access (Phase 3: Multi-tenancy) */
  wolf_lobby_pin?: string;
  /** ID of associated WorkSession */
  work_session_id?: string;
  /** Associated Zed instance ID */
  zed_instance_id?: string;
  /** Associated Zed thread ID */
  zed_thread_id?: string;
}

export enum TypesSessionMode {
  SessionModeNone = "",
  SessionModeInference = "inference",
  SessionModeFinetune = "finetune",
  SessionModeAction = "action",
}

export interface TypesSessionRAGResult {
  content?: string;
  content_offset?: number;
  distance?: number;
  document_group_id?: string;
  document_id?: string;
  filename?: string;
  id?: string;
  interaction_id?: string;
  metadata?: Record<string, string>;
  session_id?: string;
  source?: string;
}

export interface TypesSessionSummary {
  app_id?: string;
  /** these are all values of the last interaction */
  created?: string;
  /** InteractionID string      `json:"interaction_id"` */
  model_name?: string;
  name?: string;
  organization_id?: string;
  owner?: string;
  priority?: boolean;
  session_id?: string;
  /** this is either the prompt or the summary of the training data */
  summary?: string;
  type?: TypesSessionType;
  updated?: string;
}

export enum TypesSessionType {
  SessionTypeNone = "",
  SessionTypeText = "text",
  SessionTypeImage = "image",
}

export interface TypesSkillDefinition {
  /** API configuration */
  baseUrl?: string;
  category?: string;
  /** Metadata */
  configurable?: boolean;
  description?: string;
  displayName?: string;
  filePath?: string;
  headers?: Record<string, string>;
  icon?: TypesSkillIcon;
  id?: string;
  loadedAt?: string;
  name?: string;
  /** OAuth configuration */
  oauthProvider?: string;
  oauthScopes?: string[];
  provider?: string;
  requiredParameters?: TypesSkillRequiredParameter[];
  schema?: string;
  /**
   * if true, unknown keys in the response body will be removed before
   * returning to the agent for interpretation
   */
  skipUnknownKeys?: boolean;
  systemPrompt?: string;
  /**
   * Transform JSON into readable text to reduce the
   * size of the response body
   */
  transformOutput?: boolean;
}

export interface TypesSkillIcon {
  /** e.g., "GitHub", "Google" */
  name?: string;
  /** e.g., "material-ui", "custom" */
  type?: string;
}

export interface TypesSkillRequiredParameter {
  description?: string;
  name?: string;
  required?: boolean;
  /** "query", "header", "path" */
  type?: string;
}

export interface TypesSkillsListResponse {
  count?: number;
  skills?: TypesSkillDefinition[];
}

export interface TypesSlackTrigger {
  app_token?: string;
  bot_token?: string;
  channels?: string[];
  enabled?: boolean;
}

export interface TypesSpecApprovalResponse {
  approved?: boolean;
  approved_at?: string;
  approved_by?: string;
  /** Specific requested changes */
  changes?: string[];
  comments?: string;
  task_id?: string;
}

export interface TypesSpecTask {
  /** Git repository attachments (multiple repos can be attached) */
  attached_repositories?: number[];
  branch_name?: string;
  completed_at?: string;
  created_at?: string;
  /** Metadata */
  created_by?: string;
  description?: string;
  /** Simple tracking */
  estimated_hours?: number;
  /** External agent tracking (single agent per SpecTask, spans multiple sessions) */
  external_agent_id?: string;
  /** NEW: Single Helix Agent for entire workflow (App type in code) */
  helix_app_id?: string;
  id?: string;
  implementation_agent?: string;
  /** Discrete tasks breakdown (markdown) */
  implementation_plan?: string;
  implementation_session_id?: string;
  labels?: string[];
  metadata?: number[];
  name?: string;
  /** Kiro's actual approach: simple, human-readable artifacts */
  original_prompt?: string;
  /** Session tracking (same agent, different Helix sessions per phase) */
  planning_session_id?: string;
  primary_repository_id?: string;
  /** "low", "medium", "high", "critical" */
  priority?: string;
  project_id?: string;
  project_path?: string;
  /** User stories + EARS acceptance criteria (markdown) */
  requirements_spec?: string;
  /** Legacy fields (deprecated, keeping for backward compatibility) */
  spec_agent?: string;
  spec_approved_at?: string;
  /** Approval tracking */
  spec_approved_by?: string;
  /** Number of spec revisions requested */
  spec_revision_count?: number;
  spec_session_id?: string;
  started_at?: string;
  /** Spec-driven workflow statuses - see constants below */
  status?: string;
  /** Design document (markdown) */
  technical_design?: string;
  /** "feature", "bug", "refactor" */
  type?: string;
  updated_at?: string;
  workspace_config?: number[];
  /** Multi-session support */
  zed_instance_id?: string;
}

export interface TypesSpecTaskActivityLogEntry {
  activity_type?: TypesSpecTaskActivityType;
  id?: string;
  message?: string;
  metadata?: Record<string, any>;
  spec_task_id?: string;
  timestamp?: string;
  work_session_id?: string;
}

export enum TypesSpecTaskActivityType {
  SpecTaskActivitySessionCreated = "session_created",
  SpecTaskActivitySessionCompleted = "session_completed",
  SpecTaskActivitySessionSpawned = "session_spawned",
  SpecTaskActivityTaskCompleted = "task_completed",
  SpecTaskActivityZedConnected = "zed_connected",
  SpecTaskActivityZedDisconnected = "zed_disconnected",
  SpecTaskActivityPhaseTransition = "phase_transition",
}

export interface TypesSpecTaskImplementationSessionsCreateRequest {
  /** @default true */
  auto_create_sessions?: boolean;
  project_path?: string;
  spec_task_id: string;
  workspace_config?: Record<string, any>;
}

export enum TypesSpecTaskImplementationStatus {
  SpecTaskImplementationStatusPending = "pending",
  SpecTaskImplementationStatusAssigned = "assigned",
  SpecTaskImplementationStatusInProgress = "in_progress",
  SpecTaskImplementationStatusCompleted = "completed",
  SpecTaskImplementationStatusBlocked = "blocked",
}

export interface TypesSpecTaskImplementationTask {
  acceptance_criteria?: string;
  assigned_work_session?: TypesSpecTaskWorkSession;
  assigned_work_session_id?: string;
  completed_at?: string;
  created_at?: string;
  /** Array of other task indices */
  dependencies?: number[];
  description?: string;
  /** 'small', 'medium', 'large' */
  estimated_effort?: string;
  id?: string;
  /** Order within the plan */
  index?: number;
  priority?: number;
  /** Relationships */
  spec_task?: TypesSpecTask;
  spec_task_id?: string;
  /** Implementation tracking */
  status?: TypesSpecTaskImplementationStatus;
  /** Task details */
  title?: string;
}

export interface TypesSpecTaskImplementationTaskListResponse {
  implementation_tasks?: TypesSpecTaskImplementationTask[];
  total?: number;
}

export interface TypesSpecTaskMultiSessionOverviewResponse {
  active_sessions?: number;
  completed_sessions?: number;
  implementation_tasks?: TypesSpecTaskImplementationTask[];
  last_activity?: string;
  spec_task?: TypesSpecTask;
  work_session_count?: number;
  work_sessions?: TypesSpecTaskWorkSession[];
  zed_instance_id?: string;
  zed_thread_count?: number;
}

export enum TypesSpecTaskPhase {
  SpecTaskPhasePlanning = "planning",
  SpecTaskPhaseImplementation = "implementation",
  SpecTaskPhaseValidation = "validation",
}

export interface TypesSpecTaskProgressResponse {
  active_work_sessions?: TypesSpecTaskWorkSession[];
  /** Task index -> progress */
  implementation_progress?: Record<string, number>;
  /** 0.0 to 1.0 */
  overall_progress?: number;
  phase_progress?: Record<string, number>;
  recent_activity?: TypesSpecTaskActivityLogEntry[];
  spec_task?: TypesSpecTask;
}

export interface TypesSpecTaskUpdateRequest {
  description?: string;
  name?: string;
  priority?: string;
  status?: string;
}

export interface TypesSpecTaskWorkSession {
  /** Configuration */
  agent_config?: number[];
  completed_at?: string;
  /** State tracking */
  created_at?: string;
  description?: string;
  environment_config?: number[];
  /** 1:1 mapping */
  helix_session_id?: string;
  id?: string;
  implementation_task_description?: string;
  implementation_task_index?: number;
  /** Implementation context (parsed from ImplementationPlan) */
  implementation_task_title?: string;
  /** Work session details */
  name?: string;
  /** Relationships for spawning/branching */
  parent_work_session_id?: string;
  phase?: TypesSpecTaskPhase;
  spawned_by_session_id?: string;
  spec_task_id?: string;
  started_at?: string;
  status?: TypesSpecTaskWorkSessionStatus;
  updated_at?: string;
}

export interface TypesSpecTaskWorkSessionDetailResponse {
  child_work_sessions?: TypesSpecTaskWorkSession[];
  helix_session?: TypesSession;
  implementation_task?: TypesSpecTaskImplementationTask;
  spec_task?: TypesSpecTask;
  work_session?: TypesSpecTaskWorkSession;
  zed_thread?: TypesSpecTaskZedThread;
}

export interface TypesSpecTaskWorkSessionListResponse {
  total?: number;
  work_sessions?: TypesSpecTaskWorkSession[];
}

export interface TypesSpecTaskWorkSessionSpawnRequest {
  agent_config?: Record<string, any>;
  description?: string;
  environment_config?: Record<string, any>;
  /** @maxLength 255 */
  name: string;
  parent_work_session_id: string;
}

export enum TypesSpecTaskWorkSessionStatus {
  SpecTaskWorkSessionStatusPending = "pending",
  SpecTaskWorkSessionStatusActive = "active",
  SpecTaskWorkSessionStatusCompleted = "completed",
  SpecTaskWorkSessionStatusFailed = "failed",
  SpecTaskWorkSessionStatusCancelled = "cancelled",
  SpecTaskWorkSessionStatusBlocked = "blocked",
}

export interface TypesSpecTaskWorkSessionUpdateRequest {
  config?: Record<string, any>;
  description?: string;
  /** @maxLength 255 */
  name?: string;
  status?: "pending" | "active" | "completed" | "failed" | "cancelled" | "blocked";
}

export enum TypesSpecTaskZedStatus {
  SpecTaskZedStatusPending = "pending",
  SpecTaskZedStatusActive = "active",
  SpecTaskZedStatusDisconnected = "disconnected",
  SpecTaskZedStatusCompleted = "completed",
  SpecTaskZedStatusFailed = "failed",
}

export interface TypesSpecTaskZedThread {
  created_at?: string;
  id?: string;
  last_activity_at?: string;
  spec_task_id?: string;
  status?: TypesSpecTaskZedStatus;
  /** Thread-specific configuration */
  thread_config?: number[];
  updated_at?: string;
  /**
   * Relationships (loaded via joins, not stored in database)
   * NOTE: We store IDs, not nested objects, to avoid circular references
   * Use GORM preloading: db.Preload("WorkSession").Find(&zedThread)
   */
  work_session?: TypesSpecTaskWorkSession;
  /** 1:1 mapping */
  work_session_id?: string;
  zed_thread_id?: string;
}

export interface TypesSpecTaskZedThreadCreateRequest {
  thread_config?: Record<string, any>;
  work_session_id: string;
}

export interface TypesSpecTaskZedThreadListResponse {
  total?: number;
  zed_threads?: TypesSpecTaskZedThread[];
}

export interface TypesSpecTaskZedThreadUpdateRequest {
  status?: "pending" | "active" | "disconnected" | "completed" | "failed";
  thread_config?: Record<string, any>;
}

export interface TypesStepInfo {
  app_id?: string;
  created?: string;
  /** That were used to call the tool */
  details?: TypesStepInfoDetails;
  /** How long the step took in milliseconds (useful for API calls, database queries, etc.) */
  duration_ms?: number;
  error?: string;
  /** Either Material UI icon, emoji or SVG. Leave empty for default */
  icon?: string;
  id?: string;
  interaction_id?: string;
  message?: string;
  name?: string;
  session_id?: string;
  type?: string;
  updated?: string;
}

export interface TypesStepInfoDetails {
  arguments?: Record<string, any>;
}

export interface TypesSystemSettingsRequest {
  huggingface_token?: string;
}

export interface TypesSystemSettingsResponse {
  created?: string;
  /** Sensitive fields are masked */
  huggingface_token_set?: boolean;
  /** "database", "environment", or "none" */
  huggingface_token_source?: string;
  id?: string;
  updated?: string;
}

export interface TypesTeam {
  created_at?: string;
  deleted_at?: GormDeletedAt;
  id?: string;
  /** Memberships in the team */
  memberships?: TypesTeamMembership[];
  name?: string;
  organization_id?: string;
  updated_at?: string;
}

export interface TypesTeamMembership {
  created_at?: string;
  organization_id?: string;
  team?: TypesTeam;
  team_id?: string;
  updated_at?: string;
  /** extra data fields (optional) */
  user?: TypesUser;
  /** composite key */
  user_id?: string;
}

export interface TypesTestStep {
  expected_output?: string;
  prompt?: string;
}

export enum TypesTextSplitterType {
  TextSplitterTypeMarkdown = "markdown",
  TextSplitterTypeText = "text",
}

export enum TypesTokenType {
  TokenTypeNone = "",
  TokenTypeRunner = "runner",
  TokenTypeKeycloak = "keycloak",
  TokenTypeOIDC = "oidc",
  TokenTypeAPIKey = "api_key",
  TokenTypeSocket = "socket",
}

export interface TypesTool {
  config?: TypesToolConfig;
  description?: string;
  global?: boolean;
  id?: string;
  name?: string;
  /** E.g. As a restaurant expert, you provide personalized restaurant recommendations */
  system_prompt?: string;
  tool_type?: TypesToolType;
}

export interface TypesToolAPIAction {
  description?: string;
  method?: string;
  name?: string;
  path?: string;
}

export interface TypesToolAPIConfig {
  /** Read-only, parsed from schema on creation */
  actions?: TypesToolAPIAction[];
  /** Headers (authentication, etc) */
  headers?: Record<string, string>;
  model?: string;
  /** OAuth configuration */
  oauth_provider?: string;
  /** Required OAuth scopes for this API */
  oauth_scopes?: string[];
  /** Path parameters that will be substituted in URLs */
  path_params?: Record<string, string>;
  /** Query parameters that will be always set */
  query?: Record<string, string>;
  /** Template for request preparation, leave empty for default */
  request_prep_template?: string;
  /** Template for error response, leave empty for default */
  response_error_template?: string;
  /** Template for successful response, leave empty for default */
  response_success_template?: string;
  schema?: string;
  /**
   * if true, unknown keys in the response body will be removed before
   * returning to the agent for interpretation
   */
  skip_unknown_keys?: boolean;
  /** System prompt to guide the AI when using this API */
  system_prompt?: string;
  /**
   * Transform JSON into readable text to reduce the
   * size of the response body
   */
  transform_output?: boolean;
  /** Server override */
  url?: string;
}

export interface TypesToolAzureDevOpsConfig {
  enabled?: boolean;
  organization_url?: string;
  personal_access_token?: string;
}

export interface TypesToolBrowserConfig {
  enabled?: boolean;
  /** If true, the browser will return the HTML as markdown */
  markdown_post_processing?: boolean;
  /** If true, the browser will process the output of the tool call before returning it to the top loop. Useful for skills that return structured data such as Browser, */
  process_output?: boolean;
}

export interface TypesToolCalculatorConfig {
  enabled?: boolean;
}

export interface TypesToolConfig {
  api?: TypesToolAPIConfig;
  azure_devops?: TypesToolAzureDevOpsConfig;
  browser?: TypesToolBrowserConfig;
  calculator?: TypesToolCalculatorConfig;
  email?: TypesToolEmailConfig;
  mcp?: TypesToolMCPClientConfig;
  web_search?: TypesToolWebSearchConfig;
  zapier?: TypesToolZapierConfig;
}

export interface TypesToolEmailConfig {
  enabled?: boolean;
  template_example?: string;
}

export interface TypesToolMCPClientConfig {
  description?: string;
  enabled?: boolean;
  headers?: Record<string, string>;
  name?: string;
  oauth_provider?: string;
  /** Required OAuth scopes for this API */
  oauth_scopes?: string[];
  tools?: McpTool[];
  url?: string;
}

export enum TypesToolType {
  ToolTypeAPI = "api",
  ToolTypeBrowser = "browser",
  ToolTypeZapier = "zapier",
  ToolTypeCalculator = "calculator",
  ToolTypeEmail = "email",
  ToolTypeWebSearch = "web_search",
  ToolTypeAzureDevOps = "azure_devops",
  ToolTypeMCP = "mcp",
}

export interface TypesToolWebSearchConfig {
  enabled?: boolean;
  max_results?: number;
}

export interface TypesToolZapierConfig {
  api_key?: string;
  max_iterations?: number;
  model?: string;
}

export interface TypesTrigger {
  agent_work_queue?: TypesAgentWorkQueueTrigger;
  azure_devops?: TypesAzureDevOpsTrigger;
  crisp?: TypesCrispTrigger;
  cron?: TypesCronTrigger;
  discord?: TypesDiscordTrigger;
  slack?: TypesSlackTrigger;
}

export interface TypesTriggerConfiguration {
  /** App ID */
  app_id?: string;
  archived?: boolean;
  created?: string;
  enabled?: boolean;
  id?: string;
  /** Name of the trigger configuration */
  name?: string;
  ok?: boolean;
  /** Organization ID */
  organization_id?: string;
  /** User ID */
  owner?: string;
  /** User or Organization */
  owner_type?: TypesOwnerType;
  status?: string;
  trigger?: TypesTrigger;
  trigger_type?: TypesTriggerType;
  updated?: string;
  /** Webhook URL for the trigger configuration, applicable to webhook type triggers like Azure DevOps, GitHub, etc. */
  webhook_url?: string;
}

export interface TypesTriggerExecuteResponse {
  content?: string;
  session_id?: string;
}

export interface TypesTriggerExecution {
  created?: string;
  duration_ms?: number;
  error?: string;
  id?: string;
  /** Will most likely match session name, based on the trigger name at the time of execution */
  name?: string;
  output?: string;
  session_id?: string;
  status?: TypesTriggerExecutionStatus;
  trigger_configuration_id?: string;
  updated?: string;
}

export enum TypesTriggerExecutionStatus {
  TriggerExecutionStatusPending = "pending",
  TriggerExecutionStatusRunning = "running",
  TriggerExecutionStatusSuccess = "success",
  TriggerExecutionStatusError = "error",
}

export interface TypesTriggerStatus {
  message?: string;
  ok?: boolean;
  type?: TypesTriggerType;
}

export enum TypesTriggerType {
  TriggerTypeSlack = "slack",
  TriggerTypeCrisp = "crisp",
  TriggerTypeAzureDevOps = "azure_devops",
  TriggerTypeCron = "cron",
  TriggerTypeAgentWorkQueue = "agent_work_queue",
}

export interface TypesUpdateOrganizationMemberRequest {
  role?: TypesOrganizationRole;
}

export interface TypesUpdateProviderEndpoint {
  api_key?: string;
  /** Must be mounted to the container */
  api_key_file?: string;
  base_url?: string;
  description?: string;
  /** global, user (TODO: orgs, teams) */
  endpoint_type?: TypesProviderEndpointType;
  /** Custom headers for the endpoint */
  headers?: Record<string, string>;
  models?: string[];
}

export interface TypesUpdateTeamRequest {
  name?: string;
}

export interface TypesUsage {
  completion_tokens?: number;
  /** How long the request took in milliseconds */
  duration_ms?: number;
  prompt_tokens?: number;
  total_tokens?: number;
}

export interface TypesUser {
  /** if the ID of the user is contained in the env setting */
  admin?: boolean;
  /** if the token is associated with an app */
  app_id?: string;
  created_at?: string;
  deleted_at?: GormDeletedAt;
  email?: string;
  full_name?: string;
  id?: string;
  /** the actual token used and its type */
  token?: string;
  /** none, runner. keycloak, api_key */
  token_type?: TypesTokenType;
  /**
   * these are set by the keycloak user based on the token
   * if it's an app token - the keycloak user is loaded from the owner of the app
   * if it's a runner token - these values will be empty
   */
  type?: TypesOwnerType;
  updated_at?: string;
  username?: string;
}

export interface TypesUserAppAccessResponse {
  can_read?: boolean;
  can_write?: boolean;
  is_admin?: boolean;
}

export interface TypesUserResponse {
  email?: string;
  id?: string;
  name?: string;
  token?: string;
}

export interface TypesUserSearchResponse {
  limit?: number;
  offset?: number;
  total_count?: number;
  users?: TypesUser[];
}

export interface TypesUserTokenUsageResponse {
  is_pro_tier?: boolean;
  monthly_limit?: number;
  monthly_usage?: number;
  quotas_enabled?: boolean;
  usage_percentage?: number;
}

export interface TypesUsersAggregatedUsageMetric {
  metrics?: TypesAggregatedUsageMetric[];
  user?: TypesUser;
}

export interface TypesWallet {
  balance?: number;
  created_at?: string;
  id?: string;
  /** If belongs to an organization */
  org_id?: string;
  stripe_customer_id?: string;
  stripe_subscription_id?: string;
  subscription_created?: number;
  subscription_current_period_end?: number;
  subscription_current_period_start?: number;
  subscription_status?: StripeSubscriptionStatus;
  updated_at?: string;
  user_id?: string;
}

export interface TypesWebhookWorkConfig {
  enabled?: boolean;
  headers?: Record<string, string>;
  /** JSONPath to extract work description */
  json_path?: string;
  secret?: string;
}

export interface TypesWebsiteCrawler {
  enabled?: boolean;
  firecrawl?: TypesFirecrawl;
  ignore_robots_txt?: boolean;
  /** Limit crawl depth to avoid infinite crawling */
  max_depth?: number;
  /** Apply readability middleware to the HTML content */
  readability?: boolean;
  user_agent?: string;
}

export interface TypesWorkloadSummary {
  created?: string;
  id?: string;
  lora_dir?: string;
  mode?: string;
  model_name?: string;
  runtime?: string;
  summary?: string;
  updated?: string;
}

export interface TypesZedConfigResponse {
  agent?: Record<string, any>;
  assistant?: Record<string, any>;
  context_servers?: Record<string, any>;
  external_sync?: Record<string, any>;
  language_models?: Record<string, any>;
  theme?: string;
  /** Unix timestamp of app config update */
  version?: number;
}

export interface TypesZedInstanceEvent {
  data?: Record<string, any>;
  event_type?: string;
  instance_id?: string;
  spec_task_id?: string;
  thread_id?: string;
  timestamp?: string;
}

export interface TypesZedInstanceStatus {
  active_threads?: number;
  last_activity?: string;
  project_path?: string;
  spec_task_id?: string;
  status?: string;
  thread_count?: number;
  zed_instance_id?: string;
}

import type { AxiosInstance, AxiosRequestConfig, AxiosResponse, HeadersDefaults, ResponseType } from "axios";
import axios from "axios";

export type QueryParamsType = Record<string | number, any>;

export interface FullRequestParams extends Omit<AxiosRequestConfig, "data" | "params" | "url" | "responseType"> {
  /** set parameter to `true` for call `securityWorker` for this request */
  secure?: boolean;
  /** request path */
  path: string;
  /** content type of request body */
  type?: ContentType;
  /** query params */
  query?: QueryParamsType;
  /** format of response (i.e. response.json() -> format: "json") */
  format?: ResponseType;
  /** request body */
  body?: unknown;
}

export type RequestParams = Omit<FullRequestParams, "body" | "method" | "query" | "path">;

export interface ApiConfig<SecurityDataType = unknown> extends Omit<AxiosRequestConfig, "data" | "cancelToken"> {
  securityWorker?: (
    securityData: SecurityDataType | null,
  ) => Promise<AxiosRequestConfig | void> | AxiosRequestConfig | void;
  secure?: boolean;
  format?: ResponseType;
}

export enum ContentType {
  Json = "application/json",
  FormData = "multipart/form-data",
  UrlEncoded = "application/x-www-form-urlencoded",
  Text = "text/plain",
}

export class HttpClient<SecurityDataType = unknown> {
  public instance: AxiosInstance;
  private securityData: SecurityDataType | null = null;
  private securityWorker?: ApiConfig<SecurityDataType>["securityWorker"];
  private secure?: boolean;
  private format?: ResponseType;

  constructor({ securityWorker, secure, format, ...axiosConfig }: ApiConfig<SecurityDataType> = {}) {
    this.instance = axios.create({ ...axiosConfig, baseURL: axiosConfig.baseURL || "https://app.helix.ml" });
    this.secure = secure;
    this.format = format;
    this.securityWorker = securityWorker;
  }

  public setSecurityData = (data: SecurityDataType | null) => {
    this.securityData = data;
  };

  protected mergeRequestParams(params1: AxiosRequestConfig, params2?: AxiosRequestConfig): AxiosRequestConfig {
    const method = params1.method || (params2 && params2.method);

    return {
      ...this.instance.defaults,
      ...params1,
      ...(params2 || {}),
      headers: {
        ...((method && this.instance.defaults.headers[method.toLowerCase() as keyof HeadersDefaults]) || {}),
        ...(params1.headers || {}),
        ...((params2 && params2.headers) || {}),
      },
    };
  }

  protected stringifyFormItem(formItem: unknown) {
    if (typeof formItem === "object" && formItem !== null) {
      return JSON.stringify(formItem);
    } else {
      return `${formItem}`;
    }
  }

  protected createFormData(input: Record<string, unknown>): FormData {
    if (input instanceof FormData) {
      return input;
    }
    return Object.keys(input || {}).reduce((formData, key) => {
      const property = input[key];
      const propertyContent: any[] = property instanceof Array ? property : [property];

      for (const formItem of propertyContent) {
        const isFileType = formItem instanceof Blob || formItem instanceof File;
        formData.append(key, isFileType ? formItem : this.stringifyFormItem(formItem));
      }

      return formData;
    }, new FormData());
  }

  public request = async <T = any, _E = any>({
    secure,
    path,
    type,
    query,
    format,
    body,
    ...params
  }: FullRequestParams): Promise<AxiosResponse<T>> => {
    const secureParams =
      ((typeof secure === "boolean" ? secure : this.secure) &&
        this.securityWorker &&
        (await this.securityWorker(this.securityData))) ||
      {};
    const requestParams = this.mergeRequestParams(params, secureParams);
    const responseFormat = format || this.format || undefined;

    if (type === ContentType.FormData && body && body !== null && typeof body === "object") {
      body = this.createFormData(body as Record<string, unknown>);
    }

    if (type === ContentType.Text && body && body !== null && typeof body !== "string") {
      body = JSON.stringify(body);
    }

    return this.instance.request({
      ...requestParams,
      headers: {
        ...(requestParams.headers || {}),
        ...(type ? { "Content-Type": type } : {}),
      },
      params: query,
      responseType: responseFormat,
      data: body,
      url: path,
    });
  };
}

/**
 * @title HelixML API reference
 * @version 0.1
 * @baseUrl https://app.helix.ml
 * @contact Helix support <info@helix.ml> (https://app.helix.ml/)
 *
 * This is the HelixML API.
 */
export class Api<SecurityDataType extends unknown> extends HttpClient<SecurityDataType> {
  api = {
    /**
     * @description Retrieves combined debug data from Wolf (memory, lobbies, sessions) for the Agent Sandboxes dashboard
     *
     * @tags Admin
     * @name V1AdminAgentSandboxesDebugList
     * @summary Get Wolf debugging data
     * @request GET:/api/v1/admin/agent-sandboxes/debug
     * @secure
     */
    v1AdminAgentSandboxesDebugList: (params: RequestParams = {}) =>
      this.request<ServerAgentSandboxesDebugResponse, SystemHTTPError>({
        path: `/api/v1/admin/agent-sandboxes/debug`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Proxies Server-Sent Events from Wolf for real-time monitoring
     *
     * @tags Admin
     * @name V1AdminAgentSandboxesEventsList
     * @summary Get Wolf real-time events (SSE)
     * @request GET:/api/v1/admin/agent-sandboxes/events
     * @secure
     */
    v1AdminAgentSandboxesEventsList: (params: RequestParams = {}) =>
      this.request<string, SystemHTTPError>({
        path: `/api/v1/admin/agent-sandboxes/events`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Get agent fleet data including active sessions, work queue, and help requests without dashboard data
     *
     * @tags agents
     * @name V1AgentsFleetList
     * @summary Get agent fleet data
     * @request GET:/api/v1/agents/fleet
     * @secure
     */
    v1AgentsFleetList: (params: RequestParams = {}) =>
      this.request<TypesAgentFleetSummary, any>({
        path: `/api/v1/agents/fleet`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get real-time progress of all agents working on SpecTasks
     *
     * @tags SpecTasks
     * @name V1AgentsFleetLiveProgressList
     * @summary Get live agent fleet progress
     * @request GET:/api/v1/agents/fleet/live-progress
     * @secure
     */
    v1AgentsFleetLiveProgressList: (params: RequestParams = {}) =>
      this.request<ServerLiveAgentFleetProgressResponse, SystemHTTPError>({
        path: `/api/v1/agents/fleet/live-progress`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List help requests from agents needing human assistance
     *
     * @tags agents
     * @name V1AgentsHelpRequestsList
     * @summary List help requests
     * @request GET:/api/v1/agents/help-requests
     * @secure
     */
    v1AgentsHelpRequestsList: (
      query?: {
        /**
         * Page number
         * @default 0
         */
        page?: number;
        /**
         * Page size
         * @default 20
         */
        page_size?: number;
        /** Help request status filter */
        status?: string;
        /** Urgency level filter */
        urgency?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesHelpRequestsListResponse, any>({
        path: `/api/v1/agents/help-requests`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Provide resolution for a help request from an agent
     *
     * @tags agents
     * @name V1AgentsHelpRequestsResolveCreate
     * @summary Resolve a help request
     * @request POST:/api/v1/agents/help-requests/{request_id}/resolve
     * @secure
     */
    v1AgentsHelpRequestsResolveCreate: (
      requestId: string,
      request: Record<string, string>,
      params: RequestParams = {},
    ) =>
      this.request<TypesHelpRequest, any>({
        path: `/api/v1/agents/help-requests/${requestId}/resolve`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List agent sessions with filtering and pagination
     *
     * @tags agents
     * @name V1AgentsSessionsList
     * @summary List agent sessions
     * @request GET:/api/v1/agents/sessions
     * @secure
     */
    v1AgentsSessionsList: (
      query?: {
        /**
         * Page number
         * @default 0
         */
        page?: number;
        /**
         * Page size
         * @default 20
         */
        page_size?: number;
        /** Session status filter */
        status?: string;
        /** Agent type filter */
        agent_type?: string;
        /**
         * Show only active sessions
         * @default false
         */
        active_only?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAgentSessionsResponse, any>({
        path: `/api/v1/agents/sessions`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get statistics about the agent work queue
     *
     * @tags agents
     * @name V1AgentsStatsList
     * @summary Get work queue statistics
     * @request GET:/api/v1/agents/stats
     * @secure
     */
    v1AgentsStatsList: (params: RequestParams = {}) =>
      this.request<TypesAgentWorkQueueStats, any>({
        path: `/api/v1/agents/stats`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description List work items in the agent queue with filtering and pagination
     *
     * @tags agents
     * @name V1AgentsWorkList
     * @summary List agent work items
     * @request GET:/api/v1/agents/work
     * @secure
     */
    v1AgentsWorkList: (
      query?: {
        /**
         * Page number
         * @default 0
         */
        page?: number;
        /**
         * Page size
         * @default 20
         */
        page_size?: number;
        /** Work item status filter */
        status?: string;
        /** Agent type filter */
        agent_type?: string;
        /** Work item source filter */
        source?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAgentWorkItemsResponse, any>({
        path: `/api/v1/agents/work`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new work item in the agent queue
     *
     * @tags agents
     * @name V1AgentsWorkCreate
     * @summary Create a new agent work item
     * @request POST:/api/v1/agents/work
     * @secure
     */
    v1AgentsWorkCreate: (request: TypesAgentWorkItemCreateRequest, params: RequestParams = {}) =>
      this.request<TypesAgentWorkItem, any>({
        path: `/api/v1/agents/work`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Get details of a specific work item
     *
     * @tags agents
     * @name V1AgentsWorkDetail
     * @summary Get an agent work item
     * @request GET:/api/v1/agents/work/{work_item_id}
     * @secure
     */
    v1AgentsWorkDetail: (workItemId: string, params: RequestParams = {}) =>
      this.request<TypesAgentWorkItem, any>({
        path: `/api/v1/agents/work/${workItemId}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Update details of a specific work item
     *
     * @tags agents
     * @name V1AgentsWorkUpdate
     * @summary Update an agent work item
     * @request PUT:/api/v1/agents/work/{work_item_id}
     * @secure
     */
    v1AgentsWorkUpdate: (workItemId: string, request: TypesAgentWorkItemUpdateRequest, params: RequestParams = {}) =>
      this.request<TypesAgentWorkItem, any>({
        path: `/api/v1/agents/work/${workItemId}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete an API key
     *
     * @tags api-keys
     * @name V1ApiKeysDelete
     * @summary Delete an API key
     * @request DELETE:/api/v1/api_keys
     */
    v1ApiKeysDelete: (
      query: {
        /** API key to delete */
        key: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<string, any>({
        path: `/api/v1/api_keys`,
        method: "DELETE",
        query: query,
        ...params,
      }),

    /**
     * @description Get API keys
     *
     * @tags api-keys
     * @name V1ApiKeysList
     * @summary Get API keys
     * @request GET:/api/v1/api_keys
     * @secure
     */
    v1ApiKeysList: (
      query?: {
        /** Filter by types (comma-separated list) */
        types?: string;
        /** Filter by app ID */
        app_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesApiKey[], any>({
        path: `/api/v1/api_keys`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new API key
     *
     * @tags api-keys
     * @name V1ApiKeysCreate
     * @summary Create a new API key
     * @request POST:/api/v1/api_keys
     */
    v1ApiKeysCreate: (request: Record<string, any>, params: RequestParams = {}) =>
      this.request<string, any>({
        path: `/api/v1/api_keys`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List apps for the user. Apps are pre-configured to spawn sessions with specific tools and config.
     *
     * @tags apps
     * @name V1AppsList
     * @summary List apps
     * @request GET:/api/v1/apps
     * @secure
     */
    v1AppsList: (
      query?: {
        /** Organization ID */
        organization_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesApp[], any>({
        path: `/api/v1/apps`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1AppsCreate
     * @request POST:/api/v1/apps
     * @secure
     */
    v1AppsCreate: (request: TypesApp, params: RequestParams = {}) =>
      this.request<ServerAppCreateResponse, any>({
        path: `/api/v1/apps`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List triggers for the app
     *
     * @tags apps
     * @name V1AppsTriggersDetail
     * @summary List app triggers
     * @request GET:/api/v1/apps/{app_id}/triggers
     * @secure
     */
    v1AppsTriggersDetail: (appId: string, params: RequestParams = {}) =>
      this.request<TypesTriggerConfiguration[], any>({
        path: `/api/v1/apps/${appId}/triggers`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1AppsDelete
     * @request DELETE:/api/v1/apps/{id}
     * @secure
     */
    v1AppsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/apps/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1AppsDetail
     * @request GET:/api/v1/apps/{id}
     * @secure
     */
    v1AppsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesApp, any>({
        path: `/api/v1/apps/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1AppsUpdate
     * @request PUT:/api/v1/apps/{id}
     * @secure
     */
    v1AppsUpdate: (id: string, request: TypesApp, params: RequestParams = {}) =>
      this.request<TypesApp, any>({
        path: `/api/v1/apps/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List access grants for an app (organization owners and members can list access grants)
     *
     * @tags apps
     * @name V1AppsAccessGrantsDetail
     * @summary List app access grants
     * @request GET:/api/v1/apps/{id}/access-grants
     * @secure
     */
    v1AppsAccessGrantsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesAccessGrant[], any>({
        path: `/api/v1/apps/${id}/access-grants`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Grant access to an agent to a team or organization member (organization owners can grant access to teams and organization members)
     *
     * @tags apps
     * @name V1AppsAccessGrantsCreate
     * @summary Grant access to an agent to a team or organization member
     * @request POST:/api/v1/apps/{id}/access-grants
     * @secure
     */
    v1AppsAccessGrantsCreate: (id: string, request: TypesCreateAccessGrantRequest, params: RequestParams = {}) =>
      this.request<TypesAccessGrant, any>({
        path: `/api/v1/apps/${id}/access-grants`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Runs an API action for an app
     *
     * @name V1AppsApiActionsCreate
     * @summary Run an API action
     * @request POST:/api/v1/apps/{id}/api-actions
     * @secure
     */
    v1AppsApiActionsCreate: (id: string, request: TypesRunAPIActionRequest, params: RequestParams = {}) =>
      this.request<TypesRunAPIActionResponse, SystemHTTPError>({
        path: `/api/v1/apps/${id}/api-actions`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete the app's avatar image
     *
     * @tags apps
     * @name V1AppsAvatarDelete
     * @summary Delete app avatar
     * @request DELETE:/api/v1/apps/{id}/avatar
     * @secure
     */
    v1AppsAvatarDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, SystemHTTPError>({
        path: `/api/v1/apps/${id}/avatar`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get the app's avatar image
     *
     * @tags apps
     * @name V1AppsAvatarDetail
     * @summary Get app avatar
     * @request GET:/api/v1/apps/{id}/avatar
     * @secure
     */
    v1AppsAvatarDetail: (id: string, params: RequestParams = {}) =>
      this.request<File, SystemHTTPError>({
        path: `/api/v1/apps/${id}/avatar`,
        method: "GET",
        secure: true,
        format: "blob",
        ...params,
      }),

    /**
     * @description Upload a base64 encoded image as the app's avatar
     *
     * @tags apps
     * @name V1AppsAvatarCreate
     * @summary Upload app avatar
     * @request POST:/api/v1/apps/{id}/avatar
     * @secure
     */
    v1AppsAvatarCreate: (id: string, image: string, params: RequestParams = {}) =>
      this.request<void, SystemHTTPError>({
        path: `/api/v1/apps/${id}/avatar`,
        method: "POST",
        body: image,
        secure: true,
        type: ContentType.Text,
        ...params,
      }),

    /**
     * @description Get app daily usage
     *
     * @tags apps
     * @name V1AppsDailyUsageDetail
     * @summary Get app usage
     * @request GET:/api/v1/apps/{id}/daily-usage
     * @secure
     */
    v1AppsDailyUsageDetail: (
      id: string,
      query?: {
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/apps/${id}/daily-usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @name V1AppsDuplicateCreate
     * @request POST:/api/v1/apps/{id}/duplicate
     * @secure
     */
    v1AppsDuplicateCreate: (
      id: string,
      query?: {
        /** Optional new name for the app */
        name?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<void, any>({
        path: `/api/v1/apps/${id}/duplicate`,
        method: "POST",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description List interactions with pagination and optional session filtering for a specific app
     *
     * @tags interactions
     * @name V1AppsInteractionsDetail
     * @summary List interactions
     * @request GET:/api/v1/apps/{id}/interactions
     * @secure
     */
    v1AppsInteractionsDetail: (
      id: string,
      query?: {
        /** Page number */
        page?: number;
        /** Page size */
        pageSize?: number;
        /** Filter by session ID */
        session?: string;
        /** Filter by interaction ID */
        interaction?: string;
        /** Query by like/dislike */
        feedback?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPaginatedInteractions, any>({
        path: `/api/v1/apps/${id}/interactions`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List user's LLM calls with pagination and optional session filtering for a specific app
     *
     * @tags llm_calls
     * @name V1AppsLlmCallsDetail
     * @summary List LLM calls
     * @request GET:/api/v1/apps/{id}/llm-calls
     * @secure
     */
    v1AppsLlmCallsDetail: (
      id: string,
      query?: {
        /** Page number */
        page?: number;
        /** Page size */
        pageSize?: number;
        /** Filter by session ID */
        session?: string;
        /** Filter by interaction ID */
        interaction?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPaginatedLLMCalls, any>({
        path: `/api/v1/apps/${id}/llm-calls`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List memories for a specific app and user
     *
     * @tags memories
     * @name V1AppsMemoriesDetail
     * @summary List app memories
     * @request GET:/api/v1/apps/{id}/memories
     * @secure
     */
    v1AppsMemoriesDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesMemory[], any>({
        path: `/api/v1/apps/${id}/memories`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a specific memory for an app and user
     *
     * @tags memories
     * @name V1AppsMemoriesDelete
     * @summary Delete app memory
     * @request DELETE:/api/v1/apps/{id}/memories/{memory_id}
     * @secure
     */
    v1AppsMemoriesDelete: (id: string, memoryId: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/apps/${id}/memories/${memoryId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description List step info for a specific app and interaction ID, used to build the timeline of events
     *
     * @tags step_info
     * @name V1AppsStepInfoDetail
     * @summary List step info
     * @request GET:/api/v1/apps/{id}/step-info
     * @secure
     */
    v1AppsStepInfoDetail: (
      id: string,
      query?: {
        /** Interaction ID */
        interactionId?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesStepInfo[], any>({
        path: `/api/v1/apps/${id}/step-info`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the status of a specific trigger type for an app
     *
     * @tags apps
     * @name V1AppsTriggerStatusDetail
     * @summary Get app trigger status
     * @request GET:/api/v1/apps/{id}/trigger-status
     * @secure
     */
    v1AppsTriggerStatusDetail: (
      id: string,
      query: {
        /** Trigger type (e.g., slack) */
        trigger_type: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesTriggerStatus, any>({
        path: `/api/v1/apps/${id}/trigger-status`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Returns the access rights the current user has for this app
     *
     * @tags apps
     * @name V1AppsUserAccessDetail
     * @summary Get current user's access level for an app
     * @request GET:/api/v1/apps/{id}/user-access
     * @secure
     */
    v1AppsUserAccessDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesUserAppAccessResponse, any>({
        path: `/api/v1/apps/${id}/user-access`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get app users daily usage
     *
     * @tags apps
     * @name V1AppsUsersDailyUsageDetail
     * @summary Get app users daily usage
     * @request GET:/api/v1/apps/{id}/users-daily-usage
     * @secure
     */
    v1AppsUsersDailyUsageDetail: (
      id: string,
      query?: {
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/apps/${id}/users-daily-usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Check if the user is authenticated
     *
     * @tags auth
     * @name V1AuthAuthenticatedList
     * @summary Authenticated
     * @request GET:/api/v1/auth/authenticated
     */
    v1AuthAuthenticatedList: (params: RequestParams = {}) =>
      this.request<TypesAuthenticatedResponse, any>({
        path: `/api/v1/auth/authenticated`,
        method: "GET",
        ...params,
      }),

    /**
     * @description The callback receiver from the OIDC provider
     *
     * @tags auth
     * @name V1AuthCallbackList
     * @summary Callback from OIDC provider
     * @request GET:/api/v1/auth/callback
     */
    v1AuthCallbackList: (
      query: {
        /** The code from the OIDC provider */
        code: string;
        /** The state from the OIDC provider */
        state: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<void, any>({
        path: `/api/v1/auth/callback`,
        method: "GET",
        query: query,
        ...params,
      }),

    /**
     * @description Login to the application
     *
     * @tags auth
     * @name V1AuthLoginCreate
     * @summary Login
     * @request POST:/api/v1/auth/login
     */
    v1AuthLoginCreate: (request: TypesLoginRequest, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/login`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Logout the user
     *
     * @tags auth
     * @name V1AuthLogoutCreate
     * @summary Logout
     * @request POST:/api/v1/auth/logout
     */
    v1AuthLogoutCreate: (params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/logout`,
        method: "POST",
        ...params,
      }),

    /**
     * @description Refresh the access token
     *
     * @tags auth
     * @name V1AuthRefreshCreate
     * @summary Refresh the access token
     * @request POST:/api/v1/auth/refresh
     */
    v1AuthRefreshCreate: (params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/refresh`,
        method: "POST",
        ...params,
      }),

    /**
     * @description Get the current user's information
     *
     * @tags auth
     * @name V1AuthUserList
     * @summary User information
     * @request GET:/api/v1/auth/user
     */
    v1AuthUserList: (params: RequestParams = {}) =>
      this.request<TypesUserResponse, any>({
        path: `/api/v1/auth/user`,
        method: "GET",
        ...params,
      }),

    /**
     * @description Get config
     *
     * @tags config
     * @name V1ConfigList
     * @summary Get config
     * @request GET:/api/v1/config
     * @secure
     */
    v1ConfigList: (params: RequestParams = {}) =>
      this.request<TypesServerConfigForFrontend, any>({
        path: `/api/v1/config`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description contextMenuHandler
     *
     * @tags ui
     * @name V1ContextMenuList
     * @summary contextMenuHandler
     * @request GET:/api/v1/context-menu
     */
    v1ContextMenuList: (
      query: {
        /** App ID */
        app_id: string;
        /** Query string */
        q?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesContextMenuResponse, any>({
        path: `/api/v1/context-menu`,
        method: "GET",
        query: query,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1DashboardList
     * @request GET:/api/v1/dashboard
     * @secure
     */
    v1DashboardList: (params: RequestParams = {}) =>
      this.request<TypesDashboardData, any>({
        path: `/api/v1/dashboard`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get comprehensive dashboard data including agent sessions, work queue, and help requests
     *
     * @tags dashboard
     * @name V1DashboardAgentList
     * @summary Get enhanced dashboard data with agent management
     * @request GET:/api/v1/dashboard/agent
     * @secure
     */
    v1DashboardAgentList: (params: RequestParams = {}) =>
      this.request<TypesAgentDashboardSummary, any>({
        path: `/api/v1/dashboard/agent`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get keepalive session health status for an external agent
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsKeepaliveDetail
     * @summary Get keepalive session status
     * @request GET:/api/v1/external-agents/{sessionID}/keepalive
     * @secure
     */
    v1ExternalAgentsKeepaliveDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/keepalive`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the filestore configuration including user prefix and available folders
     *
     * @tags filestore
     * @name V1FilestoreConfigList
     * @summary Get filestore configuration
     * @request GET:/api/v1/filestore/config
     * @secure
     */
    v1FilestoreConfigList: (params: RequestParams = {}) =>
      this.request<FilestoreConfig, any>({
        path: `/api/v1/filestore/config`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a file or folder from the filestore
     *
     * @tags filestore
     * @name V1FilestoreDeleteDelete
     * @summary Delete filestore item
     * @request DELETE:/api/v1/filestore/delete
     * @secure
     */
    v1FilestoreDeleteDelete: (
      query: {
        /** Path to the file or folder to delete */
        path: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<
        {
          path?: string;
        },
        any
      >({
        path: `/api/v1/filestore/delete`,
        method: "DELETE",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new folder in the filestore at the specified path
     *
     * @tags filestore
     * @name V1FilestoreFolderCreate
     * @summary Create filestore folder
     * @request POST:/api/v1/filestore/folder
     * @secure
     */
    v1FilestoreFolderCreate: (
      request: {
        path?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<FilestoreItem, any>({
        path: `/api/v1/filestore/folder`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get information about a specific file or folder in the filestore
     *
     * @tags filestore
     * @name V1FilestoreGetList
     * @summary Get filestore item
     * @request GET:/api/v1/filestore/get
     * @secure
     */
    v1FilestoreGetList: (
      query: {
        /** Path to the file or folder (e.g., 'documents/file.pdf', 'apps/app_id/folder') */
        path: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<FilestoreItem, any>({
        path: `/api/v1/filestore/get`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List files and folders in the specified path. Supports both user and app-scoped paths
     *
     * @tags filestore
     * @name V1FilestoreListList
     * @summary List filestore items
     * @request GET:/api/v1/filestore/list
     * @secure
     */
    v1FilestoreListList: (
      query?: {
        /** Path to list (e.g., 'documents', 'apps/app_id/folder') */
        path?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<FilestoreItem[], any>({
        path: `/api/v1/filestore/list`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Rename a file or folder in the filestore. Cannot rename between different scopes (user/app)
     *
     * @tags filestore
     * @name V1FilestoreRenameUpdate
     * @summary Rename filestore item
     * @request PUT:/api/v1/filestore/rename
     * @secure
     */
    v1FilestoreRenameUpdate: (
      query: {
        /** Current path of the file or folder */
        path: string;
        /** New path for the file or folder */
        new_path: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<FilestoreItem, any>({
        path: `/api/v1/filestore/rename`,
        method: "PUT",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Upload one or more files to the specified path in the filestore. Supports multipart form data with 'files' field
     *
     * @tags filestore
     * @name V1FilestoreUploadCreate
     * @summary Upload files to filestore
     * @request POST:/api/v1/filestore/upload
     * @secure
     */
    v1FilestoreUploadCreate: (
      query: {
        /** Path where files should be uploaded (e.g., 'documents', 'apps/app_id/folder') */
        path: string;
      },
      data: {
        /** Files to upload (multipart form data) */
        files: File;
      },
      params: RequestParams = {},
    ) =>
      this.request<
        {
          success?: boolean;
        },
        any
      >({
        path: `/api/v1/filestore/upload`,
        method: "POST",
        query: query,
        body: data,
        secure: true,
        type: ContentType.FormData,
        format: "json",
        ...params,
      }),

    /**
     * @description Serve files from the filestore with access control. Supports both user and app-scoped paths
     *
     * @tags filestore
     * @name V1FilestoreViewerDetail
     * @summary View filestore files
     * @request GET:/api/v1/filestore/viewer/{path}
     * @secure
     */
    v1FilestoreViewerDetail: (
      path: string,
      query?: {
        /** Set to 'true' to redirect .url files to their target URLs */
        redirect_urls?: string;
        /** URL signature for public access */
        signature?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<File, any>({
        path: `/api/v1/filestore/viewer/${path}`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List all git repositories, optionally filtered by owner and type
     *
     * @tags git-repositories
     * @name V1GitRepositoriesList
     * @summary List git repositories
     * @request GET:/api/v1/git/repositories
     * @secure
     */
    v1GitRepositoriesList: (
      query?: {
        /** Filter by owner ID */
        owner_id?: string;
        /** Filter by repository type */
        repo_type?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServicesGitRepository[], TypesAPIError>({
        path: `/api/v1/git/repositories`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new git repository on the server
     *
     * @tags git-repositories
     * @name V1GitRepositoriesCreate
     * @summary Create git repository
     * @request POST:/api/v1/git/repositories
     * @secure
     */
    v1GitRepositoriesCreate: (repository: ServicesGitRepositoryCreateRequest, params: RequestParams = {}) =>
      this.request<ServicesGitRepository, TypesAPIError>({
        path: `/api/v1/git/repositories`,
        method: "POST",
        body: repository,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get information about a specific git repository
     *
     * @tags git-repositories
     * @name V1GitRepositoriesDetail
     * @summary Get git repository
     * @request GET:/api/v1/git/repositories/{id}
     * @secure
     */
    v1GitRepositoriesDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServicesGitRepository, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the git clone command for a repository with authentication
     *
     * @tags git-repositories
     * @name V1GitRepositoriesCloneCommandDetail
     * @summary Get clone command
     * @request GET:/api/v1/git/repositories/{id}/clone-command
     * @secure
     */
    v1GitRepositoriesCloneCommandDetail: (
      id: string,
      query?: {
        /** Target directory for clone */
        target_dir?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerCloneCommandResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/clone-command`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List all available Helix models, optionally filtering by type, name, or runtime.
     *
     * @tags models
     * @name V1HelixModelsList
     * @summary List Helix models
     * @request GET:/api/v1/helix-models
     * @secure
     */
    v1HelixModelsList: (
      query?: {
        /** Filter by model type (e.g., chat, embedding) */
        type?: string;
        /** Filter by model name */
        name?: string;
        /** Filter by model runtime (e.g., ollama, vllm) */
        runtime?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesModel[], any>({
        path: `/api/v1/helix-models`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new Helix model configuration. Requires admin privileges.
     *
     * @tags models
     * @name V1HelixModelsCreate
     * @summary Create a new Helix model
     * @request POST:/api/v1/helix-models
     * @secure
     */
    v1HelixModelsCreate: (request: TypesModel, params: RequestParams = {}) =>
      this.request<TypesModel, string>({
        path: `/api/v1/helix-models`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete a Helix model configuration. Requires admin privileges.
     *
     * @tags models
     * @name V1HelixModelsDelete
     * @summary Delete a Helix model
     * @request DELETE:/api/v1/helix-models/{id}
     * @secure
     */
    v1HelixModelsDelete: (id: string, params: RequestParams = {}) =>
      this.request<string, string>({
        path: `/api/v1/helix-models/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Update an existing Helix model configuration. Requires admin privileges.
     *
     * @tags models
     * @name V1HelixModelsUpdate
     * @summary Update an existing Helix model
     * @request PUT:/api/v1/helix-models/{id}
     * @secure
     */
    v1HelixModelsUpdate: (id: string, request: TypesModel, params: RequestParams = {}) =>
      this.request<TypesModel, string>({
        path: `/api/v1/helix-models/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Estimate memory requirements for a model on different GPU configurations
     *
     * @tags models
     * @name V1HelixModelsMemoryEstimateList
     * @summary Estimate model memory requirements
     * @request GET:/api/v1/helix-models/memory-estimate
     * @secure
     */
    v1HelixModelsMemoryEstimateList: (
      query: {
        /** Model ID */
        model_id: string;
        /** Number of GPUs (default: auto-detect) */
        gpu_count?: number;
        /** Context length (default: model default) */
        context_length?: number;
        /** Batch size (default: 512) */
        batch_size?: number;
        /** Number of parallel sequences/concurrent requests (default: 2) */
        num_parallel?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ControllerMemoryEstimationResponse, string>({
        path: `/api/v1/helix-models/memory-estimate`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get memory estimates for multiple models with different GPU configurations
     *
     * @tags models
     * @name V1HelixModelsMemoryEstimatesList
     * @summary List memory estimates for multiple models
     * @request GET:/api/v1/helix-models/memory-estimates
     * @secure
     */
    v1HelixModelsMemoryEstimatesList: (
      query?: {
        /** Comma-separated list of model IDs */
        model_ids?: string;
        /** Number of GPUs (default: auto-detect) */
        gpu_count?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ControllerMemoryEstimationResponse[], string>({
        path: `/api/v1/helix-models/memory-estimates`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1KnowledgeList
     * @request GET:/api/v1/knowledge
     * @secure
     */
    v1KnowledgeList: (params: RequestParams = {}) =>
      this.request<TypesKnowledge[], any>({
        path: `/api/v1/knowledge`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Delete knowledge
     *
     * @tags knowledge
     * @name V1KnowledgeDelete
     * @summary Delete knowledge
     * @request DELETE:/api/v1/knowledge/{id}
     * @secure
     */
    v1KnowledgeDelete: (id: string, params: RequestParams = {}) =>
      this.request<TypesKnowledge, any>({
        path: `/api/v1/knowledge/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1KnowledgeDetail
     * @request GET:/api/v1/knowledge/{id}
     */
    v1KnowledgeDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesKnowledge, any>({
        path: `/api/v1/knowledge/${id}`,
        method: "GET",
        ...params,
      }),

    /**
     * @description Complete knowledge preparation and move to pending state for indexing
     *
     * @tags knowledge
     * @name V1KnowledgeCompleteCreate
     * @summary Complete knowledge preparation
     * @request POST:/api/v1/knowledge/{id}/complete
     * @secure
     */
    v1KnowledgeCompleteCreate: (id: string, params: RequestParams = {}) =>
      this.request<TypesKnowledge, any>({
        path: `/api/v1/knowledge/${id}/complete`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Download all files from a filestore-backed knowledge as a zip file
     *
     * @tags knowledge
     * @name V1KnowledgeDownloadDetail
     * @summary Download knowledge files as zip
     * @request GET:/api/v1/knowledge/{id}/download
     * @secure
     */
    v1KnowledgeDownloadDetail: (id: string, params: RequestParams = {}) =>
      this.request<File, any>({
        path: `/api/v1/knowledge/${id}/download`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Refresh knowledge
     *
     * @tags knowledge
     * @name V1KnowledgeRefreshCreate
     * @summary Refresh knowledge
     * @request POST:/api/v1/knowledge/{id}/refresh
     * @secure
     */
    v1KnowledgeRefreshCreate: (id: string, params: RequestParams = {}) =>
      this.request<TypesKnowledge, any>({
        path: `/api/v1/knowledge/${id}/refresh`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description List knowledge versions
     *
     * @tags knowledge
     * @name V1KnowledgeVersionsDetail
     * @summary List knowledge versions
     * @request GET:/api/v1/knowledge/{id}/versions
     * @secure
     */
    v1KnowledgeVersionsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesKnowledgeVersion[], any>({
        path: `/api/v1/knowledge/${id}/versions`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get the license key for the current user
     *
     * @name V1LicenseList
     * @summary Get license key
     * @request GET:/api/v1/license
     * @secure
     */
    v1LicenseList: (params: RequestParams = {}) =>
      this.request<ServerLicenseKeyRequest, any>({
        path: `/api/v1/license`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Set the license key for the current user
     *
     * @name V1LicenseCreate
     * @summary Set license key
     * @request POST:/api/v1/license
     * @secure
     */
    v1LicenseCreate: (params: RequestParams = {}) =>
      this.request<ServerLicenseKeyRequest, any>({
        path: `/api/v1/license`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List LLM calls with pagination and optional session filtering
     *
     * @tags llm_calls
     * @name V1LlmCallsList
     * @summary List LLM calls
     * @request GET:/api/v1/llm_calls
     * @secure
     */
    v1LlmCallsList: (
      query?: {
        /** Page number */
        page?: number;
        /** Page size */
        pageSize?: number;
        /** Filter by session ID */
        session?: string;
        /** Filter by interaction ID */
        interaction?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPaginatedLLMCalls, any>({
        path: `/api/v1/llm_calls`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Retrieve logs for a specific slot by proxying the request to the runner
     *
     * @tags logs
     * @name V1LogsDetail
     * @summary Get logs for a specific slot
     * @request GET:/api/v1/logs/{slot_id}
     * @secure
     */
    v1LogsDetail: (
      slotId: string,
      query?: {
        /** Maximum number of lines to return (default: 500) */
        lines?: number;
        /** Return logs since this timestamp (RFC3339 format) */
        since?: string;
        /** Filter by log level (ERROR, WARN, INFO, DEBUG) */
        level?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<Record<string, any>, string>({
        path: `/api/v1/logs/${slotId}`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List all dynamic model infos. Requires admin privileges.
     *
     * @tags model-info
     * @name V1ModelInfoList
     * @summary List dynamic model infos
     * @request GET:/api/v1/model-info
     * @secure
     */
    v1ModelInfoList: (
      query?: {
        /** Filter by provider (e.g., helix, openai) */
        provider?: string;
        /** Filter by model name */
        name?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesDynamicModelInfo[], string>({
        path: `/api/v1/model-info`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new dynamic model info configuration. Requires admin privileges.
     *
     * @tags model-info
     * @name V1ModelInfoCreate
     * @summary Create a new dynamic model info
     * @request POST:/api/v1/model-info
     * @secure
     */
    v1ModelInfoCreate: (request: TypesDynamicModelInfo, params: RequestParams = {}) =>
      this.request<TypesDynamicModelInfo, string>({
        path: `/api/v1/model-info`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete a dynamic model info configuration. Requires admin privileges.
     *
     * @tags model-info
     * @name V1ModelInfoDelete
     * @summary Delete a dynamic model info
     * @request DELETE:/api/v1/model-info/{id}
     * @secure
     */
    v1ModelInfoDelete: (id: string, params: RequestParams = {}) =>
      this.request<string, string>({
        path: `/api/v1/model-info/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a specific dynamic model info by ID. Requires admin privileges.
     *
     * @tags model-info
     * @name V1ModelInfoDetail
     * @summary Get a dynamic model info by ID
     * @request GET:/api/v1/model-info/{id}
     * @secure
     */
    v1ModelInfoDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesDynamicModelInfo, string>({
        path: `/api/v1/model-info/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Update an existing dynamic model info configuration. Requires admin privileges.
     *
     * @tags model-info
     * @name V1ModelInfoUpdate
     * @summary Update an existing dynamic model info
     * @request PUT:/api/v1/model-info/{id}
     * @secure
     */
    v1ModelInfoUpdate: (id: string, request: TypesDynamicModelInfo, params: RequestParams = {}) =>
      this.request<TypesDynamicModelInfo, string>({
        path: `/api/v1/model-info/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List OAuth connections for the user.
     *
     * @tags oauth
     * @name V1OauthConnectionsList
     * @summary List OAuth connections
     * @request GET:/api/v1/oauth/connections
     * @secure
     */
    v1OauthConnectionsList: (params: RequestParams = {}) =>
      this.request<TypesOAuthConnection[], any>({
        path: `/api/v1/oauth/connections`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Delete an OAuth connection. Users can only delete their own connections unless they are admin.
     *
     * @tags oauth
     * @name V1OauthConnectionsDelete
     * @summary Delete an OAuth connection
     * @request DELETE:/api/v1/oauth/connections/{id}
     * @secure
     */
    v1OauthConnectionsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/oauth/connections/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a specific OAuth connection by ID. Users can only access their own connections unless they are admin.
     *
     * @tags oauth
     * @name V1OauthConnectionsDetail
     * @summary Get an OAuth connection
     * @request GET:/api/v1/oauth/connections/{id}
     * @secure
     */
    v1OauthConnectionsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesOAuthConnection, any>({
        path: `/api/v1/oauth/connections/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Manually refresh an OAuth connection
     *
     * @tags oauth
     * @name V1OauthConnectionsRefreshCreate
     * @summary Refresh an OAuth connection
     * @request POST:/api/v1/oauth/connections/{id}/refresh
     * @secure
     */
    v1OauthConnectionsRefreshCreate: (id: string, params: RequestParams = {}) =>
      this.request<TypesOAuthConnection, any>({
        path: `/api/v1/oauth/connections/${id}/refresh`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List OAuth providers for the user.
     *
     * @tags oauth
     * @name V1OauthProvidersList
     * @summary List OAuth providers
     * @request GET:/api/v1/oauth/providers
     * @secure
     */
    v1OauthProvidersList: (params: RequestParams = {}) =>
      this.request<TypesOAuthProvider[], any>({
        path: `/api/v1/oauth/providers`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new OAuth provider for the user.
     *
     * @tags oauth
     * @name V1OauthProvidersCreate
     * @summary Create a new OAuth provider
     * @request POST:/api/v1/oauth/providers
     * @secure
     */
    v1OauthProvidersCreate: (request: TypesOAuthProvider, params: RequestParams = {}) =>
      this.request<TypesOAuthProvider, any>({
        path: `/api/v1/oauth/providers`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete an existing OAuth provider for the user.
     *
     * @tags oauth
     * @name V1OauthProvidersDelete
     * @summary Delete an OAuth provider
     * @request DELETE:/api/v1/oauth/providers/{id}
     * @secure
     */
    v1OauthProvidersDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/oauth/providers/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1OrganizationsList
     * @request GET:/api/v1/organizations
     * @secure
     */
    v1OrganizationsList: (params: RequestParams = {}) =>
      this.request<TypesOrganization[], any>({
        path: `/api/v1/organizations`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new organization. Only admin users can create organizations.
     *
     * @tags organizations
     * @name V1OrganizationsCreate
     * @summary Create a new organization
     * @request POST:/api/v1/organizations
     * @secure
     */
    v1OrganizationsCreate: (request: TypesOrganization, params: RequestParams = {}) =>
      this.request<TypesOrganization, any>({
        path: `/api/v1/organizations`,
        method: "POST",
        body: request,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1OrganizationsDelete
     * @request DELETE:/api/v1/organizations/{id}
     * @secure
     */
    v1OrganizationsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/organizations/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1OrganizationsDetail
     * @request GET:/api/v1/organizations/{id}
     */
    v1OrganizationsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesOrganization, any>({
        path: `/api/v1/organizations/${id}`,
        method: "GET",
        ...params,
      }),

    /**
     * @description Update an organization, must be an owner of the organization
     *
     * @tags organizations
     * @name V1OrganizationsUpdate
     * @summary Update an organization
     * @request PUT:/api/v1/organizations/{id}
     * @secure
     */
    v1OrganizationsUpdate: (id: string, request: TypesOrganization, params: RequestParams = {}) =>
      this.request<TypesOrganization, any>({
        path: `/api/v1/organizations/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        ...params,
      }),

    /**
     * @description List members of an organization
     *
     * @tags organizations
     * @name V1OrganizationsMembersDetail
     * @summary List organization members
     * @request GET:/api/v1/organizations/{id}/members
     * @secure
     */
    v1OrganizationsMembersDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesOrganizationMembership[], any>({
        path: `/api/v1/organizations/${id}/members`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Add a member to an organization
     *
     * @tags organizations
     * @name V1OrganizationsMembersCreate
     * @summary Add an organization member
     * @request POST:/api/v1/organizations/{id}/members
     * @secure
     */
    v1OrganizationsMembersCreate: (
      id: string,
      request: TypesAddOrganizationMemberRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesOrganizationMembership, any>({
        path: `/api/v1/organizations/${id}/members`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Remove a member from an organization
     *
     * @tags organizations
     * @name V1OrganizationsMembersDelete
     * @summary Remove an organization member
     * @request DELETE:/api/v1/organizations/{id}/members/{user_id}
     * @secure
     */
    v1OrganizationsMembersDelete: (id: string, userId: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/organizations/${id}/members/${userId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Update a member's role in an organization
     *
     * @tags organizations
     * @name V1OrganizationsMembersUpdate
     * @summary Update an organization member
     * @request PUT:/api/v1/organizations/{id}/members/{user_id}
     * @secure
     */
    v1OrganizationsMembersUpdate: (
      id: string,
      userId: string,
      request: TypesUpdateOrganizationMemberRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesOrganizationMembership, any>({
        path: `/api/v1/organizations/${id}/members/${userId}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List all roles in an organization. Organization members can list roles.
     *
     * @tags organizations
     * @name V1OrganizationsRolesDetail
     * @summary List roles in an organization
     * @request GET:/api/v1/organizations/{id}/roles
     * @secure
     */
    v1OrganizationsRolesDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesRole[], any>({
        path: `/api/v1/organizations/${id}/roles`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description List all teams in an organization. Organization members can list teams.
     *
     * @tags organizations
     * @name V1OrganizationsTeamsDetail
     * @summary List teams in an organization
     * @request GET:/api/v1/organizations/{id}/teams
     * @secure
     */
    v1OrganizationsTeamsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesTeam[], any>({
        path: `/api/v1/organizations/${id}/teams`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new team in an organization. Only organization owners can create teams.
     *
     * @tags organizations
     * @name V1OrganizationsTeamsCreate
     * @summary Create a new team
     * @request POST:/api/v1/organizations/{id}/teams
     * @secure
     */
    v1OrganizationsTeamsCreate: (id: string, request: TypesCreateTeamRequest, params: RequestParams = {}) =>
      this.request<TypesTeam, any>({
        path: `/api/v1/organizations/${id}/teams`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a team from an organization. Only organization owners can delete teams.
     *
     * @tags organizations
     * @name V1OrganizationsTeamsDelete
     * @summary Delete a team
     * @request DELETE:/api/v1/organizations/{id}/teams/{team_id}
     * @secure
     */
    v1OrganizationsTeamsDelete: (id: string, teamId: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/organizations/${id}/teams/${teamId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Update a team's details. Only organization owners can update teams.
     *
     * @tags organizations
     * @name V1OrganizationsTeamsUpdate
     * @summary Update a team
     * @request PUT:/api/v1/organizations/{id}/teams/{team_id}
     * @secure
     */
    v1OrganizationsTeamsUpdate: (
      id: string,
      teamId: string,
      request: TypesUpdateTeamRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesTeam, any>({
        path: `/api/v1/organizations/${id}/teams/${teamId}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List all members of a team.
     *
     * @tags organizations
     * @name V1OrganizationsTeamsMembersDetail
     * @summary List members of a team
     * @request GET:/api/v1/organizations/{id}/teams/{team_id}/members
     * @secure
     */
    v1OrganizationsTeamsMembersDetail: (id: string, teamId: string, params: RequestParams = {}) =>
      this.request<TypesTeamMembership[], any>({
        path: `/api/v1/organizations/${id}/teams/${teamId}/members`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Add a new member to a team. Only organization owners can add members to teams.
     *
     * @tags organizations
     * @name V1OrganizationsTeamsMembersCreate
     * @summary Add a new member to a team
     * @request POST:/api/v1/organizations/{id}/teams/{team_id}/members
     * @secure
     */
    v1OrganizationsTeamsMembersCreate: (
      id: string,
      teamId: string,
      request: TypesAddTeamMemberRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesTeamMembership, any>({
        path: `/api/v1/organizations/${id}/teams/${teamId}/members`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all personal development environments for the current user
     *
     * @tags PersonalDevEnvironments
     * @name V1PersonalDevEnvironmentsList
     * @summary List personal development environments
     * @request GET:/api/v1/personal-dev-environments
     * @secure
     */
    v1PersonalDevEnvironmentsList: (params: RequestParams = {}) =>
      this.request<ServerPersonalDevEnvironmentResponse[], SystemHTTPError>({
        path: `/api/v1/personal-dev-environments`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new personal development environment with the specified configuration
     *
     * @tags PersonalDevEnvironments
     * @name V1PersonalDevEnvironmentsCreate
     * @summary Create a personal development environment
     * @request POST:/api/v1/personal-dev-environments
     * @secure
     */
    v1PersonalDevEnvironmentsCreate: (request: ServerCreatePersonalDevEnvironmentRequest, params: RequestParams = {}) =>
      this.request<ServerPersonalDevEnvironmentResponse, SystemHTTPError>({
        path: `/api/v1/personal-dev-environments`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a personal development environment by ID
     *
     * @tags PersonalDevEnvironments
     * @name V1PersonalDevEnvironmentsDelete
     * @summary Delete a personal development environment
     * @request DELETE:/api/v1/personal-dev-environments/{environmentID}
     * @secure
     */
    v1PersonalDevEnvironmentsDelete: (environmentId: string, params: RequestParams = {}) =>
      this.request<void, SystemHTTPError>({
        path: `/api/v1/personal-dev-environments/${environmentId}`,
        method: "DELETE",
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Start a personal development environment by ID
     *
     * @tags PersonalDevEnvironments
     * @name V1PersonalDevEnvironmentsStartCreate
     * @summary Start a personal development environment
     * @request POST:/api/v1/personal-dev-environments/{environmentID}/start
     * @secure
     */
    v1PersonalDevEnvironmentsStartCreate: (environmentId: string, params: RequestParams = {}) =>
      this.request<ServerPersonalDevEnvironmentResponse, SystemHTTPError>({
        path: `/api/v1/personal-dev-environments/${environmentId}/start`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Stop a personal development environment by ID
     *
     * @tags PersonalDevEnvironments
     * @name V1PersonalDevEnvironmentsStopCreate
     * @summary Stop a personal development environment
     * @request POST:/api/v1/personal-dev-environments/{environmentID}/stop
     * @secure
     */
    v1PersonalDevEnvironmentsStopCreate: (environmentId: string, params: RequestParams = {}) =>
      this.request<ServerPersonalDevEnvironmentResponse, SystemHTTPError>({
        path: `/api/v1/personal-dev-environments/${environmentId}/stop`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @name V1ProviderEndpointsList
     * @request GET:/api/v1/provider-endpoints
     * @secure
     */
    v1ProviderEndpointsList: (
      query?: {
        /** Include models */
        with_models?: boolean;
        /** Organization ID */
        org_id?: string;
        /** Include all endpoints (system admin only) */
        all?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesProviderEndpoint[], any>({
        path: `/api/v1/provider-endpoints`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1ProviderEndpointsCreate
     * @request POST:/api/v1/provider-endpoints
     * @secure
     */
    v1ProviderEndpointsCreate: (params: RequestParams = {}) =>
      this.request<TypesProviderEndpoint, any>({
        path: `/api/v1/provider-endpoints`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1ProviderEndpointsDelete
     * @request DELETE:/api/v1/provider-endpoints/{id}
     * @secure
     */
    v1ProviderEndpointsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/provider-endpoints/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1ProviderEndpointsUpdate
     * @request PUT:/api/v1/provider-endpoints/{id}
     * @secure
     */
    v1ProviderEndpointsUpdate: (id: string, params: RequestParams = {}) =>
      this.request<TypesUpdateProviderEndpoint, any>({
        path: `/api/v1/provider-endpoints/${id}`,
        method: "PUT",
        secure: true,
        ...params,
      }),

    /**
     * @description Get provider daily usage
     *
     * @tags providers
     * @name V1ProviderEndpointsDailyUsageDetail
     * @summary Get provider daily usage
     * @request GET:/api/v1/provider-endpoints/{id}/daily-usage
     * @secure
     */
    v1ProviderEndpointsDailyUsageDetail: (
      id: string,
      query?: {
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/provider-endpoints/${id}/daily-usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get provider daily usage per user
     *
     * @tags providers
     * @name V1ProviderEndpointsUsersDailyUsageDetail
     * @summary Get provider daily usage per user
     * @request GET:/api/v1/provider-endpoints/{id}/users-daily-usage
     * @secure
     */
    v1ProviderEndpointsUsersDailyUsageDetail: (
      id: string,
      query?: {
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesUsersAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/provider-endpoints/${id}/users-daily-usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @name V1ProvidersList
     * @request GET:/api/v1/providers
     * @secure
     */
    v1ProvidersList: (params: RequestParams = {}) =>
      this.request<TypesProvider[], any>({
        path: `/api/v1/providers`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a list of all available sample projects that users can fork and use
     *
     * @tags sample-projects
     * @name V1SampleProjectsList
     * @summary List available sample projects
     * @request GET:/api/v1/sample-projects
     * @secure
     */
    v1SampleProjectsList: (params: RequestParams = {}) =>
      this.request<ServerSampleProject[], any>({
        path: `/api/v1/sample-projects`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get details of a specific sample project by ID
     *
     * @tags sample-projects
     * @name V1SampleProjectsDetail
     * @summary Get a specific sample project
     * @request GET:/api/v1/sample-projects/{project_id}
     * @secure
     */
    v1SampleProjectsDetail: (projectId: string, params: RequestParams = {}) =>
      this.request<ServerSampleProject, any>({
        path: `/api/v1/sample-projects/${projectId}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get all files for a sample project as a flat map (for container initialization)
     *
     * @tags sample-projects
     * @name V1SampleProjectsArchiveDetail
     * @summary Get sample project code as archive
     * @request GET:/api/v1/sample-projects/{projectId}/archive
     */
    v1SampleProjectsArchiveDetail: (projectId: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/sample-projects/${projectId}/archive`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Get the starter code and file structure for a sample project
     *
     * @tags sample-projects
     * @name V1SampleProjectsCodeDetail
     * @summary Get sample project starter code
     * @request GET:/api/v1/sample-projects/{projectId}/code
     */
    v1SampleProjectsCodeDetail: (projectId: string, params: RequestParams = {}) =>
      this.request<ServicesSampleProjectCode, TypesAPIError>({
        path: `/api/v1/sample-projects/${projectId}/code`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Fork a sample project to the user's GitHub account and create a new Helix project
     *
     * @tags sample-projects
     * @name V1SampleProjectsForkCreate
     * @summary Fork a sample project
     * @request POST:/api/v1/sample-projects/fork
     * @secure
     */
    v1SampleProjectsForkCreate: (request: ServerForkSampleProjectRequest, params: RequestParams = {}) =>
      this.request<ServerForkSampleProjectResponse, any>({
        path: `/api/v1/sample-projects/fork`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Get sample projects with natural language task prompts (Kiro-style)
     *
     * @tags sample-projects
     * @name V1SampleProjectsSimpleList
     * @summary List simple sample projects
     * @request GET:/api/v1/sample-projects/simple
     * @secure
     */
    v1SampleProjectsSimpleList: (params: RequestParams = {}) =>
      this.request<ServerSimpleSampleProject[], any>({
        path: `/api/v1/sample-projects/simple`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Fork a sample project and create tasks from natural language prompts
     *
     * @tags sample-projects
     * @name V1SampleProjectsSimpleForkCreate
     * @summary Fork a simple sample project
     * @request POST:/api/v1/sample-projects/simple/fork
     * @secure
     */
    v1SampleProjectsSimpleForkCreate: (request: ServerForkSimpleProjectRequest, params: RequestParams = {}) =>
      this.request<ServerForkSimpleProjectResponse, any>({
        path: `/api/v1/sample-projects/simple/fork`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Create multiple sample repositories for development/testing
     *
     * @tags samples
     * @name V1SamplesInitializeCreate
     * @summary Initialize sample repositories
     * @request POST:/api/v1/samples/initialize
     * @secure
     */
    v1SamplesInitializeCreate: (request: ServerInitializeSampleRepositoriesRequest, params: RequestParams = {}) =>
      this.request<ServerInitializeSampleRepositoriesResponse, TypesAPIError>({
        path: `/api/v1/samples/initialize`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a sample/demo git repository from available templates
     *
     * @tags samples
     * @name V1SamplesRepositoriesCreate
     * @summary Create sample repository
     * @request POST:/api/v1/samples/repositories
     * @secure
     */
    v1SamplesRepositoriesCreate: (request: ServerCreateSampleRepositoryRequest, params: RequestParams = {}) =>
      this.request<ServicesGitRepository, TypesAPIError>({
        path: `/api/v1/samples/repositories`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the health status of all scheduler goroutines
     *
     * @tags dashboard
     * @name V1SchedulerHeartbeatsList
     * @summary Get scheduler goroutine heartbeat status
     * @request GET:/api/v1/scheduler/heartbeats
     * @secure
     */
    v1SchedulerHeartbeatsList: (params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/scheduler/heartbeats`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Search knowledges for a given app and prompt
     *
     * @tags knowledge
     * @name V1SearchList
     * @summary Search knowledges
     * @request GET:/api/v1/search
     * @secure
     */
    v1SearchList: (
      query: {
        /** App ID */
        app_id: string;
        /** Knowledge ID */
        knowledge_id?: string;
        /** Search prompt */
        prompt: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesKnowledgeSearchResult[], any>({
        path: `/api/v1/search`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description List secrets for the user.
     *
     * @tags secrets
     * @name V1SecretsList
     * @summary List secrets
     * @request GET:/api/v1/secrets
     * @secure
     */
    v1SecretsList: (params: RequestParams = {}) =>
      this.request<TypesSecret[], any>({
        path: `/api/v1/secrets`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new secret for the user.
     *
     * @tags secrets
     * @name V1SecretsCreate
     * @summary Create new secret
     * @request POST:/api/v1/secrets
     * @secure
     */
    v1SecretsCreate: (request: TypesSecret, params: RequestParams = {}) =>
      this.request<TypesSecret, any>({
        path: `/api/v1/secrets`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete a secret for the user.
     *
     * @tags secrets
     * @name V1SecretsDelete
     * @summary Delete a secret
     * @request DELETE:/api/v1/secrets/{id}
     * @secure
     */
    v1SecretsDelete: (id: string, params: RequestParams = {}) =>
      this.request<TypesSecret, any>({
        path: `/api/v1/secrets/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Update an existing secret for the user.
     *
     * @tags secrets
     * @name V1SecretsUpdate
     * @summary Update an existing secret
     * @request PUT:/api/v1/secrets/{id}
     * @secure
     */
    v1SecretsUpdate: (id: string, request: TypesSecret, params: RequestParams = {}) =>
      this.request<TypesSecret, any>({
        path: `/api/v1/secrets/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List sessions
     *
     * @tags sessions
     * @name V1SessionsList
     * @summary List sessions
     * @request GET:/api/v1/sessions
     * @secure
     */
    v1SessionsList: (
      query?: {
        /** Page number */
        page?: number;
        /** Page size */
        page_size?: number;
        /** Organization slug or ID */
        org_id?: string;
        /** Search sessions by name */
        search?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPaginatedSessionsList, any>({
        path: `/api/v1/sessions`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Delete a session by ID
     *
     * @tags sessions
     * @name V1SessionsDelete
     * @summary Delete a session by ID
     * @request DELETE:/api/v1/sessions/{id}
     * @secure
     */
    v1SessionsDelete: (id: string, params: RequestParams = {}) =>
      this.request<TypesSession, any>({
        path: `/api/v1/sessions/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a session by ID
     *
     * @tags sessions
     * @name V1SessionsDetail
     * @summary Get a session by ID
     * @request GET:/api/v1/sessions/{id}
     * @secure
     */
    v1SessionsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesSession, any>({
        path: `/api/v1/sessions/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Update a session by ID
     *
     * @tags sessions
     * @name V1SessionsUpdate
     * @summary Update a session by ID
     * @request PUT:/api/v1/sessions/{id}
     * @secure
     */
    v1SessionsUpdate: (id: string, request: TypesSession, params: RequestParams = {}) =>
      this.request<TypesSession, any>({
        path: `/api/v1/sessions/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List interactions for a session
     *
     * @tags interactions
     * @name V1SessionsInteractionsDetail
     * @summary List interactions for a session
     * @request GET:/api/v1/sessions/{id}/interactions
     * @secure
     */
    v1SessionsInteractionsDetail: (
      id: string,
      query?: {
        /** Page number */
        page?: number;
        /** Page size */
        page_size?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesInteraction[], any>({
        path: `/api/v1/sessions/${id}/interactions`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get an interaction by ID
     *
     * @tags interactions
     * @name V1SessionsInteractionsDetail2
     * @summary Get an interaction by ID
     * @request GET:/api/v1/sessions/{id}/interactions/{interaction_id}
     * @originalName v1SessionsInteractionsDetail
     * @duplicate
     * @secure
     */
    v1SessionsInteractionsDetail2: (id: string, interactionId: string, params: RequestParams = {}) =>
      this.request<TypesInteraction, any>({
        path: `/api/v1/sessions/${id}/interactions/${interactionId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Provide feedback for an interaction
     *
     * @tags interactions
     * @name V1SessionsInteractionsFeedbackCreate
     * @summary Provide feedback for an interaction
     * @request POST:/api/v1/sessions/{id}/interactions/{interaction_id}/feedback
     * @secure
     */
    v1SessionsInteractionsFeedbackCreate: (
      id: string,
      interactionId: string,
      feedback: TypesFeedbackRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesInteraction, any>({
        path: `/api/v1/sessions/${id}/interactions/${interactionId}/feedback`,
        method: "POST",
        body: feedback,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get Wolf streaming connection details for accessing a session (replaces RDP)
     *
     * @tags sessions
     * @name V1SessionsRdpConnectionDetail
     * @summary Get Wolf connection info for a session
     * @request GET:/api/v1/sessions/{id}/rdp-connection
     * @secure
     */
    v1SessionsRdpConnectionDetail: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/sessions/${id}/rdp-connection`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @name V1SessionsStepInfoDetail
     * @request GET:/api/v1/sessions/{id}/step-info
     * @secure
     */
    v1SessionsStepInfoDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesStepInfo[], any>({
        path: `/api/v1/sessions/${id}/step-info`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Returns the current Wolf app state for an external agent session (absent/running/resumable)
     *
     * @tags Sessions
     * @name V1SessionsWolfAppStateDetail
     * @summary Get Wolf app state for a session
     * @request GET:/api/v1/sessions/{id}/wolf-app-state
     * @secure
     */
    v1SessionsWolfAppStateDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerSessionWolfAppStateResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/wolf-app-state`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get Helix-managed Zed MCP configuration for a session
     *
     * @tags Zed
     * @name V1SessionsZedConfigDetail
     * @summary Get Zed configuration
     * @request GET:/api/v1/sessions/{id}/zed-config
     * @secure
     */
    v1SessionsZedConfigDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesZedConfigResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/zed-config`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Update user's custom Zed settings overrides
     *
     * @tags Zed
     * @name V1SessionsZedConfigUserCreate
     * @summary Update Zed user settings
     * @request POST:/api/v1/sessions/{id}/zed-config/user
     * @secure
     */
    v1SessionsZedConfigUserCreate: (id: string, overrides: Record<string, any>, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/zed-config/user`,
        method: "POST",
        body: overrides,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get merged Helix + user Zed settings for a session
     *
     * @tags Zed
     * @name V1SessionsZedSettingsDetail
     * @summary Get merged Zed settings
     * @request GET:/api/v1/sessions/{id}/zed-settings
     * @secure
     */
    v1SessionsZedSettingsDetail: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/zed-settings`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @name V1SessionsChatCreate
     * @request POST:/api/v1/sessions/chat
     * @secure
     */
    v1SessionsChatCreate: (request: TypesSessionChatRequest, params: RequestParams = {}) =>
      this.request<TypesOpenAIResponse, any>({
        path: `/api/v1/sessions/chat`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List all available YAML-based skills
     *
     * @tags skills
     * @name V1SkillsList
     * @summary List YAML skills
     * @request GET:/api/v1/skills
     * @secure
     */
    v1SkillsList: (
      query?: {
        /** Filter by category */
        category?: string;
        /** Filter by OAuth provider */
        provider?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSkillsListResponse, any>({
        path: `/api/v1/skills`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get details of a specific YAML skill
     *
     * @tags skills
     * @name V1SkillsDetail
     * @summary Get a skill by ID
     * @request GET:/api/v1/skills/{id}
     * @secure
     */
    v1SkillsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesSkillDefinition, any>({
        path: `/api/v1/skills/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Reload all YAML skills from the filesystem
     *
     * @tags skills
     * @name V1SkillsReloadCreate
     * @summary Reload skills
     * @request POST:/api/v1/skills/reload
     * @secure
     */
    v1SkillsReloadCreate: (params: RequestParams = {}) =>
      this.request<Record<string, string>, any>({
        path: `/api/v1/skills/reload`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Validate MCP skill configuration
     *
     * @tags skills
     * @name V1SkillsValidateCreate
     * @summary Validate MCP skill configuration
     * @request POST:/api/v1/skills/validate
     * @secure
     */
    v1SkillsValidateCreate: (request: TypesAssistantMCP, params: RequestParams = {}) =>
      this.request<TypesToolMCPClientConfig, any>({
        path: `/api/v1/skills/validate`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete a slot from the scheduler's desired state, allowing reconciliation to clean it up from the runner
     *
     * @tags dashboard
     * @name V1SlotsDelete
     * @summary Delete a slot from scheduler state
     * @request DELETE:/api/v1/slots/{slot_id}
     * @secure
     */
    v1SlotsDelete: (slotId: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/slots/${slotId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description List spec-driven tasks with optional filtering by project, status, or user
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksList
     * @summary List spec-driven tasks
     * @request GET:/api/v1/spec-tasks
     */
    v1SpecTasksList: (
      query?: {
        /** Filter by project ID */
        project_id?: string;
        /** Filter by status */
        status?: string;
        /** Filter by user ID */
        user_id?: string;
        /**
         * Limit number of results
         * @default 50
         */
        limit?: number;
        /**
         * Offset for pagination
         * @default 0
         */
        offset?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTask[], TypesAPIError>({
        path: `/api/v1/spec-tasks`,
        method: "GET",
        query: query,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the design documents from helix-design-docs worktree
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignDocsDetail
     * @summary Get design docs for SpecTask
     * @request GET:/api/v1/spec-tasks/{id}/design-docs
     * @secure
     */
    v1SpecTasksDesignDocsDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerDesignDocsResponse, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${id}/design-docs`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Generate a token-based shareable link for viewing design documents on any device
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignDocsShareCreate
     * @summary Generate shareable design docs link
     * @request POST:/api/v1/spec-tasks/{id}/design-docs/share
     * @secure
     */
    v1SpecTasksDesignDocsShareCreate: (id: string, params: RequestParams = {}) =>
      this.request<ServerDesignDocsShareLinkResponse, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${id}/design-docs/share`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Start or resume the external agent for a SpecTask (allocates GPU)
     *
     * @tags SpecTasks
     * @name V1SpecTasksExternalAgentStartCreate
     * @summary Start SpecTask external agent
     * @request POST:/api/v1/spec-tasks/{id}/external-agent/start
     * @secure
     */
    v1SpecTasksExternalAgentStartCreate: (id: string, params: RequestParams = {}) =>
      this.request<string, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${id}/external-agent/start`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Get the current status and info for a SpecTask's external agent
     *
     * @tags SpecTasks
     * @name V1SpecTasksExternalAgentStatusDetail
     * @summary Get SpecTask external agent status
     * @request GET:/api/v1/spec-tasks/{id}/external-agent/status
     * @secure
     */
    v1SpecTasksExternalAgentStatusDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerSpecTaskExternalAgentStatusResponse, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${id}/external-agent/status`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Manually stop the external agent for a SpecTask (frees GPU)
     *
     * @tags SpecTasks
     * @name V1SpecTasksExternalAgentStopCreate
     * @summary Stop SpecTask external agent
     * @request POST:/api/v1/spec-tasks/{id}/external-agent/stop
     * @secure
     */
    v1SpecTasksExternalAgentStopCreate: (id: string, params: RequestParams = {}) =>
      this.request<string, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${id}/external-agent/stop`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Get detailed information about a specific spec-driven task
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksDetail
     * @summary Get spec-driven task details
     * @request GET:/api/v1/spec-tasks/{taskId}
     */
    v1SpecTasksDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Update SpecTask status, priority, or other fields
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksUpdate
     * @summary Update SpecTask
     * @request PUT:/api/v1/spec-tasks/{taskId}
     * @secure
     */
    v1SpecTasksUpdate: (taskId: string, request: TypesSpecTaskUpdateRequest, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Human approval/rejection of specs generated by AI agent
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksApproveSpecsCreate
     * @summary Approve or reject generated specifications
     * @request POST:/api/v1/spec-tasks/{taskId}/approve-specs
     */
    v1SpecTasksApproveSpecsCreate: (taskId: string, request: TypesSpecApprovalResponse, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/approve-specs`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Combined endpoint that approves specifications and immediately executes document handoff workflow
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksApproveWithHandoffCreate
     * @summary Approve specs and execute document handoff
     * @request POST:/api/v1/spec-tasks/{taskId}/approve-with-handoff
     * @secure
     */
    v1SpecTasksApproveWithHandoffCreate: (
      taskId: string,
      request: ServerApprovalWithHandoffRequest,
      params: RequestParams = {},
    ) =>
      this.request<ServerCombinedApprovalHandoffResult, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/approve-with-handoff`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Commit a progress update to git with current implementation status
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksCommitProgressCreate
     * @summary Commit implementation progress update
     * @request POST:/api/v1/spec-tasks/{taskId}/commit-progress
     * @secure
     */
    v1SpecTasksCommitProgressCreate: (taskId: string, request: Record<string, any>, params: RequestParams = {}) =>
      this.request<ServicesHandoffResult, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/commit-progress`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the coordination log showing inter-session communication for a SpecTask
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksCoordinationLogDetail
     * @summary Get coordination log for SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/coordination-log
     * @secure
     */
    v1SpecTasksCoordinationLogDetail: (
      taskId: string,
      query?: {
        /**
         * Limit number of events
         * @default 100
         */
        limit?: number;
        /** Filter by event type */
        event_type?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerCoordinationLogResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/coordination-log`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the current status of document handoff and git integration for a SpecTask
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksDocumentStatusDetail
     * @summary Get document handoff status for SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/document-status
     * @secure
     */
    v1SpecTasksDocumentStatusDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<ServicesDocumentHandoffStatus, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/document-status`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Retrieve the content of a specific spec document (requirements.md, design.md, or tasks.md)
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksDocumentsDetail
     * @summary Get specific spec document content
     * @request GET:/api/v1/spec-tasks/{taskId}/documents/{document}
     * @secure
     */
    v1SpecTasksDocumentsDetail: (
      taskId: string,
      document: "requirements" | "design" | "tasks" | "metadata",
      params: RequestParams = {},
    ) =>
      this.request<ServerSpecDocumentContentResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/documents/${document}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Download the generated spec documents as a zip file or individual files
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksDownloadDocumentsDetail
     * @summary Download generated spec documents
     * @request GET:/api/v1/spec-tasks/{taskId}/download-documents
     * @secure
     */
    v1SpecTasksDownloadDocumentsDetail: (
      taskId: string,
      query?: {
        /**
         * Download format
         * @default "zip"
         */
        format?: "zip" | "individual";
      },
      params: RequestParams = {},
    ) =>
      this.request<File, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/download-documents`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Execute the complete document handoff when specs are approved, including git commit and implementation start
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksExecuteHandoffCreate
     * @summary Execute complete document handoff workflow
     * @request POST:/api/v1/spec-tasks/{taskId}/execute-handoff
     * @secure
     */
    v1SpecTasksExecuteHandoffCreate: (
      taskId: string,
      request: ServicesDocumentHandoffConfig,
      params: RequestParams = {},
    ) =>
      this.request<ServicesHandoffResult, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/execute-handoff`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Generate Kiro-style spec documents (requirements.md, design.md, tasks.md) and commit to git repository
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksGenerateDocumentsCreate
     * @summary Generate and commit spec documents to git
     * @request POST:/api/v1/spec-tasks/{taskId}/generate-documents
     * @secure
     */
    v1SpecTasksGenerateDocumentsCreate: (
      taskId: string,
      request: ServicesSpecDocumentConfig,
      params: RequestParams = {},
    ) =>
      this.request<ServicesSpecDocumentResult, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/generate-documents`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create multiple work sessions from an approved SpecTask implementation plan
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksImplementationSessionsCreate
     * @summary Create implementation sessions for a SpecTask
     * @request POST:/api/v1/spec-tasks/{taskId}/implementation-sessions
     * @secure
     */
    v1SpecTasksImplementationSessionsCreate: (
      taskId: string,
      request: TypesSpecTaskImplementationSessionsCreateRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskMultiSessionOverviewResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/implementation-sessions`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all parsed implementation tasks from a SpecTask's implementation plan
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksImplementationTasksDetail
     * @summary List implementation tasks for a SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/implementation-tasks
     * @secure
     */
    v1SpecTasksImplementationTasksDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskImplementationTaskListResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/implementation-tasks`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get comprehensive overview of all work sessions and progress for a SpecTask
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksMultiSessionOverviewDetail
     * @summary Get multi-session overview for a SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/multi-session-overview
     * @secure
     */
    v1SpecTasksMultiSessionOverviewDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskMultiSessionOverviewResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/multi-session-overview`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get detailed progress information including phase progress and implementation task status
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksProgressDetail
     * @summary Get detailed progress for a SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/progress
     * @secure
     */
    v1SpecTasksProgressDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskProgressResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/progress`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the generated specifications for human review
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksSpecsDetail
     * @summary Get task specifications for review
     * @request GET:/api/v1/spec-tasks/{taskId}/specs
     */
    v1SpecTasksSpecsDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<ServerTaskSpecsResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/specs`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Get all work sessions associated with a specific SpecTask
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksWorkSessionsDetail
     * @summary List work sessions for a SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/work-sessions
     * @secure
     */
    v1SpecTasksWorkSessionsDetail: (
      taskId: string,
      query?: {
        /** Filter by phase */
        phase?: "planning" | "implementation" | "validation";
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskWorkSessionListResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/work-sessions`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Manually shutdown a Zed instance and all its threads for a SpecTask
     *
     * @tags zed-integration
     * @name V1SpecTasksZedInstanceDelete
     * @summary Shutdown Zed instance for a SpecTask
     * @request DELETE:/api/v1/spec-tasks/{taskId}/zed-instance
     * @secure
     */
    v1SpecTasksZedInstanceDelete: (taskId: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/zed-instance`,
        method: "DELETE",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the current status and information about a Zed instance associated with a SpecTask
     *
     * @tags zed-integration
     * @name V1SpecTasksZedInstanceDetail
     * @summary Get Zed instance status for a SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/zed-instance
     * @secure
     */
    v1SpecTasksZedInstanceDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesZedInstanceStatus, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/zed-instance`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all Zed threads associated with a SpecTask
     *
     * @tags zed-integration
     * @name V1SpecTasksZedThreadsDetail
     * @summary List Zed threads for a SpecTask
     * @request GET:/api/v1/spec-tasks/{taskId}/zed-threads
     * @secure
     */
    v1SpecTasksZedThreadsDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskZedThreadListResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/zed-threads`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new SpecTask with a demo repository
     *
     * @tags SpecTasks
     * @name V1SpecTasksFromDemoCreate
     * @summary Create SpecTask from demo repo
     * @request POST:/api/v1/spec-tasks/from-demo
     * @secure
     */
    v1SpecTasksFromDemoCreate: (request: ServerCreateSpecTaskFromDemoRequest, params: RequestParams = {}) =>
      this.request<TypesSpecTask, SystemHTTPError>({
        path: `/api/v1/spec-tasks/from-demo`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new task from a simple description and start spec generation
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksFromPromptCreate
     * @summary Create spec-driven task from simple prompt
     * @request POST:/api/v1/spec-tasks/from-prompt
     */
    v1SpecTasksFromPromptCreate: (request: ServicesCreateTaskRequest, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/from-prompt`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a git repository specifically for a SpecTask
     *
     * @tags specs
     * @name V1SpecsRepositoriesCreate
     * @summary Create SpecTask repository
     * @request POST:/api/v1/specs/repositories
     * @secure
     */
    v1SpecsRepositoriesCreate: (request: ServerCreateSpecTaskRepositoryRequest, params: RequestParams = {}) =>
      this.request<ServicesGitRepository, TypesAPIError>({
        path: `/api/v1/specs/repositories`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get list of available sample repository types and templates
     *
     * @tags specs
     * @name V1SpecsSampleTypesList
     * @summary Get sample repository types
     * @request GET:/api/v1/specs/sample-types
     * @secure
     */
    v1SpecsSampleTypesList: (params: RequestParams = {}) =>
      this.request<ServerSampleTypesResponse, any>({
        path: `/api/v1/specs/sample-types`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all SSH keys for the current user
     *
     * @tags SSHKeys
     * @name V1SshKeysList
     * @summary List SSH keys
     * @request GET:/api/v1/ssh-keys
     * @secure
     */
    v1SshKeysList: (params: RequestParams = {}) =>
      this.request<TypesSSHKeyResponse[], SystemHTTPError>({
        path: `/api/v1/ssh-keys`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new SSH key for git operations
     *
     * @tags SSHKeys
     * @name V1SshKeysCreate
     * @summary Create SSH key
     * @request POST:/api/v1/ssh-keys
     * @secure
     */
    v1SshKeysCreate: (request: TypesSSHKeyCreateRequest, params: RequestParams = {}) =>
      this.request<TypesSSHKeyResponse, SystemHTTPError>({
        path: `/api/v1/ssh-keys`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete an SSH key
     *
     * @tags SSHKeys
     * @name V1SshKeysDelete
     * @summary Delete SSH key
     * @request DELETE:/api/v1/ssh-keys/{id}
     * @secure
     */
    v1SshKeysDelete: (id: string, params: RequestParams = {}) =>
      this.request<TypesSSHKeyResponse, SystemHTTPError>({
        path: `/api/v1/ssh-keys/${id}`,
        method: "DELETE",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Generate a new SSH key pair
     *
     * @tags SSHKeys
     * @name V1SshKeysGenerateCreate
     * @summary Generate SSH key
     * @request POST:/api/v1/ssh-keys/generate
     * @secure
     */
    v1SshKeysGenerateCreate: (request: TypesSSHKeyGenerateRequest, params: RequestParams = {}) =>
      this.request<TypesSSHKeyGenerateResponse, SystemHTTPError>({
        path: `/api/v1/ssh-keys/generate`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Manage a subscription
     *
     * @tags wallets
     * @name V1SubscriptionManageCreate
     * @summary Manage a subscription
     * @request POST:/api/v1/subscription/manage
     * @secure
     */
    v1SubscriptionManageCreate: (
      query?: {
        /** Organization ID */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<string, any>({
        path: `/api/v1/subscription/manage`,
        method: "POST",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create a subscription
     *
     * @tags wallets
     * @name V1SubscriptionNewCreate
     * @summary Create a subscription
     * @request POST:/api/v1/subscription/new
     * @secure
     */
    v1SubscriptionNewCreate: (
      query?: {
        /** Organization ID */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<string, any>({
        path: `/api/v1/subscription/new`,
        method: "POST",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get global system settings. Requires admin privileges.
     *
     * @tags system
     * @name V1SystemSettingsList
     * @summary Get system settings
     * @request GET:/api/v1/system/settings
     * @secure
     */
    v1SystemSettingsList: (params: RequestParams = {}) =>
      this.request<TypesSystemSettingsResponse, string>({
        path: `/api/v1/system/settings`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Update global system settings. Requires admin privileges.
     *
     * @tags system
     * @name V1SystemSettingsUpdate
     * @summary Update system settings
     * @request PUT:/api/v1/system/settings
     * @secure
     */
    v1SystemSettingsUpdate: (request: TypesSystemSettingsRequest, params: RequestParams = {}) =>
      this.request<TypesSystemSettingsResponse, string>({
        path: `/api/v1/system/settings`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Create a top up with specified amount
     *
     * @tags wallets
     * @name V1TopUpsNewCreate
     * @summary Create a top up
     * @request POST:/api/v1/top-ups/new
     * @secure
     */
    v1TopUpsNewCreate: (request: ServerCreateTopUpRequest, params: RequestParams = {}) =>
      this.request<string, any>({
        path: `/api/v1/top-ups/new`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List all triggers configurations for either user or the org or user within an org
     *
     * @tags apps
     * @name V1TriggersList
     * @summary List all triggers configurations for either user or the org or user within an org
     * @request GET:/api/v1/triggers
     * @secure
     */
    v1TriggersList: (
      query?: {
        /** Organization ID */
        org_id?: string;
        /** Trigger type, defaults to 'cron' */
        trigger_type?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesTriggerConfiguration[], any>({
        path: `/api/v1/triggers`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create triggers for the app. Used to create standalone trigger configurations such as cron tasks for agents that could be owned by a different user than the owner of the app
     *
     * @tags apps
     * @name V1TriggersCreate
     * @summary Create app triggers
     * @request POST:/api/v1/triggers
     * @secure
     */
    v1TriggersCreate: (request: TypesTriggerConfiguration, params: RequestParams = {}) =>
      this.request<TypesTriggerConfiguration, any>({
        path: `/api/v1/triggers`,
        method: "POST",
        body: request,
        secure: true,
        ...params,
      }),

    /**
     * @description Delete triggers for the app
     *
     * @tags apps
     * @name V1TriggersDelete
     * @summary Delete app triggers
     * @request DELETE:/api/v1/triggers/{trigger_id}
     * @secure
     */
    v1TriggersDelete: (triggerId: string, params: RequestParams = {}) =>
      this.request<TypesTriggerConfiguration, any>({
        path: `/api/v1/triggers/${triggerId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Update triggers for the app, for example to change the cron schedule or enable/disable the trigger
     *
     * @tags apps
     * @name V1TriggersUpdate
     * @summary Update app triggers
     * @request PUT:/api/v1/triggers/{trigger_id}
     * @secure
     */
    v1TriggersUpdate: (triggerId: string, request: TypesTriggerConfiguration, params: RequestParams = {}) =>
      this.request<TypesTriggerConfiguration, any>({
        path: `/api/v1/triggers/${triggerId}`,
        method: "PUT",
        body: request,
        secure: true,
        ...params,
      }),

    /**
     * @description Update triggers for the app, for example to change the cron schedule or enable/disable the trigger
     *
     * @tags apps
     * @name V1TriggersExecuteCreate
     * @summary Execute app trigger
     * @request POST:/api/v1/triggers/{trigger_id}/execute
     * @secure
     */
    v1TriggersExecuteCreate: (triggerId: string, params: RequestParams = {}) =>
      this.request<TypesTriggerExecuteResponse, any>({
        path: `/api/v1/triggers/${triggerId}/execute`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description List executions for the trigger
     *
     * @tags apps
     * @name V1TriggersExecutionsDetail
     * @summary List trigger executions
     * @request GET:/api/v1/triggers/{trigger_id}/executions
     * @secure
     */
    v1TriggersExecutionsDetail: (
      triggerId: string,
      query?: {
        /** Offset */
        offset?: number;
        /** Limit */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesTriggerExecution[], any>({
        path: `/api/v1/triggers/${triggerId}/executions`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get daily usage
     *
     * @tags usage
     * @name V1UsageList
     * @summary Get daily usage
     * @request GET:/api/v1/usage
     * @secure
     */
    v1UsageList: (
      query?: {
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
        /** Organization ID */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List users with pagination support and optional filtering by email domain or username. Supports ILIKE matching for email domains (e.g., "hotmail.com" will find all users with @hotmail.com emails) and partial username matching.
     *
     * @tags users
     * @name V1UsersList
     * @summary List users with pagination and filtering
     * @request GET:/api/v1/users
     * @secure
     */
    v1UsersList: (
      query?: {
        /** Page number (default: 1) */
        page?: number;
        /** Number of users per page (max: 200, default: 50) */
        per_page?: number;
        /** Filter by email domain (e.g., 'hotmail.com') or exact email */
        email?: string;
        /** Filter by username (partial match) */
        username?: string;
        /** Filter by admin status */
        admin?: boolean;
        /** Filter by user type */
        type?: string;
        /** Filter by token type */
        token_type?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPaginatedUsersList, any>({
        path: `/api/v1/users`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get user by ID
     *
     * @tags users
     * @name V1UsersDetail
     * @summary Get user details
     * @request GET:/api/v1/users/{id}
     * @secure
     */
    v1UsersDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesUser, any>({
        path: `/api/v1/users/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Search users by email, name, or username
     *
     * @tags users
     * @name V1UsersSearchList
     * @summary Search users
     * @request GET:/api/v1/users/search
     * @secure
     */
    v1UsersSearchList: (
      query: {
        /** Query */
        query: string;
        /** Organization ID */
        organization_id?: string;
        /** Limit */
        limit?: number;
        /** Offset */
        offset?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesUserSearchResponse, any>({
        path: `/api/v1/users/search`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get current user's monthly token usage and limits
     *
     * @tags users
     * @name V1UsersTokenUsageList
     * @summary Get user token usage
     * @request GET:/api/v1/users/token-usage
     * @secure
     */
    v1UsersTokenUsageList: (params: RequestParams = {}) =>
      this.request<TypesUserTokenUsageResponse, any>({
        path: `/api/v1/users/token-usage`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a wallet
     *
     * @tags wallets
     * @name V1WalletList
     * @summary Get a wallet
     * @request GET:/api/v1/wallet
     * @secure
     */
    v1WalletList: (
      query?: {
        /** Organization ID */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesWallet, any>({
        path: `/api/v1/wallet`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get comprehensive details about a specific work session including related entities
     *
     * @tags spec-driven-tasks
     * @name V1WorkSessionsDetail
     * @summary Get detailed information about a work session
     * @request GET:/api/v1/work-sessions/{sessionId}
     * @secure
     */
    v1WorkSessionsDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskWorkSessionDetailResponse, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Retrieve the session history log for a work session from git repository
     *
     * @tags spec-driven-tasks
     * @name V1WorkSessionsHistoryDetail
     * @summary Get session history log from git
     * @request GET:/api/v1/work-sessions/{sessionId}/history
     * @secure
     */
    v1WorkSessionsHistoryDetail: (
      sessionId: string,
      query?: {
        /** Filter by activity type */
        activity_type?: "conversation" | "code_change" | "decision" | "coordination";
        /**
         * Limit number of entries
         * @default 50
         */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerSessionHistoryResponse, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}/history`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Record session activity (conversation, code changes, decisions) to git repository
     *
     * @tags spec-driven-tasks
     * @name V1WorkSessionsRecordHistoryCreate
     * @summary Record session activity to git
     * @request POST:/api/v1/work-sessions/{sessionId}/record-history
     * @secure
     */
    v1WorkSessionsRecordHistoryCreate: (
      sessionId: string,
      request: ServicesSessionHistoryRecord,
      params: RequestParams = {},
    ) =>
      this.request<Record<string, any>, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}/record-history`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new work session that is spawned from an existing active work session
     *
     * @tags spec-driven-tasks
     * @name V1WorkSessionsSpawnCreate
     * @summary Spawn a new work session from an existing one
     * @request POST:/api/v1/work-sessions/{sessionId}/spawn
     * @secure
     */
    v1WorkSessionsSpawnCreate: (
      sessionId: string,
      request: TypesSpecTaskWorkSessionSpawnRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskWorkSessionDetailResponse, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}/spawn`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Update the status of a work session and handle state transitions
     *
     * @tags spec-driven-tasks
     * @name V1WorkSessionsStatusUpdate
     * @summary Update work session status
     * @request PUT:/api/v1/work-sessions/{sessionId}/status
     * @secure
     */
    v1WorkSessionsStatusUpdate: (
      sessionId: string,
      request: TypesSpecTaskWorkSessionUpdateRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskWorkSession, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}/status`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Manually create a Zed thread for a specific work session
     *
     * @tags zed-integration
     * @name V1WorkSessionsZedThreadCreate
     * @summary Create Zed thread for work session
     * @request POST:/api/v1/work-sessions/{sessionId}/zed-thread
     * @secure
     */
    v1WorkSessionsZedThreadCreate: (
      sessionId: string,
      request: TypesSpecTaskZedThreadCreateRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskZedThread, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}/zed-thread`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Update the status of a Zed thread associated with a work session
     *
     * @tags spec-driven-tasks
     * @name V1WorkSessionsZedThreadUpdate
     * @summary Update Zed thread status
     * @request PUT:/api/v1/work-sessions/{sessionId}/zed-thread
     * @secure
     */
    v1WorkSessionsZedThreadUpdate: (
      sessionId: string,
      request: TypesSpecTaskZedThreadUpdateRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskZedThread, TypesAPIError>({
        path: `/api/v1/work-sessions/${sessionId}/zed-thread`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new Helix work session when a Zed thread is created (reverse flow)
     *
     * @tags spec-driven-tasks
     * @name V1ZedThreadsCreateSessionCreate
     * @summary Create Helix session from Zed thread
     * @request POST:/api/v1/zed-threads/create-session
     * @secure
     */
    v1ZedThreadsCreateSessionCreate: (request: ServicesZedThreadCreationContext, params: RequestParams = {}) =>
      this.request<ServicesZedSessionCreationResult, TypesAPIError>({
        path: `/api/v1/zed-threads/create-session`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Process events from Zed instances including status changes, thread updates, and coordination
     *
     * @tags zed-integration
     * @name V1ZedEventsCreate
     * @summary Handle Zed instance events
     * @request POST:/api/v1/zed/events
     * @secure
     */
    v1ZedEventsCreate: (request: TypesZedInstanceEvent, params: RequestParams = {}) =>
      this.request<Record<string, any>, TypesAPIError>({
        path: `/api/v1/zed/events`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Process heartbeat signals from Zed instances to maintain connection status
     *
     * @tags zed-integration
     * @name V1ZedInstancesHeartbeatCreate
     * @summary Handle Zed connection heartbeat
     * @request POST:/api/v1/zed/instances/{instanceId}/heartbeat
     * @secure
     */
    v1ZedInstancesHeartbeatCreate: (instanceId: string, request: Record<string, any>, params: RequestParams = {}) =>
      this.request<Record<string, any>, TypesAPIError>({
        path: `/api/v1/zed/instances/${instanceId}/heartbeat`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Process thread-specific events from Zed instances
     *
     * @tags zed-integration
     * @name V1ZedInstancesThreadsEventsCreate
     * @summary Handle Zed thread-specific events
     * @request POST:/api/v1/zed/instances/{instanceId}/threads/{threadId}/events
     * @secure
     */
    v1ZedInstancesThreadsEventsCreate: (
      instanceId: string,
      threadId: string,
      request: Record<string, any>,
      params: RequestParams = {},
    ) =>
      this.request<Record<string, any>, TypesAPIError>({
        path: `/api/v1/zed/instances/${instanceId}/threads/${threadId}/events`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Update activity timestamp and status for a Zed thread
     *
     * @tags zed-integration
     * @name V1ZedThreadsActivityCreate
     * @summary Update Zed thread activity
     * @request POST:/api/v1/zed/threads/{threadId}/activity
     * @secure
     */
    v1ZedThreadsActivityCreate: (threadId: string, request: Record<string, any>, params: RequestParams = {}) =>
      this.request<TypesSpecTaskZedThread, TypesAPIError>({
        path: `/api/v1/zed/threads/${threadId}/activity`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),
  };
  v1 = {
    /**
     * @description Creates a model response for the given chat conversation.
     *
     * @tags chat
     * @name ChatCompletionsCreate
     * @summary Stream responses for chat
     * @request POST:/v1/chat/completions
     * @secure
     */
    chatCompletionsCreate: (request: OpenaiChatCompletionRequest, params: RequestParams = {}) =>
      this.request<OpenaiChatCompletionResponse, any>({
        path: `/v1/chat/completions`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Creates an embedding vector representing the input text. Supports both standard OpenAI embedding format and Chat Embeddings API format with messages.
     *
     * @tags embeddings
     * @name EmbeddingsCreate
     * @summary Creates an embedding vector representing the input text
     * @request POST:/v1/embeddings
     * @secure
     */
    embeddingsCreate: (request: TypesFlexibleEmbeddingRequest, params: RequestParams = {}) =>
      this.request<TypesFlexibleEmbeddingResponse, any>({
        path: `/v1/embeddings`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @name ModelsList
     * @request GET:/v1/models
     * @secure
     */
    modelsList: (
      query?: {
        /** Provider */
        provider?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesOpenAIModelsList[], any>({
        path: `/v1/models`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),
  };
}
