/* eslint-disable */
/* tslint:disable */
// @ts-nocheck
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

export enum TypesTriggerType {
  TriggerTypeAgentWorkQueue = "agent_work_queue",
  TriggerTypeSlack = "slack",
  TriggerTypeCrisp = "crisp",
  TriggerTypeAzureDevOps = "azure_devops",
  TriggerTypeCron = "cron",
}

export enum TypesTriggerExecutionStatus {
  TriggerExecutionStatusPending = "pending",
  TriggerExecutionStatusRunning = "running",
  TriggerExecutionStatusSuccess = "success",
  TriggerExecutionStatusError = "error",
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

export enum TypesTokenType {
  TokenTypeNone = "",
  TokenTypeRunner = "runner",
  TokenTypeKeycloak = "keycloak",
  TokenTypeOIDC = "oidc",
  TokenTypeAPIKey = "api_key",
  TokenTypeSocket = "socket",
}

export enum TypesTextSplitterType {
  TextSplitterTypeMarkdown = "markdown",
  TextSplitterTypeText = "text",
}

export enum TypesSpecTaskZedStatus {
  SpecTaskZedStatusPending = "pending",
  SpecTaskZedStatusActive = "active",
  SpecTaskZedStatusDisconnected = "disconnected",
  SpecTaskZedStatusCompleted = "completed",
  SpecTaskZedStatusFailed = "failed",
}

export enum TypesSpecTaskWorkSessionStatus {
  SpecTaskWorkSessionStatusPending = "pending",
  SpecTaskWorkSessionStatusActive = "active",
  SpecTaskWorkSessionStatusCompleted = "completed",
  SpecTaskWorkSessionStatusFailed = "failed",
  SpecTaskWorkSessionStatusCancelled = "cancelled",
  SpecTaskWorkSessionStatusBlocked = "blocked",
}

export enum TypesSpecTaskPhase {
  SpecTaskPhasePlanning = "planning",
  SpecTaskPhaseImplementation = "implementation",
  SpecTaskPhaseValidation = "validation",
}

export enum TypesSpecTaskImplementationStatus {
  SpecTaskImplementationStatusPending = "pending",
  SpecTaskImplementationStatusAssigned = "assigned",
  SpecTaskImplementationStatusInProgress = "in_progress",
  SpecTaskImplementationStatusCompleted = "completed",
  SpecTaskImplementationStatusBlocked = "blocked",
}

export enum TypesSpecTaskDesignReviewStatus {
  SpecTaskDesignReviewStatusPending = "pending",
  SpecTaskDesignReviewStatusInReview = "in_review",
  SpecTaskDesignReviewStatusChangesRequested = "changes_requested",
  SpecTaskDesignReviewStatusApproved = "approved",
  SpecTaskDesignReviewStatusSuperseded = "superseded",
}

export enum TypesSpecTaskDesignReviewCommentType {
  SpecTaskDesignReviewCommentTypeGeneral = "general",
  SpecTaskDesignReviewCommentTypeQuestion = "question",
  SpecTaskDesignReviewCommentTypeSuggestion = "suggestion",
  SpecTaskDesignReviewCommentTypeCritical = "critical",
  SpecTaskDesignReviewCommentTypePraise = "praise",
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

export enum TypesSessionType {
  SessionTypeNone = "",
  SessionTypeText = "text",
  SessionTypeImage = "image",
}

export enum TypesSessionMode {
  SessionModeNone = "",
  SessionModeInference = "inference",
  SessionModeFinetune = "finetune",
  SessionModeAction = "action",
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

export enum TypesRuntime {
  RuntimeOllama = "ollama",
  RuntimeDiffusers = "diffusers",
  RuntimeAxolotl = "axolotl",
  RuntimeVLLM = "vllm",
}

export enum TypesResponseFormatType {
  ResponseFormatTypeJSONObject = "json_object",
  ResponseFormatTypeText = "text",
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
  ResourceProject = "Project",
  ResourceGitRepository = "GitRepository",
}

export enum TypesQuestionSetExecutionStatus {
  QuestionSetExecutionStatusPending = "pending",
  QuestionSetExecutionStatusRunning = "running",
  QuestionSetExecutionStatusSuccess = "success",
  QuestionSetExecutionStatusError = "error",
}

export enum TypesProviderEndpointType {
  ProviderEndpointTypeGlobal = "global",
  ProviderEndpointTypeUser = "user",
  ProviderEndpointTypeOrg = "org",
  ProviderEndpointTypeTeam = "team",
}

export enum TypesProviderEndpointStatus {
  ProviderEndpointStatusOK = "ok",
  ProviderEndpointStatusError = "error",
  ProviderEndpointStatusLoading = "loading",
  ProviderEndpointStatusDisabled = "disabled",
}

export enum TypesProvider {
  ProviderOpenAI = "openai",
  ProviderTogetherAI = "togetherai",
  ProviderAnthropic = "anthropic",
  ProviderHelix = "helix",
  ProviderVLLM = "vllm",
}

export enum TypesOwnerType {
  OwnerTypeUser = "user",
  OwnerTypeRunner = "runner",
  OwnerTypeSystem = "system",
  OwnerTypeSocket = "socket",
  OwnerTypeOrg = "org",
}

export enum TypesOrganizationRole {
  OrganizationRoleOwner = "owner",
  OrganizationRoleMember = "member",
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

export enum TypesModelType {
  ModelTypeChat = "chat",
  ModelTypeImage = "image",
  ModelTypeEmbed = "embed",
}

export enum TypesModality {
  ModalityText = "text",
  ModalityImage = "image",
  ModalityFile = "file",
}

export enum TypesMessageContentType {
  MessageContentTypeText = "text",
}

export enum TypesLLMCallStep {
  LLMCallStepDefault = "default",
  LLMCallStepIsActionable = "is_actionable",
  LLMCallStepPrepareAPIRequest = "prepare_api_request",
  LLMCallStepInterpretResponse = "interpret_response",
  LLMCallStepGenerateTitle = "generate_title",
  LLMCallStepSummarizeConversation = "summarize_conversation",
}

export enum TypesKnowledgeState {
  KnowledgeStatePreparing = "preparing",
  KnowledgeStatePending = "pending",
  KnowledgeStateIndexing = "indexing",
  KnowledgeStateReady = "ready",
  KnowledgeStateError = "error",
}

export enum TypesInteractionState {
  InteractionStateNone = "",
  InteractionStateWaiting = "waiting",
  InteractionStateEditing = "editing",
  InteractionStateComplete = "complete",
  InteractionStateError = "error",
}

export enum TypesImageURLDetail {
  ImageURLDetailHigh = "high",
  ImageURLDetailLow = "low",
  ImageURLDetailAuto = "auto",
}

export enum TypesGitRepositoryType {
  GitRepositoryTypeInternal = "internal",
  GitRepositoryTypeCode = "code",
}

export enum TypesGitRepositoryStatus {
  GitRepositoryStatusActive = "active",
  GitRepositoryStatusArchived = "archived",
  GitRepositoryStatusDeleted = "deleted",
}

export enum TypesFeedback {
  FeedbackLike = "like",
  FeedbackDislike = "dislike",
}

export enum TypesExternalRepositoryType {
  ExternalRepositoryTypeGitHub = "github",
  ExternalRepositoryTypeGitLab = "gitlab",
  ExternalRepositoryTypeADO = "ado",
  ExternalRepositoryTypeBitbucket = "bitbucket",
}

export enum TypesEffect {
  EffectAllow = "allow",
  EffectDeny = "deny",
}

export enum TypesChatMessagePartType {
  ChatMessagePartTypeText = "text",
  ChatMessagePartTypeImageURL = "image_url",
}

export enum TypesAuthProvider {
  AuthProviderRegular = "regular",
  AuthProviderKeycloak = "keycloak",
  AuthProviderOIDC = "oidc",
}

export enum TypesAgentType {
  AgentTypeHelixBasic = "helix_basic",
  AgentTypeHelixAgent = "helix_agent",
  AgentTypeZedExternal = "zed_external",
}

export enum TypesAction {
  ActionGet = "Get",
  ActionList = "List",
  ActionDelete = "Delete",
  ActionUpdate = "Update",
  ActionCreate = "Create",
  ActionUseAction = "UseAction",
}

export enum TypesAPIKeyType {
  APIkeytypeNone = "",
  APIkeytypeAPI = "api",
  APIkeytypeApp = "app",
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

export enum OpenaiToolType {
  ToolTypeFunction = "function",
}

export enum OpenaiImageURLDetail {
  ImageURLDetailHigh = "high",
  ImageURLDetailLow = "low",
  ImageURLDetailAuto = "auto",
}

export enum OpenaiFinishReason {
  FinishReasonStop = "stop",
  FinishReasonLength = "length",
  FinishReasonFunctionCall = "function_call",
  FinishReasonToolCalls = "tool_calls",
  FinishReasonContentFilter = "content_filter",
  FinishReasonNull = "null",
}

export enum OpenaiChatMessagePartType {
  ChatMessagePartTypeText = "text",
  ChatMessagePartTypeImageURL = "image_url",
}

export enum OpenaiChatCompletionResponseFormatType {
  ChatCompletionResponseFormatTypeJSONObject = "json_object",
  ChatCompletionResponseFormatTypeJSONSchema = "json_schema",
  ChatCompletionResponseFormatTypeText = "text",
}

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

export interface OpenaiChatMessageImageURL {
  detail?: OpenaiImageURLDetail;
  url?: string;
}

export interface OpenaiChatMessagePart {
  image_url?: OpenaiChatMessageImageURL;
  text?: string;
  type?: OpenaiChatMessagePartType;
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
  /** GPU encoder stats from Wolf (via nvidia-smi) */
  gpu_stats?: ServerGPUStats;
  /** Actual pipeline count from Wolf */
  gstreamer_pipelines?: ServerGStreamerPipelineStats;
  /** Lobbies mode */
  lobbies?: ServerWolfLobbyInfo[];
  memory?: ServerWolfSystemMemory;
  /** moonlight-web client connections */
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

export interface ServerCreateSpecTaskFromDemoRequest {
  demo_repo: string;
  organization_id?: string;
  priority?: string;
  prompt: string;
  type?: string;
}

export interface ServerCreateTopUpRequest {
  amount?: number;
  org_id?: string;
}

export interface ServerDesignDocsResponse {
  documents?: ServerDesignDocument[];
  task_id?: string;
}

export interface ServerDesignDocsShareLinkResponse {
  expires_at?: string;
  share_url?: string;
  token?: string;
}

export interface ServerDesignDocument {
  content?: string;
  filename?: string;
  path?: string;
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

export interface ServerGPUStats {
  /** false if nvidia-smi failed */
  available?: boolean;
  encoder_average_fps?: number;
  encoder_average_latency_us?: number;
  encoder_session_count?: number;
  encoder_utilization_percent?: number;
  error?: string;
  gpu_name?: string;
  gpu_utilization_percent?: number;
  memory_total_mb?: number;
  memory_used_mb?: number;
  memory_utilization_percent?: number;
  /** How long nvidia-smi took in Wolf */
  query_duration_ms?: number;
  temperature_celsius?: number;
}

export interface ServerGStreamerPipelineStats {
  /** Video + audio consumers (2 per session) */
  consumer_pipelines?: number;
  /** Video + audio producers (2 per lobby) */
  producer_pipelines?: number;
  /** Sum of producers + consumers */
  total_pipelines?: number;
}

export interface ServerInitializeSampleRepositoriesRequest {
  organization_id?: string;
  owner_id?: string;
  /** If empty, creates all samples */
  sample_types?: string[];
}

export interface ServerInitializeSampleRepositoriesResponse {
  created_count?: number;
  created_repositories?: TypesGitRepository[];
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

export interface ServerPhaseProgress {
  agent?: string;
  completed_at?: string;
  revision_count?: number;
  session_id?: string;
  started_at?: string;
  status?: string;
}

export interface ServerPushPullResponse {
  branch?: string;
  message?: string;
  repository_id?: string;
  success?: boolean;
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
  client_count?: number;
  lobby_id?: string;
  lobby_name?: string;
  memory_bytes?: number;
  resolution?: string;
}

export interface ServerWolfSessionInfo {
  /** Wolf UI app ID in lobbies mode */
  app_id?: string;
  client_ip?: string;
  /** Helix client ID (helix-agent-{session_id}-{instance_id}) */
  client_unique_id?: string;
  display_mode?: {
    av1_supported?: boolean;
    height?: number;
    hevc_supported?: boolean;
    refresh_rate?: number;
    width?: number;
  };
  /** Which lobby this session is connected to (lobbies mode) */
  lobby_id?: string;
  /** Exposed as session_id for frontend (Wolf's client_id) */
  session_id?: string;
}

export interface ServerWolfSystemMemory {
  /** Apps mode */
  apps?: ServerWolfAppMemory[];
  clients?: ServerWolfClientConnection[];
  /** From Wolf's nvidia-smi query */
  gpu_stats?: ServerGPUStats;
  gstreamer_buffer_bytes?: number;
  /** From Wolf's state */
  gstreamer_pipelines?: ServerGStreamerPipelineStats;
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

export interface ServicesCreateTaskRequest {
  /** Optional: Helix agent to use for spec generation */
  app_id?: string;
  priority?: string;
  project_id?: string;
  prompt?: string;
  type?: string;
  user_id?: string;
  /** Optional: Skip human review and auto-approve specs */
  yolo_mode?: boolean;
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

export interface ServicesKoditEnrichmentAttributes {
  content?: string;
  created_at?: string;
  subtype?: string;
  type?: string;
  updated_at?: string;
}

export interface ServicesKoditEnrichmentData {
  attributes?: ServicesKoditEnrichmentAttributes;
  /** Added for frontend */
  commit_sha?: string;
  id?: string;
  type?: string;
}

export interface ServicesKoditEnrichmentListResponse {
  data?: ServicesKoditEnrichmentData[];
}

export interface ServicesKoditSearchResult {
  content?: string;
  id?: string;
  language?: string;
  type?: string;
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
  /** Custom startup script for this project */
  startup_script?: string;
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

export interface ServicesStartupScriptVersion {
  author?: string;
  commit_hash?: string;
  content?: string;
  message?: string;
  timestamp?: string;
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

export interface TypesAccountUpdateRequest {
  full_name?: string;
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

export interface TypesAdminCreateUserRequest {
  admin?: boolean;
  email?: string;
  full_name?: string;
  password?: string;
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
  config?: Record<string, any>;
  created_at?: string;
  deadline_at?: string;
  description?: string;
  id?: string;
  /** Labels/tags for filtering */
  labels?: string[];
  last_error?: string;
  max_retries?: number;
  metadata?: Record<string, any>;
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
  work_data?: Record<string, any>;
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
  /** If true, the browser will cache the results of the tool call */
  cache?: boolean;
  enabled?: boolean;
  /** If true, the browser will return the HTML as markdown */
  markdown_post_processing?: boolean;
  /** If true, the browser will not be used to open URLs, it will be a simple GET request */
  no_browser?: boolean;
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

export interface TypesAzureDevOps {
  organization_url?: string;
  personal_access_token?: string;
}

export interface TypesAzureDevOpsTrigger {
  enabled?: boolean;
}

export interface TypesBoardSettings {
  wip_limits?: Record<string, number>;
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

export interface TypesChoice {
  delta?: TypesOpenAIMessage;
  finish_reason?: string;
  index?: number;
  message?: TypesOpenAIMessage;
  text?: string;
}

export interface TypesClipboardData {
  /** text content or base64-encoded image */
  data?: string;
  /** "text" or "image" */
  type?: string;
}

export interface TypesCommit {
  author?: string;
  email?: string;
  message?: string;
  sha?: string;
  timestamp?: string;
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

export interface TypesCreateBranchRequest {
  base_branch?: string;
  branch_name?: string;
}

export interface TypesCreateBranchResponse {
  base_branch?: string;
  branch_name?: string;
  message?: string;
  repository_id?: string;
}

export interface TypesCreatePullRequestRequest {
  description?: string;
  source_branch?: string;
  target_branch?: string;
  title?: string;
}

export interface TypesCreatePullRequestResponse {
  id?: string;
  message?: string;
}

export interface TypesCreateSampleRepositoryRequest {
  description?: string;
  /** Enable Kodit code intelligence indexing */
  kodit_indexing?: boolean;
  name?: string;
  organization_id?: string;
  owner_id?: string;
  sample_type?: string;
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

export interface TypesExecuteQuestionSetRequest {
  app_id?: string;
  question_set_id?: string;
}

export interface TypesExecuteQuestionSetResponse {
  results?: TypesQuestionResponse[];
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

export interface TypesGitRepository {
  azure_devops?: TypesAzureDevOps;
  branches?: string[];
  /** For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git */
  clone_url?: string;
  created_at?: string;
  default_branch?: string;
  description?: string;
  /** "github", "gitlab", "ado", "bitbucket", etc. */
  external_type?: TypesExternalRepositoryType;
  /** Full URL to external repo (e.g., https://github.com/org/repo) */
  external_url?: string;
  id?: string;
  /** External repository fields */
  is_external?: boolean;
  /** Code intelligence fields */
  kodit_indexing?: boolean;
  last_activity?: string;
  /** Local filesystem path for Helix-hosted repos (empty for external) */
  local_path?: string;
  /** Stores Metadata as JSON */
  metadata?: Record<string, any>;
  name?: string;
  /** Organization ID - will be backfilled for existing repos */
  organization_id?: string;
  owner_id?: string;
  /** Password for the repository */
  password?: string;
  project_id?: string;
  repo_type?: TypesGitRepositoryType;
  status?: TypesGitRepositoryStatus;
  updated_at?: string;
  /** Authentication fields */
  username?: string;
}

export interface TypesGitRepositoryCreateRequest {
  azure_devops?: TypesAzureDevOps;
  default_branch?: string;
  description?: string;
  /** "github", "gitlab", "ado", "bitbucket", etc. */
  external_type?: TypesExternalRepositoryType;
  /** Full URL to external repo (e.g., https://github.com/org/repo) */
  external_url?: string;
  initial_files?: Record<string, string>;
  /** True for GitHub/GitLab/ADO, false for Helix-hosted */
  is_external?: boolean;
  /** Enable Kodit code intelligence indexing */
  kodit_indexing?: boolean;
  metadata?: Record<string, any>;
  name?: string;
  /** Organization ID - required for access control */
  organization_id?: string;
  owner_id?: string;
  /** Password for the repository */
  password?: string;
  project_id?: string;
  repo_type?: TypesGitRepositoryType;
  /** Username for the repository */
  username?: string;
}

export interface TypesGitRepositoryFileResponse {
  content?: string;
  path?: string;
}

export interface TypesGitRepositoryTreeResponse {
  entries?: TypesTreeEntry[];
  path?: string;
}

export interface TypesGitRepositoryUpdateRequest {
  azure_devops?: TypesAzureDevOps;
  default_branch?: string;
  description?: string;
  /** "github", "gitlab", "ado", "bitbucket", etc. */
  external_type?: TypesExternalRepositoryType;
  external_url?: string;
  metadata?: Record<string, any>;
  name?: string;
  password?: string;
  username?: string;
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

export interface TypesListCommitsResponse {
  commits?: TypesCommit[];
}

export interface TypesLoginRequest {
  email?: string;
  password?: string;
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

export interface TypesPasswordResetCompleteRequest {
  access_token?: string;
  new_password?: string;
}

export interface TypesPasswordResetRequest {
  email?: string;
}

export interface TypesPasswordUpdateRequest {
  new_password?: string;
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

export interface TypesProject {
  /** Automation settings */
  auto_start_backlog_tasks?: boolean;
  created_at?: string;
  default_branch?: string;
  /** Project-level repository management */
  default_repo_id?: string;
  /** Soft delete timestamp */
  deleted_at?: GormDeletedAt;
  description?: string;
  github_repo_url?: string;
  id?: string;
  /**
   * Internal project Git repository (stores project config, tasks, design docs)
   * IMPORTANT: Startup script is stored in .helix/startup.sh in the internal Git repo
   * It is NEVER stored in the database - Git is the single source of truth
   */
  internal_repo_path?: string;
  metadata?: number[];
  name?: string;
  organization_id?: string;
  /** Transient field - loaded from Git, never persisted to database */
  startup_script?: string;
  /** "active", "archived", "completed" */
  status?: string;
  technologies?: string[];
  updated_at?: string;
  user_id?: string;
}

export interface TypesProjectCreateRequest {
  default_branch?: string;
  default_repo_id?: string;
  description?: string;
  github_repo_url?: string;
  name?: string;
  startup_script?: string;
  technologies?: string[];
}

export interface TypesProjectUpdateRequest {
  auto_start_backlog_tasks?: boolean;
  default_branch?: string;
  default_repo_id?: string;
  description?: string;
  github_repo_url?: string;
  name?: string;
  startup_script?: string;
  status?: string;
  technologies?: string[];
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

export interface TypesPullRequest {
  author?: string;
  created_at?: string;
  description?: string;
  id?: string;
  number?: number;
  source_branch?: string;
  state?: string;
  target_branch?: string;
  title?: string;
  updated_at?: string;
  url?: string;
}

export interface TypesQuestion {
  created?: string;
  id?: string;
  question?: string;
  updated?: string;
}

export interface TypesQuestionResponse {
  /** Error */
  error?: string;
  /** Interaction ID */
  interaction_id?: string;
  /** Original question */
  question?: string;
  /** ID of the question */
  question_id?: string;
  /** Response */
  response?: string;
  /** Session ID */
  session_id?: string;
}

export interface TypesQuestionSet {
  created?: string;
  description?: string;
  id?: string;
  name?: string;
  /** The organization this session belongs to, if any */
  organization_id?: string;
  questions?: TypesQuestion[];
  updated?: string;
  /** Creator of the question set */
  user_id?: string;
}

export interface TypesQuestionSetExecution {
  app_id?: string;
  created?: string;
  duration_ms?: number;
  error?: string;
  id?: string;
  question_set_id?: string;
  results?: TypesQuestionResponse[];
  status?: TypesQuestionSetExecutionStatus;
  updated?: string;
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

export interface TypesRegisterRequest {
  email?: string;
  full_name?: string;
  password?: string;
  password_confirm?: string;
}

export interface TypesResponseFormat {
  schema?: OpenaiChatCompletionResponseFormatJSONSchema;
  type?: TypesResponseFormatType;
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
  auth_provider?: TypesAuthProvider;
  /** Charging for usage */
  billing_enabled?: boolean;
  deployment_id?: string;
  disable_llm_call_logging?: boolean;
  eval_user_id?: string;
  filestore_prefix?: string;
  google_analytics_frontend?: string;
  latest_version?: string;
  license?: TypesFrontendLicenseInfo;
  /** "single" or "multi" - determines streaming architecture */
  moonlight_web_mode?: string;
  organizations_create_enabled_for_non_admins?: boolean;
  /** Controls if users can add their own AI provider API keys */
  providers_management_enabled?: boolean;
  /**
   * used to prepend onto raw filestore paths to download files
   * the filestore path will have the user info in it - i.e.
   * it's a low level filestore path
   * if we are using an object storage thing - then this URL
   * can be the prefix to the bucket
   */
  registration_enabled?: boolean;
  rudderstack_data_plane_url?: string;
  rudderstack_write_key?: string;
  sentry_dsn_frontend?: string;
  /** Requested video streaming bitrate in Mbps (default: 40) */
  streaming_bitrate_mbps?: number;
  /** Stripe top-ups enabled */
  stripe_enabled?: boolean;
  tools_enabled?: boolean;
  version?: string;
}

export interface TypesSession {
  /** named config for backward compat */
  config?: TypesSessionMetadata;
  created?: string;
  /** Soft delete support - allows cleanup of orphaned lobbies */
  deleted_at?: GormDeletedAt;
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
  /** The question set execution this session belongs to, if any */
  question_set_execution_id?: string;
  /** The question set this session belongs to, if any */
  question_set_id?: string;
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

export interface TypesSessionIdleStatus {
  has_external_agent?: boolean;
  idle_minutes?: number;
  warning_threshold?: boolean;
  will_terminate_in?: number;
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
  /** Path to saved screenshot when agent is paused */
  paused_screenshot_path?: string;
  /** NEW: SpecTask phase (planning, implementation) */
  phase?: string;
  priority?: boolean;
  /** ID of associated Project (for exploratory sessions) */
  project_id?: string;
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
  /** "planning", "implementation", "coordination", "exploratory" */
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
  question_set_execution_id?: string;
  question_set_id?: string;
  session_id?: string;
  /** this is either the prompt or the summary of the training data */
  summary?: string;
  type?: TypesSessionType;
  updated?: string;
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
  /** Archive to hide from main view */
  archived?: boolean;
  /** Git tracking */
  branch_name?: string;
  completed_at?: string;
  created_at?: string;
  /** Metadata */
  created_by?: string;
  description?: string;
  /** When design docs were pushed to helix-specs branch */
  design_docs_pushed_at?: string;
  /** Simple tracking */
  estimated_hours?: number;
  /** External agent tracking (single agent per SpecTask, spans entire workflow) */
  external_agent_id?: string;
  /** NEW: Single Helix Agent for entire workflow (App type in code) */
  helix_app_id?: string;
  id?: string;
  implementation_approved_at?: string;
  /** Implementation tracking */
  implementation_approved_by?: string;
  /** Discrete tasks breakdown (markdown) */
  implementation_plan?: string;
  labels?: string[];
  /** When branch was last pushed */
  last_push_at?: string;
  /** Git tracking */
  last_push_commit_hash?: string;
  /** Merge commit hash */
  merge_commit_hash?: string;
  /** When merge happened */
  merged_at?: string;
  /** Whether branch was merged to main */
  merged_to_main?: boolean;
  metadata?: Record<string, any>;
  name?: string;
  /** Kiro's actual approach: simple, human-readable artifacts */
  original_prompt?: string;
  /**
   * Session tracking (single Helix session for entire workflow - planning + implementation)
   * The same external agent/session is reused throughout the entire SpecTask lifecycle
   */
  planning_session_id?: string;
  /** "low", "medium", "high", "critical" */
  priority?: string;
  project_id?: string;
  project_path?: string;
  /** User stories + EARS acceptance criteria (markdown) */
  requirements_spec?: string;
  spec_approved_at?: string;
  /** Approval tracking */
  spec_approved_by?: string;
  /** Number of spec revisions requested */
  spec_revision_count?: number;
  started_at?: string;
  /** Spec-driven workflow statuses - see constants below */
  status?: string;
  /** Design document (markdown) */
  technical_design?: string;
  /** "feature", "bug", "refactor" */
  type?: string;
  updated_at?: string;
  workspace_config?: number[];
  /** Skip human review, auto-approve specs */
  yolo_mode?: boolean;
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

export interface TypesSpecTaskDesignReview {
  approved_at?: string;
  /** Timestamps */
  created_at?: string;
  git_branch?: string;
  /** Git information */
  git_commit_hash?: string;
  git_pushed_at?: string;
  id?: string;
  implementation_plan?: string;
  /** Review decision */
  overall_comment?: string;
  rejected_at?: string;
  /** Design document snapshots at time of review */
  requirements_spec?: string;
  /** Review metadata */
  reviewer_id?: string;
  spec_task_id?: string;
  status?: TypesSpecTaskDesignReviewStatus;
  technical_design?: string;
  updated_at?: string;
}

export interface TypesSpecTaskDesignReviewComment {
  /** Agent integration (NEW FIELDS) */
  agent_response?: string;
  /** When agent responded */
  agent_response_at?: string;
  /** The actual comment */
  comment_text?: string;
  /** Made optional - simplified to single type */
  comment_type?: TypesSpecTaskDesignReviewCommentType;
  /** Comment metadata */
  commented_by?: string;
  /** Timestamps */
  created_at?: string;
  /** Location in document */
  document_type?: string;
  /** Character offset in document */
  end_offset?: number;
  id?: string;
  /** Link to Helix interaction */
  interaction_id?: string;
  /** Optional line number */
  line_number?: number;
  /** For inline comments - store the context around the comment */
  quoted_text?: string;
  /** "manual", "auto_text_removed", "agent_updated" */
  resolution_reason?: string;
  /** Status tracking */
  resolved?: boolean;
  resolved_at?: string;
  resolved_by?: string;
  review_id?: string;
  /** e.g., "## Architecture/### Database Schema" */
  section_path?: string;
  /** Character offset in document */
  start_offset?: number;
  updated_at?: string;
}

export interface TypesSpecTaskDesignReviewCommentCreateRequest {
  comment_text: string;
  comment_type?: TypesSpecTaskDesignReviewCommentType;
  document_type: "requirements" | "technical_design" | "implementation_plan";
  end_offset?: number;
  line_number?: number;
  quoted_text?: string;
  review_id: string;
  section_path?: string;
  start_offset?: number;
}

export interface TypesSpecTaskDesignReviewCommentListResponse {
  comments?: TypesSpecTaskDesignReviewComment[];
  total?: number;
}

export interface TypesSpecTaskDesignReviewDetailResponse {
  comments?: TypesSpecTaskDesignReviewComment[];
  review?: TypesSpecTaskDesignReview;
  spec_task?: TypesSpecTask;
}

export interface TypesSpecTaskDesignReviewListResponse {
  reviews?: TypesSpecTaskDesignReview[];
  total?: number;
}

export interface TypesSpecTaskDesignReviewSubmitRequest {
  /** "approve" or "request_changes" */
  decision: "approve" | "request_changes";
  overall_comment?: string;
  review_id: string;
}

export interface TypesSpecTaskImplementationSessionsCreateRequest {
  /** @default true */
  auto_create_sessions?: boolean;
  project_path?: string;
  spec_task_id: string;
  workspace_config?: Record<string, any>;
}

export interface TypesSpecTaskImplementationStartResponse {
  agent_instructions?: string;
  base_branch?: string;
  branch_name?: string;
  created_at?: string;
  local_path?: string;
  pr_template_url?: string;
  repository_id?: string;
  repository_name?: string;
  status?: string;
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
  /** Pointer to allow explicit false */
  yolo_mode?: boolean;
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

export interface TypesSpecTaskWorkSessionUpdateRequest {
  config?: Record<string, any>;
  description?: string;
  /** @maxLength 255 */
  name?: string;
  status?:
    | "pending"
    | "active"
    | "completed"
    | "failed"
    | "cancelled"
    | "blocked";
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
  /** If true, the browser will cache the results of the tool call */
  cache?: boolean;
  enabled?: boolean;
  /** If true, the browser will return the HTML as markdown */
  markdown_post_processing?: boolean;
  /** If true, the browser will not be used to open URLs, it will be a simple GET request */
  no_browser?: boolean;
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

export interface TypesToolWebSearchConfig {
  enabled?: boolean;
  max_results?: number;
}

export interface TypesToolZapierConfig {
  api_key?: string;
  max_iterations?: number;
  model?: string;
}

export interface TypesTreeEntry {
  is_dir?: boolean;
  name?: string;
  path?: string;
  size?: number;
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

export interface TypesTriggerStatus {
  message?: string;
  ok?: boolean;
  type?: TypesTriggerType;
}

export interface TypesUpdateGitRepositoryFileContentsRequest {
  /** Author name */
  author?: string;
  /** Branch name */
  branch?: string;
  /** Base64 encoded content */
  content?: string;
  /** Author email */
  email?: string;
  /** Commit message */
  message?: string;
  /** File path */
  path?: string;
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
  auth_provider?: TypesAuthProvider;
  created_at?: string;
  deactivated?: boolean;
  deleted_at?: GormDeletedAt;
  email?: string;
  full_name?: string;
  id?: string;
  /** if the user must change their password */
  must_change_password?: boolean;
  /** bcrypt hash of the password */
  password_hash?: number[];
  sb?: boolean;
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

export interface WolfSystemHealthResponse {
  /** Tests if GStreamer type lock is available */
  can_create_new_pipelines?: boolean;
  overall_status?: string;
  process_uptime_seconds?: number;
  stuck_thread_count?: number;
  success?: boolean;
  threads?: WolfThreadHealthInfo[];
  total_thread_count?: number;
}

export interface WolfThreadHealthInfo {
  current_request_path?: string;
  details?: string;
  has_active_request?: boolean;
  heartbeat_count?: number;
  is_stuck?: boolean;
  name?: string;
  request_duration_seconds?: number;
  seconds_alive?: number;
  seconds_since_heartbeat?: number;
  stack_trace?: string;
  tid?: number;
}
