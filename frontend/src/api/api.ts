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

export interface ApiAddBotParentRequest {
  parent_id?: string;
}

export interface ApiBotActivateDTO {
  activation_id?: string;
  agent_app_id?: string;
  project_id?: string;
  session_id?: string;
}

export interface ApiBotBadge {
  id?: string;
}

export interface ApiBotChatDTO {
  agent_app_id?: string;
  project_id?: string;
}

export interface ApiBotDTO {
  agent_model?: string;
  agent_runtime?: string;
  /**
   * AgentStatus is "running" when the bot's desktop sandbox is online,
   * "stopped" otherwise (no session, paused, never activated). Drives
   * the green/grey presence dot on the org chart.
   */
  agent_status?: string;
  content?: string;
  created_at?: string;
  helix_user_id?: string;
  id?: string;
  identity?: Record<string, string>;
  /**
   * Kind is "" (agent) or "human". A human node is a person placeholder,
   * never activated; Identity holds their cross-system handles and
   * HelixUserID optionally links them to a Helix org member. Identity is
   * omitted for agent bots.
   */
  kind?: string;
  /**
   * Name is the human-readable display label; empty means the UI falls
   * back to ID. Distinct from ID, which is the immutable handle.
   */
  name?: string;
  organization_id?: string;
  parent_ids?: string[];
  /**
   * PreserveContext, when true, stops the runtime from wiping this
   * Bot's chat session before each re-activation, so it accumulates
   * context across triggers (e.g. Slack). Defaults to false.
   */
  preserve_context?: boolean;
  tools?: string[];
  updated_at?: string;
}

export interface ApiBotDetailDTO {
  /** AgentAppID + ProjectID — see BotChatDTO comments. */
  agent_app_id?: string;
  bot?: ApiBotDTO;
  project_id?: string;
}

export interface ApiBotSubscriptionDTO {
  created_at?: string;
  topic_id?: string;
}

export interface ApiBotSubscriptionsResponse {
  bot_id?: string;
  subscriptions?: ApiBotSubscriptionDTO[];
}

export interface ApiChartPositionDTO {
  id?: string;
  /** Kind is bot | topic | processor (matches the ReactFlow node id prefix). */
  kind?: string;
  x?: number;
  y?: number;
}

export interface ApiChartPositionsResponse {
  positions?: ApiChartPositionDTO[];
}

export interface ApiCreateBotRequest {
  content?: string;
  id?: string;
  /**
   * Name is the human-readable display label (e.g. "Chief of Staff").
   * Optional; the ID stays the immutable handle.
   */
  name?: string;
  /**
   * Owner makes this a manager Bot: it receives the canonical owner
   * tool set (every org-graph mutation - create_bot, delete_bot,
   * set_bot_content, subscribe, ... - plus the read baseline) so it can
   * hire and manage other Bots. When true, Tools is ignored in favour
   * of that set. Used to seed a starter/root Bot for a new org.
   */
  owner?: boolean;
  parent_id?: string;
  preserve_context?: boolean;
  tools?: string[];
  topics?: string[];
}

export interface ApiCreateBotResponse {
  activation_id?: string;
  id?: string;
}

export interface ApiCreateTopicRequest {
  /**
   * As is the Bot that creates the topic — the bot whose chat
   * the human is in. Empty leaves the topic unattributed (CreatedBy is
   * cosmetic: it only anchors the node on the chart).
   */
  as?: string;
  description?: string;
  id?: string;
  name?: string;
  transport?: ApiTransportRequestField;
}

export interface ApiErrorResponse {
  error?: string;
}

export interface ApiEventCard {
  body?: string;
  created_at?: string;
  from?: string;
  has_message?: boolean;
  id?: string;
  message_body?: string;
  source?: string;
  subject?: string;
  to?: string;
  topic_id?: string;
}

export interface ApiGitHubInstallationStatus {
  /**
   * AppExists is true when a Helix GitHub App has been created/registered
   * for this org (a github_app ServiceConnection exists), even if not yet
   * installed on any repo. Drives the gate's create-vs-install branch.
   */
  app_exists?: boolean;
  /**
   * InstallURL is where the New Topic gate sends the user to install the
   * app (https://github.com/apps/<slug>/installations/new). Populated from
   * the created app's slug, or from GITHUB_APP_SLUG for a pre-existing app.
   */
  install_url?: string;
  /**
   * Installed is true when the Helix GitHub App has an installation for
   * this org (a github_app ServiceConnection with an installation id).
   */
  installed?: boolean;
  /**
   * ManageURL is the app's developer-settings page on GitHub
   * (github.com/organizations/<owner>/settings/apps/<slug>) — where you edit
   * permissions, repos, and delete the app. Empty when the owner is unknown
   * (e.g. a BYO app configured without it).
   */
  manage_url?: string;
}

export interface ApiGitHubManifestStartResponse {
  manifest?: string;
  post_url?: string;
  state?: string;
}

export interface ApiGitHubRepoDTO {
  full_name?: string;
  private?: boolean;
}

export interface ApiGitHubReposResponse {
  repos?: ApiGitHubRepoDTO[];
  /**
   * Source identifies which token paid for this list — useful
   * when debugging "I can't see repo X" reports.
   */
  source?: string;
}

export interface ApiGitHubWebhookStatusResponse {
  active?: boolean;
  /** Detail explains a "unknown" state (and is empty otherwise). */
  detail?: string;
  payload_url?: string;
  /**
   * State is one of:
   *   "installed" — a webhook for this topic's payload URL exists on the repo
   *   "missing"   — GitHub was reachable and has no such webhook (needs install)
   *   "unknown"   — couldn't determine (no repo / no public URL / no creds /
   *                 GitHub error); see Detail. The UI falls back to stored state.
   */
  state?: string;
  webhook_html_url?: string;
  webhook_id?: number;
}

export interface ApiInstallGitHubWebhookResponse {
  payload_url?: string;
  /**
   * Warning is a non-fatal message about the just-installed
   * webhook — e.g. "SERVER_URL is a loopback address so GitHub's
   * servers can't actually deliver to this URL". The webhook IS
   * installed on GitHub; the warning just tells the operator
   * what needs fixing on their side for deliveries to flow.
   */
  warning?: string;
  webhook_html_url?: string;
  webhook_id?: number;
}

export interface ApiMessageAttributes {
  body?: string;
  created_at?: string;
  from?: string;
  has_message?: boolean;
  /**
   * Raw is the canonical Message envelope JSON exactly as stored — the
   * same shape a processor's `.Message` template/filter context sees
   * ({"from":…,"subject":…,"body":…,"thread_id":…,…}). Lets the UI show
   * operators which fields are available.
   */
  raw?: string;
  source?: string;
  subject?: string;
  to?: string[];
  topic_id?: string;
}

export interface ApiMessageResource {
  attributes?: ApiMessageAttributes;
  id?: string;
  type?: string;
}

export interface ApiMessagesDocument {
  data?: ApiMessageResource[];
  links?: Record<string, string>;
  meta?: ApiMessagesMeta;
}

export interface ApiMessagesMeta {
  page?: number;
  size?: number;
  total?: number;
  total_pages?: number;
}

export interface ApiOrgOverview {
  bots?: ApiBotBadge[];
}

export interface ApiProcessorOutputDTO {
  label?: string;
  /**
   * ManagedFor is set when this route is auto-managed by a reconciler for
   * the named Worker (the Slack auto-router). Empty for human-authored
   * routes. Read-only — the UI surfaces it; reconcilers own these routes.
   */
  managed_for?: string;
  match?: string;
  owned?: boolean;
  topic_id?: string;
}

export interface ApiProcessorWriteRequest {
  data?: {
    attributes?: {
      config?: Record<string, any>;
      created_by?: string;
      input_topic_id?: string;
      kind?: string;
      name?: string;
      outputs?: ApiProcessorOutputDTO[];
    };
    type?: string;
  };
}

export interface ApiPublishRequest {
  /**
   * As is the Bot the message is sent as — the bot whose chat the
   * human is in. Empty means human/system-origin (the dispatcher treats
   * it as such). There is no global "owner" sender any more.
   */
  as?: string;
  body?: string;
  subject?: string;
  to?: string[];
}

export interface ApiPublishResponse {
  event_id?: string;
}

export interface ApiSetSettingRequest {
  value?: string;
}

export interface ApiSettingsResponse {
  db_path?: string;
  public_url?: string;
  specs?: ApiSettingsSpecDTO[];
}

export interface ApiSettingsSpecDTO {
  configured?: boolean;
  description?: string;
  key?: string;
  required?: boolean;
  type?: string;
  value?: string;
}

export interface ApiSubscribeBotRequest {
  topic_id?: string;
}

export interface ApiToolDTO {
  description?: string;
  name?: string;
}

export interface ApiTopicDTO {
  can_publish?: boolean;
  config?: Record<string, any>;
  created_at?: string;
  created_by?: string;
  description?: string;
  disable_reason?: string;
  effective_public_url?: string;
  id?: string;
  kind?: string;
  name?: string;
  recent_events?: ApiEventCard[];
  subscribers?: string[];
}

export interface ApiTopicsResponse {
  recent?: ApiEventCard[];
  topics?: ApiTopicDTO[];
}

export interface ApiTransportRequestField {
  config?: Record<string, any>;
  kind?: string;
}

export interface ApiUpdateBotRequest {
  content?: string;
  /**
   * Identity is the per-channel handle map for a human node (slack/github/
   * email/…). When present it replaces the stored map; absent leaves it
   * unchanged. Only meaningful for kind=human bots.
   */
  identity?: Record<string, string>;
  name?: string;
  preserve_context?: boolean;
  tools?: string[];
}

export interface ApiUpdateTopicRequest {
  description?: string;
  name?: string;
  transport?: ApiTransportRequestField;
}

export interface ApiUpsertChartPositionsRequest {
  positions?: ApiChartPositionDTO[];
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

export interface GithubComHelixmlHelixApiPkgServerClientInfo {
  avatar_url?: string;
  color?: string;
  id?: number;
  last_seen?: string;
  last_x?: number;
  last_y?: number;
  user_id?: string;
  user_name?: string;
}

export interface GithubComHelixmlHelixApiPkgTypesCommit {
  author?: string;
  email?: string;
  message?: string;
  sha?: string;
  timestamp?: string;
}

export interface GithubComHelixmlHelixApiPkgTypesConfig {
  rules?: TypesRule[];
}

export enum GithubComHelixmlHelixApiPkgTypesRuntime {
  RuntimeOllama = "ollama",
  RuntimeDiffusers = "diffusers",
  RuntimeAxolotl = "axolotl",
  RuntimeVLLM = "vllm",
}

export interface GithubComMark3LabsMcpGoMcpIcon {
  /** Optional MIME type (e.g., "image/png", "image/svg+xml") */
  mimeType?: string;
  /** Optional size specifications (e.g., ["48x48"], ["any"] for SVG) */
  sizes?: string[];
  /** URI pointing to the icon resource (HTTPS URL or data URI) */
  src?: string;
}

export interface GithubComMark3LabsMcpGoMcpMeta {
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

export interface GithubComMark3LabsMcpGoMcpTool {
  /** Meta is a metadata object that is reserved by MCP for storing additional information. */
  _meta?: GithubComMark3LabsMcpGoMcpMeta;
  /** Optional properties describing tool behavior */
  annotations?: McpToolAnnotation;
  /** Support for deferred loading */
  defer_loading?: boolean;
  /** A human-readable description of the tool. */
  description?: string;
  /** Execution describes execution behavior for the tool */
  execution?: McpToolExecution;
  /** Icons provides visual identifiers for the tool */
  icons?: GithubComMark3LabsMcpGoMcpIcon[];
  /** A JSON Schema object defining the expected parameters for the tool. */
  inputSchema?: McpToolInputSchema;
  /** The name of the tool. */
  name?: string;
  /** A JSON Schema object defining the expected output returned by the tool . */
  outputSchema?: McpToolOutputSchema;
}

export interface GooseRecipeParameter {
  default?: string;
  description?: string;
  input_type?: string;
  key?: string;
  options?: string[];
  requirement?: string;
}

export interface GormDeletedAt {
  time?: string;
  /** Valid is true if Time is not NULL */
  valid?: boolean;
}

export interface HydraContainerBlkioStats {
  read_bytes?: number;
  session_id?: string;
  write_bytes?: number;
}

export enum HydraDevContainerStatus {
  DevContainerStatusStarting = "starting",
  DevContainerStatusRunning = "running",
  DevContainerStatusStopped = "stopped",
  DevContainerStatusError = "error",
}

export enum HydraDevContainerType {
  DevContainerTypeSway = "sway",
  DevContainerTypeUbuntu = "ubuntu",
  DevContainerTypeHeadless = "headless",
}

export interface HydraListSandboxCommandsResponse {
  commands?: HydraSandboxCommandResponse[];
}

export interface HydraListSandboxFilesResponse {
  entries?: HydraSandboxFileEntry[];
  path?: string;
}

export interface HydraSandboxCommandResponse {
  args?: string[];
  cmd?: string;
  cwd?: string;
  detached?: boolean;
  exit_code?: number;
  finished_at?: string;
  id?: string;
  sandbox_id?: string;
  started_at?: string;
  status?: string;
  stderr?: string;
  stdout?: string;
  sudo?: boolean;
}

export interface HydraSandboxFileEntry {
  is_dir?: boolean;
  mod_time?: string;
  mode?: string;
  name?: string;
  path?: string;
  size?: number;
}

export enum McpTaskSupport {
  TaskSupportForbidden = "forbidden",
  TaskSupportOptional = "optional",
  TaskSupportRequired = "required",
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

export interface McpToolExecution {
  /** TaskSupport indicates whether the tool supports task augmentation. */
  taskSupport?: McpTaskSupport;
}

export interface McpToolInputSchema {
  $defs?: Record<string, any>;
  additionalProperties?: any;
  properties?: Record<string, any>;
  required?: string[];
  type?: string;
}

export interface McpToolOutputSchema {
  $defs?: Record<string, any>;
  additionalProperties?: any;
  properties?: Record<string, any>;
  required?: string[];
  type?: string;
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
  /**
   * This property is used for the "reasoning" feature supported by deepseek-reasoner
   * which is not in the official documentation.
   * the doc from deepseek:
   * - https://api-docs.deepseek.com/api/create-chat-completion#responses
   */
  reasoning_content?: string;
  refusal?: string;
  role?: string;
  /** For Role=tool prompts this should be set to the ID given in the assistant's prior request to call a tool. */
  tool_call_id?: string;
  /** For Role=assistant prompts this may be set to the tool calls generated by the model, such as function calls. */
  tool_calls?: OpenaiToolCall[];
}

export interface OpenaiChatCompletionRequest {
  /**
   * ChatTemplateKwargs provides a way to add non-standard parameters to the request body.
   * Additional kwargs to pass to the template renderer. Will be accessible by the chat template.
   * Such as think mode for qwen3. "chat_template_kwargs": {"enable_thinking": false}
   * https://qwen.readthedocs.io/en/latest/deployment/vllm.html#thinking-non-thinking-modes
   */
  chat_template_kwargs?: Record<string, any>;
  frequency_penalty?: number;
  /** Deprecated: use ToolChoice instead. */
  function_call?: any;
  /** Deprecated: use Tools instead. */
  functions?: OpenaiFunctionDefinition[];
  /**
   * GuidedChoice is a vLLM-specific extension that restricts the model's output
   * to one of the predefined string choices provided in this field. This feature
   * is used to constrain the model's responses to a controlled set of options,
   * ensuring predictable and consistent outputs in scenarios where specific
   * choices are required.
   */
  guided_choice?: string[];
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
   * Deprecated: use MaxCompletionTokens. Not compatible with o1-series models.
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
  /** Configuration for a predicted output. */
  prediction?: OpenaiPrediction;
  presence_penalty?: number;
  /** Controls effort on reasoning for reasoning models. It can be set to "low", "medium", or "high". */
  reasoning_effort?: string;
  response_format?: OpenaiChatCompletionResponseFormat;
  /**
   * A stable identifier used to help detect users of your application that may be violating OpenAI's usage policies.
   * The IDs should be a string that uniquely identifies each user.
   * We recommend hashing their username or email address, in order to avoid sending us any identifying information.
   * https://platform.openai.com/docs/api-reference/chat/create#chat_create-safety_identifier
   */
  safety_identifier?: string;
  seed?: number;
  /** Specifies the latency tier to use for processing the request. */
  service_tier?: OpenaiServiceTier;
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
  /**
   * Verbosity determines how many output tokens are generated. Lowering the number of
   * tokens reduces overall latency. It can be set to "low", "medium", or "high".
   * Note: This field is only confirmed to work with gpt-5, gpt-5-mini and gpt-5-nano.
   * Also, it is not in the API reference of chat completion at the time of writing,
   * though it is supported by the API.
   */
  verbosity?: string;
}

export interface OpenaiChatCompletionResponse {
  choices?: OpenaiChatCompletionChoice[];
  created?: number;
  id?: string;
  model?: string;
  object?: string;
  prompt_filter_results?: OpenaiPromptFilterResult[];
  service_tier?: OpenaiServiceTier;
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
  accepted_prediction_tokens?: number;
  audio_tokens?: number;
  reasoning_tokens?: number;
  rejected_prediction_tokens?: number;
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

export interface OpenaiPrediction {
  content?: string;
  type?: string;
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

export enum OpenaiServiceTier {
  ServiceTierAuto = "auto",
  ServiceTierDefault = "default",
  ServiceTierFlex = "flex",
  ServiceTierPriority = "priority",
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

export interface ServerActivateTrialRequest {
  credits?: number;
  days?: number;
  /**
   * Plan selects what to grant. "pro" grants a PAID plan via a PlanOverride
   * (no Stripe subscription) — for customers who paid out-of-band (bank
   * transfer). Empty or "trial" uses the Stripe trial path (Days applies).
   */
  plan?: string;
}

export interface ServerActivateTrialResponse {
  org_id?: string;
  status?: string;
  user?: TypesUser;
}

export interface ServerAddDomainRequest {
  hostname?: string;
}

export interface ServerAgentConfigAppliedResponse {
  status?: string;
}

export interface ServerAgentSandboxesDebugResponse {
  dev_containers?: ServerDevContainerWithClients[];
  gpus?: ServerGPUInfoWithSandbox[];
  message?: string;
  sandboxes?: ServerSandboxInstanceInfo[];
}

export interface ServerAppCreateResponse {
  config?: TypesAppConfig;
  created?: string;
  global?: boolean;
  id?: string;
  /**
   * IsHelixOrgAgent is true when this app backs a Helix org-chart Worker
   * (see api/pkg/org). Computed at list time, not persisted, so the frontend
   * can hide org-chart agents from the spec-task agent switchers.
   */
  is_helix_org_agent?: boolean;
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

export interface ServerBatchTaskProgressResponse {
  project_id?: string;
  /** keyed by task_id */
  tasks?: Record<string, ServerTaskProgressResponse>;
}

export interface ServerBatchTaskUsageMetric {
  date?: string;
  total_tokens?: number;
}

export interface ServerBatchTaskUsageResponse {
  project_id?: string;
  /** keyed by task_id */
  tasks?: Record<string, ServerBatchTaskUsageMetric[]>;
}

export interface ServerClaudeLoginSessionResponse {
  session_id?: string;
}

export interface ServerClaudeModel {
  description?: string;
  id?: string;
  name?: string;
}

export interface ServerClaudePollLoginResponse {
  /** Raw credentials JSON */
  credentials?: string;
  found?: boolean;
  /** OAuth URL for native browser */
  url?: string;
}

export interface ServerClientBufferStats {
  buffer_pct?: number;
  buffer_size?: number;
  buffer_used?: number;
  client_id?: number;
}

export interface ServerCloneCommandResponse {
  clone_command?: string;
  clone_url?: string;
  repository_id?: string;
  target_dir?: string;
}

export interface ServerCodexLoginSessionResponse {
  session_id?: string;
}

export interface ServerCodexPollLoginResponse {
  code?: string;
  error?: string;
  found?: boolean;
  url?: string;
}

export interface ServerConfigurePendingSessionRequest {
  client_unique_id?: string;
}

export interface ServerCreateTopUpRequest {
  amount?: number;
  org_id?: string;
}

export interface ServerDeployWebServiceRequest {
  commit_sha?: string;
}

export interface ServerDevContainerWithClients {
  clients?: GithubComHelixmlHelixApiPkgServerClientInfo[];
  container_id?: string;
  container_name?: string;
  /** Container type */
  container_type?: HydraDevContainerType;
  /** Desktop environment info (for debug panel) */
  desktop_version?: string;
  error?: string;
  /** nvidia, amd, intel, or "" */
  gpu_vendor?: string;
  /** Network info for RevDial/screenshot-server connections */
  ip_address?: string;
  organization_id?: string;
  organization_name?: string;
  owner_name?: string;
  project_id?: string;
  project_name?: string;
  /** /dev/dri/renderD128 or SOFTWARE */
  render_node?: string;
  sandbox_id?: string;
  session_age?: string;
  session_id?: string;
  session_name?: string;
  status?: HydraDevContainerStatus;
  task_id?: string;
  task_name?: string;
  task_number?: number;
  /** First ~80 chars of original prompt */
  task_prompt?: string;
  video_stats?: ServerVideoStreamingStats;
}

export interface ServerForkSessionRequest {
  /**
   * AutoCommitUncommitted, when true, runs `git add -A && git commit
   * && git push` per dirty repo in the parent's container BEFORE the
   * parent is paused. Without this, any uncommitted file edits or
   * unpushed commits in the parent's container would be invisible to
   * the child (which boots a fresh clone). Defaults to true at the
   * API level — pass false explicitly to opt out (loses changes).
   * Push failures abort the fork; the parent is NOT paused.
   */
  auto_commit_uncommitted?: boolean;
  code_agent_runtime?: TypesCodeAgentRuntime;
  helix_app_id?: string;
}

export interface ServerForkSessionResponse {
  new_session_id?: string;
}

export interface ServerGPUInfoWithSandbox {
  index?: number;
  memory_free_bytes?: number;
  memory_total_bytes?: number;
  memory_used_bytes?: number;
  name?: string;
  sandbox_id?: string;
  temperature_celsius?: number;
  /** GPU core utilization */
  utilization_percent?: number;
  /** "nvidia", "amd", "intel" */
  vendor?: string;
}

export interface ServerGrantCreditsRequest {
  credits?: number;
  org_id?: string;
}

export interface ServerGrantCreditsResponse {
  org_id?: string;
  status?: string;
  user?: TypesUser;
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

export interface ServerInteractionBrief {
  id?: string;
  summary?: string;
  turn?: number;
}

export interface ServerInteractionWithContext {
  interaction?: TypesInteraction;
  next?: ServerInteractionBrief;
  previous?: ServerInteractionBrief;
  turn?: number;
}

export interface ServerKoditAdminActiveTaskDTO {
  current?: number;
  message?: string;
  operation?: string;
  repo_name?: string;
  repository_id?: number;
  state?: string;
  total?: number;
  updated_at?: string;
}

export interface ServerKoditAdminBatchRequest {
  ids?: number[];
}

export interface ServerKoditAdminBatchResponse {
  failed?: ServerKoditBatchError[];
  succeeded?: number[];
}

export interface ServerKoditAdminPaginationMeta {
  page?: number;
  per_page?: number;
  total?: number;
  total_pages?: number;
}

export interface ServerKoditAdminPendingTaskDTO {
  created_at?: string;
  id?: number;
  operation?: string;
  priority?: number;
}

export interface ServerKoditAdminQueueListResponse {
  active_tasks?: ServerKoditAdminActiveTaskDTO[];
  data?: ServerKoditAdminQueueTaskDTO[];
  meta?: ServerKoditAdminPaginationMeta;
  stats?: ServerKoditAdminQueueStats;
}

export interface ServerKoditAdminQueueStats {
  by_operation?: Record<string, number>;
  by_priority_level?: Record<string, number>;
  newest_task_time?: string;
  oldest_task_age?: string;
  oldest_task_time?: string;
  total?: number;
}

export interface ServerKoditAdminQueueTaskDTO {
  created_at?: string;
  id?: number;
  operation?: string;
  priority?: number;
  repo_name?: string;
  repository_id?: number;
}

export interface ServerKoditAdminRepoAttributes {
  created_at?: string;
  helix_org_id?: string;
  helix_repo_id?: string;
  helix_repo_name?: string;
  remote_url?: string;
  status?: string;
  status_message?: string;
  updated_at?: string;
}

export interface ServerKoditAdminRepoDTO {
  attributes?: ServerKoditAdminRepoAttributes;
  id?: string;
  type?: string;
}

export interface ServerKoditAdminRepoDetailAttributes {
  branch_count?: number;
  commit_count?: number;
  created_at?: string;
  default_branch?: string;
  enrichment_count?: number;
  helix_org_id?: string;
  helix_org_name?: string;
  helix_repo_id?: string;
  helix_repo_name?: string;
  /** Last time Kodit scanned this repository */
  last_scanned_at?: string;
  latest_commit_author?: string;
  latest_commit_date?: string;
  latest_commit_message?: string;
  /** Latest commit tracked by Kodit */
  latest_commit_sha?: string;
  remote_url?: string;
  status?: string;
  status_message?: string;
  tag_count?: number;
  updated_at?: string;
}

export interface ServerKoditAdminRepoDetailDTO {
  attributes?: ServerKoditAdminRepoDetailAttributes;
  id?: string;
  type?: string;
}

export interface ServerKoditAdminRepoDetailResponse {
  data?: ServerKoditAdminRepoDetailDTO;
}

export interface ServerKoditAdminRepoListResponse {
  data?: ServerKoditAdminRepoDTO[];
  meta?: ServerKoditAdminPaginationMeta;
}

export interface ServerKoditAdminRepositoryTasksResponse {
  pending_tasks?: ServerKoditAdminPendingTaskDTO[];
  statuses?: ServerKoditAdminTaskStatusDTO[];
}

export interface ServerKoditAdminStatsResponse {
  commits?: number;
  enrichments?: number;
  pending_tasks?: number;
  repositories?: number;
}

export interface ServerKoditAdminTaskStatusDTO {
  current?: number;
  error?: string;
  message?: string;
  operation?: string;
  state?: string;
  total?: number;
  updated_at?: string;
}

export interface ServerKoditAdminUpdatePriorityRequest {
  priority?: number;
}

export interface ServerKoditBatchError {
  id?: number;
  message?: string;
}

export interface ServerKoditCommitAttributes {
  authored_at?: string;
  committed_at?: string;
  message?: string;
  sha?: string;
}

export interface ServerKoditCommitDTO {
  attributes?: ServerKoditCommitAttributes;
  id?: string;
  type?: string;
}

export interface ServerKoditEnrichmentAttributes {
  content?: string;
  created_at?: string;
  subtype?: string;
  type?: string;
  updated_at?: string;
}

export interface ServerKoditEnrichmentDTO {
  attributes?: ServerKoditEnrichmentAttributes;
  commit_sha?: string;
  id?: string;
  type?: string;
}

export interface ServerKoditEnrichmentListResponse {
  data?: ServerKoditEnrichmentDTO[];
}

export interface ServerKoditEnrichmentsMeta {
  commit_sha?: string;
  count?: number;
  enrichment_type?: string;
  kodit_repo_id?: number;
  page?: number;
  per_page?: number;
  total?: number;
  total_pages?: number;
}

export interface ServerKoditFileContentDTO {
  commit_sha?: string;
  content?: string;
  path?: string;
}

export interface ServerKoditFileContentResponse {
  data?: ServerKoditFileContentDTO;
  links?: Record<string, string>;
}

export interface ServerKoditFileEntryDTO {
  links?: Record<string, string>;
  path?: string;
  size?: number;
}

export interface ServerKoditFileResultDTO {
  language?: string;
  lines?: string;
  links?: Record<string, string>;
  page?: number;
  path?: string;
  preview?: string;
  score?: number;
}

export interface ServerKoditFilesMeta {
  count?: number;
  pattern?: string;
}

export interface ServerKoditFilesResponse {
  data?: ServerKoditFileEntryDTO[];
  links?: Record<string, string>;
  meta?: ServerKoditFilesMeta;
}

export interface ServerKoditGrepMatchDTO {
  content?: string;
  line?: number;
}

export interface ServerKoditGrepMeta {
  count?: number;
  glob?: string;
  limit?: number;
  pattern?: string;
}

export interface ServerKoditGrepResponse {
  data?: ServerKoditGrepResultDTO[];
  links?: Record<string, string>;
  meta?: ServerKoditGrepMeta;
}

export interface ServerKoditGrepResultDTO {
  language?: string;
  links?: Record<string, string>;
  matches?: ServerKoditGrepMatchDTO[];
  path?: string;
}

export interface ServerKoditIndexingStatusAttributes {
  message?: string;
  status?: string;
  tasks_active?: number;
  tasks_completed?: number;
  tasks_failed?: number;
  tasks_pending?: number;
  tasks_total?: number;
  updated_at?: string;
}

export interface ServerKoditIndexingStatusDTO {
  data?: ServerKoditIndexingStatusData;
}

export interface ServerKoditIndexingStatusData {
  attributes?: ServerKoditIndexingStatusAttributes;
  id?: string;
  type?: string;
}

export interface ServerKoditRepoEnrichmentsResponse {
  data?: ServerKoditEnrichmentDTO[];
  links?: Record<string, string>;
  meta?: ServerKoditEnrichmentsMeta;
}

export interface ServerKoditSearchMeta {
  count?: number;
  language?: string;
  limit?: number;
  query?: string;
}

export interface ServerKoditSearchResponse {
  data?: ServerKoditFileResultDTO[];
  links?: Record<string, string>;
  meta?: ServerKoditSearchMeta;
}

export interface ServerKoditSearchResultDTO {
  content?: string;
  id?: string;
  language?: string;
  type?: string;
}

export interface ServerKoditWikiPageDTO {
  content?: string;
  slug?: string;
  title?: string;
}

export interface ServerKoditWikiPageResponse {
  data?: ServerKoditWikiPageDTO;
  links?: Record<string, string>;
}

export interface ServerKoditWikiTreeNodeDTO {
  children?: ServerKoditWikiTreeNodeDTO[];
  links?: Record<string, string>;
  path?: string;
  slug?: string;
  title?: string;
}

export interface ServerKoditWikiTreeResponse {
  data?: ServerKoditWikiTreeNodeDTO[];
  links?: Record<string, string>;
}

export interface ServerLicenseKeyRequest {
  license_key?: string;
}

export interface ServerMintPreviewTokenRequest {
  port?: number;
}

export interface ServerModelSubstitution {
  assistant_name?: string;
  new_model?: string;
  new_provider?: string;
  original_model?: string;
  original_provider?: string;
  reason?: string;
}

export interface ServerOrganizationDomainInfo {
  auto_join_domain?: string;
  organization_id?: string;
  organization_name?: string;
}

export interface ServerOwnedOrgSummary {
  display_name?: string;
  id?: string;
  name?: string;
}

export interface ServerPhaseProgress {
  agent?: string;
  completed_at?: string;
  revision_count?: number;
  session_id?: string;
  started_at?: string;
  status?: string;
}

export interface ServerPinnedProjectsResponse {
  pinned_project_ids?: string[];
}

export interface ServerProjectGooseRecipe {
  description?: string;
  /**
   * Error, when non-empty, indicates that the recipe was declared on the
   * agent but couldn't be loaded — repo not cloned yet, file missing,
   * YAML malformed, etc. The UI surfaces this so the user can fix the
   * project YAML before creating a task that would silently fall back to
   * vanilla goose.
   */
  error?: string;
  name?: string;
  parameters?: GooseRecipeParameter[];
  title?: string;
}

export interface ServerProjectWebServiceLogsResponse {
  /**
   * Log is the combined stdout/stderr of the project's startup script — build
   * output, app logs, and the reason a deploy did or didn't come up. Empty
   * when the service isn't deployed yet.
   */
  log?: string;
}

export interface ServerProjectWebServiceResponse {
  /**
   * ACMEChallengeTarget is the fixed CNAME value customers point
   * "_acme-challenge.<their-domain>" at when the domain is behind a
   * proxy/CDN that hides the origin from Let's Encrypt. Empty when the
   * operator has not configured delegation (HELIX_VHOST_ACME_CHALLENGE_TARGET).
   */
  acme_challenge_target?: string;
  /**
   * CNAMETarget is the hostname customers should add as the value of
   * their CNAME record when registering a custom domain — i.e. the
   * canonical Helix hostname parsed from SERVER_URL. Empty when the
   * vhost feature is not configured on this instance.
   */
  cname_target?: string;
  deploys?: TypesWebServiceDeploy[];
  domains?: TypesVHostRoute[];
  /**
   * Health is the real, probe-based status of the web service — "disabled",
   * "deploying", "live" or "unhealthy" — so the UI reflects whether the app
   * actually answers, not just the last deploy row (which stays "live" long
   * after its container dies).
   */
  health?: string;
  state?: TypesProjectWebServiceState;
}

export interface ServerPromptPinRequest {
  pinned?: boolean;
}

export interface ServerPromptTagsRequest {
  /** JSON array of tags */
  tags?: string;
}

export interface ServerPushPullResponse {
  branch?: string;
  message?: string;
  repository_id?: string;
  success?: boolean;
}

export interface ServerPutProjectWebServiceRequest {
  container_port?: number;
  enabled?: boolean;
}

export interface ServerQuickCreateProjectRequest {
  name?: string;
  repo_id?: string;
}

export interface ServerRequiredGitHubRepo {
  /** AllowFork allows users without write access to fork the repo to their account */
  allow_fork?: boolean;
  /** DefaultBranch overrides the default branch for this repo */
  default_branch?: string;
  /** GitHubURL is the GitHub repository URL (e.g., "github.com/helixml/helix") */
  github_url?: string;
  /** IsPrimary marks this as the primary/default repository for the project */
  is_primary?: boolean;
  /**
   * SubPath is the directory name to clone into (e.g., "helix", "zed")
   * If empty, uses the repo name
   */
  sub_path?: string;
}

export interface ServerSampleTaskPrompt {
  /** Tags for organization */
  labels?: string[];
  /** "low", "medium", "high", "critical" */
  priority?: TypesSpecTaskPriority;
  /** Natural language request (include all context here) */
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

export interface ServerSandboxBillingResponse {
  enabled?: boolean;
  pending_credits?: number;
  price_credits_per_second?: number;
  runtime?: string;
  total_credits_charged?: number;
  vcpus?: number;
}

export interface ServerSandboxInstanceInfo {
  /**
   * Inference profile state — populated from the heartbeat. Empty for
   * pure-agent sandboxes with no profile assigned.
   */
  active_profile_id?: string;
  container_id?: string;
  /** nvidia | amd | neuron */
  gpu_vendor?: string;
  /** per-accelerator inventory */
  gpus?: TypesGPUStatus[];
  id?: string;
  /**
   * Hardware reported by the sandbox heartbeat — drives the admin UI's
   * per-runner architecture display so an operator can pick a compatible
   * profile. InstanceType is empty on bare-metal hosts (e.g. prime).
   */
  instance_type?: string;
  profile_error?: string;
  profile_progress?: Record<string, TypesServiceDownloadProgress>;
  profile_status?: string;
  service_health?: Record<string, string>;
  session_id?: string;
  status?: string;
}

export interface ServerSandboxTerminalSession {
  attached?: boolean;
  created?: number;
  name?: string;
  windows?: number;
}

export interface ServerSandboxTerminalSessionsResponse {
  sessions?: ServerSandboxTerminalSession[];
}

export interface ServerSessionClaudeCredentialsResponse {
  /** "oauth" or "setup_token" */
  credential_type?: string;
  oauth_credentials?: TypesClaudeOAuthCredentials;
  setup_token?: string;
}

export interface ServerSessionMessageRequest {
  content?: string;
  interrupt?: boolean;
  notify_user_id?: string;
}

export interface ServerSessionMessageResponse {
  prompt_id?: string;
}

export interface ServerSessionSandboxStateResponse {
  container_id?: string;
  session_id?: string;
  /** "absent", "running", "starting" */
  state?: string;
}

export interface ServerSessionTOCEntry {
  /** When this turn happened */
  created?: string;
  /** Whether there's a user prompt */
  has_prompt?: boolean;
  /** Whether there's an assistant response */
  has_response?: boolean;
  /** Interaction ID */
  id?: string;
  /** One-line summary */
  summary?: string;
  /** 1-indexed turn number */
  turn?: number;
}

export interface ServerSessionTOCResponse {
  entries?: ServerSessionTOCEntry[];
  /** Formatted is a pre-formatted numbered list for easy consumption */
  formatted?: string;
  session_id?: string;
  session_name?: string;
  total_turns?: number;
}

export interface ServerSetActiveSandboxRequest {
  sandbox_id?: string;
}

export interface ServerSetOrgPlanRequest {
  /**
   * Plan: "pro" | "free" forces the org's quota tier independent of Stripe
   * (for customers who paid out-of-band). "" clears the override and reverts
   * to the Stripe-derived tier.
   */
  plan?: string;
}

export interface ServerSharePointSiteResolveRequest {
  provider_id?: string;
  site_url?: string;
}

export interface ServerSharePointSiteResolveResponse {
  display_name?: string;
  site_id?: string;
  web_url?: string;
}

export interface ServerSimpleSampleProject {
  category?: string;
  default_branch?: string;
  demo_url?: string;
  description?: string;
  difficulty?: string;
  /** Whether this sample project is shown to users */
  enabled?: boolean;
  github_repo?: string;
  /**
   * Guidelines are project-specific instructions for AI agents
   * These get injected into implementation prompts and help the agent understand
   * project conventions, available tools, and how to use them effectively
   */
  guidelines?: string;
  id?: string;
  name?: string;
  readme_url?: string;
  /**
   * RequiredGitHubRepos specifies GitHub repos that must be cloned for this sample project.
   * When set, the project creation flow will:
   * 1. Check if user has GitHub OAuth connected
   * 2. Verify write access to each repo (or offer to fork)
   * 3. Clone repos with authentication
   * 4. Wait for cloning to complete before starting session
   */
  required_repositories?: ServerRequiredGitHubRepo[];
  /**
   * RequiredScopes specifies the OAuth scopes needed for this sample project
   * These are passed when initiating the OAuth flow, so the user authorizes exactly what's needed
   */
  required_scopes?: string[];
  /** RequiresGitHubAuth indicates this sample project needs GitHub OAuth for push access */
  requires_github_auth?: boolean;
  /**
   * Skills configures project-level skills that will be added when the project is created
   * These overlay on top of agent-level skills
   */
  skills?: TypesAssistantSkills;
  task_prompts?: ServerSampleTaskPrompt[];
  technologies?: string[];
}

export interface ServerSwitchAgentRequest {
  code_agent_runtime?: TypesCodeAgentRuntime;
  helix_app_id?: string;
}

export interface ServerSwitchAgentResponse {
  agent_runtime?: TypesCodeAgentRuntime;
  helix_app_id?: string;
  session_id?: string;
}

export interface ServerTaskProgressResponse {
  /** Progress from tasks.md */
  checklist?: TypesChecklistProgress;
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
  status?: TypesSpecTaskStatus;
  task_id?: string;
  technical_design?: string;
}

export interface ServerVideoStreamingStats {
  client_buffers?: ServerClientBufferStats[];
  client_count?: number;
  frames_received?: number;
  gop_buffer_size?: number;
}

export interface ServerWorkspaceRepoStatus {
  branch?: string;
  /**
   * Error is set when we couldn't determine the status (container
   * unreachable, path missing, git command failed). The repo is then
   * excluded from "dirty" totals — we don't refuse a fork because we
   * can't see one repo's state.
   */
  error?: string;
  name?: string;
  repo_id?: string;
  uncommitted_files?: number;
  unpushed_commits?: number;
}

export interface ServerWorkspaceStatusResponse {
  /**
   * CanSaveChanges is false when there ARE dirty changes but the
   * fork's pre-commit safety net has nowhere viable to push them.
   * Concretely: the session has no spec task, or the spec task has
   * no branch name set, or the spec task's branch is a protected
   * branch (main / master) that the remote pre-receive hook will
   * reject. In any of those cases the frontend should refuse to
   * offer "Fork with auto-commit" — the user has to fix git state
   * manually (commit/push to a feature branch from the terminal)
   * before forking, OR explicitly abandon the changes.
   */
  can_save_changes?: boolean;
  /**
   * CannotSaveReason is a human-readable explanation surfaced in
   * the blocking modal. Empty when CanSaveChanges is true.
   */
  cannot_save_reason?: string;
  /**
   * ContainerReachable=false means we couldn't talk to the desktop
   * at all (e.g. it's been reaped). The frontend should treat this
   * as "unknown" and let the user decide whether to fork anyway.
   */
  container_reachable?: boolean;
  /**
   * ExpectedBranch is the branch the pre-fork commit will target,
   * resolved from the spec task. Empty for sessions without a
   * spec task. Exposed so the frontend can say "will commit to
   * <branch>" instead of just "will commit" — helps the user
   * understand what's about to happen.
   */
  expected_branch?: string;
  is_dirty?: boolean;
  repos?: ServerWorkspaceRepoStatus[];
  session_id?: string;
  total_dirty?: number;
}

export interface ServerAddLabelRequest {
  label?: string;
}

export interface ServerConnectSlackWorkspaceRequest {
  app_connection_id?: string;
  bot_token?: string;
}

export interface ServerOpenaiModelEntry {
  created?: number;
  id?: string;
  object?: string;
  owned_by?: string;
}

export interface ServerOpenaiModelsResponse {
  data?: ServerOpenaiModelEntry[];
  object?: string;
}

export interface ServerRunnerProfileAssignRequest {
  profile_id?: string;
}

export interface ServerRunnerProfileSaveRequest {
  architectures?: string[];
  compose_yaml?: string;
  description?: string;
  min_vram_bytes?: number;
  model_match?: string;
  name?: string;
  vendor?: TypesGPUVendor;
}

export interface ServicesStartupScriptVersion {
  author?: string;
  commit_hash?: string;
  content?: string;
  message?: string;
  timestamp?: string;
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
  status_code?: number;
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

export interface TypesAccountUpdateRequest {
  full_name?: string;
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
  /**
   * AppID + GrantRoles are optional and only meaningful when the
   * request creates an invitation (because the email doesn't match an
   * existing user). They tell the server which app to attach the
   * invitation to, so the access grant can be materialised when the
   * invitee accepts. When set, the invitation will also be filtered
   * to this app in the access-management list.
   */
  app_id?: string;
  grant_roles?: string[];
  role?: TypesOrganizationRole;
  /** Either user ID or user email */
  user_reference?: string;
}

export interface TypesAddOrganizationMemberResponse {
  invitation?: TypesOrganizationInvitation;
  /**
   * Invited is true when no user exists yet and an invitation row was
   * created instead of a membership.
   */
  invited?: boolean;
  membership?: TypesOrganizationMembership;
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

export interface TypesAdminResetPasswordRequest {
  new_password?: string;
}

export interface TypesAffectedProjectInfo {
  id?: string;
  name?: string;
}

export enum TypesAgentType {
  AgentTypeHelixBasic = "helix_basic",
  AgentTypeHelixAgent = "helix_agent",
  AgentTypeZedExternal = "zed_external",
}

export enum TypesAgentWorkState {
  AgentWorkStateIdle = "idle",
  AgentWorkStateWorking = "working",
  AgentWorkStateDone = "done",
}

export interface TypesAggregatedUsageMetric {
  cache_read_cost?: number;
  cache_read_tokens?: number;
  cache_write_cost?: number;
  cache_write_tokens?: number;
  completion_cost?: number;
  completion_tokens?: number;
  /** ID    string    `json:"id" gorm:"primaryKey"` */
  date?: string;
  latency_ms?: number;
  prompt_cost?: number;
  prompt_tokens?: number;
  request_size_bytes?: number;
  response_size_bytes?: number;
  sandbox_cost?: number;
  /** Prompt + completion + cache read + cache write */
  total_cost?: number;
  total_requests?: number;
  total_tokens?: number;
}

export interface TypesApiKey {
  app_id?: SqlNullString;
  created?: string;
  key?: string;
  name?: string;
  /** Used for isolation and metrics tracking */
  organization_id?: string;
  owner?: string;
  owner_type?: TypesOwnerType;
  /** Used for isolation and metrics tracking */
  project_id?: string;
  /** Session this key is scoped to (ephemeral keys) */
  session_id?: string;
  /** Used for isolation and metrics tracking */
  spec_task_id?: string;
  type?: TypesAPIKeyType;
}

export interface TypesApp {
  config?: TypesAppConfig;
  created?: string;
  global?: boolean;
  id?: string;
  /**
   * IsHelixOrgAgent is true when this app backs a Helix org-chart Worker
   * (see api/pkg/org). Computed at list time, not persisted, so the frontend
   * can hide org-chart agents from the spec-task agent switchers.
   */
  is_helix_org_agent?: boolean;
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
  /** AgentType specifies the type of agent to use */
  agent_type?: TypesAgentType;
  apis?: TypesAssistantAPI[];
  avatar?: string;
  azure_devops?: TypesAssistantAzureDevOps;
  browser?: TypesAssistantBrowser;
  calculator?: TypesAssistantCalculator;
  /**
   * ClaudeSubscriptionModel is the Anthropic model to use when CodeAgentRuntime is
   * "claude_code" and CodeAgentCredentialType is "subscription". It flows through
   * CodeAgentConfig.Model into the container's /etc/claude-code/managed-settings.json,
   * which the claude-agent-acp package reads (resolveModelPreference) to pick the
   * model — otherwise Claude Code defaults to Sonnet. Empty means "opus"
   * (resolveModelPreference resolves this to the latest Opus version).
   */
  claude_subscription_model?: string;
  /**
   * CodeAgentCredentialType specifies how the code agent authenticates with the LLM provider.
   * "api_key" (default/empty): uses an API key routed through the Helix proxy.
   * "subscription": uses OAuth credentials directly (e.g., Claude subscription).
   */
  code_agent_credential_type?: TypesCodeAgentCredentialType;
  /**
   * CodeAgentRuntime specifies which code agent runtime to use inside Zed (for zed_external agent type).
   * Options: "zed_agent" (Zed's built-in agent) or "qwen_code" (qwen command as custom agent).
   * If empty, defaults to "zed_agent".
   */
  code_agent_runtime?: TypesCodeAgentRuntime;
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
  /**
   * GooseRecipeRepoURL is the external git URL of the attached repository
   * that holds the project's Goose recipes (e.g. https://github.com/foo/bar).
   * Resolved against attached GitRepositories at sandbox-start time.
   * Empty means recipes are looked up under the primary repository.
   */
  goose_recipe_repo_url?: string;
  /**
   * GooseRecipes are the project-declared Goose recipes (slash-command name
   * + repo-relative path to the recipe YAML).
   */
  goose_recipes?: TypesAssistantGooseRecipe[];
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
  project_manager?: TypesAssistantProjectManager;
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

export interface TypesAssistantGooseRecipe {
  name?: string;
  path?: string;
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
  /** Command arguments */
  args?: string[];
  /**
   * Stdio transport fields (used when Transport is "stdio")
   * The MCP server runs as a subprocess inside the dev container
   */
  command?: string;
  description?: string;
  /** Environment variables for the subprocess */
  env?: Record<string, string>;
  headers?: Record<string, string>;
  name?: string;
  /** The name of the OAuth provider to use for authentication */
  oauth_provider?: string;
  /** Required OAuth scopes for this API */
  oauth_scopes?: string[];
  tools?: GithubComMark3LabsMcpGoMcpTool[];
  /**
   * Transport type: "http" (default, Streamable HTTP), "sse" (legacy SSE), or "stdio" (command execution)
   * For stdio transport, use Command/Args/Env fields instead of URL
   */
  transport?: string;
  /** HTTP/SSE transport fields (used when Transport is "http" or "sse", or URL is set) */
  url?: string;
}

export interface TypesAssistantProjectManager {
  enabled?: boolean;
  project_id?: string;
}

export interface TypesAssistantSkills {
  apis?: TypesAssistantAPI[];
  azure_devops?: TypesAssistantAzureDevOps;
  browser?: TypesAssistantBrowser;
  calculator?: TypesAssistantCalculator;
  email?: TypesAssistantEmail;
  mcps?: TypesAssistantMCP[];
  project_manager?: TypesAssistantProjectManager;
  web_search?: TypesAssistantWebSearch;
  zapier?: TypesAssistantZapier[];
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

export interface TypesAttentionEvent {
  acknowledged_at?: string;
  created_at?: string;
  description?: string;
  dismissed_at?: string;
  event_type?: TypesAttentionEventType;
  id?: string;
  idempotency_key?: string;
  metadata?: number[];
  organization_id?: string;
  project_id?: string;
  /** Denormalized for display without joins */
  project_name?: string;
  /**
   * RepliedAt is set when the user answers an org_message inline from the
   * notification bell. It keeps replied messages visible (marked "Replied")
   * so the user has a record the message came through and was answered.
   */
  replied_at?: string;
  snoozed_until?: string;
  spec_task_description?: string;
  spec_task_id?: string;
  spec_task_name?: string;
  title?: string;
  user_id?: string;
}

export enum TypesAttentionEventType {
  AttentionEventSpecsPushed = "specs_pushed",
  AttentionEventAgentInteractionCompleted = "agent_interaction_completed",
  AttentionEventSpecFailed = "spec_failed",
  AttentionEventImplementationFailed = "implementation_failed",
  AttentionEventPRReady = "pr_ready",
  AttentionEventOrgMessage = "org_message",
  AttentionEventCIPassed = "ci_passed",
  AttentionEventCIFailed = "ci_failed",
}

export interface TypesAttentionEventUpdateRequest {
  acknowledge?: boolean;
  dismiss?: boolean;
  /**
   * Reply marks an org_message answered — sets replied_at (and acknowledges),
   * keeping it visible as "Replied" instead of dismissing it.
   */
  reply?: boolean;
  snoozed_until?: string;
}

export enum TypesAuditEventType {
  AuditEventTaskCreated = "task_created",
  AuditEventTaskCloned = "task_cloned",
  AuditEventTaskApproved = "task_approved",
  AuditEventTaskCompleted = "task_completed",
  AuditEventTaskArchived = "task_archived",
  AuditEventTaskUnarchived = "task_unarchived",
  AuditEventAgentPrompt = "agent_prompt",
  AuditEventUserMessage = "user_message",
  AuditEventAgentStarted = "agent_started",
  AuditEventSpecGenerated = "spec_generated",
  AuditEventSpecUpdated = "spec_updated",
  AuditEventReviewComment = "review_comment",
  AuditEventReviewCommentReply = "review_comment_reply",
  AuditEventPRCreated = "pr_created",
  AuditEventPRMerged = "pr_merged",
  AuditEventGitPush = "git_push",
  AuditEventProjectCreated = "project_created",
  AuditEventProjectDeleted = "project_deleted",
  AuditEventProjectSettingsUpdated = "project_settings_updated",
  AuditEventProjectGuidelinesUpdated = "project_guidelines_updated",
}

export interface TypesAuditMetadata {
  branch_name?: string;
  /** Group ID linking related cloned tasks */
  clone_group_id?: string;
  /** Clone tracking (matches SpecTask fields) */
  cloned_from_id?: string;
  /** Source project ID if cloned from another project */
  cloned_from_project_id?: string;
  comment_id?: string;
  /** Design review tracking */
  design_review_id?: string;
  /** External system tracking (e.g., Azure DevOps) */
  external_task_id?: string;
  external_task_url?: string;
  /** Hash of implementation plan content */
  implementation_plan_hash?: string;
  /** For scrolling to specific interaction in session view */
  interaction_id?: string;
  /** Project information */
  project_name?: string;
  /** Pull request information */
  pull_requests?: TypesRepoPR[];
  /** Hash of requirements spec content */
  requirements_spec_hash?: string;
  /** Helix session/interaction linking */
  session_id?: string;
  /** Spec versioning - capture spec state at time of event */
  spec_version?: number;
  task_name?: string;
  /** Task information */
  task_number?: number;
  /** Hash of technical design content */
  technical_design_hash?: string;
}

export enum TypesAuthProvider {
  AuthProviderRegular = "regular",
  AuthProviderOIDC = "oidc",
}

export interface TypesAuthenticatedResponse {
  authenticated?: boolean;
}

export interface TypesAzureDevOps {
  /** App registration client ID */
  client_id?: string;
  /** App registration client secret */
  client_secret?: string;
  organization_url?: string;
  personal_access_token?: string;
  /**
   * Service Principal authentication (service-to-service via Azure AD/Entra ID)
   * Uses OAuth 2.0 client credentials flow for automated system access
   */
  tenant_id?: string;
}

export interface TypesAzureDevOpsTrigger {
  enabled?: boolean;
}

export interface TypesBitbucket {
  /** Bitbucket App Password (recommended over regular password) */
  app_password?: string;
  /** For Bitbucket Server/Data Center (empty for bitbucket.org) */
  base_url?: string;
  /** Bitbucket username (required for API auth) */
  username?: string;
}

export interface TypesBoardSettings {
  wip_limits?: TypesWIPLimits;
}

export enum TypesBranchMode {
  BranchModeNew = "new",
  BranchModeExisting = "existing",
}

export interface TypesBrowseRemoteRepositoriesRequest {
  /** Base URL for self-hosted instances (for GitHub Enterprise, GitLab Enterprise, or Bitbucket Server) */
  base_url?: string;
  /** Organization URL (required for Azure DevOps) */
  organization_url?: string;
  /** Provider type: "github", "gitlab", "ado", "bitbucket" */
  provider_type?: TypesExternalRepositoryType;
  /** Personal Access Token or App Password for authentication */
  token?: string;
  /** Username for authentication (required for Bitbucket) */
  username?: string;
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

export interface TypesCheckSampleProjectAccessRequest {
  github_connection_id?: string;
  sample_project_id?: string;
}

export interface TypesCheckSampleProjectAccessResponse {
  all_have_write_access?: boolean;
  github_username?: string;
  has_github_connected?: boolean;
  repositories?: TypesRepositoryAccessCheck[];
  sample_project_id?: string;
}

export interface TypesChecklistItem {
  description?: string;
  index?: number;
  /** pending, in_progress, completed */
  status?: string;
}

export interface TypesChecklistProgress {
  completed_tasks?: number;
  in_progress_task?: TypesChecklistItem;
  /** 0-100 */
  progress_pct?: number;
  tasks?: TypesChecklistItem[];
  total_tasks?: number;
}

export interface TypesChoice {
  delta?: TypesOpenAIMessage;
  finish_reason?: string;
  index?: number;
  message?: TypesOpenAIMessage;
  text?: string;
}

export interface TypesClaudeOAuthCredentials {
  accessToken?: string;
  /** Unix milliseconds */
  expiresAt?: number;
  rateLimitTier?: string;
  refreshToken?: string;
  scopes?: string[];
  subscriptionType?: string;
}

export interface TypesClaudeSubscription {
  access_token_expires_at?: string;
  created?: string;
  created_by?: string;
  /** "oauth" or "setup_token" */
  credential_type?: string;
  id?: string;
  last_error?: string;
  last_refreshed_at?: string;
  name?: string;
  owner_id?: string;
  /** "user" or "org" */
  owner_type?: TypesOwnerType;
  rate_limit_tier?: string;
  scopes?: string[];
  /** "active", "expired", "error" */
  status?: string;
  /** "max", "pro" */
  subscription_type?: string;
  updated?: string;
}

export interface TypesClipboardData {
  /** text content or base64-encoded image */
  data?: string;
  /** "text" or "image" */
  type?: string;
}

export interface TypesCloneGroup {
  created_at?: string;
  created_by?: string;
  id?: string;
  source_project_id?: string;
  source_prompt?: string;
  source_requirements_spec?: string;
  source_task_id?: string;
  source_task_name?: string;
  source_technical_spec?: string;
  total_targets?: number;
}

export interface TypesCloneGroupProgress {
  clone_group_id?: string;
  completed_tasks?: number;
  /** Full task objects for TaskCard rendering */
  full_tasks?: TypesSpecTaskWithProject[];
  progress_pct?: number;
  source_task?: TypesCloneGroupSourceTask;
  /** status -> count */
  status_breakdown?: Record<string, number>;
  tasks?: TypesCloneGroupTaskProgress[];
  total_tasks?: number;
}

export interface TypesCloneGroupSourceTask {
  name?: string;
  project_id?: string;
  project_name?: string;
  task_id?: string;
}

export interface TypesCloneGroupTaskProgress {
  name?: string;
  project_id?: string;
  project_name?: string;
  status?: string;
  task_id?: string;
}

export interface TypesCloneProgress {
  /** Bytes received so far */
  bytes_received?: number;
  /** Current object count */
  current?: number;
  /** 0-100 */
  percentage?: number;
  /** "counting", "compressing", "receiving", "resolving", "done" */
  phase?: string;
  /** e.g., "1.25 MiB/s" */
  speed?: string;
  /** When clone started */
  started_at?: string;
  /** Total object count */
  total?: number;
}

export interface TypesCloneTaskCreateProjectSpec {
  /** Optional, will use repo name if not provided */
  name?: string;
  repo_id?: string;
}

export interface TypesCloneTaskError {
  error?: string;
  project_id?: string;
  repo_id?: string;
}

export interface TypesCloneTaskRequest {
  /** Auto-start cloned tasks */
  auto_start?: boolean;
  /** Create new projects for repos */
  create_projects?: TypesCloneTaskCreateProjectSpec[];
  target_project_ids?: string[];
}

export interface TypesCloneTaskResponse {
  clone_group_id?: string;
  cloned_tasks?: TypesCloneTaskResult[];
  errors?: TypesCloneTaskError[];
  total_cloned?: number;
  total_failed?: number;
}

export interface TypesCloneTaskResult {
  project_id?: string;
  project_name?: string;
  /** "created", "started", "failed" */
  status?: string;
  task_id?: string;
}

export interface TypesCodeAgentBakedRecipe {
  /** Content is the substituted recipe YAML (full file content). */
  content?: string;
  /** Name is the slash-command slug (no leading slash). */
  name?: string;
}

export interface TypesCodeAgentConfig {
  /** AgentName is the name used in Zed's agent_servers config (e.g., "qwen", "claude-code") */
  agent_name?: string;
  /** APIType specifies the API format: "anthropic", "openai", or "azure_openai" */
  api_type?: string;
  /** BaseURL is the Helix proxy endpoint URL (e.g., "https://helix.example.com/v1") */
  base_url?: string;
  /**
   * GooseBakedRecipe, when set, holds a single recipe with parameters
   * pre-substituted, used by Phase 2b spec-task automation. The daemon
   * writes it to disk and registers a single slash_command so an initial
   * "/<slug>" prompt fires the recipe.
   */
  goose_baked_recipe?: TypesCodeAgentBakedRecipe;
  /**
   * GooseRecipeRootDir is the absolute container path to the root of the
   * recipes git repo (used as GOOSE_RECIPE_PATH so subrecipes/fragments
   * resolve relative paths correctly).
   */
  goose_recipe_root_dir?: string;
  /**
   * GooseRecipes lists project-declared Goose recipes with absolute paths
   * resolved inside the desktop container. Only set when Runtime is
   * goose_code; consumed by settings-sync-daemon to write the goose
   * slash_commands config.
   */
  goose_recipes?: TypesCodeAgentGooseRecipe[];
  /**
   * MaxOutputTokens is the model's max completion tokens
   * Looked up from model_info.json, 0 if not found
   */
  max_output_tokens?: number;
  /**
   * MaxTokens is the model's context window size (max input tokens)
   * Looked up from model_info.json, 0 if not found
   */
  max_tokens?: number;
  /** Model is the model identifier (e.g., "claude-sonnet-4-5-latest", "gpt-4o") */
  model?: string;
  /** Provider is the LLM provider name (e.g., "anthropic", "openai", "openrouter") */
  provider?: string;
  /**
   * ReasoningEffort controls the selected Claude Code or Codex model's reasoning effort.
   * Empty means the runtime/model default.
   */
  reasoning_effort?: string;
  /** Runtime specifies which code agent runtime to use: "zed_agent" or "qwen_code" */
  runtime?: TypesCodeAgentRuntime;
}

export enum TypesCodeAgentCredentialType {
  CodeAgentCredentialTypeAPIKey = "api_key",
  CodeAgentCredentialTypeSubscription = "subscription",
}

export interface TypesCodeAgentGooseRecipe {
  name?: string;
  path?: string;
}

export enum TypesCodeAgentRuntime {
  CodeAgentRuntimeZedAgent = "zed_agent",
  CodeAgentRuntimeQwenCode = "qwen_code",
  CodeAgentRuntimeClaudeCode = "claude_code",
  CodeAgentRuntimeGeminiCLI = "gemini_cli",
  CodeAgentRuntimeCodexCLI = "codex_cli",
  CodeAgentRuntimeGooseCode = "goose_code",
}

export interface TypesCodexAuthCredentials {
  OPENAI_API_KEY?: string;
  auth_mode?: string;
  last_refresh?: string;
  tokens?: TypesCodexAuthTokens;
}

export interface TypesCodexAuthTokens {
  access_token?: string;
  account_id?: string;
  id_token?: string;
  refresh_token?: string;
}

export interface TypesCodexSubscription {
  account_id?: string;
  auth_mode?: string;
  created?: string;
  created_by?: string;
  id?: string;
  last_error?: string;
  last_refreshed_at?: string;
  name?: string;
  owner_id?: string;
  owner_type?: TypesOwnerType;
  status?: string;
  updated?: string;
}

export interface TypesCommentQueueStatusResponse {
  /** Comment currently being processed (response streaming) */
  current_comment_id?: string;
  /** Session ID for WebSocket subscription */
  planning_session_id?: string;
  /** Comments waiting in queue */
  queued_comment_ids?: string[];
}

export interface TypesContainerDiskUsage {
  container_id?: string;
  container_name?: string;
  /** Writable layer size */
  rw_size_bytes?: number;
  size_bytes?: number;
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

export interface TypesCreateAccessGrantResponse {
  added_to_organization?: boolean;
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

export interface TypesCreateClaudeSubscriptionRequest {
  credentials?: {
    claudeAiOauth?: TypesClaudeOAuthCredentials;
  };
  name?: string;
  /** Required for org-level, auto-set for user */
  owner_id?: string;
  /** "user" or "org" */
  owner_type?: TypesOwnerType;
  /** From `claude setup-token` (alternative to credentials) */
  setup_token?: string;
}

export interface TypesCreateCodexSubscriptionRequest {
  credentials?: TypesCodexAuthCredentials;
  name?: string;
  owner_id?: string;
  owner_type?: TypesOwnerType;
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

export interface TypesCreateSandboxRequest {
  display_fps?: number;
  display_height?: number;
  display_width?: number;
  env?: Record<string, string>;
  /**
   * Image is an optional explicit Docker image override. Only honoured
   * when the operator has set HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE=true.
   * Mutually exclusive with Runtime.
   */
  image?: string;
  memory_mb?: number;
  name?: string;
  /**
   * Persistent makes the sandbox keep a workspace mount across container
   * restarts. Files written under /home/retro/work survive teardown until
   * the sandbox is explicitly deleted.
   */
  persistent?: boolean;
  /**
   * ProjectID optionally associates the sandbox with a project the caller
   * belongs to. Empty means org-scoped only.
   */
  project_id?: string;
  /**
   * Purpose is an optional marker (e.g. "web-service") that selects extra
   * provisioning behaviour. Empty for ordinary sandboxes. Not settable via
   * the public REST API — set internally by the web-service controller.
   */
  purpose?: string;
  /**
   * Runtime selects one of the operator-configured runtimes
   * (e.g. "headless-ubuntu", "node22", "ubuntu-desktop"). Mutually
   * exclusive with Image.
   */
  runtime?: TypesSandboxRuntime;
  tags?: Record<string, string>;
  timeout_seconds?: number;
  vcpus?: number;
}

export interface TypesCreateSecretRequest {
  app_id?: string;
  name?: string;
  /** optional, if set, the secret will be available to the specified project */
  project_id?: string;
  /** optional, one of "dev", "prod", "both"; defaults to "dev" */
  scope?: string;
  value?: string;
}

export interface TypesCreateTaskRequest {
  /** Optional: Helix agent to use for spec generation */
  app_id?: string;
  /** Optional: team member assigned to the task */
  assignee_id?: string;
  /** Optional: Skip backlog and start immediately, regardless of project auto-start setting */
  auto_start?: boolean;
  /** For new mode: branch to create from (defaults to repo default) */
  base_branch?: string;
  /** Branch configuration */
  branch_mode?: TypesBranchMode;
  /** For new mode: user-specified prefix (task# appended) */
  branch_prefix?: string;
  /** Optional: IDs of tasks this task depends on */
  depends_on?: string[];
  /**
   * Goose recipe selection (only meaningful when the chosen agent's runtime
   * is goose_code). GooseRecipeName must match one of the agent's declared
   * recipes; GooseRecipeParams are substituted into the recipe at session
   * start. Recipes declared on the agent but not selected here are still
   * available as runtime slash-commands inside the desktop.
   */
  goose_recipe_name?: string;
  goose_recipe_params?: Record<string, string>;
  /** Optional: Skip spec planning, go straight to implementation */
  just_do_it_mode?: boolean;
  priority?: TypesSpecTaskPriority;
  project_id?: string;
  prompt?: string;
  type?: string;
  /** Optional: User email for audit trail */
  user_email?: string;
  user_id?: string;
  /** For existing mode: branch to continue working on */
  working_branch?: string;
}

export interface TypesCreateTeamRequest {
  name?: string;
  organization_id?: string;
}

export interface TypesCreatedRepository {
  /** "pending", "cloning", "ready", "error" */
  clone_status?: string;
  github_url?: string;
  id?: string;
  is_forked?: boolean;
  is_primary?: boolean;
  name?: string;
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
  /** "session" (default) or "spec_task" */
  action?: string;
  /** "helix" (default) or "zed_external" */
  agent_type?: string;
  /** Webhook URL to POST on completion */
  callback_url?: string;
  emails?: string[];
  enabled?: boolean;
  input?: string;
  /** File path in helix-specs worktree to use as prompt (overrides Input) */
  input_file?: string;
  /** Target project for spec_task action */
  project_id?: string;
  schedule?: string;
}

export interface TypesDiscordTrigger {
  server_name?: string;
}

export interface TypesDiskUsageHistory {
  alert_level?: string;
  avail_bytes?: number;
  id?: string;
  mount_point?: string;
  recorded?: string;
  sandbox_id?: string;
  total_bytes?: number;
  used_bytes?: number;
}

export interface TypesDiskUsageMetric {
  /** "ok", "warning", "critical" */
  alert_level?: string;
  avail_bytes?: number;
  mount_point?: string;
  total_bytes?: number;
  used_bytes?: number;
  used_percent?: number;
}

export interface TypesDockerCacheState {
  sandboxes?: Record<string, TypesSandboxCacheState>;
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

export interface TypesEvaluationAssertion {
  /** Custom prompt for LLM judge mode */
  llm_judge_prompt?: string;
  type?: TypesEvaluationAssertionType;
  /** Expected string, regex pattern, or skill name */
  value?: string;
}

export interface TypesEvaluationAssertionResult {
  assertion_type?: TypesEvaluationAssertionType;
  assertion_value?: string;
  /** e.g. LLM judge reasoning */
  details?: string;
  passed?: boolean;
}

export enum TypesEvaluationAssertionType {
  EvaluationAssertionTypeContains = "contains",
  EvaluationAssertionTypeNotContains = "not_contains",
  EvaluationAssertionTypeRegex = "regex",
  EvaluationAssertionTypeLLMJudge = "llm_judge",
  EvaluationAssertionTypeSkillUsed = "skill_used",
}

export interface TypesEvaluationQuestion {
  assertions?: TypesEvaluationAssertion[];
  id?: string;
  question?: string;
}

export interface TypesEvaluationQuestionResult {
  assertion_results?: TypesEvaluationAssertionResult[];
  cost?: number;
  duration_ms?: number;
  error?: string;
  interaction_id?: string;
  passed?: boolean;
  question?: string;
  question_id?: string;
  response?: string;
  session_id?: string;
  skills_used?: string[];
  tokens_used?: TypesUsage;
}

export interface TypesEvaluationRun {
  app_config_snapshot?: TypesAppConfig;
  app_id?: string;
  created?: string;
  error?: string;
  id?: string;
  organization_id?: string;
  results?: TypesEvaluationQuestionResult[];
  status?: TypesEvaluationRunStatus;
  suite_id?: string;
  summary?: TypesEvaluationRunSummary;
  updated?: string;
  user_id?: string;
}

export enum TypesEvaluationRunStatus {
  EvaluationRunStatusPending = "pending",
  EvaluationRunStatusRunning = "running",
  EvaluationRunStatusCompleted = "completed",
  EvaluationRunStatusFailed = "failed",
  EvaluationRunStatusCancelled = "cancelled",
}

export interface TypesEvaluationRunSummary {
  failed?: number;
  passed?: number;
  skills_used?: string[];
  total_cost?: number;
  total_duration_ms?: number;
  total_questions?: number;
  total_tokens?: number;
}

export interface TypesEvaluationSuite {
  app_id?: string;
  created?: string;
  description?: string;
  id?: string;
  name?: string;
  organization_id?: string;
  questions?: TypesEvaluationQuestion[];
  updated?: string;
  user_id?: string;
}

export interface TypesExecuteQuestionSetRequest {
  app_id?: string;
  question_set_id?: string;
}

export interface TypesExecuteQuestionSetResponse {
  results?: TypesQuestionResponse[];
}

export interface TypesExternalAgentConfig {
  /** Desktop environment */
  desktop_type?: string;
  /** Explicit height (default: 1080) */
  display_height?: number;
  /** Refresh rate (default: 60) */
  display_refresh_rate?: number;
  /** Explicit width (default: 1920) */
  display_width?: number;
  /** Display resolution - either use Resolution preset or explicit dimensions */
  resolution?: string;
  /** Video capture/encoding mode */
  video_mode?: string;
  /** GNOME zoom percentage (100 default, 200 for 4k/5k) */
  zoom_level?: number;
}

export enum TypesExternalRepositoryType {
  ExternalRepositoryTypeGitHub = "github",
  ExternalRepositoryTypeGitLab = "gitlab",
  ExternalRepositoryTypeADO = "ado",
  ExternalRepositoryTypeBitbucket = "bitbucket",
}

export interface TypesExternalStatus {
  commits_ahead?: number;
  commits_behind?: number;
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

export interface TypesForkRepositoriesRequest {
  fork_to_organization?: string;
  github_connection_id?: string;
  /** List of GitHub URLs to fork */
  repositories_to_fork?: string[];
  sample_project_id?: string;
}

export interface TypesForkRepositoriesResponse {
  forked_repositories?: TypesForkedRepository[];
}

export interface TypesForkSimpleProjectRequest {
  /**
   * ConfiguredSkillEnvVars contains user-configured env vars for skills
   * Outer key: skill name, Inner key: env var name, Value: user-provided value
   * This allows users to configure skills (like API tokens) during project creation
   */
  configured_skill_env_vars?: Record<string, Record<string, string>>;
  description?: string;
  /**
   * For repos the user doesn't have write access to, fork them to this target
   * If empty, forks to user's personal GitHub account
   */
  fork_to_organization?: string;
  /**
   * GitHub OAuth connection ID for authenticated cloning
   * Required for sample projects with RequiresGitHubAuth=true
   */
  github_connection_id?: string;
  /** Optional: agent app to use for spec tasks (uses default if empty) */
  helix_app_id?: string;
  /** Optional: if empty, project is personal */
  organization_id?: string;
  project_name?: string;
  /**
   * RepositoryDecisions maps repo URLs to the user's decision about access
   * Key: GitHub URL (e.g., "github.com/helixml/helix")
   * Value: "use_original" (has write access) or "fork" (will fork)
   */
  repository_decisions?: Record<string, string>;
  sample_project_id?: string;
}

export interface TypesForkSimpleProjectResponse {
  /** CloningInProgress indicates repos are still being cloned */
  cloning_in_progress?: boolean;
  github_repo_url?: string;
  message?: string;
  project_id?: string;
  /** RepositoriesCreated lists the repositories attached to the project */
  repositories_created?: TypesCreatedRepository[];
  tasks_created?: number;
}

export interface TypesForkedRepository {
  forked_url?: string;
  original_url?: string;
  owner?: string;
  repo?: string;
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

export interface TypesGPUStatus {
  /** canonical arch from gpuarch (e.g. "hopper") */
  architecture?: string;
  /** NVIDIA only — raw "9.0" / "8.6" etc. */
  compute_capability?: string;
  /** GPU driver version (NVIDIA or AMD) */
  driver_version?: string;
  /** Free memory in bytes */
  free_memory?: number;
  /** GPU index (0, 1, 2, etc.) */
  index?: number;
  /** GPU model name (e.g., "NVIDIA H100 PCIe", "AMD Radeon RX 7900 XTX") */
  model_name?: string;
  /** GPU SDK version (CUDA for NVIDIA, ROCm for AMD) */
  sdk_version?: string;
  /** Total memory in bytes */
  total_memory?: number;
  /** Used memory in bytes */
  used_memory?: number;
  /**
   * Sandbox-absorbs-runner pivot fields (AC2 in requirements.md).
   * Populated by the worker on its periodic status report; consumed by
   * the API server's profile-compatibility check.
   */
  vendor?: TypesGPUVendor;
}

export enum TypesGPUVendor {
  GPUVendorNVIDIA = "nvidia",
  GPUVendorAMD = "amd",
  GPUVendorNeuron = "neuron",
}

export interface TypesGitHub {
  /**
   * GitHub App authentication (service-to-service)
   * When AppID and PrivateKey are set, uses GitHub App installation tokens
   */
  app_id?: number;
  /** For GitHub Enterprise instances (empty for github.com) */
  base_url?: string;
  /** Installation ID for the app on the org/repo */
  installation_id?: number;
  personal_access_token?: string;
  /** PEM-encoded private key for JWT signing */
  private_key?: string;
}

export interface TypesGitLab {
  /** For self-hosted GitLab instances (empty for gitlab.com) */
  base_url?: string;
  personal_access_token?: string;
}

export interface TypesGitProviderConnection {
  avatar_url?: string;
  /** For GitHub Enterprise or GitLab Enterprise: base URL (empty = github.com/gitlab.com) */
  base_url?: string;
  created_at?: string;
  deleted_at?: GormDeletedAt;
  email?: string;
  id?: string;
  /** Last successful connection test */
  last_tested_at?: string;
  /** Display name for the connection (e.g., "My GitHub Account") */
  name?: string;
  /** For Azure DevOps: organization URL */
  organization_url?: string;
  /** Provider type: github, gitlab, ado */
  provider_type?: TypesExternalRepositoryType;
  updated_at?: string;
  /** User who owns this connection (PAT is personal, not org-level) */
  user_id?: string;
  /** User info from the provider (cached from last successful auth) */
  username?: string;
}

export interface TypesGitProviderConnectionCreateRequest {
  /** Username for authentication (required for Bitbucket) */
  auth_username?: string;
  base_url?: string;
  name?: string;
  organization_url?: string;
  provider_type?: TypesExternalRepositoryType;
  token?: string;
}

export interface TypesGitRepository {
  /** Provider-specific settings */
  azure_devops?: TypesAzureDevOps;
  bitbucket?: TypesBitbucket;
  branches?: string[];
  /** Clone progress tracking for async cloning */
  clone_error?: string;
  /** Live progress during cloning */
  clone_progress?: TypesCloneProgress;
  /** For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git */
  clone_url?: string;
  created_at?: string;
  default_branch?: string;
  description?: string;
  /** "github", "gitlab", "ado", "bitbucket", etc. */
  external_type?: TypesExternalRepositoryType;
  /** Full URL to external repo (e.g., https://github.com/org/repo) */
  external_url?: string;
  /**
   * GitProviderConnectionID - references a GitProviderConnection (saved PAT) for authentication
   * When set, the encrypted token is decrypted and used for clone/push operations
   */
  git_provider_connection_id?: string;
  github?: TypesGitHub;
  gitlab?: TypesGitLab;
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
  /**
   * OAuth connection ID - references an OAuthConnection for authentication
   * When set, uses the OAuth access token instead of username/password or PAT
   */
  oauth_connection_id?: string;
  /** Organization ID - will be backfilled for existing repos */
  organization_id?: string;
  owner_id?: string;
  /** Password for the repository */
  password?: string;
  repo_type?: TypesGitRepositoryType;
  status?: TypesGitRepositoryStatus;
  updated_at?: string;
  /** Authentication fields */
  username?: string;
}

export interface TypesGitRepositoryCreateRequest {
  /** Provider-specific settings */
  azure_devops?: TypesAzureDevOps;
  bitbucket?: TypesBitbucket;
  default_branch?: string;
  description?: string;
  /** "github", "gitlab", "ado", "bitbucket", etc. */
  external_type?: TypesExternalRepositoryType;
  /** Full URL to external repo (e.g., https://github.com/org/repo) */
  external_url?: string;
  /** GitProviderConnectionID - references a saved PAT connection for authentication */
  git_provider_connection_id?: string;
  github?: TypesGitHub;
  gitlab?: TypesGitLab;
  initial_files?: Record<string, string>;
  /** True for GitHub/GitLab/ADO, false for Helix-hosted */
  is_external?: boolean;
  /** Enable Kodit code intelligence indexing */
  kodit_indexing?: boolean;
  metadata?: Record<string, any>;
  name?: string;
  /** OAuth connection ID - references an OAuthConnection for authentication */
  oauth_connection_id?: string;
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

export enum TypesGitRepositoryStatus {
  GitRepositoryStatusActive = "active",
  GitRepositoryStatusCloning = "cloning",
  GitRepositoryStatusError = "error",
  GitRepositoryStatusArchived = "archived",
  GitRepositoryStatusDeleted = "deleted",
}

export interface TypesGitRepositoryTreeResponse {
  entries?: TypesTreeEntry[];
  path?: string;
}

export enum TypesGitRepositoryType {
  GitRepositoryTypeInternal = "internal",
  GitRepositoryTypeCode = "code",
}

export interface TypesGitRepositoryUpdateRequest {
  azure_devops?: TypesAzureDevOps;
  bitbucket?: TypesBitbucket;
  default_branch?: string;
  description?: string;
  /** "github", "gitlab", "ado", "bitbucket", etc. */
  external_type?: TypesExternalRepositoryType;
  external_url?: string;
  github?: TypesGitHub;
  gitlab?: TypesGitLab;
  /** Enable Kodit code intelligence indexing (pointer to distinguish unset from false) */
  kodit_indexing?: boolean;
  metadata?: Record<string, any>;
  name?: string;
  /** OAuth connection for authentication */
  oauth_connection_id?: string;
  password?: string;
  username?: string;
}

export interface TypesGuidelinesHistory {
  /** Optional description of what changed */
  change_note?: string;
  guidelines?: string;
  id?: string;
  /** Set for org-level guidelines */
  organization_id?: string;
  /** Set for project-level guidelines */
  project_id?: string;
  updated_at?: string;
  /** User ID */
  updated_by?: string;
  /** User email (not persisted, populated at query time) */
  updated_by_email?: string;
  /** User display name (not persisted, populated at query time) */
  updated_by_name?: string;
  /** Set for user-level (personal workspace) guidelines */
  user_id?: string;
  version?: number;
}

export enum TypesImageURLDetail {
  ImageURLDetailHigh = "high",
  ImageURLDetailLow = "low",
  ImageURLDetailAuto = "auto",
}

export interface TypesInteraction {
  app_id?: string;
  /**
   * AutoWakeCount tracks how many times the auto-wake worker has sent a
   * follow-up "continue" prompt to unstick this interaction. Zero means
   * this is a normal user-initiated interaction; non-zero on an
   * auto-wake interaction itself records which retry attempt it is.
   * See design/2026-04-25-zed-claude-async-event-flush-on-user-input.md.
   */
  auto_wake_count?: number;
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
  /**
   * LastZedMessageID tracks the last Zed message ID received for this interaction.
   * Used to detect multi-message responses: same ID = streaming update (overwrite),
   * different ID = new distinct message (append). Persisted in DB for restart resilience.
   */
  last_zed_message_id?: string;
  /**
   * LastZedMessageOffset is the byte offset in ResponseMessage where the current
   * message_id's content begins. Used by the accumulator to replace only the
   * current message's portion during streaming updates, preserving earlier messages.
   */
  last_zed_message_offset?: number;
  mode?: TypesSessionMode;
  /**
   * PromptID links this interaction back to the prompt_history_entry that
   * created it (when the interaction was dispatched by the queue, as opposed
   * to being initiated by Zed when the user types in the IDE). Empty for
   * Zed-initiated interactions. Used by handleMessageAdded /
   * handleMessageCompleted to mark the originating prompt as 'sent' without
   * relying on an in-memory map that doesn't survive API restarts. See
   * design/2026-04-30-queue-and-other-stuck-state-bugs.md.
   */
  prompt_id?: string;
  /** User prompt (text) */
  prompt_message?: string;
  /** User prompt (multi-part) */
  prompt_message_content?: TypesMessageContent;
  rag_results?: TypesSessionRAGResult[];
  /**
   * ResponseEntries holds the structured response as an ordered list of typed entries.
   * Each entry is either "text" (assistant prose) or "tool_call" (tool invocation),
   * preserving the ordering and boundaries that Zed's internal Vec<AgentThreadEntry> has.
   * This is populated on completion alongside ResponseMessage (flat string, backward compat).
   * The frontend uses this to render entries with the correct component in the correct order.
   */
  response_entries?: number[];
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
  /**
   * Summary is a one-line description of this interaction for search/indexing.
   * Generated lazily on first access or via background job.
   */
  summary?: string;
  summary_updated_at?: string;
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
  InteractionStateInterrupted = "interrupted",
}

export interface TypesItem {
  b64_json?: string;
  embedding?: number[];
  index?: number;
  object?: string;
  /** Images */
  url?: string;
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
  /** Organization scope for search */
  organization_id?: string;
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
  sharepoint?: TypesKnowledgeSourceSharePoint;
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

export interface TypesKnowledgeSourceSharePoint {
  /** DriveID is the document library drive ID (optional, defaults to the site's default drive) */
  drive_id?: string;
  /** FilterExtensions limits which file types to include (e.g., [".pdf", ".docx", ".txt"]) */
  filter_extensions?: string[];
  /** FolderPath is the path to a specific folder within the drive (optional, defaults to root) */
  folder_path?: string;
  /** OAuthProviderID is the ID of the Microsoft OAuth provider to use for authentication */
  oauth_provider_id?: string;
  /** Recursive determines whether to include files in subfolders */
  recursive?: boolean;
  /** SiteID is the SharePoint site ID (can be obtained from Graph API or site URL) */
  site_id?: string;
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
  cache_read_cost?: number;
  /** prompt tokens served from provider cache (subset of PromptTokens) */
  cache_read_tokens?: number;
  cache_write_cost?: number;
  /** prompt tokens written to provider cache (Anthropic only; subset of PromptTokens) */
  cache_write_tokens?: number;
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
  project_id?: string;
  prompt_cost?: number;
  prompt_tokens?: number;
  provider?: string;
  request?: number[];
  response?: number[];
  session_id?: string;
  spec_task_id?: string;
  step?: TypesLLMCallStep;
  stream?: boolean;
  /**
   * TimeToFirstTokenMs is the wall time from request start to the first
   * streamed chunk. It isolates provider prefill / cold-start latency from
   * generation time (a cold or overloaded provider shows a large TTFT while
   * generation stays normal). 0 means no chunk was received (the call errored
   * or was cut before the first token). For non-streaming calls it equals the
   * time to the full response.
   */
  time_to_first_token_ms?: number;
  /** Prompt + completion + cache read + cache write */
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

export interface TypesListCommitsResponse {
  commits?: GithubComHelixmlHelixApiPkgTypesCommit[];
  external_status?: TypesExternalStatus;
  page?: number;
  per_page?: number;
  /** Pagination info */
  total?: number;
}

export interface TypesListOAuthRepositoriesResponse {
  repositories?: TypesRepositoryInfo[];
}

export interface TypesLoginRequest {
  email?: string;
  password?: string;
  redirect_uri?: string;
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
  runtime?: GithubComHelixmlHelixApiPkgTypesRuntime;
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
  permaslug?: string;
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

export interface TypesMoveProjectPreviewItem {
  current_name?: string;
  has_conflict?: boolean;
  /** nil if no conflict */
  new_name?: string;
}

export interface TypesMoveProjectPreviewResponse {
  project?: TypesMoveProjectPreviewItem;
  repositories?: TypesMoveRepositoryPreviewItem[];
  /** Warnings about things that won't be moved automatically */
  warnings?: string[];
}

export interface TypesMoveProjectRequest {
  organization_id?: string;
}

export interface TypesMoveRepositoryPreviewItem {
  /** Other projects that will lose this repo */
  affected_projects?: TypesAffectedProjectInfo[];
  current_name?: string;
  has_conflict?: boolean;
  id?: string;
  /** nil if no conflict */
  new_name?: string;
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
  /** Misc configuration */
  enabled?: boolean;
  id?: string;
  name?: string;
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
  OAuthProviderTypeGitLab = "gitlab",
  OAuthProviderTypeAzureDevOps = "azure_devops",
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
  /** Provider-specific username (e.g., GitHub login) */
  username?: string;
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

export interface TypesOrgDetails {
  members?: TypesUser[];
  organization?: TypesOrganization;
  projects?: TypesProject[];
  wallet?: TypesWallet;
}

export interface TypesOrgUsageSummaryResponse {
  active_apps?: number;
  active_projects?: number;
  active_sessions?: number;
  active_users?: number;
  apps?: TypesUsageBreakdownRow[];
  export_apps?: TypesUsageBreakdownRow[];
  export_models?: TypesUsageBreakdownRow[];
  export_projects?: TypesUsageBreakdownRow[];
  export_sessions?: TypesUsageBreakdownRow[];
  export_tasks?: TypesUsageBreakdownRow[];
  export_users?: TypesUsageBreakdownRow[];
  filter_apps?: TypesUsageFilterOption[];
  filter_models?: TypesUsageFilterOption[];
  filter_projects?: TypesUsageFilterOption[];
  filter_users?: TypesUsageFilterOption[];
  metrics?: TypesAggregatedUsageMetric[];
  model_time_series?: TypesUsageModelTimeSeries[];
  models?: TypesUsageBreakdownRow[];
  project_models?: TypesUsageBreakdownRow[];
  projects?: TypesUsageBreakdownRow[];
  projects_total?: number;
  sessions?: TypesUsageBreakdownRow[];
  sessions_total?: number;
  tasks?: TypesUsageBreakdownRow[];
  tasks_total?: number;
  users?: TypesUsageBreakdownRow[];
  users_total?: number;
}

export interface TypesOrgUserLookupResponse {
  email?: string;
  /** a Helix user account exists with this email */
  exists?: boolean;
  full_name?: string;
  invitation_id?: string;
  /**
   * IsInvited — a pending invitation exists for this email in the queried
   * org. Used by the invite UI to disable the "Send invitation" button so
   * admins don't accidentally double-invite.
   */
  is_invited?: boolean;
  /** user is also a member of the queried org */
  is_member?: boolean;
  user_id?: string;
}

export interface TypesOrganization {
  /**
   * AutoJoinDomain - if set, users logging in via OIDC with this email domain are automatically added as members
   * Note: Uniqueness is enforced in application code (updateOrganization handler) rather than DB constraint
   * because empty strings would conflict with each other in a unique index
   */
  auto_join_domain?: string;
  created_at?: string;
  deleted_at?: GormDeletedAt;
  display_name?: string;
  /** Guidelines for AI agents - style guides, conventions, and instructions that apply to all projects */
  guidelines?: string;
  /** When guidelines were last updated */
  guidelines_updated_at?: string;
  /** User ID who last updated guidelines */
  guidelines_updated_by?: string;
  /** Incremented on each update */
  guidelines_version?: number;
  id?: string;
  /** Whether the current user is a member of the organization */
  member?: boolean;
  /** Memberships in the organization */
  memberships?: TypesOrganizationMembership[];
  name?: string;
  /** Who created the org */
  owner?: string;
  /** Number of projects in the organization */
  project_count?: number;
  /** Roles in the organization */
  roles?: TypesRole[];
  /** Teams in the organization */
  teams?: TypesTeam[];
  updated_at?: string;
}

export interface TypesOrganizationInvitation {
  /**
   * AppID + GrantRoles record the optional access-grant context. When an
   * invitation is sent from a project/app's access management dialog,
   * we store the resource id and the role names the inviter chose, so
   * that consuming the invitation at register time can also materialise
   * the access grant — the invitee then shows up in the project access
   * list immediately, exactly as if they had been added directly.
   */
  app_id?: string;
  created_at?: string;
  /** Normalised to lowercase */
  email?: string;
  grant_roles?: string[];
  id?: string;
  /** User ID of the inviter */
  invited_by?: string;
  organization_id?: string;
  role?: TypesOrganizationRole;
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
  /** price per cached input token read (hit) */
  input_cache_read?: string;
  /** price per cached input token written (cache creation) */
  input_cache_write?: string;
  internal_reasoning?: string;
  prompt?: string;
  request?: string;
  web_search?: string;
}

export interface TypesProfileGPURequirement {
  /** optional whitelist (canonical strings from gpuarch) */
  architectures?: string[];
  /** derived from compose: union of device_ids */
  count?: number;
  /** optional, per-GPU minimum */
  min_vram_bytes?: number;
  /** optional regex against GPU marketing name */
  model_match?: string;
  /** optional */
  vendor?: TypesGPUVendor;
}

export interface TypesProfileModel {
  /** service.container_name (or service key if absent) */
  container_name?: string;
  /** first published or exposed port */
  internal_port?: number;
  /** --served-model-name (preferred) or --model basename */
  name?: string;
}

export interface TypesProject {
  /** Automation settings */
  auto_start_backlog_tasks?: boolean;
  created_at?: string;
  default_branch?: string;
  /**
   * Default agent for spec tasks in this project (App ID)
   * New spec tasks inherit this agent; can be overridden per-task
   */
  default_helix_app_id?: string;
  /**
   * Project-level repository management
   * DefaultRepoID is the PRIMARY repository - startup script lives at .helix/startup.sh in this repo
   */
  default_repo_id?: string;
  /** Soft delete timestamp */
  deleted_at?: GormDeletedAt;
  description?: string;
  github_repo_url?: string;
  /**
   * Guidelines for AI agents - project-specific style guides, conventions, and instructions
   * Combined with organization guidelines when constructing prompts
   */
  guidelines?: string;
  /** When guidelines were last updated */
  guidelines_updated_at?: string;
  /** User ID who last updated guidelines */
  guidelines_updated_by?: string;
  /** Incremented on each update */
  guidelines_version?: number;
  id?: string;
  kodit_enabled?: boolean;
  metadata?: TypesProjectMetadata;
  /** Indexed for search prefix matching */
  name?: string;
  /**
   * Auto-incrementing task number for human-readable directory names
   * Each SpecTask gets assigned the next number (install-cowsay_1, add-api_2, etc.)
   */
  next_task_number?: number;
  organization_id?: string;
  project_manager_helix_app_id?: string;
  pull_request_reviewer_helix_app_id?: string;
  pull_request_reviews_enabled?: boolean;
  /**
   * Project-level skills - these overlay on top of agent skills
   * Useful for project-specific tools like CI integration (e.g., drone-ci-mcp)
   */
  skills?: TypesAssistantSkills;
  /**
   * Startup commands from declarative project YAML (persisted) - DEPRECATED
   * Use StartupScriptYAML instead. Kept for backward compatibility.
   */
  startup_install?: string;
  /** Transient field - loaded from primary code repo's .helix/startup.sh, never persisted to database */
  startup_script?: string;
  /**
   * StartupScriptFromYAML indicates the startup script was set via project YAML
   * When true, the UI should show the script as read-only
   */
  startup_script_from_yaml?: boolean;
  /**
   * StartupScriptYAML is the startup script content from project YAML (persisted)
   * This is the source of truth when StartupScriptFromYAML is true.
   * At runtime, helix-specs/.helix/startup.sh takes precedence if it exists,
   * otherwise this field is used as fallback.
   */
  startup_script_yaml?: string;
  startup_start?: string;
  /** Computed */
  stats?: TypesProjectStats;
  /** "active", "archived", "completed" */
  status?: string;
  technologies?: string[];
  updated_at?: string;
  /** Populated by the server if UserID is set */
  user?: TypesUser;
  user_id?: string;
}

export interface TypesProjectAgentDisplay {
  /** Desktop environment: "ubuntu" (default GNOME) or "sway" */
  desktop_type?: string;
  /** Display refresh rate in Hz (default 60) */
  fps?: number;
  /** Resolution preset: "1080p" (default), "4k", or "5k" */
  resolution?: string;
}

export interface TypesProjectAgentGoose {
  recipe_repo_url?: string;
  recipes?: TypesProjectAgentGooseRecipe[];
}

export interface TypesProjectAgentGooseRecipe {
  name?: string;
  path?: string;
}

export interface TypesProjectAgentSpec {
  credentials?: string;
  display?: TypesProjectAgentDisplay;
  goose?: TypesProjectAgentGoose;
  model?: string;
  name?: string;
  provider?: string;
  runtime?: string;
  tools?: TypesProjectAgentTools;
}

export interface TypesProjectAgentTools {
  browser?: boolean;
  calculator?: boolean;
  web_search?: boolean;
}

export interface TypesProjectApplyRequest {
  name?: string;
  organization_id?: string;
  spec?: TypesProjectSpec;
}

export interface TypesProjectApplyResponse {
  agent_app_id?: string;
  /** true if created, false if updated */
  created?: boolean;
  project_id?: string;
}

export interface TypesProjectAuditLog {
  created_at?: string;
  event_type?: TypesAuditEventType;
  id?: string;
  metadata?: TypesAuditMetadata;
  project_id?: string;
  prompt_text?: string;
  spec_task_id?: string;
  user_email?: string;
  user_id?: string;
}

export interface TypesProjectAuditLogResponse {
  limit?: number;
  logs?: TypesProjectAuditLog[];
  offset?: number;
  total?: number;
}

export interface TypesProjectCreateRequest {
  default_branch?: string;
  /** Default agent for spec tasks */
  default_helix_app_id?: string;
  default_repo_id?: string;
  description?: string;
  github_repo_url?: string;
  /** Project-specific AI agent guidelines */
  guidelines?: string;
  name?: string;
  organization_id?: string;
  /** Project-level skills */
  skills?: TypesAssistantSkills;
  startup_script?: string;
  technologies?: string[];
}

export interface TypesProjectKanban {
  wip_limits?: TypesProjectWIPLimits;
}

export interface TypesProjectMetadata {
  auto_warm_docker_cache?: boolean;
  board_settings?: TypesBoardSettings;
  docker_cache_status?: TypesDockerCacheState;
}

export interface TypesProjectRepositorySpec {
  default_branch?: string;
  primary?: boolean;
  url?: string;
}

export interface TypesProjectSpec {
  agent?: TypesProjectAgentSpec;
  auto_start_backlog_tasks?: boolean;
  description?: string;
  guidelines?: string;
  kanban?: TypesProjectKanban;
  name?: string;
  /** Multi-repo list */
  repositories?: TypesProjectRepositorySpec[];
  /** Singular shorthand */
  repository?: TypesProjectRepositorySpec;
  startup?: TypesProjectStartup;
  tasks?: TypesProjectTaskSpec[];
  technologies?: string[];
}

export interface TypesProjectStartup {
  /**
   * Install and Start are deprecated - use Script instead
   * Kept for backward compatibility with existing YAML files
   */
  install?: string;
  /** Script is the unified startup script content (preferred) */
  script?: string;
  start?: string;
}

export interface TypesProjectStats {
  active_agent_sessions?: number;
  average_task_completion_hours?: number;
  backlog_tasks?: number;
  completed_tasks?: number;
  in_progress_tasks?: number;
  pending_review_tasks?: number;
  planning_tasks?: number;
  total_tasks?: number;
}

export interface TypesProjectTaskSpec {
  description?: string;
  title?: string;
}

export interface TypesProjectUpdateRequest {
  auto_start_backlog_tasks?: boolean;
  default_branch?: string;
  /** Default agent for spec tasks */
  default_helix_app_id?: string;
  default_repo_id?: string;
  description?: string;
  github_repo_url?: string;
  /** Project-specific AI agent guidelines */
  guidelines?: string;
  /** Whether Kodit code intelligence is enabled */
  kodit_enabled?: boolean;
  metadata?: TypesProjectMetadata;
  name?: string;
  /** Project manager agent */
  project_manager_helix_app_id?: string;
  /** Pull request reviewer agent */
  pull_request_reviewer_helix_app_id?: string;
  /** Whether pull request reviews are enabled */
  pull_request_reviews_enabled?: boolean;
  /** Project-level skills */
  skills?: TypesAssistantSkills;
  startup_script?: string;
  status?: string;
  technologies?: string[];
}

export interface TypesProjectWIPLimits {
  implementation?: number;
  planning?: number;
  review?: number;
}

export interface TypesProjectWebServiceState {
  active_sandbox_id?: string;
  /** port the project's web app binds to inside its container */
  container_port?: number;
  created_at?: string;
  enabled?: boolean;
  /**
   * HostDeviceID is the runner the project's web service is pinned to. It is
   * recorded from the web-service sandbox after first provision and surfaced
   * for visibility. Enforcement of the pin lives in the sandbox scheduler's
   * persistent-sandbox sticky guard; this column mirrors it for the UI/API.
   */
  host_device_id?: string;
  project_id?: string;
  updated_at?: string;
}

export interface TypesPromptHistoryEntry {
  /** Content */
  content?: string;
  /** Timestamps */
  created_at?: string;
  /** Soft-delete: non-nil means user removed from queue */
  deleted_at?: string;
  /** Last failure reason (server-side error string), shown in UI under "Failed - retrying" */
  error_message?: string;
  /** Composite primary key: ID is globally unique, but we also index by user+spec_task */
  id?: string;
  /**
   * Interrupt indicates this message should interrupt the current conversation
   * When false, message waits until current conversation completes
   * Default is false: queue mode is the default, interrupt is explicit
   */
  interrupt?: boolean;
  /** Saved as a reusable template */
  is_template?: boolean;
  /** Last time reused */
  last_used_at?: string;
  /** When to retry (for exponential backoff) */
  next_retry_at?: string;
  /**
   * NotifyUserID, when set, is the user who should be streamed the agent's
   * response (e.g. a design-review commenter). At dispatch the queue registers
   * requestToCommenterMapping/sessionToCommenterMapping from this field — the
   * same routing the old direct send set up synchronously.
   */
  notify_user_id?: string;
  /** Organization scope for search */
  organization_id?: string;
  /** Library features for prompt reuse */
  pinned?: boolean;
  /** For reference, but primary grouping is by spec_task */
  project_id?: string;
  /**
   * QueuePosition tracks ordering for drag-and-drop reordering
   * Lower values = earlier in queue. Null for sent messages.
   */
  queue_position?: number;
  /** Retry tracking for failed prompts */
  retry_count?: number;
  /** Which session this was sent to (the delivery unit) */
  session_id?: string;
  /**
   * SpecTaskID is nullable: frontend queue-mode messages always carry it, but
   * automated/system and general session sends (e.g. org bots via
   * POST /sessions/{id}/messages) enqueue by SessionID with no spec task.
   */
  spec_task_id?: string;
  /**
   * Status tracks whether this was successfully sent
   * Values: "pending", "sent", "failed"
   */
  status?: string;
  /** JSON array of user-defined tags */
  tags?: string;
  updated_at?: string;
  /** How many times reused */
  usage_count?: number;
  user_id?: string;
}

export interface TypesPromptHistoryEntrySync {
  content?: string;
  id?: string;
  /** If true, interrupts current conversation */
  interrupt?: boolean;
  /** If true, saved as a reusable template */
  is_template?: boolean;
  /** If true, pinned by user */
  pinned?: boolean;
  /** Position in queue for drag-and-drop ordering */
  queue_position?: number;
  session_id?: string;
  status?: string;
  /** JSON array of tags */
  tags?: string;
  /** Unix timestamp in milliseconds */
  timestamp?: number;
}

export interface TypesPromptHistoryListResponse {
  entries?: TypesPromptHistoryEntry[];
  total?: number;
}

export interface TypesPromptHistorySyncRequest {
  entries?: TypesPromptHistoryEntrySync[];
  project_id?: string;
  /**
   * SessionID is used for session-scoped queues (e.g. org-chat / bot sessions
   * that have no spec task). Exactly one of SpecTaskID / SessionID is set.
   */
  session_id?: string;
  spec_task_id?: string;
}

export interface TypesPromptHistorySyncResponse {
  /** All entries for this user+project (for client merge) */
  entries?: TypesPromptHistoryEntry[];
  /** Number that already existed */
  existing?: number;
  /** Number of entries synced */
  synced?: number;
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
  vertex_credentials_file?: string;
  /** Service account JSON string; takes precedence over file */
  vertex_credentials_json?: string;
  /** Google Vertex AI fields — when VertexProjectID is set, this endpoint routes through Vertex */
  vertex_project_id?: string;
  vertex_region?: string;
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

export interface TypesPublicInvitationInfo {
  email?: string;
  id?: string;
  organization_display_name?: string;
  organization_name?: string;
}

export interface TypesPullRequest {
  author?: string;
  created_at?: string;
  description?: string;
  /**
   * HeadSHA is the commit SHA at the tip of the PR's source branch.
   * Used by the CI status poller to detect new pushes and to query the
   * provider's CI/build APIs for the right commit. Empty if the
   * provider response did not include it.
   */
  head_sha?: string;
  id?: string;
  number?: number;
  source_branch?: string;
  state?: TypesPullRequestState;
  target_branch?: string;
  title?: string;
  updated_at?: string;
  url?: string;
}

export enum TypesPullRequestState {
  PullRequestStateOpen = "open",
  PullRequestStateClosed = "closed",
  PullRequestStateMerged = "merged",
  PullRequestStateUnknown = "unknown",
}

export interface TypesPullResponse {
  branch?: string;
  message?: string;
  repository_id?: string;
  success?: boolean;
}

export interface TypesPushError {
  /** VCS account the push was attempted as, e.g. "@linuxrecruit" */
  account?: string;
  /** translated human-readable cause */
  cause?: string;
  failed_at?: string;
  /** translated actionable next step */
  next_step?: string;
  /** e.g. "github" */
  provider?: TypesExternalRepositoryType;
  /** verbatim provider error */
  raw_message?: string;
  /** e.g. "helixml/find-ai" */
  repo?: string;
}

export interface TypesPushResponse {
  branch?: string;
  message?: string;
  repository_id?: string;
  success?: boolean;
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

export enum TypesQuestionSetExecutionStatus {
  QuestionSetExecutionStatusPending = "pending",
  QuestionSetExecutionStatusRunning = "running",
  QuestionSetExecutionStatusSuccess = "success",
  QuestionSetExecutionStatusError = "error",
}

export interface TypesQuotaResponse {
  active_concurrent_desktops?: number;
  /**
   * Sandbox API concurrency. Distinct from ActiveConcurrentDesktops above —
   * that one counts external_agent sessions (the spec-task desktop stack),
   * while these count rows in the sandboxes table by runtime category.
   * "Active" = pending|running|stopping (matches ensureSandboxLimits).
   */
  active_desktop_sandboxes?: number;
  active_headless_sandboxes?: number;
  /**
   * MaxConcurrentDesktops: cap on concurrent desktop sessions. Enforced per
   * organisation when the session has an org, per user otherwise.
   * -1 = unlimited.
   */
  max_concurrent_desktops?: number;
  max_desktop_sandboxes?: number;
  max_headless_sandboxes?: number;
  max_projects?: number;
  max_repositories?: number;
  max_spec_tasks?: number;
  /** If applicable */
  organization_id?: string;
  projects?: number;
  repositories?: number;
  spec_tasks?: number;
  user_id?: string;
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
}

export interface TypesRegisterRequest {
  email?: string;
  full_name?: string;
  password?: string;
  password_confirm?: string;
}

export interface TypesRepoPR {
  ci_head_sha?: string;
  /**
   * CI status, populated by the spec task orchestrator's PR poll loop.
   * CIStatus is one of: "" (not yet evaluated), "running", "passed",
   * "failed", "none" (CI not configured for the PR's head SHA).
   * CIHeadSHA is the head commit we last evaluated; it lets the poller
   * detect a new push and reset CIStatus so a stale "passed" doesn't
   * suppress a fresh notification when the next commit fails.
   */
  ci_status?: string;
  ci_updated_at?: string;
  ci_url?: string;
  pr_id?: string;
  pr_number?: number;
  /** "open", "closed", "merged" */
  pr_state?: string;
  pr_url?: string;
  repository_id?: string;
  repository_name?: string;
}

export interface TypesRepositoryAccessCheck {
  can_fork?: boolean;
  default_branch?: string;
  /** URL of existing fork if any */
  existing_fork?: string;
  github_url?: string;
  has_write_access?: boolean;
  is_primary?: boolean;
  owner?: string;
  repo?: string;
}

export interface TypesRepositoryInfo {
  /**
   * CanWrite is true when the authenticated user has push or admin access to the repo.
   * Helix needs write access to push branches and open pull requests, so read-only
   * repos can be listed but cannot be linked as a project repo.
   */
  can_write?: boolean;
  /** HTTPS clone URL */
  clone_url?: string;
  default_branch?: string;
  description?: string;
  /** e.g., "owner/repo" for GitHub or "group/project" for GitLab */
  full_name?: string;
  /** Web URL to view the repository */
  html_url?: string;
  name?: string;
  private?: boolean;
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
  ResourceSpecTask = "SpecTask",
  ResourceSession = "Session",
  ResourcePrompt = "Prompt",
  ResourceDesktop = "Desktop",
}

export interface TypesResourceSearchRequest {
  limit?: number;
  organization_id?: string;
  query?: string;
  types?: TypesResource[];
  user_id?: string;
}

export interface TypesResourceSearchResponse {
  results?: TypesResourceSearchResult[];
  total?: number;
}

export interface TypesResourceSearchResult {
  contents?: string;
  description?: string;
  id?: string;
  name?: string;
  parent_id?: string;
  type?: TypesResource;
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

export interface TypesRunSandboxCommandRequest {
  args?: string[];
  cmd?: string;
  cwd?: string;
  detached?: boolean;
  env?: Record<string, string>;
  sudo?: boolean;
  /** TimeoutSeconds is per-command timeout. Defaults to 60s if 0 and !detached. */
  timeout_seconds?: number;
}

export interface TypesRunnerAssignment {
  assigned_at?: string;
  /** user ID for audit */
  assigned_by?: string;
  profile_id?: string;
  runner_id?: string;
}

export interface TypesRunnerProfile {
  compose_yaml?: string;
  created_at?: string;
  description?: string;
  gpu_requirement?: TypesProfileGPURequirement;
  id?: string;
  models?: TypesProfileModel[];
  name?: string;
  updated_at?: string;
}

export interface TypesSandbox {
  billing_last_charged_at?: string;
  container_id?: string;
  created_at?: string;
  deleted_at?: string;
  display_fps?: number;
  display_height?: number;
  /** Display fields apply to desktop runtimes. */
  display_width?: number;
  env?: number[];
  expires_at?: string;
  /**
   * HostDeviceID is the RevDial device ID of the hydra host that runs the
   * underlying container. Empty until the controller schedules it.
   */
  host_device_id?: string;
  id?: string;
  image?: string;
  memory_mb?: number;
  name?: string;
  organization_id?: string;
  owner?: string;
  /**
   * Persistent indicates that the sandbox should mount a persistent
   * workspace volume (so files survive across reboots/restarts of the
   * underlying container). Non-persistent sandboxes use the container's
   * ephemeral filesystem only.
   */
  persistent?: boolean;
  /**
   * ProjectID is optional. When set, the sandbox is associated with a
   * specific project for organisational/UI grouping purposes; nothing in the
   * lifecycle path branches on it. Empty means org-scoped only.
   */
  project_id?: string;
  /**
   * Purpose is an optional marker describing what the sandbox is used for.
   * Empty for ordinary agent/dev sandboxes. "web-service" marks the single
   * long-lived sandbox that hosts a project's web service; the provisioner
   * uses it to bind-mount the per-project durable data dir at /data.
   */
  purpose?: string;
  runtime?: TypesSandboxRuntime;
  started_at?: string;
  status?: TypesSandboxStatus;
  status_message?: string;
  stopped_at?: string;
  tags?: number[];
  /** TimeoutSeconds is the lifetime in seconds; ExpiresAt = CreatedAt + TimeoutSeconds. */
  timeout_seconds?: number;
  updated_at?: string;
  vcpus?: number;
}

export interface TypesSandboxCacheState {
  build_session_id?: string;
  error?: string;
  last_build_at?: string;
  last_ready_at?: string;
  size_bytes?: number;
  status?: string;
}

export interface TypesSandboxFileUploadResponse {
  /** Original filename */
  filename?: string;
  /** Full path: /home/retro/work/incoming/filename */
  path?: string;
  /** File size in bytes */
  size?: number;
}

export interface TypesSandboxHeartbeatRequest {
  /** Container disk usage (per-container storage) */
  container_usage?: TypesContainerDiskUsage[];
  /**
   * Desktop image versions (content-addressable Docker image hashes)
   * Key: desktop name (e.g., "sway", "zorin", "ubuntu")
   * Value: image hash (e.g., "a1b2c3d4e5f6...")
   */
  desktop_versions?: Record<string, string>;
  /** Disk usage metrics for monitored mount points */
  disk_usage?: TypesDiskUsageMetric[];
  /** GPU configuration */
  gpu_vendor?: string;
  /**
   * GPUs is the per-GPU inventory used by the API server's profile
   * compatibility check. Empty for sandboxes that don't host inference.
   */
  gpus?: TypesGPUStatus[];
  /** Helix version running on this sandbox (git commit hash or release version) */
  helix_version?: string;
  /**
   * InstanceType is the cloud instance type (e.g. "inf2.8xlarge", "g5.xlarge")
   * detected via the AWS IMDS. Empty on bare-metal hosts (e.g. prime) and any
   * non-AWS environment — the admin UI then just shows the GPU model instead.
   */
  instance_type?: string;
  /** Privileged mode (host Docker access for development) */
  privileged_mode_enabled?: boolean;
  /** ProfileError carries the failure detail when ProfileStatus="failed". */
  profile_error?: string;
  /**
   * ProfileProgress is per-service model-weights download progress,
   * surfaced when ProfileStatus="starting". Empty once all services
   * finish downloading.
   */
  profile_progress?: Record<string, TypesServiceDownloadProgress>;
  /**
   * ProfileStatus reports the compose stack lifecycle. Empty when no
   * profile is assigned. Allowed values: "assigning" | "pulling" |
   * "starting" | "running" | "failed".
   */
  profile_status?: string;
  /** /dev/dri/renderD128 or SOFTWARE */
  render_node?: string;
  /**
   * ServiceHealth maps compose service name -> health string.
   * "healthy" | "starting" | "failed" | "unknown".
   */
  service_health?: Record<string, string>;
}

export interface TypesSandboxInstance {
  /**
   * ActiveProfileID is the ID of the runner profile this sandbox is
   * currently running (or attempting to run). Empty for pure-agent
   * sandboxes with no profile assigned.
   */
  active_profile_id?: string;
  /**
   * Sandbox capacity. MaxSandboxes is set explicitly at auto-register
   * and Manager-provisioned paths from HELIX_SANDBOX_MAX_DEV_CONTAINERS
   * (default 20); the gorm default below only applies to rows inserted
   * via paths that don't set the field. Kept aligned with the env-var
   * default to avoid surprises.
   */
  active_sandboxes?: number;
  /**
   * ComputeState tracks the provider's view of the host's provisioning
   * lifecycle. Distinct from Status (which is the heartbeat-derived
   * online/offline/degraded view). Values: "provisioning" | "ready" |
   * "terminating" | "terminated" | "failed". See compute.State for
   * the canonical enum. Empty for self-registered hosts.
   */
  compute_state?: string;
  created?: string;
  /**
   * Desktop image versions available on this sandbox
   * Key: desktop name (e.g., "sway", "ubuntu"), Value: image hash
   */
  desktop_versions?: Record<string, string>;
  /** GPU configuration */
  gpu_vendor?: string;
  /**
   * GPUs is the per-GPU inventory the sandbox reports for inference
   * scheduling. Vendor / Architecture / ComputeCapability on each
   * entry are the load-bearing fields for profile compatibility.
   * Explicit column tag because GORM's default snake_case derivation
   * turns `GPUs` into `gp_us`.
   */
  gpus?: object[];
  /** Helix version running on this sandbox (git commit hash or release version) */
  helix_version?: string;
  /** Hostname for DNS resolution */
  hostname?: string;
  id?: string;
  /**
   * InstanceType is the cloud instance type reported by the sandbox heartbeat
   * (e.g. "inf2.8xlarge"). Empty on bare-metal / non-AWS hosts.
   */
  instance_type?: string;
  /** IP address of the sandbox */
  ip_address?: string;
  /** Last heartbeat time */
  last_seen?: string;
  /** Maximum allowed containers */
  max_sandboxes?: number;
  privileged_mode?: boolean;
  /** ProfileError carries the failure detail when ProfileStatus="failed". */
  profile_error?: string;
  /**
   * ProfileProgress is per-service download progress for model weights,
   * surfaced when ProfileStatus="starting" and a vLLM container is
   * pulling weights from Hugging Face Hub. Empty once all services are
   * healthy. Map key is compose service name.
   */
  profile_progress?: Record<string, object>;
  /**
   * ProfileStatus tracks the compose stack lifecycle:
   * "" | "assigning" | "pulling" | "starting" | "running" | "failed".
   */
  profile_status?: string;
  /**
   * Provider is the Name() of the compute.Provider that owns this host.
   * For pool-discovery providers this is a composite key baked from the
   * deployment tag, worker tag and instance type (e.g.
   * "yellowdog-helix-development-worker-psamuel-g5-xlarge-164e3a34"), so it
   * needs the same width as ProviderID. Empty for self-registered hosts.
   */
  provider?: string;
  /**
   * ProviderID is the upstream system's opaque identifier for this
   * host (e.g. a YellowDog work-requirement YDID). Forms a composite
   * index with Provider so the reconciler can look hosts up cheaply.
   */
  provider_id?: string;
  /**
   * ProviderMetadata is provider-specific opaque data for
   * reconciliation, debugging, and admin display. Examples for YD:
   * worker-pool ID, compute requirement ID, region, public IP.
   */
  provider_metadata?: Record<string, string>;
  /**
   * ProvisionedAt is when Helix asked the provider to bring this host
   * up. Earlier than Created (which is the first heartbeat). Nil for
   * self-registered hosts.
   */
  provisioned_at?: string;
  /** /dev/dri/renderD128 or SOFTWARE */
  render_node?: string;
  /**
   * ServiceHealth maps compose service name -> health string
   * ("healthy" | "starting" | "failed" | "unknown"). Reported by
   * compose-manager polling each container's /v1/models endpoint
   * (or vendor-specific health endpoint).
   */
  service_health?: Record<string, string>;
  /** "online", "offline", "degraded" */
  status?: string;
  updated?: string;
}

export interface TypesSandboxListResponse {
  sandboxes?: TypesSandbox[];
  total?: number;
}

export enum TypesSandboxRuntime {
  SandboxRuntimeUbuntuDesktop = "ubuntu-desktop",
  SandboxRuntimeHeadlessUbuntu = "headless-ubuntu",
}

export enum TypesSandboxStatus {
  SandboxStatusPending = "pending",
  SandboxStatusRunning = "running",
  SandboxStatusStopping = "stopping",
  SandboxStatusStopped = "stopped",
  SandboxStatusFailed = "failed",
}

export interface TypesSecret {
  /** optional, if set, the secret will be available to the specified app */
  app_id?: string;
  created?: string;
  id?: string;
  name?: string;
  owner?: string;
  ownerType?: TypesOwnerType;
  /** optional, if set, the secret will be available as env var in project sessions */
  project_id?: string;
  /**
   * Scope controls which environment a project secret is injected into.
   * Defaults to "dev" so pre-existing secrets keep their original (dev-only)
   * behaviour and dev stays the primary path.
   */
  scope?: TypesSecretScope;
  updated?: string;
  value?: number[];
}

export enum TypesSecretScope {
  SecretScopeDev = "dev",
  SecretScopeProd = "prod",
  SecretScopeBoth = "both",
}

export interface TypesServerConfigForFrontend {
  apps_enabled?: boolean;
  auth_provider?: TypesAuthProvider;
  /** Charging for usage */
  billing_enabled?: boolean;
  /**
   * DefaultChatSystemPrompt is the system prompt the platform applies to
   * direct model chats when the user has not customised one. Surfaced to
   * the frontend so the chat-settings page can prefill the textbox.
   */
  default_chat_system_prompt?: string;
  deployment_id?: string;
  disable_llm_call_logging?: boolean;
  /** "mac-desktop", "server", "cloud", etc. */
  edition?: string;
  filestore_prefix?: string;
  google_analytics_frontend?: string;
  /** Whether any global AI provider with enabled chat models exists */
  has_providers?: boolean;
  latest_version?: string;
  /**
   * MaxConcurrentDesktops: cap on concurrent desktop sessions. Enforced per
   * organisation when the session has an org, per user otherwise.
   * -1 = unlimited. Note: /config is unauthenticated, so this is the
   * Free-tier floor; real enforcement uses the resolved per-user/per-org cap.
   */
  max_concurrent_desktops?: number;
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
  /** Require an active subscription before allowing to use the product */
  require_active_subscription?: boolean;
  rudderstack_data_plane_url?: string;
  rudderstack_write_key?: string;
  sentry_dsn_frontend?: string;
  /**
   * ServerURL is the operator-configured public origin for this helix
   * instance (env SERVER_URL → WebServer.URL). Empty when not
   * configured; the frontend then falls back to
   * `window.location.origin`. The github-stream New Stream dialog
   * uses this to surface a webhook URL that's actually reachable by
   * GitHub — `window.location.origin` is wrong whenever the user is
   * hitting the app via localhost / a dev port that GitHub can't
   * reach.
   */
  server_url?: string;
  /** Stripe top-ups enabled */
  stripe_enabled?: boolean;
  tools_enabled?: boolean;
  version?: string;
}

export interface TypesServiceConnectionCreateRequest {
  ado_client_id?: string;
  ado_client_secret?: string;
  /** Azure DevOps Service Principal fields */
  ado_organization_url?: string;
  ado_tenant_id?: string;
  /** Base URL for enterprise/self-hosted instances */
  base_url?: string;
  description?: string;
  /** GitHub App fields */
  github_app_id?: number;
  github_installation_id?: number;
  github_private_key?: string;
  name?: string;
  slack_app_token?: string;
  slack_bot_token?: string;
  /** Slack global app fields (type=slack_app) */
  slack_client_id?: string;
  slack_client_secret?: string;
  slack_ingress_mode?: string;
  slack_signing_secret?: string;
  type?: TypesServiceConnectionType;
}

export interface TypesServiceConnectionResponse {
  ado_client_id?: string;
  /** Azure DevOps Service Principal (non-sensitive fields only) */
  ado_organization_url?: string;
  ado_tenant_id?: string;
  base_url?: string;
  created_at?: string;
  description?: string;
  /** GitHub App (non-sensitive fields only) */
  github_app_id?: number;
  github_installation_id?: number;
  has_ado_client_secret?: boolean;
  has_github_private_key?: boolean;
  has_slack_app_token?: boolean;
  has_slack_bot_token?: boolean;
  has_slack_client_secret?: boolean;
  has_slack_signing_secret?: boolean;
  id?: string;
  last_error?: string;
  last_tested_at?: string;
  name?: string;
  organization_id?: string;
  provider_type?: TypesExternalRepositoryType;
  slack_app_connection_id?: string;
  slack_app_id?: string;
  slack_bot_user_id?: string;
  /** Slack (non-sensitive fields + has-secret flags) */
  slack_client_id?: string;
  slack_ingress_mode?: string;
  slack_team_id?: string;
  slack_team_name?: string;
  type?: TypesServiceConnectionType;
  updated_at?: string;
}

export enum TypesServiceConnectionType {
  ServiceConnectionTypeGitHubApp = "github_app",
  ServiceConnectionTypeADOServicePrincipal = "ado_service_principal",
  ServiceConnectionTypeSlackApp = "slack_app",
  ServiceConnectionTypeSlackWorkspace = "slack_workspace",
}

export interface TypesServiceConnectionUpdateRequest {
  ado_client_id?: string;
  ado_client_secret?: string;
  /** Azure DevOps Service Principal fields (only update if provided) */
  ado_organization_url?: string;
  ado_tenant_id?: string;
  /** Base URL for enterprise/self-hosted instances */
  base_url?: string;
  description?: string;
  /** GitHub App fields (only update if provided) */
  github_app_id?: number;
  github_installation_id?: number;
  github_private_key?: string;
  name?: string;
  slack_app_token?: string;
  slack_bot_token?: string;
  /** Slack global app fields (only update if provided) */
  slack_client_id?: string;
  slack_client_secret?: string;
  slack_ingress_mode?: string;
  slack_signing_secret?: string;
}

export interface TypesServiceDownloadProgress {
  /**
   * Current and Total are the raw N/M from the progress line (e.g.
   * shards 12 of 47). Useful when Percent is computed.
   */
  current?: number;
  /**
   * ETA is the rendered remaining-time string from the source line
   * (e.g. "27:18"). Verbatim — not parsed into a duration.
   */
  eta?: string;
  /** Percent is 0-100. Zero means "no progress line parsed yet". */
  percent?: number;
  /**
   * Stage is a short tag for what's downloading: "shards", "files",
   * "weights" or "" if unknown. Drives UI labelling.
   */
  stage?: string;
  total?: number;
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
  project_id?: string;
  /**
   * huggingface model name e.g. mistralai/Mistral-7B-Instruct-v0.1 or
   * stabilityai/stable-diffusion-xl-base-1.0
   */
  provider?: string;
  /** The question set execution this session belongs to, if any */
  question_set_execution_id?: string;
  /** The question set this session belongs to, if any */
  question_set_id?: string;
  /** SandboxID tracks which sandbox instance is running this session's dev container (if any) */
  sandbox_id?: string;
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
  /** Autonomous surfaces: auto-recover the agent on crash (no human to click Restart) */
  auto_restart_on_crash?: boolean;
  /** Webhook URL to POST on session completion */
  callback_url?: string;
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
  /** The project this session belongs to, if any */
  project_id?: string;
  /** The provider to use */
  provider?: TypesProvider;
  /** If true, we will regenerate the response for the last message */
  regenerate?: boolean;
  /** If empty, we will start a new session */
  session_id?: string;
  /** e.g. "job" — categorizes sessions for filtering */
  session_role?: string;
  /** If true, we will stream the response */
  stream?: boolean;
  /** System message, only applicable when starting a new session */
  system?: string;
  /** Available tools to use in the session */
  tools?: string[];
  /** e.g. text, image */
  type?: TypesSessionType;
}

export interface TypesSessionInfo {
  auth_provider?: TypesAuthProvider;
  created_at?: string;
  expires_at?: string;
  id?: string;
  user_id?: string;
}

export interface TypesSessionMetadata {
  active_tools?: string[];
  /**
   * AgentSwitchedAt is set when the agent framework is switched IN PLACE on
   * this same session (no fork / new container) — see
   * design/tasks/002111_so-we-recently-added-a/design.md. It marks that a
   * fork_seed interaction carrying the prior thread's transcript exists on
   * THIS session, so maybePrependTranscript seeds the new Zed thread even
   * though ParentSessionID is empty (the session continues from itself).
   */
  agent_switched_at?: string;
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
  auto_restart_count?: number;
  /**
   * Autonomous crash recovery. Set true at session creation for surfaces with
   * no human present to click the in-chat Restart button (spec tasks, org
   * workers). When the external agent crashes mid-turn, the websocket crash
   * handler auto-invokes the canonical restart primitive instead of leaving
   * the session errored+idle. Human desktop sessions leave this false and keep
   * the explicit button. AutoRestartCount bounds consecutive auto-restarts
   * without an intervening successful turn (anti-storm guard); it is reset to 0
   * on the next successful completion and lives on the SESSION (not the prompt)
   * so ResetCrashedPromptsForSession can't zero the restart budget.
   */
  auto_restart_on_crash?: boolean;
  avatar?: string;
  /** Webhook URL to POST on session completion */
  callback_url?: string;
  /** Which code agent runtime is used (zed_agent, qwen_code, claude_code, etc.) */
  code_agent_runtime?: TypesCodeAgentRuntime;
  /** Docker container ID */
  container_id?: string;
  /** Container IP on bridge network */
  container_ip?: string;
  /** Container fields (Hydra executor) */
  container_name?: string;
  /** Dev container ID for streaming */
  dev_container_id?: string;
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
  /** Executor mode (deprecated - always "hydra") */
  executor_mode?: string;
  /** Configuration for external agents */
  external_agent_config?: TypesExternalAgentConfig;
  /** NEW: External agent ID for this session */
  external_agent_id?: string;
  /** NEW: External agent status (running, stopped, terminated_idle) */
  external_agent_status?: string;
  forked_at?: string;
  forked_at_interaction_id?: string;
  /** GPU vendor of sandbox running this session (nvidia, amd, intel, none) */
  gpu_vendor?: string;
  helix_version?: string;
  /** Index of implementation task this session handles */
  implementation_task_index?: number;
  last_auto_restart_at?: string;
  manually_review_questions?: boolean;
  /**
   * Fork lineage — set on a session created by forking from a parent.
   * See design/tasks/002081_kickoff-mid-session/design.md.
   */
  parent_session_id?: string;
  /**
   * Pause state — sessions cannot accept new messages while paused.
   * PausedReason is the only producer in v1: "forked_to:<child_id>".
   */
  paused?: boolean;
  paused_at?: string;
  paused_reason?: string;
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
  /** GPU render node of sandbox (/dev/dri/renderD128 or SOFTWARE) */
  render_node?: string;
  session_rag_results?: TypesSessionRAGResult[];
  /** "planning", "implementation", "coordination", "exploratory" */
  session_role?: string;
  /** Multi-session SpecTask context */
  spec_task_id?: string;
  /** Transient status message shown during startup (e.g., "Unpacking build cache (2.1/7.0 GB)") */
  status_message?: string;
  stream?: boolean;
  /** helix-sway image version (commit hash) running in this session */
  sway_version?: string;
  system_prompt?: string;
  /** True for internal system sessions (e.g., summary generation) - skip summary generation to avoid loops */
  system_session?: boolean;
  /** without any user input, this will default to true */
  text_finetune_enabled?: boolean;
  /**
   * Title history - tracks evolution of session topics (newest first)
   * Shown on hover in the SpecTask tab view to see what topics were covered
   */
  title_history?: TypesTitleHistoryEntry[];
  /** when we do fine tuning or RAG, we need to know which data entity we used */
  uploaded_data_entity_id?: string;
  /** ID of associated WorkSession */
  work_session_id?: string;
  /** Agent name used when thread was created (e.g., "zed-agent", "claude", "qwen") */
  zed_agent_name?: string;
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

export interface TypesSessionOutputResponse {
  duration_ms?: number;
  interaction_id?: string;
  /** Last interaction's response text */
  output?: string;
  session_id?: string;
  /** "waiting", "complete", "error", "interrupted" */
  status?: string;
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
  /** Metadata includes container information for external agent sessions */
  metadata?: TypesSessionMetadata;
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
  /** MCP configuration (present when this skill is MCP-backed rather than API-backed) */
  mcp?: TypesSkillMCPSpec;
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

export interface TypesSkillMCPSpec {
  /** if true, URL+auth are generated server-side */
  autoProvision?: boolean;
  /** "http" or "sse" */
  transport?: string;
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
  /** Send project updates to this channel */
  project_channel?: string;
  /** Send project updates (spec task creation, completion, etc) */
  project_updates?: boolean;
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
  /** Current agent work state (idle/working/done) from activity tracking */
  agent_work_state?: TypesAgentWorkState;
  /** Archive to hide from main view */
  archived?: boolean;
  /** Team member assigned to work on this task */
  assignee_id?: string;
  /** The base branch this was created from */
  base_branch?: string;
  /** "new" or "existing" */
  branch_mode?: TypesBranchMode;
  /** Git tracking */
  branch_name?: string;
  /** User-specified prefix for new branches (task# appended) */
  branch_prefix?: string;
  /** Groups tasks from same clone operation */
  clone_group_id?: string;
  /** Clone tracking */
  cloned_from_id?: string;
  /** Original project */
  cloned_from_project_id?: string;
  completed_at?: string;
  created_at?: string;
  /** Metadata */
  created_by?: string;
  depends_on?: TypesSpecTask[];
  description?: string;
  design_doc_path?: string;
  /** When design docs were pushed to helix-specs branch */
  design_docs_pushed_at?: string;
  /** Simple tracking */
  estimated_hours?: number;
  /** External agent tracking (single agent per SpecTask, spans entire workflow) */
  external_agent_id?: string;
  /**
   * Goose recipe binding (Phase 2b). When the parent project's agent uses
   * the goose_code runtime and the user picked a recipe at task-creation
   * time, GooseRecipeName names the AssistantGooseRecipe to invoke and
   * GooseRecipeParams holds the parameter values to substitute. The Helix
   * API bakes these into a CodeAgentBakedRecipe and pushes it to the
   * settings-sync-daemon, which writes a single slash_command pointing at
   * the substituted recipe YAML. Empty when no recipe was selected.
   */
  goose_recipe_name?: string;
  goose_recipe_params?: Record<string, string>;
  /** NEW: Single Helix Agent for entire workflow (App type in code) */
  helix_app_id?: string;
  id?: string;
  implementation_approved_at?: string;
  /** Implementation tracking */
  implementation_approved_by?: string;
  /** Discrete tasks breakdown (markdown) */
  implementation_plan?: string;
  /** Skip spec planning, go straight to implementation */
  just_do_it_mode?: boolean;
  /** Keep alive — prevent auto-idle-shutdown of desktop container */
  keep_alive?: boolean;
  labels?: string[];
  /** Last prompt sent to agent (for continue functionality) */
  last_prompt_content?: string;
  /** When branch was last pushed */
  last_push_at?: string;
  /** Git tracking */
  last_push_commit_hash?: string;
  /**
   * Structured error from the last external (mirror) push. Set when a user-initiated
   * push to the external repo fails and refs are rolled back; cleared (nil) on the
   * next successful push. Surfaced on the board so failures aren't silent.
   */
  last_push_error?: TypesPushError;
  /** Merge commit hash */
  merge_commit_hash?: string;
  /** When merge happened */
  merged_at?: string;
  /** Whether branch was merged to main */
  merged_to_main?: boolean;
  metadata?: Record<string, any>;
  /** Indexed for search prefix matching */
  name?: string;
  /** Organization scope for search */
  organization_id?: string;
  /** Kiro's actual approach: simple, human-readable artifacts */
  original_prompt?: string;
  planning_options?: TypesStartPlanningOptions;
  /**
   * Session tracking (single Helix session for entire workflow - planning + implementation)
   * The same external agent/session is reused throughout the entire SpecTask lifecycle
   */
  planning_session_id?: string;
  planning_started_at?: string;
  /** User who kicked off planning (may differ from CreatedBy) */
  planning_started_by?: string;
  /** "low", "medium", "high", "critical" */
  priority?: TypesSpecTaskPriority;
  project_id?: string;
  project_path?: string;
  /** Public sharing */
  public_design_docs?: boolean;
  /** Set when approveImplementation hits a divergent branch and asks the agent to rebase. Used to make the approve handler idempotent (no duplicate prompts) and to gate the Accept button until the agent's next push. */
  rebase_requested_at?: string;
  /** Multi-repo PR tracking: list of PRs across all project repositories */
  repo_pull_requests?: TypesRepoPR[];
  /** User stories + EARS acceptance criteria (markdown) */
  requirements_spec?: string;
  /** "absent", "running", "starting" — derived from session config in listTasks */
  sandbox_state?: string;
  /** Transient startup message e.g. "Unpacking build cache" */
  sandbox_status_message?: string;
  /** Agent activity tracking (computed from session/activity data, not stored) */
  session_updated_at?: string;
  /**
   * Short title for tab display (auto-generated from agent writing short-title.txt)
   * UserShortTitle takes precedence if set (user override)
   */
  short_title?: string;
  spec_approval?: TypesSpecApprovalResponse;
  spec_approved_at?: string;
  /** Approval tracking */
  spec_approved_by?: string;
  /** Number of spec revisions requested */
  spec_revision_count?: number;
  started_at?: string;
  /** Spec-driven workflow statuses - see constants below */
  status?: TypesSpecTaskStatus;
  /** When status last changed (for Kanban column sorting) */
  status_updated_at?: string;
  /**
   * Human-readable directory naming for design docs in helix-specs branch
   * TaskNumber is auto-assigned from project.NextTaskNumber when task starts
   * DesignDocPath format: "YYYY-MM-DD_shortname_N" e.g., "2025-12-09_install-cowsay_1"
   */
  task_number?: number;
  /** Design document (markdown) */
  technical_design?: string;
  /** "feature", "bug", "refactor" */
  type?: string;
  updated_at?: string;
  /** Owner user ID for search */
  user_id?: string;
  /** User override */
  user_short_title?: string;
  workspace_config?: number[];
  /** Multi-session support */
  zed_instance_id?: string;
}

export interface TypesSpecTaskArchiveRequest {
  archived?: boolean;
}

export interface TypesSpecTaskAttachment {
  /** optional user note */
  caption?: string;
  /** helix-specs commit hash once staged */
  committed_sha?: string;
  created_at?: string;
  /** original filename, sanitised */
  filename?: string;
  /** att_01k... */
  id?: string;
  mime_type?: string;
  /** denormalised for fast authz */
  project_id?: string;
  size_bytes?: number;
  spec_task_id?: string;
  /** who uploaded */
  user_id?: string;
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
  /** Agent's structured entries (for tool call rendering) */
  agent_response_entries?: number[];
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
  /** Link to the prompt_history_entry enqueued for this comment; RequestID/InteractionID are backfilled from it at dispatch */
  prompt_id?: string;
  /**
   * Database-backed queue for agent processing (restart-resilient)
   * QueuedAt is set when comment is submitted for agent processing.
   * Processing order: QueuedAt ASC. Cleared when agent response is received.
   */
  queued_at?: string;
  /** For inline comments - store the context around the comment */
  quoted_text?: string;
  /** Request ID used when sending to agent (for response linking) */
  request_id?: string;
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

export enum TypesSpecTaskDesignReviewCommentType {
  SpecTaskDesignReviewCommentTypeGeneral = "general",
  SpecTaskDesignReviewCommentTypeQuestion = "question",
  SpecTaskDesignReviewCommentTypeSuggestion = "suggestion",
  SpecTaskDesignReviewCommentTypeCritical = "critical",
  SpecTaskDesignReviewCommentTypePraise = "praise",
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

export enum TypesSpecTaskDesignReviewStatus {
  SpecTaskDesignReviewStatusPending = "pending",
  SpecTaskDesignReviewStatusInReview = "in_review",
  SpecTaskDesignReviewStatusChangesRequested = "changes_requested",
  SpecTaskDesignReviewStatusApproved = "approved",
  SpecTaskDesignReviewStatusSuperseded = "superseded",
}

export interface TypesSpecTaskDesignReviewSubmitRequest {
  /** "approve" or "request_changes" */
  decision: "approve" | "request_changes";
  overall_comment?: string;
  review_id: string;
}

export enum TypesSpecTaskPhase {
  SpecTaskPhasePlanning = "planning",
  SpecTaskPhaseImplementation = "implementation",
  SpecTaskPhaseValidation = "validation",
}

export enum TypesSpecTaskPriority {
  SpecTaskPriorityLow = "low",
  SpecTaskPriorityMedium = "medium",
  SpecTaskPriorityHigh = "high",
  SpecTaskPriorityCritical = "critical",
}

export enum TypesSpecTaskStatus {
  TaskStatusBacklog = "backlog",
  TaskStatusQueuedImplementation = "queued_implementation",
  TaskStatusQueuedSpecGeneration = "queued_spec_generation",
  TaskStatusSpecGeneration = "spec_generation",
  TaskStatusSpecReview = "spec_review",
  TaskStatusSpecRevision = "spec_revision",
  TaskStatusSpecApproved = "spec_approved",
  TaskStatusImplementationQueued = "implementation_queued",
  TaskStatusImplementation = "implementation",
  TaskStatusImplementationReview = "implementation_review",
  TaskStatusPullRequest = "pull_request",
  TaskStatusDone = "done",
  TaskStatusSpecFailed = "spec_failed",
  TaskStatusImplementationFailed = "implementation_failed",
}

export interface TypesSpecTaskUpdateRequest {
  /** Pointer to allow clearing (set to empty string to unassign) */
  assignee_id?: string;
  /** IDs of tasks this task depends on */
  depends_on?: string[];
  description?: string;
  /** Agent to use for this task */
  helix_app_id?: string;
  /** Pointer to allow explicit false */
  just_do_it_mode?: boolean;
  /** Pointer to allow explicit false — prevent auto-idle-shutdown */
  keep_alive?: boolean;
  name?: string;
  priority?: TypesSpecTaskPriority;
  /** Pointer to allow explicit false */
  public_design_docs?: boolean;
  status?: TypesSpecTaskStatus;
  /** User override for tab title (pointer to allow clearing with empty string) */
  user_short_title?: string;
}

export interface TypesSpecTaskWithProject {
  /** Current agent work state (idle/working/done) from activity tracking */
  agent_work_state?: TypesAgentWorkState;
  /** Archive to hide from main view */
  archived?: boolean;
  /** Team member assigned to work on this task */
  assignee_id?: string;
  /** The base branch this was created from */
  base_branch?: string;
  /** "new" or "existing" */
  branch_mode?: TypesBranchMode;
  /** Git tracking */
  branch_name?: string;
  /** User-specified prefix for new branches (task# appended) */
  branch_prefix?: string;
  /** Groups tasks from same clone operation */
  clone_group_id?: string;
  /** Clone tracking */
  cloned_from_id?: string;
  /** Original project */
  cloned_from_project_id?: string;
  completed_at?: string;
  created_at?: string;
  /** Metadata */
  created_by?: string;
  depends_on?: TypesSpecTask[];
  description?: string;
  design_doc_path?: string;
  /** When design docs were pushed to helix-specs branch */
  design_docs_pushed_at?: string;
  /** Simple tracking */
  estimated_hours?: number;
  /** External agent tracking (single agent per SpecTask, spans entire workflow) */
  external_agent_id?: string;
  /**
   * Goose recipe binding (Phase 2b). When the parent project's agent uses
   * the goose_code runtime and the user picked a recipe at task-creation
   * time, GooseRecipeName names the AssistantGooseRecipe to invoke and
   * GooseRecipeParams holds the parameter values to substitute. The Helix
   * API bakes these into a CodeAgentBakedRecipe and pushes it to the
   * settings-sync-daemon, which writes a single slash_command pointing at
   * the substituted recipe YAML. Empty when no recipe was selected.
   */
  goose_recipe_name?: string;
  goose_recipe_params?: Record<string, string>;
  /** NEW: Single Helix Agent for entire workflow (App type in code) */
  helix_app_id?: string;
  id?: string;
  implementation_approved_at?: string;
  /** Implementation tracking */
  implementation_approved_by?: string;
  /** Discrete tasks breakdown (markdown) */
  implementation_plan?: string;
  /** Skip spec planning, go straight to implementation */
  just_do_it_mode?: boolean;
  /** Keep alive — prevent auto-idle-shutdown of desktop container */
  keep_alive?: boolean;
  labels?: string[];
  /** Last prompt sent to agent (for continue functionality) */
  last_prompt_content?: string;
  /** When branch was last pushed */
  last_push_at?: string;
  /** Git tracking */
  last_push_commit_hash?: string;
  /**
   * Structured error from the last external (mirror) push. Set when a user-initiated
   * push to the external repo fails and refs are rolled back; cleared (nil) on the
   * next successful push. Surfaced on the board so failures aren't silent.
   */
  last_push_error?: TypesPushError;
  /** Merge commit hash */
  merge_commit_hash?: string;
  /** When merge happened */
  merged_at?: string;
  /** Whether branch was merged to main */
  merged_to_main?: boolean;
  metadata?: Record<string, any>;
  /** Indexed for search prefix matching */
  name?: string;
  /** Organization scope for search */
  organization_id?: string;
  /** Kiro's actual approach: simple, human-readable artifacts */
  original_prompt?: string;
  planning_options?: TypesStartPlanningOptions;
  /**
   * Session tracking (single Helix session for entire workflow - planning + implementation)
   * The same external agent/session is reused throughout the entire SpecTask lifecycle
   */
  planning_session_id?: string;
  planning_started_at?: string;
  /** User who kicked off planning (may differ from CreatedBy) */
  planning_started_by?: string;
  /** "low", "medium", "high", "critical" */
  priority?: TypesSpecTaskPriority;
  project_id?: string;
  project_name?: string;
  project_path?: string;
  /** Public sharing */
  public_design_docs?: boolean;
  /** Set when approveImplementation hits a divergent branch and asks the agent to rebase. Used to make the approve handler idempotent (no duplicate prompts) and to gate the Accept button until the agent's next push. */
  rebase_requested_at?: string;
  /** Multi-repo PR tracking: list of PRs across all project repositories */
  repo_pull_requests?: TypesRepoPR[];
  /** User stories + EARS acceptance criteria (markdown) */
  requirements_spec?: string;
  /** "absent", "running", "starting" — derived from session config in listTasks */
  sandbox_state?: string;
  /** Transient startup message e.g. "Unpacking build cache" */
  sandbox_status_message?: string;
  /** Agent activity tracking (computed from session/activity data, not stored) */
  session_updated_at?: string;
  /**
   * Short title for tab display (auto-generated from agent writing short-title.txt)
   * UserShortTitle takes precedence if set (user override)
   */
  short_title?: string;
  spec_approval?: TypesSpecApprovalResponse;
  spec_approved_at?: string;
  /** Approval tracking */
  spec_approved_by?: string;
  /** Number of spec revisions requested */
  spec_revision_count?: number;
  started_at?: string;
  /** Spec-driven workflow statuses - see constants below */
  status?: TypesSpecTaskStatus;
  /** When status last changed (for Kanban column sorting) */
  status_updated_at?: string;
  /**
   * Human-readable directory naming for design docs in helix-specs branch
   * TaskNumber is auto-assigned from project.NextTaskNumber when task starts
   * DesignDocPath format: "YYYY-MM-DD_shortname_N" e.g., "2025-12-09_install-cowsay_1"
   */
  task_number?: number;
  /** Design document (markdown) */
  technical_design?: string;
  /** "feature", "bug", "refactor" */
  type?: string;
  updated_at?: string;
  /** Owner user ID for search */
  user_id?: string;
  /** User override */
  user_short_title?: string;
  workspace_config?: number[];
  /** Multi-session support */
  zed_instance_id?: string;
}

export interface TypesSpecTaskWorkSession {
  /** Configuration */
  agent_config?: number[];
  completed_at?: string;
  /** State tracking */
  created_at?: string;
  description?: string;
  environment_config?: number[];
  /** Maps to Helix session (multiple Zed threads can share one session) */
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

export enum TypesSpecTaskWorkSessionStatus {
  SpecTaskWorkSessionStatusPending = "pending",
  SpecTaskWorkSessionStatusActive = "active",
  SpecTaskWorkSessionStatusCompleted = "completed",
  SpecTaskWorkSessionStatusFailed = "failed",
  SpecTaskWorkSessionStatusCancelled = "cancelled",
  SpecTaskWorkSessionStatusBlocked = "blocked",
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

export interface TypesStartPlanningOptions {
  /**
   * KeyboardLayout is the XKB keyboard layout code (e.g., "us", "fr", "de")
   * Used to configure the desktop container's keyboard layout from browser detection
   */
  keyboard_layout?: string;
  /** Timezone is the IANA timezone (e.g., "America/New_York", "Europe/Paris") */
  timezone?: string;
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

export interface TypesSyncAllResponse {
  message?: string;
  repository_id?: string;
  success?: boolean;
}

export interface TypesSystemSettingsRequest {
  enforce_quotas?: boolean;
  huggingface_token?: string;
  kodit_enrichment_model?: string;
  /** Kodit enrichment model configuration */
  kodit_enrichment_provider?: string;
  kodit_text_embedding_model?: string;
  /** Kodit text embedding model configuration */
  kodit_text_embedding_provider?: string;
  kodit_vision_embedding_model?: string;
  /** Kodit vision embedding model configuration */
  kodit_vision_embedding_provider?: string;
  max_concurrent_desktop_sandboxes?: number;
  max_concurrent_headless_sandboxes?: number;
  optimus_generation_model?: string;
  optimus_generation_model_provider?: string;
  optimus_reasoning_model?: string;
  optimus_reasoning_model_effort?: string;
  optimus_reasoning_model_provider?: string;
  optimus_small_generation_model?: string;
  optimus_small_generation_model_provider?: string;
  optimus_small_reasoning_model?: string;
  optimus_small_reasoning_model_effort?: string;
  optimus_small_reasoning_model_provider?: string;
  providers_management_enabled?: boolean;
  sandbox_billing_enabled?: boolean;
  sandbox_desktop_price_credits_per_second?: number;
  sandbox_headless_price_credits_per_second?: number;
}

export interface TypesSystemSettingsResponse {
  created?: string;
  enforce_quotas?: boolean;
  /** Sensitive fields are masked */
  huggingface_token_set?: boolean;
  /** "database", "environment", or "none" */
  huggingface_token_source?: string;
  id?: string;
  kodit_enrichment_model?: string;
  /** true if both provider and model are configured */
  kodit_enrichment_model_set?: boolean;
  /** Kodit enrichment model configuration (not sensitive, returned as-is) */
  kodit_enrichment_provider?: string;
  kodit_text_embedding_model?: string;
  kodit_text_embedding_model_set?: boolean;
  /** Kodit text embedding model configuration */
  kodit_text_embedding_provider?: string;
  kodit_vision_embedding_model?: string;
  kodit_vision_embedding_model_set?: boolean;
  /** Kodit vision embedding model configuration */
  kodit_vision_embedding_provider?: string;
  max_concurrent_desktop_sandboxes?: number;
  max_concurrent_headless_sandboxes?: number;
  optimus_generation_model?: string;
  optimus_generation_model_provider?: string;
  optimus_reasoning_model?: string;
  optimus_reasoning_model_effort?: string;
  /** Optimus configuration */
  optimus_reasoning_model_provider?: string;
  optimus_small_generation_model?: string;
  optimus_small_generation_model_provider?: string;
  optimus_small_reasoning_model?: string;
  optimus_small_reasoning_model_effort?: string;
  optimus_small_reasoning_model_provider?: string;
  providers_management_enabled?: boolean;
  sandbox_billing_enabled?: boolean;
  sandbox_desktop_price_credits_per_second?: number;
  sandbox_headless_price_credits_per_second?: number;
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

export interface TypesTeamsTrigger {
  /** Microsoft App ID */
  app_id?: string;
  /** Microsoft App Password */
  app_password?: string;
  enabled?: boolean;
  /** Optional: restrict to specific tenant */
  tenant_id?: string;
}

export interface TypesTestStep {
  expected_output?: string;
  prompt?: string;
}

export enum TypesTextSplitterType {
  TextSplitterTypeMarkdown = "markdown",
  TextSplitterTypeText = "text",
}

export interface TypesTitleHistoryEntry {
  /** When the title was changed */
  changed_at?: string;
  /** Interaction ID for navigation - click to jump here */
  interaction_id?: string;
  /** The title that was set */
  title?: string;
  /** Turn number that triggered the change (1-indexed) */
  turn?: number;
}

export enum TypesTokenType {
  TokenTypeNone = "",
  TokenTypeRunner = "runner",
  TokenTypeOIDC = "oidc",
  TokenTypeAPIKey = "api_key",
  TokenTypeSocket = "socket",
  TokenTypeSession = "session",
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
  /** Helix project management skill */
  project?: TypesToolProjectManagerConfig;
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
  tools?: GithubComMark3LabsMcpGoMcpTool[];
  /** "http" (default, Streamable HTTP) or "sse" (legacy SSE transport) */
  transport?: string;
  url?: string;
}

export interface TypesToolProjectManagerConfig {
  enabled?: boolean;
  project_id?: string;
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
  ToolTypeProjectManager = "project_manager",
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
  azure_devops?: TypesAzureDevOpsTrigger;
  crisp?: TypesCrispTrigger;
  cron?: TypesCronTrigger;
  discord?: TypesDiscordTrigger;
  slack?: TypesSlackTrigger;
  teams?: TypesTeamsTrigger;
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
  TriggerTypeTeams = "teams",
  TriggerTypeCrisp = "crisp",
  TriggerTypeAzureDevOps = "azure_devops",
  TriggerTypeCron = "cron",
}

export interface TypesUnifiedSearchResponse {
  /** Echo back query */
  query?: string;
  results?: TypesUnifiedSearchResult[];
  /** Total results across all types */
  total?: number;
}

export interface TypesUnifiedSearchResult {
  /** ISO timestamp */
  created_at?: string;
  /** Brief description/content preview */
  description?: string;
  /** Icon hint for UI */
  icon?: string;
  /** Entity ID */
  id?: string;
  /** Additional context (status, owner, etc) */
  metadata?: Record<string, string>;
  /** Relevance score */
  score?: number;
  /** Display title */
  title?: string;
  /** "project", "task", "session", "prompt" */
  type?: string;
  /** ISO timestamp */
  updated_at?: string;
  /** Frontend URL to navigate to */
  url?: string;
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
  name?: string;
  vertex_credentials_file?: string;
  vertex_credentials_json?: string;
  /** Google Vertex AI fields */
  vertex_project_id?: string;
  vertex_region?: string;
}

export interface TypesUpdateSandboxRequest {
  name?: string;
  tags?: Record<string, string>;
  timeout_seconds?: number;
}

export interface TypesUpdateTeamRequest {
  name?: string;
}

export interface TypesUpdateUserColorSchemeRequest {
  /** "light", "dark", or "" (follow OS) */
  color_scheme?: string;
}

export interface TypesUpdateUserGuidelinesRequest {
  guidelines?: string;
}

export interface TypesUsage {
  completion_tokens?: number;
  /** How long the request took in milliseconds */
  duration_ms?: number;
  prompt_tokens?: number;
  total_tokens?: number;
}

export interface TypesUsageBreakdownRow {
  cache_read_cost?: number;
  cache_read_tokens?: number;
  cache_write_cost?: number;
  cache_write_tokens?: number;
  completion_cost?: number;
  completion_tokens?: number;
  email?: string;
  ended_at?: string;
  id?: string;
  interaction_id?: string;
  last_activity_at?: string;
  latency_ms?: number;
  model?: string;
  name?: string;
  prompt_cost?: number;
  prompt_tokens?: number;
  provider?: string;
  request_size_bytes?: number;
  response_size_bytes?: number;
  session_count?: number;
  session_id?: string;
  started_at?: string;
  total_cost?: number;
  total_requests?: number;
  total_tokens?: number;
  unique_apps?: number;
  unique_projects?: number;
  unique_sessions?: number;
  unique_users?: number;
  username?: string;
}

export interface TypesUsageFilterOption {
  email?: string;
  id?: string;
  model?: string;
  name?: string;
  provider?: string;
  username?: string;
}

export interface TypesUsageModelTimeSeries {
  id?: string;
  metrics?: TypesAggregatedUsageMetric[];
  model?: string;
  name?: string;
  provider?: string;
}

export interface TypesUser {
  /** if the ID of the user is contained in the env setting */
  admin?: boolean;
  /**
   * AlphaFeatures lists the feature flags this user has been granted
   * access to. Server-enforced via requireFeature middleware — the
   * frontend uses it only to decide whether to render the entry
   * point. Granted per-user via SQL (no deploy).
   */
  alpha_features?: string[];
  /** if the token is associated with an app */
  app_id?: string;
  auth_provider?: TypesAuthProvider;
  created_at?: string;
  deactivated?: boolean;
  deleted_at?: GormDeletedAt;
  email?: string;
  full_name?: string;
  id?: string;
  /**
   * LastSeenAt is the most recent time the user authenticated against the API.
   * Updated (throttled) from auth middleware so the column isn't hammered on every request.
   */
  last_seen_at?: string;
  /** if the user must change their password */
  must_change_password?: boolean;
  onboarding_completed?: boolean;
  onboarding_completed_at?: string;
  /** Organization this API key is scoped to (ephemeral keys) */
  organization_id?: string;
  /**
   * PendingAdminCreditsOnFirstOrg holds credits stashed by admin via the
   * /admin/users/{id}/credits endpoint when the user has no owned org yet.
   * Consumed by consumeUserAdminCredits on first owned org, then cleared.
   * Kept separate from TrialCreditsOnFirstOrg so admins can comp credits
   * without entangling the grant with trial-state UI or revocation flows.
   */
  pending_admin_credits_on_first_org?: number;
  /**
   * PlanOnFirstOrg, when set ("pro"), grants a paid plan override to the
   * user's first owned org's wallet on creation — admin "Activate" with a
   * paid (non-Stripe) plan for a user who has no org yet. Consumed alongside
   * the trial intent, then cleared.
   */
  plan_on_first_org?: string;
  /** When running in Helix Code sandbox */
  project_id?: string;
  sb?: boolean;
  /** Session this API key is scoped to (ephemeral keys) */
  session_id?: string;
  /** When running in Helix Code sandbox */
  spec_task_id?: string;
  /** the actual token used and its type */
  token?: string;
  /** none, runner. keycloak, api_key */
  token_type?: TypesTokenType;
  trial_credits_on_first_org?: number;
  /**
   * Trial intent stashed by admin before the user has created their first org.
   * Consumed by wallet creation on first owned org, then cleared.
   */
  trial_days_on_first_org?: number;
  trial_ends_at?: number;
  trial_org_id?: string;
  /**
   * Transient trial-display fields populated by the admin users list when
   * ?include=trial is set. Not persisted (gorm:"-") and not emitted unless
   * explicitly populated (json:"...,omitempty").
   */
  trial_status?: string;
  /**
   * these are set by the keycloak user based on the token
   * if it's an app token - the keycloak user is loaded from the owner of the app
   * if it's a runner token - these values will be empty
   */
  type?: TypesOwnerType;
  updated_at?: string;
  username?: string;
  waitlisted?: boolean;
}

export interface TypesUserAppAccessResponse {
  can_read?: boolean;
  can_write?: boolean;
  is_admin?: boolean;
}

export interface TypesUserChatSettings {
  frequency_penalty?: number;
  max_tokens?: number;
  presence_penalty?: number;
  system_prompt?: string;
  /**
   * SystemPromptEnabled toggles whether any system prompt at all is sent
   * to the model. Pointer so nil means "not set" and we fall back to the
   * default-on behaviour. When explicitly false, no system prompt is sent
   * regardless of SystemPrompt.
   */
  system_prompt_enabled?: boolean;
  temperature?: number;
  top_p?: number;
}

export interface TypesUserConfig {
  /**
   * ColorScheme is the user's preferred UI color scheme: "light" or "dark".
   * Empty string means follow OS preference. Propagated to the GNOME desktop
   * (gsettings color-scheme) and Zed editor inside spec-task sessions owned
   * by this user.
   */
  color_scheme?: string;
  pinned_project_ids?: string[];
  stripe_customer_id?: string;
  stripe_subscription_active?: boolean;
  stripe_subscription_id?: string;
}

export interface TypesUserGuidelinesResponse {
  guidelines?: string;
  guidelines_updated_at?: string;
  guidelines_updated_by?: string;
  guidelines_version?: number;
}

export interface TypesUserModelUsage {
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  completion_tokens?: number;
  first_used?: string;
  last_used?: string;
  model?: string;
  prompt_tokens?: number;
  provider?: string;
  total_cost?: number;
  total_requests?: number;
  total_tokens?: number;
}

export interface TypesUserResponse {
  admin?: boolean;
  alpha_features?: string[];
  email?: string;
  id?: string;
  name?: string;
  onboarding_completed?: boolean;
  token?: string;
  waitlisted?: boolean;
}

export interface TypesUserSearchResponse {
  limit?: number;
  offset?: number;
  total_count?: number;
  users?: TypesUser[];
}

export interface TypesUserStatsResponse {
  last_active_at?: string;
  models?: TypesUserModelUsage[];
  projects_count?: number;
  spec_tasks_count?: number;
  user?: TypesUser;
}

export interface TypesUserStatus {
  admin?: boolean;
  config?: TypesUserConfig;
  license?: TypesFrontendLicenseInfo;
  /** User slug for GitHub-style URLs */
  slug?: string;
  user?: string;
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

export interface TypesVCSActingUser {
  id?: string;
  name?: string;
}

export interface TypesVCSConnectionInfo {
  acting_user?: TypesVCSActingUser;
  missing_scopes?: string[];
  provider?: TypesExternalRepositoryType;
  pushing_as?: TypesVCSPushingAs;
  repos?: TypesVCSRepoAccess[];
  state?: TypesVCSConnectionState;
}

export enum TypesVCSConnectionState {
  VCSConnectionVerified = "verified",
  VCSConnectionNeedsAttention = "needs_attention",
  VCSConnectionDisconnected = "disconnected",
}

export interface TypesVCSPushingAs {
  /** OAuthConnection ID (for switch/disconnect) */
  connection_id?: string;
  /** e.g. "@tonychapman-prog" */
  username?: string;
}

export interface TypesVCSRepoAccess {
  /** true if the connection can reach it (or access is unverifiable for this provider) */
  has_access?: boolean;
  /** "owner/repo" */
  repo?: string;
  /** true if we actually probed the provider (false = optimistic/unverifiable) */
  verified?: boolean;
}

export interface TypesVHostRoute {
  created_at?: string;
  /** always lowercased */
  hostname?: string;
  id?: string;
  /**
   * IsDefault is true for project default subdomains (<slug>.<base>).
   * User-added custom domains and preview tokens are false.
   */
  is_default?: boolean;
  /** destination port inside the container */
  port?: number;
  rotated_at?: string;
  target_id?: string;
  target_kind?: TypesVHostTargetKind;
  /**
   * VerificationToken is only meaningful for custom domains awaiting
   * DNS-based verification. Null for default and preview rows.
   */
  verification_token?: string;
  /**
   * VerifiedAt is non-null once the route is usable. Auto-set for default
   * subdomains and preview tokens; set after DNS verification for custom
   * domains.
   */
  verified_at?: string;
}

export enum TypesVHostTargetKind {
  VHostTargetProjectWebService = "project_web_service",
  VHostTargetSandboxPreview = "sandbox_preview",
}

export interface TypesWIPLimits {
  implementation?: number;
  planning?: number;
  review?: number;
}

export interface TypesWallet {
  balance?: number;
  created_at?: string;
  id?: string;
  /** If belongs to an organization */
  org_id?: string;
  /**
   * PlanOverride, when set ("free"|"pro"), forces the quota tier for this
   * wallet regardless of the Stripe subscription — used to grant a paid plan
   * to a customer who paid out-of-band (bank transfer, no card / no Stripe).
   * "" means derive the tier from the Stripe subscription as usual. Stripe
   * sync only ever writes the Subscription* fields, never this one, so an
   * admin grant can't be reverted by a webhook.
   */
  plan_override?: string;
  stripe_customer_id?: string;
  stripe_subscription_id?: string;
  subscription_cancel_at_period_end?: boolean;
  subscription_created?: number;
  subscription_current_period_end?: number;
  subscription_current_period_start?: number;
  subscription_status?: StripeSubscriptionStatus;
  updated_at?: string;
  user_id?: string;
}

export interface TypesWebServiceDeploy {
  commit_sha?: string;
  error?: string;
  finished_at?: string;
  id?: string;
  log_path?: string;
  project_id?: string;
  sandbox_id?: string;
  started_at?: string;
  status?: TypesWebServiceDeployStatus;
}

export enum TypesWebServiceDeployStatus {
  WebServiceDeployStatusPending = "pending",
  WebServiceDeployStatusBuilding = "building",
  WebServiceDeployStatusLive = "live",
  WebServiceDeployStatusFailed = "failed",
  WebServiceDeployStatusSuperseded = "superseded",
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

export interface TypesZFSTree {
  available?: boolean;
  golden?: TypesZFSTreeNode;
  orphans?: TypesZFSTreeNode[];
  pool_root?: string;
}

export interface TypesZFSTreeNode {
  children?: TypesZFSTreeNode[];
  mounted?: boolean;
  name?: string;
  refer?: string;
  session_id?: string;
  type?: string;
  used?: string;
}

export interface TypesZedConfigResponse {
  agent?: Record<string, any>;
  assistant?: Record<string, any>;
  /** True if user has an active Claude subscription for credential sync */
  claude_subscription_available?: boolean;
  /** Code agent configuration for Zed agentic coding */
  code_agent_config?: TypesCodeAgentConfig;
  /** True if user has active ChatGPT credentials for Codex CLI */
  codex_subscription_available?: boolean;
  /** Session owner's UI color scheme: "light", "dark", or "" (follow OS). Daemon applies via gsettings to GNOME. */
  color_scheme?: string;
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
     * @description Retrieves debug data for agent sandboxes (Hydra-based) including GPU stats, dev containers, and connected clients
     *
     * @tags Admin
     * @name V1AdminAgentSandboxesDebugList
     * @summary Get sandbox debugging data
     * @request GET:/api/v1/admin/agent-sandboxes/debug
     * @secure
     */
    v1AdminAgentSandboxesDebugList: (
      query?: {
        /** Sandbox instance ID to query */
        sandbox_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerAgentSandboxesDebugResponse, SystemHTTPError>({
        path: `/api/v1/admin/agent-sandboxes/debug`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Streams Server-Sent Events for real-time sandbox monitoring
     *
     * @tags Admin
     * @name V1AdminAgentSandboxesEventsList
     * @summary Get sandbox real-time events (SSE)
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
     * @description List all pending tasks across all repositories with pagination. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditQueueList
     * @summary List Kodit task queue (admin)
     * @request GET:/api/v1/admin/kodit/queue
     * @secure
     */
    v1AdminKoditQueueList: (
      query?: {
        /** Page number (default 1) */
        page?: number;
        /** Items per page (default 25, max 100) */
        per_page?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditAdminQueueListResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/queue`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a specific task from the Kodit task queue by ID. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditQueueDelete
     * @summary Delete Kodit queue task (admin)
     * @request DELETE:/api/v1/admin/kodit/queue/{taskId}
     * @secure
     */
    v1AdminKoditQueueDelete: (taskId: number, params: RequestParams = {}) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/admin/kodit/queue/${taskId}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update the priority of a specific task in the Kodit task queue. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditQueuePriorityPartialUpdate
     * @summary Update Kodit queue task priority (admin)
     * @request PATCH:/api/v1/admin/kodit/queue/{taskId}/priority
     * @secure
     */
    v1AdminKoditQueuePriorityPartialUpdate: (
      taskId: number,
      body: ServerKoditAdminUpdatePriorityRequest,
      params: RequestParams = {},
    ) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/admin/kodit/queue/${taskId}/priority`,
        method: "PATCH",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List all Kodit-indexed repositories with pagination. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesList
     * @summary List Kodit repositories (admin)
     * @request GET:/api/v1/admin/kodit/repositories
     * @secure
     */
    v1AdminKoditRepositoriesList: (
      query?: {
        /** Page number (default 1) */
        page?: number;
        /** Items per page (default 25, max 100) */
        per_page?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditAdminRepoListResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Queue a Kodit repository for deletion. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesDelete
     * @summary Delete Kodit repository (admin)
     * @request DELETE:/api/v1/admin/kodit/repositories/{koditRepoId}
     * @secure
     */
    v1AdminKoditRepositoriesDelete: (koditRepoId: number, params: RequestParams = {}) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/${koditRepoId}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get detailed information about a Kodit repository including summary stats. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesDetail
     * @summary Get Kodit repository detail (admin)
     * @request GET:/api/v1/admin/kodit/repositories/{koditRepoId}
     * @secure
     */
    v1AdminKoditRepositoriesDetail: (koditRepoId: number, params: RequestParams = {}) =>
      this.request<ServerKoditAdminRepoDetailResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/${koditRepoId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Trigger a rescan of the HEAD commit for a Kodit repository. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesRescanCreate
     * @summary Rescan Kodit repository HEAD (admin)
     * @request POST:/api/v1/admin/kodit/repositories/{koditRepoId}/rescan
     * @secure
     */
    v1AdminKoditRepositoriesRescanCreate: (koditRepoId: number, params: RequestParams = {}) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/${koditRepoId}/rescan`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Trigger a full sync (git fetch + branch scan + re-index) for a Kodit repository. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesSyncCreate
     * @summary Sync Kodit repository (admin)
     * @request POST:/api/v1/admin/kodit/repositories/{koditRepoId}/sync
     * @secure
     */
    v1AdminKoditRepositoriesSyncCreate: (koditRepoId: number, params: RequestParams = {}) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/${koditRepoId}/sync`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns tracking statuses and pending queue tasks for a Kodit repository. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesTasksDetail
     * @summary Get Kodit repository tasks (admin)
     * @request GET:/api/v1/admin/kodit/repositories/{koditRepoId}/tasks
     * @secure
     */
    v1AdminKoditRepositoriesTasksDetail: (koditRepoId: number, params: RequestParams = {}) =>
      this.request<ServerKoditAdminRepositoryTasksResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/${koditRepoId}/tasks`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Queue multiple Kodit repositories for deletion. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesBatchDeleteCreate
     * @summary Batch delete Kodit repositories (admin)
     * @request POST:/api/v1/admin/kodit/repositories/batch/delete
     * @secure
     */
    v1AdminKoditRepositoriesBatchDeleteCreate: (body: ServerKoditAdminBatchRequest, params: RequestParams = {}) =>
      this.request<ServerKoditAdminBatchResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/batch/delete`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Trigger a HEAD commit rescan for multiple Kodit repositories. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditRepositoriesBatchRescanCreate
     * @summary Batch rescan Kodit repositories (admin)
     * @request POST:/api/v1/admin/kodit/repositories/batch/rescan
     * @secure
     */
    v1AdminKoditRepositoriesBatchRescanCreate: (body: ServerKoditAdminBatchRequest, params: RequestParams = {}) =>
      this.request<ServerKoditAdminBatchResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/repositories/batch/rescan`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns aggregate counts: repositories, enrichments, commits, pending tasks. Admin only.
     *
     * @tags admin
     * @name V1AdminKoditStatsList
     * @summary Get Kodit system stats (admin)
     * @request GET:/api/v1/admin/kodit/stats
     * @secure
     */
    v1AdminKoditStatsList: (params: RequestParams = {}) =>
      this.request<ServerKoditAdminStatsResponse, TypesAPIError>({
        path: `/api/v1/admin/kodit/stats`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List all organizations that have auto-join domains configured
     *
     * @tags organizations
     * @name V1AdminOrganizationDomainsList
     * @summary List organization domains (admin only)
     * @request GET:/api/v1/admin/organization-domains
     * @secure
     */
    v1AdminOrganizationDomainsList: (params: RequestParams = {}) =>
      this.request<ServerOrganizationDomainInfo[], any>({
        path: `/api/v1/admin/organization-domains`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description List all organizations
     *
     * @tags organizations
     * @name V1AdminOrgsList
     * @summary List organizations with wallets (admin only)
     * @request GET:/api/v1/admin/orgs
     * @secure
     */
    v1AdminOrgsList: (params: RequestParams = {}) =>
      this.request<TypesOrgDetails[], any>({
        path: `/api/v1/admin/orgs`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Force an org's quota tier independent of Stripe — for customers who paid out-of-band. plan: "pro" | "free" | "" (clear). Never reverted by a Stripe webhook.
     *
     * @tags organizations
     * @name V1AdminOrgsPlanCreate
     * @summary Set an organization's plan override (admin only)
     * @request POST:/api/v1/admin/orgs/{id}/plan
     * @secure
     */
    v1AdminOrgsPlanCreate: (id: string, request: ServerSetOrgPlanRequest, params: RequestParams = {}) =>
      this.request<TypesWallet, any>({
        path: `/api/v1/admin/orgs/${id}/plan`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Permanently delete a user and all associated data. Only admins can use this endpoint.
     *
     * @tags users
     * @name V1AdminUsersDelete
     * @summary Delete a user (Admin only)
     * @request DELETE:/api/v1/admin/users/{id}
     * @secure
     */
    v1AdminUsersDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/admin/users/${id}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Approve a waitlisted user, removing them from the waitlist. Only admins can use this endpoint.
     *
     * @tags users
     * @name V1AdminUsersApproveCreate
     * @summary Approve a user (Admin only)
     * @request POST:/api/v1/admin/users/{id}/approve
     * @secure
     */
    v1AdminUsersApproveCreate: (id: string, params: RequestParams = {}) =>
      this.request<TypesUser, SystemHTTPError>({
        path: `/api/v1/admin/users/${id}/approve`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Adds credits to the wallet of an explicitly chosen organisation the user owns, or stashes the grant on the user when they own no organisations yet (the grant is applied to their first owned org on creation). Works regardless of subscription state.
     *
     * @tags users
     * @name V1AdminUsersCreditsCreate
     * @summary Grant credits to a user (Admin, cloud only)
     * @request POST:/api/v1/admin/users/{id}/credits
     * @secure
     */
    v1AdminUsersCreditsCreate: (id: string, request: ServerGrantCreditsRequest, params: RequestParams = {}) =>
      this.request<ServerGrantCreditsResponse, any>({
        path: `/api/v1/admin/users/${id}/credits`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns the organisations the target user is the owner of, sorted by creation time ascending. Used by the admin "Grant credits" dialog to populate its org picker.
     *
     * @tags users
     * @name V1AdminUsersOwnedOrgsDetail
     * @summary List a user's owned organisations (Admin, cloud only)
     * @request GET:/api/v1/admin/users/{id}/owned-orgs
     * @secure
     */
    v1AdminUsersOwnedOrgsDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerOwnedOrgSummary[], any>({
        path: `/api/v1/admin/users/${id}/owned-orgs`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Reset the password for any user. Only admins can use this endpoint.
     *
     * @tags users
     * @name V1AdminUsersPasswordUpdate
     * @summary Reset a user's password (Admin only)
     * @request PUT:/api/v1/admin/users/{id}/password
     * @secure
     */
    v1AdminUsersPasswordUpdate: (id: string, request: TypesAdminResetPasswordRequest, params: RequestParams = {}) =>
      this.request<TypesUser, SystemHTTPError>({
        path: `/api/v1/admin/users/${id}/password`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Clears any stashed trial intent on the user and cancels the Stripe subscription on the user's oldest owned org if it is currently in a trialing state. Paid (active) subscriptions are never cancelled.
     *
     * @tags users
     * @name V1AdminUsersTrialActivateDelete
     * @summary Revoke an admin-granted trial (Admin, cloud only)
     * @request DELETE:/api/v1/admin/users/{id}/trial-activate
     * @secure
     */
    v1AdminUsersTrialActivateDelete: (id: string, params: RequestParams = {}) =>
      this.request<ServerActivateTrialResponse, any>({
        path: `/api/v1/admin/users/${id}/trial-activate`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Stash a trial intent on the user, or immediately create a Stripe trial subscription on the user's oldest-owned org. Days defaults to 90; credits are taken verbatim from the request (0 means no admin top-up beyond what Stripe's subscription invoice contributes).
     *
     * @tags users
     * @name V1AdminUsersTrialActivateCreate
     * @summary Activate a trial for a user (Admin, cloud only)
     * @request POST:/api/v1/admin/users/{id}/trial-activate
     * @secure
     */
    v1AdminUsersTrialActivateCreate: (id: string, request: ServerActivateTrialRequest, params: RequestParams = {}) =>
      this.request<ServerActivateTrialResponse, any>({
        path: `/api/v1/admin/users/${id}/trial-activate`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
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
     * @description Delete an evaluation run
     *
     * @tags evaluations
     * @name V1AppsEvaluationRunsDelete
     * @summary Delete an evaluation run
     * @request DELETE:/api/v1/apps/{app_id}/evaluation-runs/{run_id}
     * @secure
     */
    v1AppsEvaluationRunsDelete: (appId: string, runId: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-runs/${runId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get evaluation run details
     *
     * @tags evaluations
     * @name V1AppsEvaluationRunsDetail
     * @summary Get an evaluation run
     * @request GET:/api/v1/apps/{app_id}/evaluation-runs/{run_id}
     * @secure
     */
    v1AppsEvaluationRunsDetail: (appId: string, runId: string, params: RequestParams = {}) =>
      this.request<TypesEvaluationRun, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-runs/${runId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List all evaluation suites for an app
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesDetail
     * @summary List evaluation suites for an app
     * @request GET:/api/v1/apps/{app_id}/evaluation-suites
     * @secure
     */
    v1AppsEvaluationSuitesDetail: (appId: string, params: RequestParams = {}) =>
      this.request<TypesEvaluationSuite[], SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new evaluation suite for an agent
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesCreate
     * @summary Create an evaluation suite
     * @request POST:/api/v1/apps/{app_id}/evaluation-suites
     * @secure
     */
    v1AppsEvaluationSuitesCreate: (appId: string, suite: TypesEvaluationSuite, params: RequestParams = {}) =>
      this.request<TypesEvaluationSuite, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites`,
        method: "POST",
        body: suite,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete an evaluation suite
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesDelete
     * @summary Delete an evaluation suite
     * @request DELETE:/api/v1/apps/{app_id}/evaluation-suites/{id}
     * @secure
     */
    v1AppsEvaluationSuitesDelete: (appId: string, id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get an evaluation suite by ID
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesDetail2
     * @summary Get an evaluation suite
     * @request GET:/api/v1/apps/{app_id}/evaluation-suites/{id}
     * @originalName v1AppsEvaluationSuitesDetail
     * @duplicate
     * @secure
     */
    v1AppsEvaluationSuitesDetail2: (appId: string, id: string, params: RequestParams = {}) =>
      this.request<TypesEvaluationSuite, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update an evaluation suite
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesUpdate
     * @summary Update an evaluation suite
     * @request PUT:/api/v1/apps/{app_id}/evaluation-suites/{id}
     * @secure
     */
    v1AppsEvaluationSuitesUpdate: (
      appId: string,
      id: string,
      suite: TypesEvaluationSuite,
      params: RequestParams = {},
    ) =>
      this.request<TypesEvaluationSuite, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites/${id}`,
        method: "PUT",
        body: suite,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List evaluation runs for a suite
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesRunsDetail
     * @summary List evaluation runs
     * @request GET:/api/v1/apps/{app_id}/evaluation-suites/{id}/runs
     * @secure
     */
    v1AppsEvaluationSuitesRunsDetail: (appId: string, id: string, params: RequestParams = {}) =>
      this.request<TypesEvaluationRun[], SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites/${id}/runs`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Start running an evaluation suite against an agent
     *
     * @tags evaluations
     * @name V1AppsEvaluationSuitesRunsCreate
     * @summary Start an evaluation run
     * @request POST:/api/v1/apps/{app_id}/evaluation-suites/{id}/runs
     * @secure
     */
    v1AppsEvaluationSuitesRunsCreate: (appId: string, id: string, params: RequestParams = {}) =>
      this.request<TypesEvaluationRun, SystemHTTPError>({
        path: `/api/v1/apps/${appId}/evaluation-suites/${id}/runs`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
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
     * @description Enable a marketplace skill on an app. For autoProvision MCP skills the server generates URL and auth automatically.
     *
     * @tags skills
     * @name V1AppsSkillsEnableCreate
     * @summary Enable a marketplace skill on an app
     * @request POST:/api/v1/apps/{id}/skills/{skill}/enable
     * @secure
     */
    v1AppsSkillsEnableCreate: (id: string, skill: string, params: RequestParams = {}) =>
      this.request<TypesApp, any>({
        path: `/api/v1/apps/${id}/skills/${skill}/enable`,
        method: "POST",
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
     * @description Returns attention events that need human action for the current user. Only returns events that have not been dismissed and are not currently snoozed.
     *
     * @tags attention-events
     * @name V1AttentionEventsList
     * @summary List active attention events
     * @request GET:/api/v1/attention-events
     */
    v1AttentionEventsList: (
      query?: {
        /** Filter to active (non-dismissed, non-snoozed) events only (default: true) */
        active?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAttentionEvent[], string>({
        path: `/api/v1/attention-events`,
        method: "GET",
        query: query,
        format: "json",
        ...params,
      }),

    /**
     * @description Acknowledge, dismiss, or snooze an attention event.
     *
     * @tags attention-events
     * @name V1AttentionEventsPartialUpdate
     * @summary Update an attention event
     * @request PATCH:/api/v1/attention-events/{id}
     */
    v1AttentionEventsPartialUpdate: (
      id: string,
      request: TypesAttentionEventUpdateRequest,
      params: RequestParams = {},
    ) =>
      this.request<Record<string, string>, string>({
        path: `/api/v1/attention-events/${id}`,
        method: "PATCH",
        body: request,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Bulk-dismiss all active (non-dismissed) attention events for the current user.
     *
     * @tags attention-events
     * @name V1AttentionEventsDismissAllCreate
     * @summary Dismiss all active attention events
     * @request POST:/api/v1/attention-events/dismiss-all
     */
    v1AttentionEventsDismissAllCreate: (params: RequestParams = {}) =>
      this.request<Record<string, any>, string>({
        path: `/api/v1/attention-events/dismiss-all`,
        method: "POST",
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
     * @description Reset the password for a user
     *
     * @tags auth
     * @name V1AuthPasswordResetCreate
     * @summary Password Reset
     * @request POST:/api/v1/auth/password-reset
     */
    v1AuthPasswordResetCreate: (request: TypesPasswordResetRequest, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/password-reset`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Complete the password reset process using the access token from the reset email
     *
     * @tags auth
     * @name V1AuthPasswordResetCompleteCreate
     * @summary Complete Password Reset
     * @request POST:/api/v1/auth/password-reset-complete
     */
    v1AuthPasswordResetCompleteCreate: (request: TypesPasswordResetCompleteRequest, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/password-reset-complete`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Update the password for the authenticated user
     *
     * @tags auth
     * @name V1AuthPasswordUpdateCreate
     * @summary Update Password
     * @request POST:/api/v1/auth/password-update
     * @secure
     */
    v1AuthPasswordUpdateCreate: (request: TypesPasswordUpdateRequest, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/password-update`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
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
     * @description Register a new user
     *
     * @tags auth
     * @name V1AuthRegisterCreate
     * @summary Register
     * @request POST:/api/v1/auth/register
     */
    v1AuthRegisterCreate: (request: TypesRegisterRequest, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/auth/register`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Returns session info for BFF authentication. The frontend uses this to check if the user is logged in.
     *
     * @tags auth
     * @name V1AuthSessionList
     * @summary Get current session info
     * @request GET:/api/v1/auth/session
     */
    v1AuthSessionList: (params: RequestParams = {}) =>
      this.request<TypesSessionInfo, string>({
        path: `/api/v1/auth/session`,
        method: "GET",
        ...params,
      }),

    /**
     * @description Update the account information for the authenticated user
     *
     * @tags auth
     * @name V1AuthUpdateCreate
     * @summary Update Account
     * @request POST:/api/v1/auth/update
     * @secure
     */
    v1AuthUpdateCreate: (request: TypesAccountUpdateRequest, params: RequestParams = {}) =>
      this.request<TypesUserResponse, any>({
        path: `/api/v1/auth/update`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
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
     * @description Returns random uncompressible data for measuring available bandwidth. Used by adaptive bitrate algorithm to probe network throughput. Only requires authentication, not session ownership.
     *
     * @tags ExternalAgents
     * @name V1BandwidthProbeList
     * @summary Bandwidth probe for adaptive bitrate
     * @request GET:/api/v1/bandwidth-probe
     * @secure
     */
    v1BandwidthProbeList: (
      query?: {
        /** Size of data to return in bytes (default 524288 = 512KB, max 2MB) */
        size?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<File, SystemHTTPError>({
        path: `/api/v1/bandwidth-probe`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description List Claude subscriptions for the current user and their org
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsList
     * @summary List Claude subscriptions
     * @request GET:/api/v1/claude-subscriptions
     * @secure
     */
    v1ClaudeSubscriptionsList: (params: RequestParams = {}) =>
      this.request<TypesClaudeSubscription[], SystemHTTPError>({
        path: `/api/v1/claude-subscriptions`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Connect a Claude subscription by providing OAuth credentials
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsCreate
     * @summary Create a Claude subscription
     * @request POST:/api/v1/claude-subscriptions
     * @secure
     */
    v1ClaudeSubscriptionsCreate: (body: TypesCreateClaudeSubscriptionRequest, params: RequestParams = {}) =>
      this.request<TypesClaudeSubscription, SystemHTTPError>({
        path: `/api/v1/claude-subscriptions`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Disconnect a Claude subscription
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsDelete
     * @summary Delete a Claude subscription
     * @request DELETE:/api/v1/claude-subscriptions/{id}
     * @secure
     */
    v1ClaudeSubscriptionsDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/claude-subscriptions/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get details of a specific Claude subscription (no secrets)
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsDetail
     * @summary Get a Claude subscription
     * @request GET:/api/v1/claude-subscriptions/{id}
     * @secure
     */
    v1ClaudeSubscriptionsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesClaudeSubscription, SystemHTTPError>({
        path: `/api/v1/claude-subscriptions/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List Claude models available through Claude Code subscriptions
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsModelsList
     * @summary List available Claude models
     * @request GET:/api/v1/claude-subscriptions/models
     * @secure
     */
    v1ClaudeSubscriptionsModelsList: (params: RequestParams = {}) =>
      this.request<ServerClaudeModel[], any>({
        path: `/api/v1/claude-subscriptions/models`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Check if Claude credentials file has been written inside the desktop container
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsPollLoginDetail
     * @summary Poll for Claude login credentials
     * @request GET:/api/v1/claude-subscriptions/poll-login/{sessionId}
     * @secure
     */
    v1ClaudeSubscriptionsPollLoginDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<ServerClaudePollLoginResponse, SystemHTTPError>({
        path: `/api/v1/claude-subscriptions/poll-login/${sessionId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Launch a temporary desktop session for interactive Claude OAuth login
     *
     * @tags Claude
     * @name V1ClaudeSubscriptionsStartLoginCreate
     * @summary Start a Claude login session
     * @request POST:/api/v1/claude-subscriptions/start-login
     * @secure
     */
    v1ClaudeSubscriptionsStartLoginCreate: (params: RequestParams = {}) =>
      this.request<ServerClaudeLoginSessionResponse, SystemHTTPError>({
        path: `/api/v1/claude-subscriptions/start-login`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get status breakdown and progress of all cloned tasks
     *
     * @tags CloneGroups
     * @name V1CloneGroupsProgressDetail
     * @summary Get progress of all tasks in a clone group
     * @request GET:/api/v1/clone-groups/{groupId}/progress
     * @secure
     */
    v1CloneGroupsProgressDetail: (groupId: string, params: RequestParams = {}) =>
      this.request<TypesCloneGroupProgress, TypesAPIError>({
        path: `/api/v1/clone-groups/${groupId}/progress`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Codex
     * @name V1CodexSubscriptionsList
     * @summary List Codex subscriptions
     * @request GET:/api/v1/codex-subscriptions
     * @secure
     */
    v1CodexSubscriptionsList: (params: RequestParams = {}) =>
      this.request<TypesCodexSubscription[], any>({
        path: `/api/v1/codex-subscriptions`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Connect a ChatGPT subscription using Codex CLI credentials
     *
     * @tags Codex
     * @name V1CodexSubscriptionsCreate
     * @summary Create a Codex subscription
     * @request POST:/api/v1/codex-subscriptions
     * @secure
     */
    v1CodexSubscriptionsCreate: (body: TypesCreateCodexSubscriptionRequest, params: RequestParams = {}) =>
      this.request<TypesCodexSubscription, any>({
        path: `/api/v1/codex-subscriptions`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Codex
     * @name V1CodexSubscriptionsDelete
     * @summary Delete a Codex subscription
     * @request DELETE:/api/v1/codex-subscriptions/{id}
     * @secure
     */
    v1CodexSubscriptionsDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, any>({
        path: `/api/v1/codex-subscriptions/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Codex
     * @name V1CodexSubscriptionsDetail
     * @summary Get a Codex subscription
     * @request GET:/api/v1/codex-subscriptions/{id}
     * @secure
     */
    v1CodexSubscriptionsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesCodexSubscription, any>({
        path: `/api/v1/codex-subscriptions/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Return device authentication instructions or persist completed credentials
     *
     * @tags Codex
     * @name V1CodexSubscriptionsPollLoginDetail
     * @summary Poll Codex login
     * @request GET:/api/v1/codex-subscriptions/poll-login/{sessionId}
     * @secure
     */
    v1CodexSubscriptionsPollLoginDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<ServerCodexPollLoginResponse, any>({
        path: `/api/v1/codex-subscriptions/poll-login/${sessionId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Launch a temporary container and start Codex device authentication
     *
     * @tags Codex
     * @name V1CodexSubscriptionsStartLoginCreate
     * @summary Start a Codex login session
     * @request POST:/api/v1/codex-subscriptions/start-login
     * @secure
     */
    v1CodexSubscriptionsStartLoginCreate: (params: RequestParams = {}) =>
      this.request<ServerCodexLoginSessionResponse, any>({
        path: `/api/v1/codex-subscriptions/start-login`,
        method: "POST",
        secure: true,
        format: "json",
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
     * @description Fetch current clipboard content from remote desktop
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsClipboardDetail
     * @summary Get session clipboard content
     * @request GET:/api/v1/external-agents/{sessionID}/clipboard
     * @secure
     */
    v1ExternalAgentsClipboardDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<TypesClipboardData, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/clipboard`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Send clipboard content to remote desktop
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsClipboardCreate
     * @summary Set session clipboard content
     * @request POST:/api/v1/external-agents/{sessionID}/clipboard
     * @secure
     */
    v1ExternalAgentsClipboardCreate: (sessionId: string, clipboard: TypesClipboardData, params: RequestParams = {}) =>
      this.request<void, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/clipboard`,
        method: "POST",
        body: clipboard,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description This endpoint was used for session configuration but is now a no-op.
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsConfigurePendingSessionCreate
     * @summary Configure pending session (deprecated - no-op)
     * @request POST:/api/v1/external-agents/{sessionID}/configure-pending-session
     * @secure
     */
    v1ExternalAgentsConfigurePendingSessionCreate: (
      sessionId: string,
      request: ServerConfigurePendingSessionRequest,
      params: RequestParams = {},
    ) =>
      this.request<Record<string, string>, string>({
        path: `/api/v1/external-agents/${sessionId}/configure-pending-session`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns git diff information from the running desktop container. Shows changes between the current working directory and base branch, including uncommitted changes.
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsDiffDetail
     * @summary Get file diff from container
     * @request GET:/api/v1/external-agents/{sessionID}/diff
     * @secure
     */
    v1ExternalAgentsDiffDetail: (
      sessionId: string,
      query?: {
        /** Base branch to compare against (default: main) */
        base?: string;
        /** Include full diff content for each file (default: false) */
        include_content?: boolean;
        /** Filter to specific file path */
        path?: string;
        /** Name of the workspace/repo to diff (optional, defaults to first found) */
        workspace?: string;
        /** If true, diff the helix-specs branch uncommitted changes instead */
        helix_specs?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<object, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/diff`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Executes a command inside the sandbox container for benchmarking and debugging. Only specific safe commands are allowed (vkcube, glxgears, pkill).
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsExecCreate
     * @summary Execute command in sandbox
     * @request POST:/api/v1/external-agents/{sessionID}/exec
     * @secure
     */
    v1ExternalAgentsExecCreate: (sessionId: string, body: object, params: RequestParams = {}) =>
      this.request<object, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/exec`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Send keyboard and mouse input events to the remote desktop. Supports single events or batches.
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsInputCreate
     * @summary Send input events to sandbox
     * @request POST:/api/v1/external-agents/{sessionID}/input
     * @secure
     */
    v1ExternalAgentsInputCreate: (sessionId: string, input: object, params: RequestParams = {}) =>
      this.request<object, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/input`,
        method: "POST",
        body: input,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Upload a file to the sandbox incoming folder (~/work/incoming/). Files can be dragged and dropped onto the sandbox viewer to upload them.
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsUploadCreate
     * @summary Upload file to sandbox
     * @request POST:/api/v1/external-agents/{sessionID}/upload
     * @secure
     */
    v1ExternalAgentsUploadCreate: (
      sessionId: string,
      data: {
        /** File to upload */
        file: File;
      },
      query?: {
        /** Open file manager to show uploaded file (default: true) */
        open_file_manager?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSandboxFileUploadResponse, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/upload`,
        method: "POST",
        query: query,
        body: data,
        secure: true,
        type: ContentType.FormData,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns a list of git workspaces (repositories) in the container. Each workspace includes the repo name, path, current branch, and whether it has a helix-specs branch.
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsWorkspacesDetail
     * @summary Get workspaces from container
     * @request GET:/api/v1/external-agents/{sessionID}/workspaces
     * @secure
     */
    v1ExternalAgentsWorkspacesDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<object, SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/workspaces`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Provides a WebSocket connection for sending input events directly to the screenshot-server
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsWsInputDetail
     * @summary Direct WebSocket input for PipeWire/GNOME sessions
     * @request GET:/api/v1/external-agents/{sessionID}/ws/input
     * @secure
     */
    v1ExternalAgentsWsInputDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<any, void | SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/ws/input`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Provides a WebSocket connection for receiving H.264 video frames directly from the
     *
     * @tags ExternalAgents
     * @name V1ExternalAgentsWsStreamDetail
     * @summary Direct WebSocket video streaming for PipeWire/GNOME sessions
     * @request GET:/api/v1/external-agents/{sessionID}/ws/stream
     * @secure
     */
    v1ExternalAgentsWsStreamDetail: (sessionId: string, params: RequestParams = {}) =>
      this.request<any, void | SystemHTTPError>({
        path: `/api/v1/external-agents/${sessionId}/ws/stream`,
        method: "GET",
        secure: true,
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
     * @description List all PAT-based git provider connections for the current user
     *
     * @tags git-provider-connections
     * @name V1GitProviderConnectionsList
     * @summary List git provider connections
     * @request GET:/api/v1/git-provider-connections
     * @secure
     */
    v1GitProviderConnectionsList: (params: RequestParams = {}) =>
      this.request<TypesGitProviderConnection[], TypesAPIError>({
        path: `/api/v1/git-provider-connections`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new PAT-based git provider connection for the current user
     *
     * @tags git-provider-connections
     * @name V1GitProviderConnectionsCreate
     * @summary Create git provider connection
     * @request POST:/api/v1/git-provider-connections
     * @secure
     */
    v1GitProviderConnectionsCreate: (request: TypesGitProviderConnectionCreateRequest, params: RequestParams = {}) =>
      this.request<TypesGitProviderConnection, TypesAPIError>({
        path: `/api/v1/git-provider-connections`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a PAT-based git provider connection
     *
     * @tags git-provider-connections
     * @name V1GitProviderConnectionsDelete
     * @summary Delete git provider connection
     * @request DELETE:/api/v1/git-provider-connections/{id}
     * @secure
     */
    v1GitProviderConnectionsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, TypesAPIError>({
        path: `/api/v1/git-provider-connections/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description List repositories from a saved PAT-based git provider connection
     *
     * @tags git-provider-connections
     * @name V1GitProviderConnectionsRepositoriesDetail
     * @summary Browse repositories from saved connection
     * @request GET:/api/v1/git-provider-connections/{id}/repositories
     * @secure
     */
    v1GitProviderConnectionsRepositoriesDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesListOAuthRepositoriesResponse, TypesAPIError>({
        path: `/api/v1/git-provider-connections/${id}/repositories`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List repositories from a remote provider (GitHub, GitLab, Azure DevOps) using PAT credentials
     *
     * @tags git-repositories
     * @name V1GitBrowseRemoteCreate
     * @summary Browse remote repositories
     * @request POST:/api/v1/git/browse-remote
     * @secure
     */
    v1GitBrowseRemoteCreate: (request: TypesBrowseRemoteRepositoriesRequest, params: RequestParams = {}) =>
      this.request<TypesListOAuthRepositoriesResponse, TypesAPIError>({
        path: `/api/v1/git/browse-remote`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List all git repositories, optionally filtered by owner, type, and project
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
        /** Filter by organization ID */
        organization_id?: string;
        /** Filter by project ID */
        project_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesGitRepository[], TypesAPIError>({
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
    v1GitRepositoriesCreate: (repository: TypesGitRepositoryCreateRequest, params: RequestParams = {}) =>
      this.request<TypesGitRepository, TypesAPIError>({
        path: `/api/v1/git/repositories`,
        method: "POST",
        body: repository,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a git repository and its metadata
     *
     * @tags git-repositories
     * @name V1GitRepositoriesDelete
     * @summary Delete git repository
     * @request DELETE:/api/v1/git/repositories/{id}
     * @secure
     */
    v1GitRepositoriesDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}`,
        method: "DELETE",
        secure: true,
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
      this.request<TypesGitRepository, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update an existing git repository's metadata
     *
     * @tags git-repositories
     * @name V1GitRepositoriesUpdate
     * @summary Update git repository
     * @request PUT:/api/v1/git/repositories/{id}
     * @secure
     */
    v1GitRepositoriesUpdate: (id: string, repository: TypesGitRepositoryUpdateRequest, params: RequestParams = {}) =>
      this.request<TypesGitRepository, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}`,
        method: "PUT",
        body: repository,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List access grants for a git repository (repository owners can list access grants)
     *
     * @tags gitrepositories
     * @name V1GitRepositoriesAccessGrantsDetail
     * @summary List repository access grants
     * @request GET:/api/v1/git/repositories/{id}/access-grants
     * @secure
     */
    v1GitRepositoriesAccessGrantsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesAccessGrant[], any>({
        path: `/api/v1/git/repositories/${id}/access-grants`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Grant access to a repository to a user (repository owners can grant access)
     *
     * @tags gitrepositories
     * @name V1GitRepositoriesAccessGrantsCreate
     * @summary Grant access to a repository to a user
     * @request POST:/api/v1/git/repositories/{id}/access-grants
     * @secure
     */
    v1GitRepositoriesAccessGrantsCreate: (
      id: string,
      request: TypesCreateAccessGrantRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesAccessGrant, any>({
        path: `/api/v1/git/repositories/${id}/access-grants`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Get list of all branches in a repository
     *
     * @tags git-repositories
     * @name ListGitRepositoryBranches
     * @summary List repository branches
     * @request GET:/api/v1/git/repositories/{id}/branches
     * @secure
     */
    listGitRepositoryBranches: (id: string, params: RequestParams = {}) =>
      this.request<string[], TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/branches`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new branch in a repository
     *
     * @tags git-repositories
     * @name CreateGitRepositoryBranch
     * @summary Create branch
     * @request POST:/api/v1/git/repositories/{id}/branches
     * @secure
     */
    createGitRepositoryBranch: (id: string, request: TypesCreateBranchRequest, params: RequestParams = {}) =>
      this.request<TypesCreateBranchResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/branches`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
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
     * @description List all commits in a repository
     *
     * @tags git-repositories
     * @name ListGitRepositoryCommits
     * @summary List repository commits
     * @request GET:/api/v1/git/repositories/{id}/commits
     * @secure
     */
    listGitRepositoryCommits: (
      id: string,
      query?: {
        /** Branch name (defaults to repository default branch) */
        branch?: string;
        /** Filter commits since this date (RFC3339 format) */
        since?: string;
        /** Filter commits until this date (RFC3339 format) */
        until?: string;
        /** Number of commits per page (default: 30) */
        per_page?: number;
        /** Page number (default: 1) */
        page?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesListCommitsResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/commits`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the contents of a file at a specific path in a repository
     *
     * @tags git-repositories
     * @name GetGitRepositoryFile
     * @summary Get file contents
     * @request GET:/api/v1/git/repositories/{id}/contents
     * @secure
     */
    getGitRepositoryFile: (
      id: string,
      query: {
        /** File path */
        path: string;
        /** Branch name (defaults to HEAD if not specified) */
        branch?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesGitRepositoryFileResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/contents`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create or update the contents of a file in a repository
     *
     * @tags git-repositories
     * @name CreateOrUpdateGitRepositoryFileContents
     * @summary Create or update file contents
     * @request PUT:/api/v1/git/repositories/{id}/contents
     * @secure
     */
    createOrUpdateGitRepositoryFileContents: (
      id: string,
      request: TypesUpdateGitRepositoryFileContentsRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesGitRepositoryFileResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/contents`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get code intelligence enrichments for a repository from Kodit
     *
     * @tags git-repositories
     * @name V1GitRepositoriesEnrichmentsDetail
     * @summary Get repository enrichments
     * @request GET:/api/v1/git/repositories/{id}/enrichments
     * @secure
     */
    v1GitRepositoriesEnrichmentsDetail: (
      id: string,
      query?: {
        /** Filter by enrichment type (usage, developer, living_documentation) */
        enrichment_type?: string;
        /** Filter by commit SHA */
        commit_sha?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditEnrichmentListResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/enrichments`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get a specific code intelligence enrichment by ID from Kodit
     *
     * @tags git-repositories
     * @name V1GitRepositoriesEnrichmentsDetail2
     * @summary Get enrichment by ID
     * @request GET:/api/v1/git/repositories/{id}/enrichments/{enrichmentId}
     * @originalName v1GitRepositoriesEnrichmentsDetail
     * @duplicate
     * @secure
     */
    v1GitRepositoriesEnrichmentsDetail2: (id: string, enrichmentId: string, params: RequestParams = {}) =>
      this.request<ServerKoditEnrichmentDTO, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/enrichments/${enrichmentId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Read the content of a file from the repository, optionally with line range filtering
     *
     * @tags git-repositories
     * @name V1GitRepositoriesFileContentDetail
     * @summary Read repository file
     * @request GET:/api/v1/git/repositories/{id}/file-content
     * @secure
     */
    v1GitRepositoriesFileContentDetail: (
      id: string,
      query: {
        /** File path within the repository */
        path: string;
        /** Start line (1-indexed, inclusive) */
        start_line?: number;
        /** End line (1-indexed, inclusive) */
        end_line?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditFileContentResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/file-content`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List files in a repository matching a glob pattern
     *
     * @tags git-repositories
     * @name V1GitRepositoriesFilesDetail
     * @summary List repository files
     * @request GET:/api/v1/git/repositories/{id}/files
     * @secure
     */
    v1GitRepositoriesFilesDetail: (
      id: string,
      query?: {
        /** Glob pattern to filter files */
        pattern?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditFilesResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/files`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Run pattern matching (grep) against repository files
     *
     * @tags git-repositories
     * @name V1GitRepositoriesGrepDetail
     * @summary Grep repository
     * @request GET:/api/v1/git/repositories/{id}/grep
     * @secure
     */
    v1GitRepositoriesGrepDetail: (
      id: string,
      query: {
        /** Search pattern (regex) */
        pattern: string;
        /** Glob pattern to filter files (e.g. *.go) */
        glob?: string;
        /** Maximum results (default 50, max 200) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditGrepResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/grep`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Search repository code using BM25 keyword matching
     *
     * @tags git-repositories
     * @name V1GitRepositoriesKeywordSearchDetail
     * @summary Keyword search repository
     * @request GET:/api/v1/git/repositories/{id}/keyword-search
     * @secure
     */
    v1GitRepositoriesKeywordSearchDetail: (
      id: string,
      query: {
        /** Keywords to search for */
        keywords: string;
        /** Maximum results (default 10, max 100) */
        limit?: number;
        /** Filter by language extension (e.g. .go, .ts) */
        language?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditSearchResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/keyword-search`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get commits for a repository from Kodit (used for enrichment filtering)
     *
     * @tags git-repositories
     * @name V1GitRepositoriesKoditCommitsDetail
     * @summary Get repository commits from Kodit
     * @request GET:/api/v1/git/repositories/{id}/kodit-commits
     * @secure
     */
    v1GitRepositoriesKoditCommitsDetail: (
      id: string,
      query?: {
        /** Limit number of commits (default 100) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditCommitDTO[], TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/kodit-commits`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Trigger a rescan of a specific commit in Kodit to refresh code intelligence
     *
     * @tags git-repositories
     * @name V1GitRepositoriesKoditRescanCreate
     * @summary Rescan repository commit
     * @request POST:/api/v1/git/repositories/{id}/kodit-rescan
     * @secure
     */
    v1GitRepositoriesKoditRescanCreate: (
      id: string,
      query: {
        /** Commit SHA to rescan */
        commit_sha: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<Record<string, string>, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/kodit-rescan`,
        method: "POST",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get indexing status for a repository from Kodit
     *
     * @tags git-repositories
     * @name V1GitRepositoriesKoditStatusDetail
     * @summary Get repository indexing status
     * @request GET:/api/v1/git/repositories/{id}/kodit-status
     * @secure
     */
    v1GitRepositoriesKoditStatusDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerKoditIndexingStatusDTO, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/kodit-status`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Rasterizes a document page (PDF, etc.) and returns it as a PNG image
     *
     * @tags git-repositories
     * @name V1GitRepositoriesPageImageDetail
     * @summary Render document page image
     * @request GET:/api/v1/git/repositories/{id}/page-image
     * @secure
     */
    v1GitRepositoriesPageImageDetail: (
      id: string,
      query: {
        /** File path within the repository */
        path: string;
        /** 1-based page number */
        page: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<File, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/page-image`,
        method: "GET",
        query: query,
        secure: true,
        format: "blob",
        ...params,
      }),

    /**
     * @description Pulls latest commits from remote repository
     *
     * @tags git-repositories
     * @name PullFromRemote
     * @summary Pull from remote repository
     * @request POST:/api/v1/git/repositories/{id}/pull
     * @secure
     */
    pullFromRemote: (
      id: string,
      query: {
        /** Force pull (default: false) */
        force: boolean;
        /** Branch name (required) */
        branch?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPullResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/pull`,
        method: "POST",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List all pull requests in a repository
     *
     * @tags git-repositories
     * @name ListGitRepositoryPullRequests
     * @summary List pull requests
     * @request GET:/api/v1/git/repositories/{id}/pull-requests
     * @secure
     */
    listGitRepositoryPullRequests: (id: string, params: RequestParams = {}) =>
      this.request<TypesPullRequest[], TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/pull-requests`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new pull request in a repository. Changes must be committed and pushed to the branch first.
     *
     * @tags git-repositories
     * @name CreateGitRepositoryPullRequest
     * @summary Create pull request
     * @request POST:/api/v1/git/repositories/{id}/pull-requests
     * @secure
     */
    createGitRepositoryPullRequest: (id: string, request: TypesCreatePullRequestRequest, params: RequestParams = {}) =>
      this.request<TypesCreatePullRequestResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/pull-requests`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Pushes the local branch to the remote repository
     *
     * @tags git-repositories
     * @name PushToRemote
     * @summary Push to remote repository
     * @request POST:/api/v1/git/repositories/{id}/push
     * @secure
     */
    pushToRemote: (
      id: string,
      query: {
        /** Branch name */
        branch: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPushResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/push`,
        method: "POST",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Pulls latest commits from remote and pushes local commits. Automatically merges if needed.
     *
     * @tags git-repositories
     * @name PushPullGitRepository
     * @summary Push and pull repository
     * @request POST:/api/v1/git/repositories/{id}/push-pull
     * @secure
     */
    pushPullGitRepository: (
      id: string,
      query?: {
        /** Branch name (defaults to repository default branch) */
        branch?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerPushPullResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/push-pull`,
        method: "POST",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Search for code snippets in a repository from Kodit
     *
     * @tags git-repositories
     * @name V1GitRepositoriesSearchSnippetsDetail
     * @summary Search repository snippets
     * @request GET:/api/v1/git/repositories/{id}/search-snippets
     * @secure
     */
    v1GitRepositoriesSearchSnippetsDetail: (
      id: string,
      query: {
        /** Search query */
        query: string;
        /** Limit number of results (default 20) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditSearchResultDTO[], TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/search-snippets`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Search repository code using vector similarity (semantic meaning)
     *
     * @tags git-repositories
     * @name V1GitRepositoriesSemanticSearchDetail
     * @summary Semantic search repository
     * @request GET:/api/v1/git/repositories/{id}/semantic-search
     * @secure
     */
    v1GitRepositoriesSemanticSearchDetail: (
      id: string,
      query: {
        /** Natural language search query */
        query: string;
        /** Maximum results (default 10, max 100) */
        limit?: number;
        /** Filter by language extension (e.g. .go, .ts) */
        language?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditSearchResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/semantic-search`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Syncs all branches from the upstream remote repository to the local repository
     *
     * @tags git-repositories
     * @name SyncAllBranches
     * @summary Sync all branches from upstream
     * @request POST:/api/v1/git/repositories/{id}/sync-all
     * @secure
     */
    syncAllBranches: (
      id: string,
      query?: {
        /** Force sync (default: false) */
        force?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSyncAllResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/sync-all`,
        method: "POST",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get list of files and directories at a specific path in a repository
     *
     * @tags git-repositories
     * @name BrowseGitRepositoryTree
     * @summary Browse repository tree
     * @request GET:/api/v1/git/repositories/{id}/tree
     * @secure
     */
    browseGitRepositoryTree: (
      id: string,
      query?: {
        /** Path to browse (default: root) */
        path?: string;
        /** Branch to browse (default: HEAD) */
        branch?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesGitRepositoryTreeResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/tree`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Search document pages (PDFs, etc.) using cross-modal visual similarity
     *
     * @tags git-repositories
     * @name V1GitRepositoriesVisualSearchDetail
     * @summary Visual search repository
     * @request GET:/api/v1/git/repositories/{id}/visual-search
     * @secure
     */
    v1GitRepositoriesVisualSearchDetail: (
      id: string,
      query: {
        /** Natural language search query */
        query: string;
        /** Maximum results (default 10, max 100) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditSearchResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/visual-search`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the wiki navigation tree (titles and paths, no content) for a repository. Each node includes a link to fetch the full page content.
     *
     * @tags git-repositories
     * @name V1GitRepositoriesWikiDetail
     * @summary Get repository wiki tree
     * @request GET:/api/v1/git/repositories/{id}/wiki
     * @secure
     */
    v1GitRepositoriesWikiDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerKoditWikiTreeResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/wiki`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get a wiki page by hierarchical path as markdown content
     *
     * @tags git-repositories
     * @name V1GitRepositoriesWikiPageDetail
     * @summary Get wiki page
     * @request GET:/api/v1/git/repositories/{id}/wiki-page
     * @secure
     */
    v1GitRepositoriesWikiPageDetail: (
      id: string,
      query: {
        /** Wiki page path (e.g. architecture/database-layer.md) */
        path: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditWikiPageResponse, TypesAPIError>({
        path: `/api/v1/git/repositories/${id}/wiki-page`,
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
     * @description Unauthenticated. Returns the invited email and organization display name so the registration page can pre-fill the form. The invitation ID itself acts as the secret token (same threat model as password-reset tokens).
     *
     * @tags organizations
     * @name V1InvitationsInfoDetail
     * @summary Look up basic info for an invitation by id
     * @request GET:/api/v1/invitations/{id}/info
     */
    v1InvitationsInfoDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesPublicInvitationInfo, any>({
        path: `/api/v1/invitations/${id}/info`,
        method: "GET",
        ...params,
      }),

    /**
     * No description
     *
     * @name V1KnowledgeList
     * @request GET:/api/v1/knowledge
     * @secure
     */
    v1KnowledgeList: (
      query?: {
        /** Organization ID or name. When set, lists org-owned knowledge instead of personal knowledge. */
        organization_id?: string;
        /** Filter by app ID */
        app_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesKnowledge[], any>({
        path: `/api/v1/knowledge`,
        method: "GET",
        query: query,
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
     * @description Fetch code intelligence enrichments for any Kodit repository (git or knowledge-backed).
     *
     * @tags kodit
     * @name V1KoditRepositoriesEnrichmentsDetail
     * @summary Get enrichments by Kodit repo ID
     * @request GET:/api/v1/kodit/repositories/{koditRepoId}/enrichments
     * @secure
     */
    v1KoditRepositoriesEnrichmentsDetail: (
      koditRepoId: number,
      query?: {
        /** Filter by enrichment type */
        enrichment_type?: string;
        /** Filter by commit SHA */
        commit_sha?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerKoditRepoEnrichmentsResponse, TypesAPIError>({
        path: `/api/v1/kodit/repositories/${koditRepoId}/enrichments`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
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
     * @description List repositories accessible via an OAuth connection (GitHub repos, GitLab projects, etc.)
     *
     * @tags oauth
     * @name V1OauthConnectionsRepositoriesDetail
     * @summary List repositories from an OAuth connection
     * @request GET:/api/v1/oauth/connections/{id}/repositories
     * @secure
     */
    v1OauthConnectionsRepositoriesDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesListOAuthRepositoriesResponse, any>({
        path: `/api/v1/oauth/connections/${id}/repositories`,
        method: "GET",
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
     * @description Resolve a SharePoint site URL to its site ID using Microsoft Graph API
     *
     * @tags oauth
     * @name V1OauthSharepointResolveSiteCreate
     * @summary Resolve SharePoint site URL to site ID
     * @request POST:/api/v1/oauth/sharepoint/resolve-site
     * @secure
     */
    v1OauthSharepointResolveSiteCreate: (request: ServerSharePointSiteResolveRequest, params: RequestParams = {}) =>
      this.request<ServerSharePointSiteResolveResponse, any>({
        path: `/api/v1/oauth/sharepoint/resolve-site`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
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
     * @description List API keys for an organization. Owners see all keys, members see only their own.
     *
     * @tags organizations
     * @name V1OrganizationsApiKeysDetail
     * @summary List organization API keys
     * @request GET:/api/v1/organizations/{id}/api_keys
     * @secure
     */
    v1OrganizationsApiKeysDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesApiKey[], any>({
        path: `/api/v1/organizations/${id}/api_keys`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new API key scoped to the organization. Any member can create keys.
     *
     * @tags organizations
     * @name V1OrganizationsApiKeysCreate
     * @summary Create an organization API key
     * @request POST:/api/v1/organizations/{id}/api_keys
     * @secure
     */
    v1OrganizationsApiKeysCreate: (id: string, request: object, params: RequestParams = {}) =>
      this.request<TypesApiKey, any>({
        path: `/api/v1/organizations/${id}/api_keys`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Delete an API key. Owners can delete any org key, members only their own.
     *
     * @tags organizations
     * @name V1OrganizationsApiKeysDelete
     * @summary Delete an organization API key
     * @request DELETE:/api/v1/organizations/{id}/api_keys/{key}
     * @secure
     */
    v1OrganizationsApiKeysDelete: (id: string, key: string, params: RequestParams = {}) =>
      this.request<string, any>({
        path: `/api/v1/organizations/${id}/api_keys/${key}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get the version history of guidelines for an organization
     *
     * @tags Organizations
     * @name V1OrganizationsGuidelinesHistoryDetail
     * @summary Get organization guidelines history
     * @request GET:/api/v1/organizations/{id}/guidelines-history
     * @secure
     */
    v1OrganizationsGuidelinesHistoryDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesGuidelinesHistory[], SystemHTTPError>({
        path: `/api/v1/organizations/${id}/guidelines-history`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List pending invitations for users who haven't joined the org yet. Use the optional `app_id` query parameter to filter to invitations sent from a specific project/app's access management dialog.
     *
     * @tags organizations
     * @name V1OrganizationsInvitationsDetail
     * @summary List pending organization invitations
     * @request GET:/api/v1/organizations/{id}/invitations
     * @secure
     */
    v1OrganizationsInvitationsDetail: (
      id: string,
      query?: {
        /** Filter invitations by the app/project they were sent from */
        app_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesOrganizationInvitation[], any>({
        path: `/api/v1/organizations/${id}/invitations`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Create a pending invitation for a non-Helix user. When they register with this email they will automatically join the organization.
     *
     * @tags organizations
     * @name V1OrganizationsInvitationsCreate
     * @summary Invite a user to the organization by email
     * @request POST:/api/v1/organizations/{id}/invitations
     * @secure
     */
    v1OrganizationsInvitationsCreate: (
      id: string,
      request: TypesAddOrganizationMemberRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesOrganizationInvitation, any>({
        path: `/api/v1/organizations/${id}/invitations`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Revoke a pending invitation by ID
     *
     * @tags organizations
     * @name V1OrganizationsInvitationsDelete
     * @summary Revoke a pending organization invitation
     * @request DELETE:/api/v1/organizations/{id}/invitations/{invitation_id}
     * @secure
     */
    v1OrganizationsInvitationsDelete: (id: string, invitationId: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/organizations/${id}/invitations/${invitationId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description List members of an organization, including pending invitations as placeholder rows (user_id starts with "inv_").
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
     * @description Add a member to an organization. When the user_reference is an email that doesn't match any existing user, a pending invitation is created instead (and an invitation email sent if email is configured).
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
      this.request<TypesAddOrganizationMemberResponse, any>({
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
     * @description Returns whether a user account exists for the given email, and whether they are already a member of this organization. Used by the invite UI to choose between "send invitation", "add to org", or "add to project" CTAs without revealing arbitrary user information.
     *
     * @tags organizations
     * @name V1OrganizationsUsersLookupDetail
     * @summary Look up a user by email within the context of an organization
     * @request GET:/api/v1/organizations/{id}/users/lookup
     * @secure
     */
    v1OrganizationsUsersLookupDetail: (
      id: string,
      query: {
        /** Email to look up */
        email: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesOrgUserLookupResponse, any>({
        path: `/api/v1/organizations/${id}/users/lookup`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description List sandboxes belonging to an organization
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesDetail
     * @summary List sandboxes
     * @request GET:/api/v1/organizations/{org_id}/sandboxes
     * @secure
     */
    v1OrganizationsSandboxesDetail: (orgId: string, params: RequestParams = {}) =>
      this.request<TypesSandboxListResponse, SystemHTTPError>({
        path: `/api/v1/organizations/${orgId}/sandboxes`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new sandbox in an organization
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesCreate
     * @summary Create sandbox
     * @request POST:/api/v1/organizations/{org_id}/sandboxes
     * @secure
     */
    v1OrganizationsSandboxesCreate: (orgId: string, payload: TypesCreateSandboxRequest, params: RequestParams = {}) =>
      this.request<TypesSandbox, SystemHTTPError>({
        path: `/api/v1/organizations/${orgId}/sandboxes`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesDelete
     * @summary Delete sandbox
     * @request DELETE:/api/v1/organizations/{org_id}/sandboxes/{id}
     * @secure
     */
    v1OrganizationsSandboxesDelete: (orgId: string, id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesDetail2
     * @summary Get sandbox
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}
     * @originalName v1OrganizationsSandboxesDetail
     * @duplicate
     * @secure
     */
    v1OrganizationsSandboxesDetail2: (orgId: string, id: string, params: RequestParams = {}) =>
      this.request<TypesSandbox, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesPartialUpdate
     * @summary Update sandbox
     * @request PATCH:/api/v1/organizations/{org_id}/sandboxes/{id}
     * @secure
     */
    v1OrganizationsSandboxesPartialUpdate: (
      orgId: string,
      id: string,
      payload: TypesUpdateSandboxRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSandbox, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}`,
        method: "PATCH",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesBillingDetail
     * @summary Sandbox billing summary
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/billing
     * @secure
     */
    v1OrganizationsSandboxesBillingDetail: (orgId: string, id: string, params: RequestParams = {}) =>
      this.request<ServerSandboxBillingResponse, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/billing`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesCommandsDetail
     * @summary List sandbox commands
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/commands
     * @secure
     */
    v1OrganizationsSandboxesCommandsDetail: (orgId: string, id: string, params: RequestParams = {}) =>
      this.request<HydraListSandboxCommandsResponse, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/commands`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesCommandsCreate
     * @summary Run a command in a sandbox
     * @request POST:/api/v1/organizations/{org_id}/sandboxes/{id}/commands
     * @secure
     */
    v1OrganizationsSandboxesCommandsCreate: (
      orgId: string,
      id: string,
      payload: TypesRunSandboxCommandRequest,
      params: RequestParams = {},
    ) =>
      this.request<HydraSandboxCommandResponse, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/commands`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesCommandsDetail2
     * @summary Get a sandbox command
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}
     * @originalName v1OrganizationsSandboxesCommandsDetail
     * @duplicate
     * @secure
     */
    v1OrganizationsSandboxesCommandsDetail2: (orgId: string, id: string, cmdId: string, params: RequestParams = {}) =>
      this.request<HydraSandboxCommandResponse, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/commands/${cmdId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesCommandsKillCreate
     * @summary Kill a sandbox command
     * @request POST:/api/v1/organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}/kill
     * @secure
     */
    v1OrganizationsSandboxesCommandsKillCreate: (
      orgId: string,
      id: string,
      cmdId: string,
      query?: {
        /** Signal name (default TERM) */
        signal?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<void, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/commands/${cmdId}/kill`,
        method: "POST",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesCommandsLogsDetail
     * @summary Stream sandbox command logs
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}/logs
     * @secure
     */
    v1OrganizationsSandboxesCommandsLogsDetail: (
      orgId: string,
      id: string,
      cmdId: string,
      query?: {
        /** stdout|stderr|both */
        stream?: string;
        /** 1 to follow */
        follow?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<any, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/commands/${cmdId}/logs`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesFilesDelete
     * @summary Read/write/delete sandbox file
     * @request DELETE:/api/v1/organizations/{org_id}/sandboxes/{id}/files
     * @secure
     */
    v1OrganizationsSandboxesFilesDelete: (
      orgId: string,
      id: string,
      query: {
        /** Absolute path inside the sandbox */
        path: string;
        /** Octal permission for write */
        mode?: string;
        /** 1 to delete recursively */
        recursive?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<any, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/files`,
        method: "DELETE",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesFilesDetail
     * @summary Read/write/delete sandbox file
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/files
     * @secure
     */
    v1OrganizationsSandboxesFilesDetail: (
      orgId: string,
      id: string,
      query: {
        /** Absolute path inside the sandbox */
        path: string;
        /** Octal permission for write */
        mode?: string;
        /** 1 to delete recursively */
        recursive?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<any, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/files`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesFilesUpdate
     * @summary Read/write/delete sandbox file
     * @request PUT:/api/v1/organizations/{org_id}/sandboxes/{id}/files
     * @secure
     */
    v1OrganizationsSandboxesFilesUpdate: (
      orgId: string,
      id: string,
      query: {
        /** Absolute path inside the sandbox */
        path: string;
        /** Octal permission for write */
        mode?: string;
        /** 1 to delete recursively */
        recursive?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<any, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/files`,
        method: "PUT",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesFilesListDetail
     * @summary List directory in sandbox
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/files/list
     * @secure
     */
    v1OrganizationsSandboxesFilesListDetail: (
      orgId: string,
      id: string,
      query?: {
        /** Directory path (default /root) */
        path?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<HydraListSandboxFilesResponse, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/files/list`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesScreenshotDetail
     * @summary Get a sandbox screenshot
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/screenshot
     * @secure
     */
    v1OrganizationsSandboxesScreenshotDetail: (
      orgId: string,
      id: string,
      query?: {
        /** JPEG quality (1-100, default 60) */
        quality?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<string, string>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/screenshot`,
        method: "GET",
        query: query,
        secure: true,
        format: "blob",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesTerminalDetail
     * @summary Sandbox terminal websocket
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/terminal
     * @secure
     */
    v1OrganizationsSandboxesTerminalDetail: (orgId: string, id: string, params: RequestParams = {}) =>
      this.request<any, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/terminal`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sandboxes
     * @name V1OrganizationsSandboxesTerminalSessionsDetail
     * @summary List sandbox tmux sessions
     * @request GET:/api/v1/organizations/{org_id}/sandboxes/{id}/terminal/sessions
     * @secure
     */
    v1OrganizationsSandboxesTerminalSessionsDetail: (orgId: string, id: string, params: RequestParams = {}) =>
      this.request<ServerSandboxTerminalSessionsResponse, any>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${id}/terminal/sessions`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsDetail
     * @summary Helix-org: list bots
     * @request GET:/api/v1/orgs/{org}/bots
     * @secure
     */
    v1OrgsBotsDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiBotDTO[], any>({
        path: `/api/v1/orgs/${org}/bots`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a Bot. Wraps the lifecycle Create so REST + chat creates share semantics (base-tool union, reporting line, transcript topics, create dispatch).
     *
     * @tags HelixOrg
     * @name V1OrgsBotsCreate
     * @summary Helix-org: create a bot
     * @request POST:/api/v1/orgs/{org}/bots
     * @secure
     */
    v1OrgsBotsCreate: (org: string, payload: ApiCreateBotRequest, params: RequestParams = {}) =>
      this.request<ApiCreateBotResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a Bot. Cascades: stops sessions, deletes the Helix project + agent app, clears runtime state, drops subscriptions + reporting lines, then the bot row. Activations are preserved as audit.
     *
     * @tags HelixOrg
     * @name V1OrgsBotsDelete
     * @summary Helix-org: delete a bot
     * @request DELETE:/api/v1/orgs/{org}/bots/{id}
     * @secure
     */
    v1OrgsBotsDelete: (id: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsDetail2
     * @summary Helix-org: get bot detail
     * @request GET:/api/v1/orgs/{org}/bots/{id}
     * @originalName v1OrgsBotsDetail
     * @duplicate
     * @secure
     */
    v1OrgsBotsDetail2: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiBotDetailDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsPartialUpdate
     * @summary Helix-org: update a bot
     * @request PATCH:/api/v1/orgs/{org}/bots/{id}
     * @secure
     */
    v1OrgsBotsPartialUpdate: (org: string, id: string, payload: ApiUpdateBotRequest, params: RequestParams = {}) =>
      this.request<ApiBotDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}`,
        method: "PATCH",
        body: payload,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsActivateCreate
     * @summary Helix-org: manually trigger a bot activation
     * @request POST:/api/v1/orgs/{org}/bots/{id}/activate
     * @secure
     */
    v1OrgsBotsActivateCreate: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiBotActivateDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/activate`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsChatCreate
     * @summary Helix-org: provision a per-bot chat app
     * @request POST:/api/v1/orgs/{org}/bots/{id}/chat
     * @secure
     */
    v1OrgsBotsChatCreate: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiBotChatDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/chat`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsParentsCreate
     * @summary Helix-org: add a bot reporting line (manager)
     * @request POST:/api/v1/orgs/{org}/bots/{id}/parents
     * @secure
     */
    v1OrgsBotsParentsCreate: (id: string, org: string, payload: ApiAddBotParentRequest, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/parents`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsParentsDelete
     * @summary Helix-org: remove a bot reporting line (manager)
     * @request DELETE:/api/v1/orgs/{org}/bots/{id}/parents/{parent_id}
     * @secure
     */
    v1OrgsBotsParentsDelete: (id: string, parentId: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/parents/${parentId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsRestartAgentCreate
     * @summary Helix-org: restart a bot's agent session (fresh session + desktop)
     * @request POST:/api/v1/orgs/{org}/bots/{id}/restart-agent
     * @secure
     */
    v1OrgsBotsRestartAgentCreate: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiBotActivateDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/restart-agent`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsStopAgentCreate
     * @summary Helix-org: stop a bot's agent desktop
     * @request POST:/api/v1/orgs/{org}/bots/{id}/stop-agent
     * @secure
     */
    v1OrgsBotsStopAgentCreate: (id: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/stop-agent`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsSubscriptionsDetail
     * @summary Helix-org: list a bot's subscriptions
     * @request GET:/api/v1/orgs/{org}/bots/{id}/subscriptions
     * @secure
     */
    v1OrgsBotsSubscriptionsDetail: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiBotSubscriptionsResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/subscriptions`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsSubscriptionsCreate
     * @summary Helix-org: subscribe a bot to a topic
     * @request POST:/api/v1/orgs/{org}/bots/{id}/subscriptions
     * @secure
     */
    v1OrgsBotsSubscriptionsCreate: (
      id: string,
      org: string,
      payload: ApiSubscribeBotRequest,
      params: RequestParams = {},
    ) =>
      this.request<ApiBotSubscriptionDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/subscriptions`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsBotsSubscriptionsDelete
     * @summary Helix-org: unsubscribe a bot from a topic
     * @request DELETE:/api/v1/orgs/{org}/bots/{id}/subscriptions/{topic_id}
     * @secure
     */
    v1OrgsBotsSubscriptionsDelete: (id: string, topicId: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/bots/${id}/subscriptions/${topicId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Deletes every saved node position for the org; the chart reverts to auto-layout.
     *
     * @tags HelixOrg
     * @name V1OrgsChartPositionsDelete
     * @summary Helix-org: reset chart layout
     * @request DELETE:/api/v1/orgs/{org}/chart/positions
     * @secure
     */
    v1OrgsChartPositionsDelete: (org: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/orgs/${org}/chart/positions`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Returns free-placed (x, y) coordinates for org-chart nodes. Nodes without a row fall back to auto-layout.
     *
     * @tags HelixOrg
     * @name V1OrgsChartPositionsDetail
     * @summary Helix-org: list chart node positions
     * @request GET:/api/v1/orgs/{org}/chart/positions
     * @secure
     */
    v1OrgsChartPositionsDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiChartPositionsResponse, any>({
        path: `/api/v1/orgs/${org}/chart/positions`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Upserts (x, y) coordinates for one or more org-chart nodes after the user drags them.
     *
     * @tags HelixOrg
     * @name V1OrgsChartPositionsUpdate
     * @summary Helix-org: upsert chart node positions
     * @request PUT:/api/v1/orgs/{org}/chart/positions
     * @secure
     */
    v1OrgsChartPositionsUpdate: (org: string, body: ApiUpsertChartPositionsRequest, params: RequestParams = {}) =>
      this.request<ApiChartPositionsResponse, any>({
        path: `/api/v1/orgs/${org}/chart/positions`,
        method: "PUT",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsGithubAppInstallationDetail
     * @summary Helix-org: GitHub App install status for the org
     * @request GET:/api/v1/orgs/{org}/github/app-installation
     * @secure
     */
    v1OrgsGithubAppInstallationDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiGitHubInstallationStatus, any>({
        path: `/api/v1/orgs/${org}/github/app-installation`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsGithubAppManifestCreate
     * @summary Helix-org: start the GitHub App manifest (create) flow
     * @request POST:/api/v1/orgs/{org}/github/app-manifest
     * @secure
     */
    v1OrgsGithubAppManifestCreate: (org: string, params: RequestParams = {}) =>
      this.request<ApiGitHubManifestStartResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/github/app-manifest`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsGithubReposDetail
     * @summary Helix-org: list GitHub repos accessible to the org's connected token
     * @request GET:/api/v1/orgs/{org}/github/repos
     * @secure
     */
    v1OrgsGithubReposDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiGitHubReposResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/github/repos`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsGithubWebhookCreate
     * @summary Helix-org: inbound GitHub webhook
     * @request POST:/api/v1/orgs/{org}/github/webhook
     */
    v1OrgsGithubWebhookCreate: (org: string, payload: object, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/github/webhook`,
        method: "POST",
        body: payload,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Returns the flat set of Bots in the org for the helix-org React Overview page.
     *
     * @tags HelixOrg
     * @name V1OrgsOverviewDetail
     * @summary Helix-org: get org overview
     * @request GET:/api/v1/orgs/{org}/overview
     * @secure
     */
    v1OrgsOverviewDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiOrgOverview, any>({
        path: `/api/v1/orgs/${org}/overview`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsProcessorsDetail
     * @summary Helix-org: list processors
     * @request GET:/api/v1/orgs/{org}/processors
     */
    v1OrgsProcessorsDetail: (org: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/orgs/${org}/processors`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsProcessorsCreate
     * @summary Helix-org: create a processor
     * @request POST:/api/v1/orgs/{org}/processors
     */
    v1OrgsProcessorsCreate: (org: string, payload: ApiProcessorWriteRequest, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/orgs/${org}/processors`,
        method: "POST",
        body: payload,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsProcessorsDelete
     * @summary Helix-org: delete a processor
     * @request DELETE:/api/v1/orgs/{org}/processors/{id}
     */
    v1OrgsProcessorsDelete: (org: string, id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/orgs/${org}/processors/${id}`,
        method: "DELETE",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsProcessorsDetail2
     * @summary Helix-org: get a processor
     * @request GET:/api/v1/orgs/{org}/processors/{id}
     * @originalName v1OrgsProcessorsDetail
     * @duplicate
     */
    v1OrgsProcessorsDetail2: (org: string, id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/orgs/${org}/processors/${id}`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsProcessorsUpdate
     * @summary Helix-org: update a processor
     * @request PUT:/api/v1/orgs/{org}/processors/{id}
     */
    v1OrgsProcessorsUpdate: (org: string, id: string, payload: ApiProcessorWriteRequest, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/orgs/${org}/processors/${id}`,
        method: "PUT",
        body: payload,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsSettingsDetail
     * @summary Helix-org: list settings
     * @request GET:/api/v1/orgs/{org}/settings
     * @secure
     */
    v1OrgsSettingsDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiSettingsResponse, any>({
        path: `/api/v1/orgs/${org}/settings`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsSettingsDelete
     * @summary Helix-org: delete a setting
     * @request DELETE:/api/v1/orgs/{org}/settings/{key}
     * @secure
     */
    v1OrgsSettingsDelete: (key: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/settings/${key}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsSettingsUpdate
     * @summary Helix-org: set a setting
     * @request PUT:/api/v1/orgs/{org}/settings/{key}
     * @secure
     */
    v1OrgsSettingsUpdate: (key: string, org: string, payload: ApiSetSettingRequest, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/settings/${key}`,
        method: "PUT",
        body: payload,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description List the deployment's global Slack apps available to install into a workspace
     *
     * @tags slack
     * @name V1OrgsSlackAppsDetail
     * @summary List installable Slack apps
     * @request GET:/api/v1/orgs/{org}/slack/apps
     * @secure
     */
    v1OrgsSlackAppsDetail: (org: string, params: RequestParams = {}) =>
      this.request<TypesServiceConnectionResponse[], any>({
        path: `/api/v1/orgs/${org}/slack/apps`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Build the Slack OAuth authorize URL for installing the global app into an org's workspace
     *
     * @tags slack
     * @name V1OrgsSlackOauthStartDetail
     * @summary Start Slack workspace install
     * @request GET:/api/v1/orgs/{org}/slack/oauth/start
     * @secure
     */
    v1OrgsSlackOauthStartDetail: (
      org: string,
      query?: {
        /** Slack app id to install (when multiple are configured) */
        app_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<Record<string, string>, any>({
        path: `/api/v1/orgs/${org}/slack/oauth/start`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description List the Slack workspaces installed for an organization
     *
     * @tags slack
     * @name V1OrgsSlackWorkspacesDetail
     * @summary List org Slack workspaces
     * @request GET:/api/v1/orgs/{org}/slack/workspaces
     * @secure
     */
    v1OrgsSlackWorkspacesDetail: (org: string, params: RequestParams = {}) =>
      this.request<TypesServiceConnectionResponse[], any>({
        path: `/api/v1/orgs/${org}/slack/workspaces`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Connect a Slack workspace to an org from a bot token (Socket Mode / on-prem)
     *
     * @tags slack
     * @name V1OrgsSlackWorkspacesCreate
     * @summary Connect a Slack workspace by bot token
     * @request POST:/api/v1/orgs/{org}/slack/workspaces
     * @secure
     */
    v1OrgsSlackWorkspacesCreate: (
      org: string,
      request: ServerConnectSlackWorkspaceRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesServiceConnectionResponse, any>({
        path: `/api/v1/orgs/${org}/slack/workspaces`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Remove a Slack workspace install from an organization
     *
     * @tags slack
     * @name V1OrgsSlackWorkspacesDelete
     * @summary Disconnect an org Slack workspace
     * @request DELETE:/api/v1/orgs/{org}/slack/workspaces/{id}
     * @secure
     */
    v1OrgsSlackWorkspacesDelete: (org: string, id: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/orgs/${org}/slack/workspaces/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsToolsDetail
     * @summary Helix-org: list available MCP tools
     * @request GET:/api/v1/orgs/{org}/tools
     * @secure
     */
    v1OrgsToolsDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiToolDTO[], any>({
        path: `/api/v1/orgs/${org}/tools`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsDetail
     * @summary Helix-org: list topics
     * @request GET:/api/v1/orgs/{org}/topics
     * @secure
     */
    v1OrgsTopicsDetail: (org: string, params: RequestParams = {}) =>
      this.request<ApiTopicsResponse, any>({
        path: `/api/v1/orgs/${org}/topics`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsCreate
     * @summary Helix-org: create a topic
     * @request POST:/api/v1/orgs/{org}/topics
     * @secure
     */
    v1OrgsTopicsCreate: (org: string, payload: ApiCreateTopicRequest, params: RequestParams = {}) =>
      this.request<ApiTopicDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsDelete
     * @summary Helix-org: delete a topic
     * @request DELETE:/api/v1/orgs/{org}/topics/{id}
     * @secure
     */
    v1OrgsTopicsDelete: (id: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsDetail2
     * @summary Helix-org: get a topic
     * @request GET:/api/v1/orgs/{org}/topics/{id}
     * @originalName v1OrgsTopicsDetail
     * @duplicate
     * @secure
     */
    v1OrgsTopicsDetail2: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiTopicDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsUpdate
     * @summary Helix-org: update a topic
     * @request PUT:/api/v1/orgs/{org}/topics/{id}
     * @secure
     */
    v1OrgsTopicsUpdate: (id: string, org: string, payload: ApiUpdateTopicRequest, params: RequestParams = {}) =>
      this.request<ApiTopicDTO, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}`,
        method: "PUT",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsEventsDetail
     * @summary Helix-org: SSE topic of events for one topic
     * @request GET:/api/v1/orgs/{org}/topics/{id}/events
     * @secure
     */
    v1OrgsTopicsEventsDetail: (id: string, org: string, params: RequestParams = {}) =>
      this.request<string, any>({
        path: `/api/v1/orgs/${org}/topics/${id}/events`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsGithubInstallWebhookCreate
     * @summary Helix-org: auto-install the webhook for a github topic
     * @request POST:/api/v1/orgs/{org}/topics/{id}/github/install-webhook
     * @secure
     */
    v1OrgsTopicsGithubInstallWebhookCreate: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiInstallGitHubWebhookResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}/github/install-webhook`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsGithubWebhookStatusDetail
     * @summary Helix-org: live webhook status for a github topic
     * @request GET:/api/v1/orgs/{org}/topics/{id}/github/webhook-status
     * @secure
     */
    v1OrgsTopicsGithubWebhookStatusDetail: (id: string, org: string, params: RequestParams = {}) =>
      this.request<ApiGitHubWebhookStatusResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}/github/webhook-status`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsMessagesDelete
     * @summary Helix-org: clear all messages from a topic
     * @request DELETE:/api/v1/orgs/{org}/topics/{id}/messages
     * @secure
     */
    v1OrgsTopicsMessagesDelete: (id: string, org: string, params: RequestParams = {}) =>
      this.request<void, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}/messages`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsMessagesDetail
     * @summary Helix-org: list a topic's messages (JSON:API, paginated)
     * @request GET:/api/v1/orgs/{org}/topics/{id}/messages
     * @secure
     */
    v1OrgsTopicsMessagesDetail: (
      id: string,
      org: string,
      query?: {
        /** 1-based page number (default 1) */
        "page[number]"?: number;
        /** page size (default 50, max 200) */
        "page[size]"?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ApiMessagesDocument, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}/messages`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags HelixOrg
     * @name V1OrgsTopicsPublishCreate
     * @summary Helix-org: publish a message to a topic
     * @request POST:/api/v1/orgs/{org}/topics/{id}/publish
     * @secure
     */
    v1OrgsTopicsPublishCreate: (id: string, org: string, payload: ApiPublishRequest, params: RequestParams = {}) =>
      this.request<ApiPublishResponse, ApiErrorResponse>({
        path: `/api/v1/orgs/${org}/topics/${id}/publish`,
        method: "POST",
        body: payload,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all projects for the current user
     *
     * @tags Projects
     * @name V1ProjectsList
     * @summary List projects
     * @request GET:/api/v1/projects
     * @secure
     */
    v1ProjectsList: (
      query?: {
        /** Organization ID */
        organization_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesProject[], SystemHTTPError>({
        path: `/api/v1/projects`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new project
     *
     * @tags Projects
     * @name V1ProjectsCreate
     * @summary Create project
     * @request POST:/api/v1/projects
     * @secure
     */
    v1ProjectsCreate: (request: TypesProjectCreateRequest, params: RequestParams = {}) =>
      this.request<TypesProject, SystemHTTPError>({
        path: `/api/v1/projects`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a project by ID
     *
     * @tags Projects
     * @name V1ProjectsDelete
     * @summary Delete project
     * @request DELETE:/api/v1/projects/{id}
     * @secure
     */
    v1ProjectsDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}`,
        method: "DELETE",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get a project by ID
     *
     * @tags Projects
     * @name V1ProjectsDetail
     * @summary Get project
     * @request GET:/api/v1/projects/{id}
     * @secure
     */
    v1ProjectsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesProject, SystemHTTPError>({
        path: `/api/v1/projects/${id}`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Update an existing project
     *
     * @tags Projects
     * @name V1ProjectsUpdate
     * @summary Update project
     * @request PUT:/api/v1/projects/{id}
     * @secure
     */
    v1ProjectsUpdate: (id: string, request: TypesProjectUpdateRequest, params: RequestParams = {}) =>
      this.request<TypesProject, SystemHTTPError>({
        path: `/api/v1/projects/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List access grants for a project (project owners and org owners can list access grants)
     *
     * @tags projects
     * @name V1ProjectsAccessGrantsDetail
     * @summary List project access grants
     * @request GET:/api/v1/projects/{id}/access-grants
     * @secure
     */
    v1ProjectsAccessGrantsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesAccessGrant[], any>({
        path: `/api/v1/projects/${id}/access-grants`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Grant access to a project to a team or organization member (project owners and org owners can grant access)
     *
     * @tags projects
     * @name V1ProjectsAccessGrantsCreate
     * @summary Grant access to a project to a team or organization member
     * @request POST:/api/v1/projects/{id}/access-grants
     * @secure
     */
    v1ProjectsAccessGrantsCreate: (id: string, request: TypesCreateAccessGrantRequest, params: RequestParams = {}) =>
      this.request<TypesCreateAccessGrantResponse, any>({
        path: `/api/v1/projects/${id}/access-grants`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Get paginated audit logs for a project
     *
     * @tags Projects
     * @name V1ProjectsAuditLogsDetail
     * @summary List project audit logs
     * @request GET:/api/v1/projects/{id}/audit-logs
     * @secure
     */
    v1ProjectsAuditLogsDetail: (
      id: string,
      query?: {
        /** Filter by event type */
        event_type?: string;
        /** Filter by user ID */
        user_id?: string;
        /** Filter by spec task ID */
        spec_task_id?: string;
        /** Filter by start date (RFC3339) */
        start_date?: string;
        /** Filter by end date (RFC3339) */
        end_date?: string;
        /** Search prompt text */
        search?: string;
        /** Page size (default 50, max 100) */
        limit?: number;
        /** Pagination offset */
        offset?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesProjectAuditLogResponse, TypesAPIError>({
        path: `/api/v1/projects/${id}/audit-logs`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Remove the golden Docker cache for a project from all sandboxes
     *
     * @tags Projects
     * @name V1ProjectsDockerCacheDelete
     * @summary Clear golden Docker cache
     * @request DELETE:/api/v1/projects/{id}/docker-cache
     * @secure
     */
    v1ProjectsDockerCacheDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/docker-cache`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Manually trigger a golden Docker cache build for a project
     *
     * @tags Projects
     * @name V1ProjectsDockerCacheBuildCreate
     * @summary Trigger golden Docker cache build
     * @request POST:/api/v1/projects/{id}/docker-cache/build
     * @secure
     */
    v1ProjectsDockerCacheBuildCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/docker-cache/build`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Stop all running golden builds for a project
     *
     * @tags Projects
     * @name V1ProjectsDockerCacheCancelCreate
     * @summary Cancel running golden Docker cache builds
     * @request POST:/api/v1/projects/{id}/docker-cache/cancel
     * @secure
     */
    v1ProjectsDockerCacheCancelCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/docker-cache/cancel`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns the ZFS snapshot and clone tree showing golden cache, snapshots, and active session clones.
     *
     * @tags projects
     * @name V1ProjectsDockerCacheZfsTreeDetail
     * @summary Get ZFS snapshot/clone tree for project's Docker cache
     * @request GET:/api/v1/projects/{id}/docker-cache/zfs-tree
     * @secure
     */
    v1ProjectsDockerCacheZfsTreeDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesZFSTree, any>({
        path: `/api/v1/projects/${id}/docker-cache/zfs-tree`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Stop the running exploratory session for a project (stops sandbox container, keeps session record)
     *
     * @tags Projects
     * @name V1ProjectsExploratorySessionDelete
     * @summary Stop project exploratory session
     * @request DELETE:/api/v1/projects/{id}/exploratory-session
     * @secure
     */
    v1ProjectsExploratorySessionDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/exploratory-session`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the active exploratory session for a project (returns null if none exists)
     *
     * @tags Projects
     * @name V1ProjectsExploratorySessionDetail
     * @summary Get project exploratory session
     * @request GET:/api/v1/projects/{id}/exploratory-session
     * @secure
     */
    v1ProjectsExploratorySessionDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesSession, SystemHTTPError>({
        path: `/api/v1/projects/${id}/exploratory-session`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Start or return existing exploratory session for a project
     *
     * @tags Projects
     * @name V1ProjectsExploratorySessionCreate
     * @summary Start project exploratory session
     * @request POST:/api/v1/projects/{id}/exploratory-session
     * @secure
     */
    v1ProjectsExploratorySessionCreate: (id: string, params: RequestParams = {}) =>
      this.request<TypesSession, SystemHTTPError>({
        path: `/api/v1/projects/${id}/exploratory-session`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns the parsed Goose recipes declared on the project's default agent, including each recipe's parameter schema so the spec-task creation form can render dynamic inputs.
     *
     * @tags Projects
     * @name V1ProjectsGooseRecipesDetail
     * @summary List Goose recipes available to a project
     * @request GET:/api/v1/projects/{id}/goose-recipes
     * @secure
     */
    v1ProjectsGooseRecipesDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerProjectGooseRecipe[], SystemHTTPError>({
        path: `/api/v1/projects/${id}/goose-recipes`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the version history of guidelines for a project
     *
     * @tags Projects
     * @name V1ProjectsGuidelinesHistoryDetail
     * @summary Get project guidelines history
     * @request GET:/api/v1/projects/{id}/guidelines-history
     * @secure
     */
    v1ProjectsGuidelinesHistoryDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesGuidelinesHistory[], SystemHTTPError>({
        path: `/api/v1/projects/${id}/guidelines-history`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Move a project from personal workspace to an organization
     *
     * @tags Projects
     * @name V1ProjectsMoveCreate
     * @summary Move a project to an organization
     * @request POST:/api/v1/projects/{id}/move
     * @secure
     */
    v1ProjectsMoveCreate: (id: string, request: TypesMoveProjectRequest, params: RequestParams = {}) =>
      this.request<TypesProject, SystemHTTPError>({
        path: `/api/v1/projects/${id}/move`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Check for naming conflicts before moving a project to an organization
     *
     * @tags Projects
     * @name V1ProjectsMovePreviewCreate
     * @summary Preview moving a project to an organization
     * @request POST:/api/v1/projects/{id}/move/preview
     * @secure
     */
    v1ProjectsMovePreviewCreate: (id: string, request: TypesMoveProjectRequest, params: RequestParams = {}) =>
      this.request<TypesMoveProjectPreviewResponse, SystemHTTPError>({
        path: `/api/v1/projects/${id}/move/preview`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Remove a project from the current user's pinned projects
     *
     * @tags Projects
     * @name V1ProjectsPinDelete
     * @summary Unpin a project
     * @request DELETE:/api/v1/projects/{id}/pin
     * @secure
     */
    v1ProjectsPinDelete: (id: string, params: RequestParams = {}) =>
      this.request<ServerPinnedProjectsResponse, SystemHTTPError>({
        path: `/api/v1/projects/${id}/pin`,
        method: "DELETE",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Pin a project for the current user so it appears at the top of the projects board
     *
     * @tags Projects
     * @name V1ProjectsPinCreate
     * @summary Pin a project
     * @request POST:/api/v1/projects/{id}/pin
     * @secure
     */
    v1ProjectsPinCreate: (id: string, params: RequestParams = {}) =>
      this.request<ServerPinnedProjectsResponse, SystemHTTPError>({
        path: `/api/v1/projects/${id}/pin`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all repositories attached to a project
     *
     * @tags Projects
     * @name GetProjectRepositories
     * @summary Get project repositories
     * @request GET:/api/v1/projects/{id}/repositories
     * @secure
     */
    getProjectRepositories: (id: string, params: RequestParams = {}) =>
      this.request<TypesGitRepository[], SystemHTTPError>({
        path: `/api/v1/projects/${id}/repositories`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Attach an existing repository to a project
     *
     * @tags Projects
     * @name V1ProjectsRepositoriesAttachUpdate
     * @summary Attach repository to project
     * @request PUT:/api/v1/projects/{id}/repositories/{repo_id}/attach
     * @secure
     */
    v1ProjectsRepositoriesAttachUpdate: (id: string, repoId: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/repositories/${repoId}/attach`,
        method: "PUT",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Detach a repository from its project
     *
     * @tags Projects
     * @name V1ProjectsRepositoriesDetachUpdate
     * @summary Detach repository from project
     * @request PUT:/api/v1/projects/{id}/repositories/{repo_id}/detach
     * @secure
     */
    v1ProjectsRepositoriesDetachUpdate: (id: string, repoId: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/repositories/${repoId}/detach`,
        method: "PUT",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Set the primary repository for a project
     *
     * @tags Projects
     * @name V1ProjectsRepositoriesPrimaryUpdate
     * @summary Set project primary repository
     * @request PUT:/api/v1/projects/{id}/repositories/{repo_id}/primary
     * @secure
     */
    v1ProjectsRepositoriesPrimaryUpdate: (id: string, repoId: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/projects/${id}/repositories/${repoId}/primary`,
        method: "PUT",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List all secrets associated with a specific project.
     *
     * @tags secrets
     * @name V1ProjectsSecretsDetail
     * @summary List secrets for a project
     * @request GET:/api/v1/projects/{id}/secrets
     * @secure
     */
    v1ProjectsSecretsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesSecret[], any>({
        path: `/api/v1/projects/${id}/secrets`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Create a new secret associated with a specific project. The secret will be injected as an environment variable in project sessions.
     *
     * @tags secrets
     * @name V1ProjectsSecretsCreate
     * @summary Create a secret for a project
     * @request POST:/api/v1/projects/{id}/secrets
     * @secure
     */
    v1ProjectsSecretsCreate: (id: string, request: TypesCreateSecretRequest, params: RequestParams = {}) =>
      this.request<TypesSecret, any>({
        path: `/api/v1/projects/${id}/secrets`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Get git commit history for project startup script
     *
     * @tags Projects
     * @name V1ProjectsStartupScriptHistoryDetail
     * @summary Get startup script version history
     * @request GET:/api/v1/projects/{id}/startup-script/history
     * @secure
     */
    v1ProjectsStartupScriptHistoryDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServicesStartupScriptVersion[], any>({
        path: `/api/v1/projects/${id}/startup-script/history`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Get progress information for all spec-driven tasks in a project in a single request. This is more efficient than calling the individual progress endpoint for each task.
     *
     * @tags spec-driven-tasks
     * @name V1ProjectsTasksProgressDetail
     * @summary Get progress for all tasks in a project
     * @request GET:/api/v1/projects/{id}/tasks-progress
     */
    v1ProjectsTasksProgressDetail: (
      id: string,
      query?: {
        /**
         * Include checklist progress (slower, parses git files)
         * @default false
         */
        include_checklist?: boolean;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerBatchTaskProgressResponse, TypesAPIError>({
        path: `/api/v1/projects/${id}/tasks-progress`,
        method: "GET",
        query: query,
        format: "json",
        ...params,
      }),

    /**
     * @description Get usage metrics for all spec-driven tasks in a project in a single request. This is more efficient than calling the individual usage endpoint for each task.
     *
     * @tags spec-driven-tasks
     * @name V1ProjectsTasksUsageDetail
     * @summary Get usage for all tasks in a project
     * @request GET:/api/v1/projects/{id}/tasks-usage
     */
    v1ProjectsTasksUsageDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerBatchTaskUsageResponse, TypesAPIError>({
        path: `/api/v1/projects/${id}/tasks-usage`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Get token usage metrics for a project (combined across all tasks)
     *
     * @tags Projects
     * @name V1ProjectsUsageDetail
     * @summary Get project token usage
     * @request GET:/api/v1/projects/{id}/usage
     * @secure
     */
    v1ProjectsUsageDetail: (
      id: string,
      query?: {
        /**
         * Aggregation level (5min, hourly, daily)
         * @default "hourly"
         */
        aggregation_level?: string;
        /** Start time (RFC3339 format) */
        from?: string;
        /** End time (RFC3339 format) */
        to?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/projects/${id}/usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description One entry per distinct VCS provider present among the project's external repos, with the acting user, the account pushes are attributed to, and per-repo verified access. Backs the project board connection lozenge.
     *
     * @tags Projects
     * @name GetProjectVcsConnections
     * @summary Get project VCS connection status
     * @request GET:/api/v1/projects/{id}/vcs-connections
     * @secure
     */
    getProjectVcsConnections: (id: string, params: RequestParams = {}) =>
      this.request<TypesVCSConnectionInfo[], SystemHTTPError>({
        path: `/api/v1/projects/${id}/vcs-connections`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Return enable/disable state, hostnames, and recent deploys for a project's web service.
     *
     * @tags Projects
     * @name V1ProjectsWebServiceDetail
     * @summary Get project web service state
     * @request GET:/api/v1/projects/{id}/web-service
     * @secure
     */
    v1ProjectsWebServiceDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerProjectWebServiceResponse, any>({
        path: `/api/v1/projects/${id}/web-service`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Toggle web service enable/disable and update container_port. Enabling pre-seeds the default subdomain.
     *
     * @tags Projects
     * @name V1ProjectsWebServiceUpdate
     * @summary Update project web service state
     * @request PUT:/api/v1/projects/{id}/web-service
     * @secure
     */
    v1ProjectsWebServiceUpdate: (id: string, body: ServerPutProjectWebServiceRequest, params: RequestParams = {}) =>
      this.request<ServerProjectWebServiceResponse, any>({
        path: `/api/v1/projects/${id}/web-service`,
        method: "PUT",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Manual deploy primitive — set the sandbox that vhost requests route to.
     *
     * @tags Projects
     * @name V1ProjectsWebServiceActiveSandboxCreate
     * @summary Point a project web service at a sandbox
     * @request POST:/api/v1/projects/{id}/web-service/active-sandbox
     * @secure
     */
    v1ProjectsWebServiceActiveSandboxCreate: (
      id: string,
      body: ServerSetActiveSandboxRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesProjectWebServiceState, any>({
        path: `/api/v1/projects/${id}/web-service/active-sandbox`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Provisions a fresh sandbox, clones the primary repo at the requested SHA, runs .helix/startup.sh, and cuts routing over once it's up.
     *
     * @tags Projects
     * @name V1ProjectsWebServiceDeployCreate
     * @summary Trigger an auto-deploy of the project's web service
     * @request POST:/api/v1/projects/{id}/web-service/deploy
     * @secure
     */
    v1ProjectsWebServiceDeployCreate: (id: string, body: ServerDeployWebServiceRequest, params: RequestParams = {}) =>
      this.request<TypesWebServiceDeploy, any>({
        path: `/api/v1/projects/${id}/web-service/deploy`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Insert an unverified domain row. Verification happens out-of-band via the .well-known endpoint.
     *
     * @tags Projects
     * @name V1ProjectsWebServiceDomainsCreate
     * @summary Add a custom domain to a project web service
     * @request POST:/api/v1/projects/{id}/web-service/domains
     * @secure
     */
    v1ProjectsWebServiceDomainsCreate: (id: string, body: ServerAddDomainRequest, params: RequestParams = {}) =>
      this.request<TypesVHostRoute, any>({
        path: `/api/v1/projects/${id}/web-service/domains`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Projects
     * @name V1ProjectsWebServiceDomainsDelete
     * @summary Remove a custom domain from a project web service
     * @request DELETE:/api/v1/projects/{id}/web-service/domains/{domain_id}
     * @secure
     */
    v1ProjectsWebServiceDomainsDelete: (id: string, domainId: string, params: RequestParams = {}) =>
      this.request<Record<string, boolean>, any>({
        path: `/api/v1/projects/${id}/web-service/domains/${domainId}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns the tail of the project's web-service startup log (combined stdout/stderr of .helix/startup.sh in the active sandbox) so an authorized user can see why a deploy did or didn't come up. Never exposed on the public web-service host.
     *
     * @tags projects
     * @name V1ProjectsWebServiceLogsDetail
     * @summary Get web service deploy/startup logs
     * @request GET:/api/v1/projects/{id}/web-service/logs
     * @secure
     */
    v1ProjectsWebServiceLogsDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerProjectWebServiceLogsResponse, any>({
        path: `/api/v1/projects/${id}/web-service/logs`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns a sorted list of unique labels across all spec tasks in a project
     *
     * @tags spec-driven-tasks
     * @name V1ProjectsLabelsDetail
     * @summary List all labels used in a project
     * @request GET:/api/v1/projects/{projectId}/labels
     */
    v1ProjectsLabelsDetail: (projectId: string, params: RequestParams = {}) =>
      this.request<string[], TypesAPIError>({
        path: `/api/v1/projects/${projectId}/labels`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Idempotent upsert of a project from a declarative YAML spec
     *
     * @tags Projects
     * @name V1ProjectsApplyUpdate
     * @summary Apply a project YAML
     * @request PUT:/api/v1/projects/apply
     * @secure
     */
    v1ProjectsApplyUpdate: (request: TypesProjectApplyRequest, params: RequestParams = {}) =>
      this.request<TypesProjectApplyResponse, SystemHTTPError>({
        path: `/api/v1/projects/apply`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a minimal project for a repository that doesn't have one
     *
     * @tags Projects
     * @name V1ProjectsQuickCreateCreate
     * @summary Quick-create a project for a repository
     * @request POST:/api/v1/projects/quick-create
     * @secure
     */
    v1ProjectsQuickCreateCreate: (request: ServerQuickCreateProjectRequest, params: RequestParams = {}) =>
      this.request<TypesProject, TypesAPIError>({
        path: `/api/v1/projects/quick-create`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get prompt history entries for the current user
     *
     * @tags PromptHistory
     * @name V1PromptHistoryList
     * @summary List prompt history
     * @request GET:/api/v1/prompt-history
     * @secure
     */
    v1PromptHistoryList: (
      query?: {
        /** Spec Task ID (required unless session_id is given) */
        spec_task_id?: string;
        /** Session ID (optional filter) */
        session_id?: string;
        /** Project ID (optional filter) */
        project_id?: string;
        /** Only entries after this timestamp (Unix milliseconds) */
        since?: number;
        /** Max entries to return (default 100) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPromptHistoryListResponse, SystemHTTPError>({
        path: `/api/v1/prompt-history`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Soft-deletes a prompt history entry so it is removed from the queue and no longer synced to clients
     *
     * @tags PromptHistory
     * @name V1PromptHistoryDelete
     * @summary Delete a prompt history entry
     * @request DELETE:/api/v1/prompt-history/{id}
     * @secure
     */
    v1PromptHistoryDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, boolean>, SystemHTTPError>({
        path: `/api/v1/prompt-history/${id}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Pin or unpin a prompt for quick access
     *
     * @tags PromptHistory
     * @name V1PromptHistoryPinUpdate
     * @summary Update prompt pin status
     * @request PUT:/api/v1/prompt-history/{id}/pin
     * @secure
     */
    v1PromptHistoryPinUpdate: (id: string, request: ServerPromptPinRequest, params: RequestParams = {}) =>
      this.request<Record<string, boolean>, SystemHTTPError>({
        path: `/api/v1/prompt-history/${id}/pin`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Update tags for a prompt
     *
     * @tags PromptHistory
     * @name V1PromptHistoryTagsUpdate
     * @summary Update prompt tags
     * @request PUT:/api/v1/prompt-history/{id}/tags
     * @secure
     */
    v1PromptHistoryTagsUpdate: (id: string, request: ServerPromptTagsRequest, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/prompt-history/${id}/tags`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Increment usage count when a prompt is reused
     *
     * @tags PromptHistory
     * @name V1PromptHistoryUseCreate
     * @summary Increment prompt usage
     * @request POST:/api/v1/prompt-history/{id}/use
     * @secure
     */
    v1PromptHistoryUseCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, boolean>, SystemHTTPError>({
        path: `/api/v1/prompt-history/${id}/use`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all pinned prompts for the current user
     *
     * @tags PromptHistory
     * @name V1PromptHistoryPinnedList
     * @summary List pinned prompts
     * @request GET:/api/v1/prompt-history/pinned
     * @secure
     */
    v1PromptHistoryPinnedList: (
      query?: {
        /** Filter by spec task ID */
        spec_task_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPromptHistoryEntry[], SystemHTTPError>({
        path: `/api/v1/prompt-history/pinned`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Search prompts by content
     *
     * @tags PromptHistory
     * @name V1PromptHistorySearchList
     * @summary Search prompts
     * @request GET:/api/v1/prompt-history/search
     * @secure
     */
    v1PromptHistorySearchList: (
      query: {
        /** Search query */
        q: string;
        /** Max results (default 50) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPromptHistoryEntry[], SystemHTTPError>({
        path: `/api/v1/prompt-history/search`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Sync prompt history entries from the frontend (union merge - no deletes)
     *
     * @tags PromptHistory
     * @name V1PromptHistorySyncCreate
     * @summary Sync prompt history
     * @request POST:/api/v1/prompt-history/sync
     * @secure
     */
    v1PromptHistorySyncCreate: (request: TypesPromptHistorySyncRequest, params: RequestParams = {}) =>
      this.request<TypesPromptHistorySyncResponse, SystemHTTPError>({
        path: `/api/v1/prompt-history/sync`,
        method: "POST",
        body: request,
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
     * @description List question sets for the current user or organization
     *
     * @tags question-sets
     * @name V1QuestionSetsList
     * @summary List question sets
     * @request GET:/api/v1/question-sets
     * @secure
     */
    v1QuestionSetsList: (
      query?: {
        /** Organization ID or slug */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesQuestionSet[], SystemHTTPError>({
        path: `/api/v1/question-sets`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new question set
     *
     * @tags question-sets
     * @name V1QuestionSetsCreate
     * @summary Create a new question set
     * @request POST:/api/v1/question-sets
     * @secure
     */
    v1QuestionSetsCreate: (questionSet: TypesQuestionSet, params: RequestParams = {}) =>
      this.request<TypesQuestionSet, SystemHTTPError>({
        path: `/api/v1/question-sets`,
        method: "POST",
        body: questionSet,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a question set
     *
     * @tags question-sets
     * @name V1QuestionSetsDelete
     * @summary Delete a question set
     * @request DELETE:/api/v1/question-sets/{id}
     * @secure
     */
    v1QuestionSetsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, SystemHTTPError>({
        path: `/api/v1/question-sets/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a question set by ID
     *
     * @tags question-sets
     * @name V1QuestionSetsDetail
     * @summary Get a question set by ID
     * @request GET:/api/v1/question-sets/{id}
     * @secure
     */
    v1QuestionSetsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesQuestionSet, SystemHTTPError>({
        path: `/api/v1/question-sets/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update a question set
     *
     * @tags question-sets
     * @name V1QuestionSetsUpdate
     * @summary Update a question set
     * @request PUT:/api/v1/question-sets/{id}
     * @secure
     */
    v1QuestionSetsUpdate: (id: string, questionSet: TypesQuestionSet, params: RequestParams = {}) =>
      this.request<TypesQuestionSet, SystemHTTPError>({
        path: `/api/v1/question-sets/${id}`,
        method: "PUT",
        body: questionSet,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List executions for the question set
     *
     * @tags question-sets
     * @name V1QuestionSetsExecutionsDetail
     * @summary List question set executions
     * @request GET:/api/v1/question-sets/{id}/executions
     * @secure
     */
    v1QuestionSetsExecutionsDetail: (
      id: string,
      query?: {
        /** Offset */
        offset?: number;
        /** Limit */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesQuestionSetExecution[], any>({
        path: `/api/v1/question-sets/${id}/executions`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Execute a question set, this is a blocking operation and will return a response for each question in the question set
     *
     * @tags question-sets
     * @name V1QuestionSetsExecutionsCreate
     * @summary Execute a question set
     * @request POST:/api/v1/question-sets/{id}/executions
     * @secure
     */
    v1QuestionSetsExecutionsCreate: (
      id: string,
      executeQuestionSetRequest: TypesExecuteQuestionSetRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesExecuteQuestionSetResponse, SystemHTTPError>({
        path: `/api/v1/question-sets/${id}/executions`,
        method: "POST",
        body: executeQuestionSetRequest,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get results for a question set execution
     *
     * @tags question-sets
     * @name V1QuestionSetsExecutionsDetail2
     * @summary Get question set execution results
     * @request GET:/api/v1/question-sets/{question_set_id}/executions/{id}
     * @originalName v1QuestionSetsExecutionsDetail
     * @duplicate
     * @secure
     */
    v1QuestionSetsExecutionsDetail2: (
      id: string,
      questionSetId: string,
      query?: {
        /** Format, one of: json (default), markdown */
        format?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesQuestionSetExecution, any>({
        path: `/api/v1/question-sets/${questionSetId}/executions/${id}`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get quotas for the user. Returns current usage and limits for desktops, projects, repositories, and spec tasks. Optionally pass org_id query parameter to get organization quotas.
     *
     * @tags quotas
     * @name V1QuotasList
     * @summary Get quotas
     * @request GET:/api/v1/quotas
     * @secure
     */
    v1QuotasList: (
      query?: {
        /** Organization ID to get quotas for */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesQuotaResponse, any>({
        path: `/api/v1/quotas`,
        method: "GET",
        query: query,
        secure: true,
        ...params,
      }),

    /**
     * @description Get all repositories that don't have an associated project
     *
     * @tags Repositories
     * @name V1RepositoriesWithoutProjectsList
     * @summary List repositories without projects
     * @request GET:/api/v1/repositories/without-projects
     * @secure
     */
    v1RepositoriesWithoutProjectsList: (
      query?: {
        /** Filter by organization ID */
        organization_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesGitRepository[], any>({
        path: `/api/v1/repositories/without-projects`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Search across projects, tasks, sessions, prompts, knowledge, repositories, and apps concurrently
     *
     * @tags search
     * @name V1ResourceSearchCreate
     * @summary Search across resources
     * @request POST:/api/v1/resource-search
     * @secure
     */
    v1ResourceSearchCreate: (request: TypesResourceSearchRequest, params: RequestParams = {}) =>
      this.request<TypesResourceSearchResponse, any>({
        path: `/api/v1/resource-search`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Return all compose-based runner profiles, ordered by name.
     *
     * @tags runner_profiles
     * @name V1RunnerProfilesList
     * @summary List runner profiles
     * @request GET:/api/v1/runner-profiles
     * @secure
     */
    v1RunnerProfilesList: (params: RequestParams = {}) =>
      this.request<TypesRunnerProfile[], any>({
        path: `/api/v1/runner-profiles`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags runner_profiles
     * @name V1RunnerProfilesCreate
     * @summary Create a runner profile
     * @request POST:/api/v1/runner-profiles
     * @secure
     */
    v1RunnerProfilesCreate: (body: ServerRunnerProfileSaveRequest, params: RequestParams = {}) =>
      this.request<TypesRunnerProfile, string>({
        path: `/api/v1/runner-profiles`,
        method: "POST",
        body: body,
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags runner_profiles
     * @name V1RunnerProfilesDelete
     * @summary Delete a runner profile
     * @request DELETE:/api/v1/runner-profiles/{id}
     * @secure
     */
    v1RunnerProfilesDelete: (id: string, params: RequestParams = {}) =>
      this.request<string, string>({
        path: `/api/v1/runner-profiles/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags runner_profiles
     * @name V1RunnerProfilesDetail
     * @summary Get a runner profile by ID
     * @request GET:/api/v1/runner-profiles/{id}
     * @secure
     */
    v1RunnerProfilesDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesRunnerProfile, string>({
        path: `/api/v1/runner-profiles/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags runner_profiles
     * @name V1RunnerProfilesUpdate
     * @summary Update a runner profile (full replace)
     * @request PUT:/api/v1/runner-profiles/{id}
     * @secure
     */
    v1RunnerProfilesUpdate: (id: string, body: ServerRunnerProfileSaveRequest, params: RequestParams = {}) =>
      this.request<TypesRunnerProfile, string>({
        path: `/api/v1/runner-profiles/${id}`,
        method: "PUT",
        body: body,
        secure: true,
        ...params,
      }),

    /**
     * @description Validates GPU compatibility, persists the assignment, and notifies the runner over NATS to apply the profile.
     *
     * @tags runner_profiles
     * @name V1RunnersAssignProfileCreate
     * @summary Assign a profile to a runner
     * @request POST:/api/v1/runners/{runner_id}/assign-profile
     * @secure
     */
    v1RunnersAssignProfileCreate: (
      runnerId: string,
      body: ServerRunnerProfileAssignRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesRunnerAssignment, string>({
        path: `/api/v1/runners/${runnerId}/assign-profile`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags runner_profiles
     * @name V1RunnersAssignmentDetail
     * @summary Get a runner's current profile assignment
     * @request GET:/api/v1/runners/{runner_id}/assignment
     * @secure
     */
    v1RunnersAssignmentDetail: (runnerId: string, params: RequestParams = {}) =>
      this.request<TypesRunnerAssignment, string>({
        path: `/api/v1/runners/${runnerId}/assignment`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Deletes the runner-to-profile assignment and tells the runner to tear down any active compose stack. Idempotent.
     *
     * @tags runner_profiles
     * @name V1RunnersClearProfileCreate
     * @summary Clear a runner's profile assignment
     * @request POST:/api/v1/runners/{runner_id}/clear-profile
     * @secure
     */
    v1RunnersClearProfileCreate: (runnerId: string, params: RequestParams = {}) =>
      this.request<string, any>({
        path: `/api/v1/runners/${runnerId}/clear-profile`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Returns the subset of profiles whose GPU compatibility specification is satisfied by the runner's reported hardware inventory.
     *
     * @tags runner_profiles
     * @name V1RunnersCompatibleProfilesDetail
     * @summary List runner profiles compatible with the given runner
     * @request GET:/api/v1/runners/{runner_id}/compatible-profiles
     * @secure
     */
    v1RunnersCompatibleProfilesDetail: (runnerId: string, params: RequestParams = {}) =>
      this.request<TypesRunnerProfile[], any>({
        path: `/api/v1/runners/${runnerId}/compatible-profiles`,
        method: "GET",
        secure: true,
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
     * @description Check if the authenticated user has write access to the required repositories for a sample project
     *
     * @tags sample-projects
     * @name V1SampleProjectsSimpleCheckAccessCreate
     * @summary Check repository access for a sample project
     * @request POST:/api/v1/sample-projects/simple/check-access
     * @secure
     */
    v1SampleProjectsSimpleCheckAccessCreate: (
      request: TypesCheckSampleProjectAccessRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesCheckSampleProjectAccessResponse, SystemHTTPError>({
        path: `/api/v1/sample-projects/simple/check-access`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
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
    v1SampleProjectsSimpleForkCreate: (request: TypesForkSimpleProjectRequest, params: RequestParams = {}) =>
      this.request<TypesForkSimpleProjectResponse, any>({
        path: `/api/v1/sample-projects/simple/fork`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Fork the specified repositories to the user's GitHub account
     *
     * @tags sample-projects
     * @name V1SampleProjectsSimpleForkReposCreate
     * @summary Fork repositories for a sample project
     * @request POST:/api/v1/sample-projects/simple/fork-repos
     * @secure
     */
    v1SampleProjectsSimpleForkReposCreate: (request: TypesForkRepositoriesRequest, params: RequestParams = {}) =>
      this.request<TypesForkRepositoriesResponse, SystemHTTPError>({
        path: `/api/v1/sample-projects/simple/fork-repos`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
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
    v1SamplesRepositoriesCreate: (request: TypesCreateSampleRepositoryRequest, params: RequestParams = {}) =>
      this.request<TypesGitRepository, TypesAPIError>({
        path: `/api/v1/samples/repositories`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List the sandbox runtimes available on this server
     *
     * @tags Sandboxes
     * @name V1SandboxRuntimesList
     * @summary List sandbox runtimes
     * @request GET:/api/v1/sandbox-runtimes
     * @secure
     */
    v1SandboxRuntimesList: (params: RequestParams = {}) =>
      this.request<Record<string, string[]>, any>({
        path: `/api/v1/sandbox-runtimes`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all registered sandboxes
     *
     * @tags sandbox
     * @name V1SandboxesList
     * @summary List sandbox instances
     * @request GET:/api/v1/sandboxes
     */
    v1SandboxesList: (params: RequestParams = {}) =>
      this.request<TypesSandboxInstance[], any>({
        path: `/api/v1/sandboxes`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Remove a sandbox from the registry
     *
     * @tags sandbox
     * @name V1SandboxesDelete
     * @summary Deregister a sandbox instance
     * @request DELETE:/api/v1/sandboxes/{id}
     */
    v1SandboxesDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, any>({
        path: `/api/v1/sandboxes/${id}`,
        method: "DELETE",
        ...params,
      }),

    /**
     * @description Get per-container disk I/O stats (write_bytes, read_bytes) from cgroup blkio counters
     *
     * @tags sandbox
     * @name V1SandboxesContainersBlkioDetail
     * @summary Get container blkio stats
     * @request GET:/api/v1/sandboxes/{id}/containers/{session_id}/blkio
     */
    v1SandboxesContainersBlkioDetail: (id: string, sessionId: string, params: RequestParams = {}) =>
      this.request<HydraContainerBlkioStats, any>({
        path: `/api/v1/sandboxes/${id}/containers/${sessionId}/blkio`,
        method: "GET",
        format: "json",
        ...params,
      }),

    /**
     * @description Get disk usage history for trending and alerts
     *
     * @tags sandbox
     * @name V1SandboxesDiskHistoryDetail
     * @summary Get disk usage history
     * @request GET:/api/v1/sandboxes/{id}/disk-history
     */
    v1SandboxesDiskHistoryDetail: (
      id: string,
      query?: {
        /** Since timestamp (RFC3339) */
        since?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesDiskUsageHistory[], any>({
        path: `/api/v1/sandboxes/${id}/disk-history`,
        method: "GET",
        query: query,
        format: "json",
        ...params,
      }),

    /**
     * @description Update sandbox status and metrics
     *
     * @tags sandbox
     * @name V1SandboxesHeartbeatCreate
     * @summary Update sandbox heartbeat
     * @request POST:/api/v1/sandboxes/{id}/heartbeat
     */
    v1SandboxesHeartbeatCreate: (id: string, heartbeat: TypesSandboxHeartbeatRequest, params: RequestParams = {}) =>
      this.request<Record<string, string>, any>({
        path: `/api/v1/sandboxes/${id}/heartbeat`,
        method: "POST",
        body: heartbeat,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Register a new sandbox or update an existing one
     *
     * @tags sandbox
     * @name V1SandboxesRegisterCreate
     * @summary Register a sandbox instance
     * @request POST:/api/v1/sandboxes/register
     */
    v1SandboxesRegisterCreate: (sandbox: TypesSandboxInstance, params: RequestParams = {}) =>
      this.request<TypesSandboxInstance, any>({
        path: `/api/v1/sandboxes/register`,
        method: "POST",
        body: sandbox,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Search across projects, tasks, sessions, prompts, and code
     *
     * @tags Search
     * @name V1SearchList
     * @summary Unified search across Helix entities
     * @request GET:/api/v1/search
     * @secure
     */
    v1SearchList: (
      query: {
        /** Search query */
        q: string;
        /** Entity types to search: projects, tasks, sessions, prompts, code */
        types?: string[];
        /** Max results per type (default 10) */
        limit?: number;
        /** Filter by organization ID */
        org_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesUnifiedSearchResponse, SystemHTTPError>({
        path: `/api/v1/search`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List secrets for the user, or for an organization when organization_id is set.
     *
     * @tags secrets
     * @name V1SecretsList
     * @summary List secrets
     * @request GET:/api/v1/secrets
     * @secure
     */
    v1SecretsList: (
      query?: {
        /** Organization ID or name. When set, lists org-owned secrets instead of personal secrets. */
        organization_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSecret[], any>({
        path: `/api/v1/secrets`,
        method: "GET",
        query: query,
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
     * @description List all service connections (GitHub Apps, ADO Service Principals) for the organization
     *
     * @tags service-connections
     * @name V1ServiceConnectionsList
     * @summary List service connections
     * @request GET:/api/v1/service-connections
     * @secure
     */
    v1ServiceConnectionsList: (
      query?: {
        /** Organization ID (optional, defaults to user's org) */
        organization_id?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesServiceConnectionResponse[], TypesAPIError>({
        path: `/api/v1/service-connections`,
        method: "GET",
        query: query,
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new service connection (GitHub App or ADO Service Principal)
     *
     * @tags service-connections
     * @name V1ServiceConnectionsCreate
     * @summary Create service connection
     * @request POST:/api/v1/service-connections
     * @secure
     */
    v1ServiceConnectionsCreate: (request: TypesServiceConnectionCreateRequest, params: RequestParams = {}) =>
      this.request<TypesServiceConnectionResponse, TypesAPIError>({
        path: `/api/v1/service-connections`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Delete a service connection
     *
     * @tags service-connections
     * @name V1ServiceConnectionsDelete
     * @summary Delete service connection
     * @request DELETE:/api/v1/service-connections/{id}
     * @secure
     */
    v1ServiceConnectionsDelete: (id: string, params: RequestParams = {}) =>
      this.request<void, TypesAPIError>({
        path: `/api/v1/service-connections/${id}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Get a specific service connection by ID
     *
     * @tags service-connections
     * @name V1ServiceConnectionsDetail
     * @summary Get service connection
     * @request GET:/api/v1/service-connections/{id}
     * @secure
     */
    v1ServiceConnectionsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesServiceConnectionResponse, TypesAPIError>({
        path: `/api/v1/service-connections/${id}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update a service connection
     *
     * @tags service-connections
     * @name V1ServiceConnectionsUpdate
     * @summary Update service connection
     * @request PUT:/api/v1/service-connections/{id}
     * @secure
     */
    v1ServiceConnectionsUpdate: (
      id: string,
      request: TypesServiceConnectionUpdateRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesServiceConnectionResponse, TypesAPIError>({
        path: `/api/v1/service-connections/${id}`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Test a service connection by attempting to authenticate
     *
     * @tags service-connections
     * @name V1ServiceConnectionsTestCreate
     * @summary Test service connection
     * @request POST:/api/v1/service-connections/{id}/test
     * @secure
     */
    v1ServiceConnectionsTestCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, TypesAPIError>({
        path: `/api/v1/service-connections/${id}/test`,
        method: "POST",
        secure: true,
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
        /** Question set ID */
        question_set_id?: string;
        /** Question set execution ID */
        question_set_execution_id?: string;
        /** App ID */
        app_id?: string;
        /** Search sessions by name */
        search?: string;
        /** Project ID */
        project_id?: string;
        /** Filter by session role (e.g. job) */
        session_role?: string;
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
    v1SessionsDetail: (
      id: string,
      query?: {
        /** Set to '1' to omit interactions from the response */
        skipInteractions?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSession, any>({
        path: `/api/v1/sessions/${id}`,
        method: "GET",
        query: query,
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
     * @description Called by the in-desktop settings-sync daemon after it hot-reloads Zed's config for an agent switch. Delivers the pending handoff to the live Zed thread without waiting for a process restart. Internal coordination endpoint.
     *
     * @tags sessions
     * @name V1SessionsAgentConfigAppliedCreate
     * @summary Notify that an in-place agent switch's config has been applied in the container
     * @request POST:/api/v1/sessions/{id}/agent-config-applied
     * @secure
     */
    v1SessionsAgentConfigAppliedCreate: (id: string, params: RequestParams = {}) =>
      this.request<ServerAgentConfigAppliedResponse, any>({
        path: `/api/v1/sessions/${id}/agent-config-applied`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Sends cancel_current_turn to the active Zed agent. Returns 202 immediately; the interaction state update (interrupted) flows to the frontend via WebSocket.
     *
     * @tags Sessions
     * @name V1SessionsCancelCreate
     * @summary Cancel the current agent turn
     * @request POST:/api/v1/sessions/{id}/cancel
     * @secure
     */
    v1SessionsCancelCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/cancel`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get decrypted Claude credentials for use inside a desktop container. Only accepts runner/session-scoped tokens.
     *
     * @tags Claude
     * @name V1SessionsClaudeCredentialsDetail
     * @summary Get Claude credentials for a session
     * @request GET:/api/v1/sessions/{id}/claude-credentials
     * @secure
     */
    v1SessionsClaudeCredentialsDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerSessionClaudeCredentialsResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/claude-credentials`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Push refreshed Claude OAuth credentials back to the API (e.g. after Claude Code refreshes its token). Only accepts runner/session-scoped tokens.
     *
     * @tags Claude
     * @name V1SessionsClaudeCredentialsUpdate
     * @summary Update Claude credentials for a session
     * @request PUT:/api/v1/sessions/{id}/claude-credentials
     * @secure
     */
    v1SessionsClaudeCredentialsUpdate: (id: string, body: TypesClaudeOAuthCredentials, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/claude-credentials`,
        method: "PUT",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Removes all interactions for a session while preserving the session record (ID, name, project, owner, model, metadata). For Zed-backed sessions the Zed thread is also reset so the agent starts fresh.
     *
     * @tags sessions
     * @name V1SessionsClearCreate
     * @summary Clear a session's conversation
     * @request POST:/api/v1/sessions/{id}/clear
     * @secure
     */
    v1SessionsClearCreate: (id: string, params: RequestParams = {}) =>
      this.request<TypesSession, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/clear`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Codex
     * @name V1SessionsCodexCredentialsDetail
     * @summary Get Codex credentials for a session
     * @request GET:/api/v1/sessions/{id}/codex-credentials
     * @secure
     */
    v1SessionsCodexCredentialsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesCodexAuthCredentials, any>({
        path: `/api/v1/sessions/${id}/codex-credentials`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Persist credentials refreshed by Codex CLI. Stale refreshes are ignored.
     *
     * @tags Codex
     * @name V1SessionsCodexCredentialsUpdate
     * @summary Update Codex credentials for a session
     * @request PUT:/api/v1/sessions/{id}/codex-credentials
     * @secure
     */
    v1SessionsCodexCredentialsUpdate: (id: string, body: TypesCodexAuthCredentials, params: RequestParams = {}) =>
      this.request<Record<string, string>, any>({
        path: `/api/v1/sessions/${id}/codex-credentials`,
        method: "PUT",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Tells the per-spec-task Zed desktop to open (foreground) the thread that belongs to THIS session, so the streamed desktop tracks the session the user is viewing. A spec task can have multiple sessions/threads sharing one desktop; the chat panel and message routing are already session-scoped, but nothing previously told the desktop to follow the selected session — so the foregrounded thread could differ from the one messages were sent to. This is session-scoped and never guesses a "latest" thread. It no-ops (200) when the session has no thread yet or the desktop WS is not connected, and crucially NEVER auto-starts a dev container (foregrounding must not boot a desktop).
     *
     * @tags Sessions
     * @name V1SessionsForegroundThreadCreate
     * @summary Foreground this session's Zed thread on the desktop
     * @request POST:/api/v1/sessions/{id}/foreground-thread
     * @secure
     */
    v1SessionsForegroundThreadCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/foreground-thread`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Creates a new session with the target agent, seeded with the parent's transcript, and pauses the parent. The parent remains as a frozen checkpoint.
     *
     * @tags sessions
     * @name V1SessionsForkCreate
     * @summary Fork a session to a different agent (fork-and-pause)
     * @request POST:/api/v1/sessions/{id}/fork
     * @secure
     */
    v1SessionsForkCreate: (id: string, request: ServerForkSessionRequest, params: RequestParams = {}) =>
      this.request<ServerForkSessionResponse, any>({
        path: `/api/v1/sessions/${id}/fork`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List interactions for a session with pagination
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
        /** Page number (0-indexed) */
        page?: number;
        /** Page size (default 100) */
        per_page?: number;
        /** Sort order: 'asc' (oldest first, default) or 'desc' (newest first) */
        order?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesPaginatedInteractions, any>({
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
     * @description Persists a Waiting interaction and dispatches it via the external-agent WebSocket. If no agent is connected the interaction is held until the agent reconnects, at which point pickupWaitingInteraction delivers it — callers do not need to manage WebSocket readiness or retries. Distinct from POST /sessions/chat (synchronous SSE chat); use this endpoint for fire-and-forget delivery to an external (e.g. desktop) agent.
     *
     * @tags Sessions
     * @name V1SessionsMessagesCreate
     * @summary Queue a message to a session's external agent
     * @request POST:/api/v1/sessions/{id}/messages
     * @secure
     */
    v1SessionsMessagesCreate: (id: string, request: ServerSessionMessageRequest, params: RequestParams = {}) =>
      this.request<ServerSessionMessageResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/messages`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns the last interaction's response for a session
     *
     * @tags Sessions
     * @name V1SessionsOutputDetail
     * @summary Get session output
     * @request GET:/api/v1/sessions/{id}/output
     * @secure
     */
    v1SessionsOutputDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesSessionOutputResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/output`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sessions
     * @name V1SessionsPreviewTokensDetail
     * @summary List session preview tokens
     * @request GET:/api/v1/sessions/{id}/preview-tokens
     * @secure
     */
    v1SessionsPreviewTokensDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesVHostRoute[], any>({
        path: `/api/v1/sessions/${id}/preview-tokens`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Mints a share-<adj>-<noun>-<8hex> hostname pointing at the session's container on the given port.
     *
     * @tags Sessions
     * @name V1SessionsPreviewTokensCreate
     * @summary Mint a preview token for a session port
     * @request POST:/api/v1/sessions/{id}/preview-tokens
     * @secure
     */
    v1SessionsPreviewTokensCreate: (id: string, body: ServerMintPreviewTokenRequest, params: RequestParams = {}) =>
      this.request<TypesVHostRoute, any>({
        path: `/api/v1/sessions/${id}/preview-tokens`,
        method: "POST",
        body: body,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sessions
     * @name V1SessionsPreviewTokensDelete
     * @summary Revoke a session preview token
     * @request DELETE:/api/v1/sessions/{id}/preview-tokens/{token_id}
     * @secure
     */
    v1SessionsPreviewTokensDelete: (id: string, tokenId: string, params: RequestParams = {}) =>
      this.request<Record<string, boolean>, any>({
        path: `/api/v1/sessions/${id}/preview-tokens/${tokenId}`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags Sessions
     * @name V1SessionsPreviewTokensRotateCreate
     * @summary Rotate a session preview token hostname
     * @request POST:/api/v1/sessions/{id}/preview-tokens/{token_id}/rotate
     * @secure
     */
    v1SessionsPreviewTokensRotateCreate: (id: string, tokenId: string, params: RequestParams = {}) =>
      this.request<TypesVHostRoute, any>({
        path: `/api/v1/sessions/${id}/preview-tokens/${tokenId}/rotate`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get streaming connection details for accessing a session
     *
     * @tags sessions
     * @name V1SessionsRdpConnectionDetail
     * @summary Get streaming connection info for a session
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
     * @description Tears down the half-dead desktop container and brings up a fresh one via the resume path. The session's ZedThreadID is cleared so Zed opens a fresh thread: a crash often poisons the thread itself, so reattaching reproduces the wedge; use /sessions/{id}/resume to restart while keeping the thread. The workspace volume persists, so files and the agent's own state survive; only the conversation thread resets. Crashed prompts are reset to pending and the queue is kicked so they re-dispatch on the new container. Requires the session to be an external Zed agent. Returns the count of prompts that were reset.
     *
     * @tags Sessions
     * @name V1SessionsRestartAgentCreate
     * @summary Restart the external agent after an in-container crash
     * @request POST:/api/v1/sessions/{id}/restart-agent
     * @secure
     */
    v1SessionsRestartAgentCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/restart-agent`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Restarts the external agent container for a session that has been stopped
     *
     * @tags sessions
     * @name V1SessionsResumeCreate
     * @summary Resume a paused external agent session
     * @request POST:/api/v1/sessions/{id}/resume
     * @secure
     */
    v1SessionsResumeCreate: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, any>, any>({
        path: `/api/v1/sessions/${id}/resume`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Returns the current sandbox state for an external agent session (absent/running/starting)
     *
     * @tags Sessions
     * @name V1SessionsSandboxStateDetail
     * @summary Get sandbox state for a session
     * @request GET:/api/v1/sessions/{id}/sandbox-state
     * @secure
     */
    v1SessionsSandboxStateDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerSessionSandboxStateResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/sandbox-state`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Search for interactions within a session by content
     *
     * @tags Sessions
     * @name V1SessionsSearchDetail
     * @summary Search session interactions
     * @request GET:/api/v1/sessions/{id}/search
     * @secure
     */
    v1SessionsSearchDetail: (
      id: string,
      query: {
        /** Search query */
        q: string;
        /** Max results (default 10) */
        limit?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<ServerSessionTOCResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/search`,
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
     * @description Stop the external Zed agent for any session (stops container, keeps session record)
     *
     * @tags Sessions
     * @name V1SessionsStopExternalAgentDelete
     * @summary Stop external Zed agent session
     * @request DELETE:/api/v1/sessions/{id}/stop-external-agent
     * @secure
     */
    v1SessionsStopExternalAgentDelete: (id: string, params: RequestParams = {}) =>
      this.request<Record<string, string>, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/stop-external-agent`,
        method: "DELETE",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Switches the agentic framework on the SAME session without forking or restarting the container. Rewrites Zed's config to the new agent, which Zed hot-reloads live (MCP context servers reconcile without a process restart), then repopulates a fresh thread with the prior transcript. Falls back to a clean Zed restart only if the live reload doesn't take.
     *
     * @tags sessions
     * @name V1SessionsSwitchAgentCreate
     * @summary Switch the agent framework on a running session in place
     * @request POST:/api/v1/sessions/{id}/switch-agent
     * @secure
     */
    v1SessionsSwitchAgentCreate: (id: string, request: ServerSwitchAgentRequest, params: RequestParams = {}) =>
      this.request<ServerSwitchAgentResponse, any>({
        path: `/api/v1/sessions/${id}/switch-agent`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns a numbered list of interaction summaries for a session
     *
     * @tags Sessions
     * @name V1SessionsTocDetail
     * @summary Get session table of contents
     * @request GET:/api/v1/sessions/{id}/toc
     * @secure
     */
    v1SessionsTocDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerSessionTOCResponse, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/toc`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns a specific interaction with surrounding context
     *
     * @tags Sessions
     * @name V1SessionsTurnsDetail
     * @summary Get interaction by turn number
     * @request GET:/api/v1/sessions/{id}/turns/{turn}
     * @secure
     */
    v1SessionsTurnsDetail: (id: string, turn: number, params: RequestParams = {}) =>
      this.request<ServerInteractionWithContext, SystemHTTPError>({
        path: `/api/v1/sessions/${id}/turns/${turn}`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Used by the fork-confirm modal so we can show "N files will be committed & pushed" or just proceed silently when the workspace is clean. Aborts gracefully on unreachable containers — the frontend treats that as "unknown".
     *
     * @tags sessions
     * @name V1SessionsWorkspaceStatusDetail
     * @summary Check uncommitted / unpushed git state in a session's desktop container
     * @request GET:/api/v1/sessions/{id}/workspace-status
     * @secure
     */
    v1SessionsWorkspaceStatusDetail: (id: string, params: RequestParams = {}) =>
      this.request<ServerWorkspaceStatusResponse, any>({
        path: `/api/v1/sessions/${id}/workspace-status`,
        method: "GET",
        secure: true,
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
     * @description Returns the union of helix-managed and user-side MCP context_servers, for the session "MCP Tools" panel in the UI. Other Zed settings (agent.*, language_models, theme) are owned by the daemon — anything that needs the full Zed view goes through the settings-sync-daemon on /zed-config + a local merge.
     *
     * @tags Zed
     * @name V1SessionsZedSettingsDetail
     * @summary Get merged Zed MCP context_servers for a session
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
     * @description List spec-driven tasks with optional filtering by project, status, or user
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksList
     * @summary List spec-driven tasks
     * @request GET:/api/v1/spec-tasks
     */
    v1SpecTasksList: (
      query: {
        /** Project ID */
        project_id: string;
        /** Filter by status */
        status?: string;
        /** Filter by user ID */
        user_id?: string;
        /**
         * Include archived tasks
         * @default false
         */
        include_archived?: boolean;
        /**
         * Include depends on tasks
         * @default false
         */
        with_depends_on?: boolean;
        /** Filter by labels (comma-separated, AND semantics) */
        labels?: string;
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
     * @description Approve the implementation and instruct agent to merge to main branch
     *
     * @tags spec-tasks
     * @name V1SpecTasksApproveImplementationCreate
     * @summary Approve implementation and merge to main
     * @request POST:/api/v1/spec-tasks/{spec_task_id}/approve-implementation
     * @secure
     */
    v1SpecTasksApproveImplementationCreate: (specTaskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTask, any>({
        path: `/api/v1/spec-tasks/${specTaskId}/approve-implementation`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description List all design reviews for a spec task
     *
     * @tags spec-tasks
     * @name V1SpecTasksDesignReviewsDetail
     * @summary List design reviews
     * @request GET:/api/v1/spec-tasks/{spec_task_id}/design-reviews
     * @secure
     */
    v1SpecTasksDesignReviewsDetail: (specTaskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskDesignReviewListResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get a specific design review for a spec task with comments and spec task details
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignReviewsDetail2
     * @summary Get design review details
     * @request GET:/api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}
     * @originalName v1SpecTasksDesignReviewsDetail
     * @duplicate
     * @secure
     */
    v1SpecTasksDesignReviewsDetail2: (specTaskId: string, reviewId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskDesignReviewDetailResponse, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the current comment being processed and the queue of pending comments for a review
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignReviewsCommentQueueStatusDetail
     * @summary Get comment queue status
     * @request GET:/api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comment-queue-status
     * @secure
     */
    v1SpecTasksDesignReviewsCommentQueueStatusDetail: (
      specTaskId: string,
      reviewId: string,
      params: RequestParams = {},
    ) =>
      this.request<TypesCommentQueueStatusResponse, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}/comment-queue-status`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all comments for a specific design review
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignReviewsCommentsDetail
     * @summary List design review comments
     * @request GET:/api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments
     * @secure
     */
    v1SpecTasksDesignReviewsCommentsDetail: (specTaskId: string, reviewId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskDesignReviewCommentListResponse, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}/comments`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Create a new comment on a design review document
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignReviewsCommentsCreate
     * @summary Create design review comment
     * @request POST:/api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments
     * @secure
     */
    v1SpecTasksDesignReviewsCommentsCreate: (
      specTaskId: string,
      reviewId: string,
      request: TypesSpecTaskDesignReviewCommentCreateRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskDesignReviewComment, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}/comments`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Mark a design review comment as resolved
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignReviewsCommentsResolveCreate
     * @summary Resolve design review comment
     * @request POST:/api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments/{comment_id}/resolve
     * @secure
     */
    v1SpecTasksDesignReviewsCommentsResolveCreate: (
      specTaskId: string,
      reviewId: string,
      commentId: string,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskDesignReviewComment, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}/comments/${commentId}/resolve`,
        method: "POST",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Approve or request changes for a design review
     *
     * @tags SpecTasks
     * @name V1SpecTasksDesignReviewsSubmitCreate
     * @summary Submit design review decision
     * @request POST:/api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/submit
     * @secure
     */
    v1SpecTasksDesignReviewsSubmitCreate: (
      specTaskId: string,
      reviewId: string,
      request: TypesSpecTaskDesignReviewSubmitRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskDesignReview, SystemHTTPError>({
        path: `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}/submit`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Stop the running agent session for a spec task
     *
     * @tags spec-tasks
     * @name V1SpecTasksStopAgentCreate
     * @summary Stop agent session
     * @request POST:/api/v1/spec-tasks/{spec_task_id}/stop-agent
     * @secure
     */
    v1SpecTasksStopAgentCreate: (specTaskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTask, any>({
        path: `/api/v1/spec-tasks/${specTaskId}/stop-agent`,
        method: "POST",
        secure: true,
        ...params,
      }),

    /**
     * @description Delete a spec task
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksDelete
     * @summary Delete a spec task
     * @request DELETE:/api/v1/spec-tasks/{taskId}
     * @secure
     */
    v1SpecTasksDelete: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}`,
        method: "DELETE",
        secure: true,
        type: ContentType.Json,
        format: "json",
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
     * @description Archive a spec task to hide it from the main view, or unarchive to restore it
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksArchivePartialUpdate
     * @summary Archive or unarchive a spec task
     * @request PATCH:/api/v1/spec-tasks/{taskId}/archive
     * @secure
     */
    v1SpecTasksArchivePartialUpdate: (
      taskId: string,
      request: TypesSpecTaskArchiveRequest,
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/archive`,
        method: "PATCH",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksAttachmentsDetail
     * @summary List attachments for a spec task
     * @request GET:/api/v1/spec-tasks/{taskId}/attachments
     * @secure
     */
    v1SpecTasksAttachmentsDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesSpecTaskAttachment[], any>({
        path: `/api/v1/spec-tasks/${taskId}/attachments`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Upload one or more files (images, PDFs, text) to be made available to the agent.
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksAttachmentsCreate
     * @summary Upload attachments for a spec task
     * @request POST:/api/v1/spec-tasks/{taskId}/attachments
     * @secure
     */
    v1SpecTasksAttachmentsCreate: (
      taskId: string,
      data: {
        /** Files to attach (multipart form data, field 'files') */
        files: File;
        /** Optional caption for the attachment (single file uploads only) */
        caption?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTaskAttachment[], TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/attachments`,
        method: "POST",
        body: data,
        secure: true,
        type: ContentType.FormData,
        format: "json",
        ...params,
      }),

    /**
     * No description
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksAttachmentsDelete
     * @summary Delete a spec task attachment
     * @request DELETE:/api/v1/spec-tasks/{taskId}/attachments/{attId}
     * @secure
     */
    v1SpecTasksAttachmentsDelete: (taskId: string, attId: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/v1/spec-tasks/${taskId}/attachments/${attId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * No description
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksAttachmentsContentDetail
     * @summary Stream the bytes of a spec task attachment
     * @request GET:/api/v1/spec-tasks/{taskId}/attachments/{attId}/content
     * @secure
     */
    v1SpecTasksAttachmentsContentDetail: (taskId: string, attId: string, params: RequestParams = {}) =>
      this.request<File, any>({
        path: `/api/v1/spec-tasks/${taskId}/attachments/${attId}/content`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Clone a spec task (with its prompt, spec, and plan) to other projects
     *
     * @tags SpecTasks
     * @name V1SpecTasksCloneCreate
     * @summary Clone a spec task to multiple projects
     * @request POST:/api/v1/spec-tasks/{taskId}/clone
     * @secure
     */
    v1SpecTasksCloneCreate: (taskId: string, request: TypesCloneTaskRequest, params: RequestParams = {}) =>
      this.request<TypesCloneTaskResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/clone`,
        method: "POST",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get all clone groups where this task was the source
     *
     * @tags SpecTasks
     * @name V1SpecTasksCloneGroupsDetail
     * @summary List clone groups for a task
     * @request GET:/api/v1/spec-tasks/{taskId}/clone-groups
     * @secure
     */
    v1SpecTasksCloneGroupsDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<TypesCloneGroup[], any>({
        path: `/api/v1/spec-tasks/${taskId}/clone-groups`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Adds a label to a spec task (idempotent - no error if label already exists)
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksLabelsCreate
     * @summary Add a label to a spec task
     * @request POST:/api/v1/spec-tasks/{taskId}/labels
     */
    v1SpecTasksLabelsCreate: (taskId: string, request: ServerAddLabelRequest, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/labels`,
        method: "POST",
        body: request,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Removes a label from a spec task (no-op if label does not exist)
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksLabelsDelete
     * @summary Remove a label from a spec task
     * @request DELETE:/api/v1/spec-tasks/{taskId}/labels/{label}
     */
    v1SpecTasksLabelsDelete: (taskId: string, label: string, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/labels/${label}`,
        method: "DELETE",
        format: "json",
        ...params,
      }),

    /**
     * @description Get detailed progress information for a spec-driven task including specification and implementation phases
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksProgressDetail
     * @summary Get spec-driven task progress
     * @request GET:/api/v1/spec-tasks/{taskId}/progress
     */
    v1SpecTasksProgressDetail: (taskId: string, params: RequestParams = {}) =>
      this.request<ServerTaskProgressResponse, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/progress`,
        method: "GET",
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
     * @description Explicitly start spec generation (planning phase) for a backlog task. This transitions the task to planning status and starts a spec generation session.
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksStartPlanningCreate
     * @summary Start planning for a SpecTask
     * @request POST:/api/v1/spec-tasks/{taskId}/start-planning
     * @secure
     */
    v1SpecTasksStartPlanningCreate: (
      taskId: string,
      query?: {
        /** XKB keyboard layout code (e.g., 'us', 'fr', 'de') - for testing browser locale detection */
        keyboard?: string;
        /** IANA timezone (e.g., 'Europe/Paris') - for testing browser locale detection */
        timezone?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/${taskId}/start-planning`,
        method: "POST",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get spec task usage
     *
     * @tags spec-tasks
     * @name V1SpecTasksUsageDetail
     * @summary Get spec task usage
     * @request GET:/api/v1/spec-tasks/{taskId}/usage
     * @secure
     */
    v1SpecTasksUsageDetail: (
      taskId: string,
      query?: {
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
        /** Aggregation level */
        aggregation_level?: string;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesAggregatedUsageMetric[], SystemHTTPError>({
        path: `/api/v1/spec-tasks/${taskId}/usage`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
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
     * @description Get the Kanban board settings (WIP limits) for the default project
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksBoardSettingsList
     * @summary Get board settings for spec tasks
     * @request GET:/api/v1/spec-tasks/board-settings
     * @secure
     */
    v1SpecTasksBoardSettingsList: (params: RequestParams = {}) =>
      this.request<TypesBoardSettings, TypesAPIError>({
        path: `/api/v1/spec-tasks/board-settings`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update the Kanban board settings (WIP limits) for the default project
     *
     * @tags spec-driven-tasks
     * @name V1SpecTasksBoardSettingsUpdate
     * @summary Update board settings for spec tasks
     * @request PUT:/api/v1/spec-tasks/board-settings
     * @secure
     */
    v1SpecTasksBoardSettingsUpdate: (request: TypesBoardSettings, params: RequestParams = {}) =>
      this.request<TypesBoardSettings, TypesAPIError>({
        path: `/api/v1/spec-tasks/board-settings`,
        method: "PUT",
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
    v1SpecTasksFromPromptCreate: (request: TypesCreateTaskRequest, params: RequestParams = {}) =>
      this.request<TypesSpecTask, TypesAPIError>({
        path: `/api/v1/spec-tasks/from-prompt`,
        method: "POST",
        body: request,
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
     * @description Per-user status: credits, admin flag, slug, user config, plus the licence payload (moved here from /api/v1/config so it is not disclosed unauthenticated).
     *
     * @tags config
     * @name V1StatusList
     * @summary Get user status
     * @request GET:/api/v1/status
     * @secure
     */
    v1StatusList: (params: RequestParams = {}) =>
      this.request<TypesUserStatus, any>({
        path: `/api/v1/status`,
        method: "GET",
        secure: true,
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
        /** Custom return URL path (e.g. /onboarding) */
        return_url?: string;
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
     * @description Process incoming activities from Microsoft Teams Bot Framework
     *
     * @tags Teams
     * @name V1TeamsWebhookCreate
     * @summary Handle Teams webhook
     * @request POST:/api/v1/teams/webhook/{appID}
     */
    v1TeamsWebhookCreate: (appId: string, params: RequestParams = {}) =>
      this.request<string, string>({
        path: `/api/v1/teams/webhook/${appId}`,
        method: "POST",
        type: ContentType.Json,
        format: "json",
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
        /** Project ID */
        project_id?: string;
        /** Spec Task ID */
        spec_task_id?: string;
        /** Aggregation level */
        aggregation_level?: string;
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
     * @description Get organization usage summary with breakdowns by user, project, app, session, task/model, and model/provider
     *
     * @tags usage
     * @name V1UsageOrgSummaryList
     * @summary Get organization usage summary
     * @request GET:/api/v1/usage/org-summary
     * @secure
     */
    v1UsageOrgSummaryList: (
      query: {
        /** Organization ID */
        org_id: string;
        /** Start date */
        from?: string;
        /** End date */
        to?: string;
        /** User ID */
        user_id?: string;
        /** Project ID */
        project_id?: string;
        /** App ID */
        app_id?: string;
        /** Session ID */
        session_id?: string;
        /** Provider */
        provider?: string;
        /** Model */
        model?: string;
        /** User search */
        user_search?: string;
        /** User page size */
        user_limit?: number;
        /** User page offset */
        user_offset?: number;
        /** Project page size */
        project_limit?: number;
        /** Project page offset */
        project_offset?: number;
        /** Task page size */
        task_limit?: number;
        /** Task page offset */
        task_offset?: number;
        /** Session page size */
        session_limit?: number;
        /** Session page offset */
        session_offset?: number;
      },
      params: RequestParams = {},
    ) =>
      this.request<TypesOrgUsageSummaryResponse, SystemHTTPError>({
        path: `/api/v1/usage/org-summary`,
        method: "GET",
        query: query,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description List users with pagination support and optional filtering by email domain or username. Supports ILIKE matching for email domains (e.g., "hotmail.com" will find all users with @hotmail.com emails) and partial username matching. Pass `query` to match across email, username, and full_name in one go.
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
        /** Free-text search across email, username, and full_name (ILIKE) */
        query?: string;
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
     * @description Create a new user with the specified details. Only admins can create users.
     *
     * @tags users
     * @name V1UsersCreate
     * @summary Create a new user (Admin only)
     * @request POST:/api/v1/users
     * @secure
     */
    v1UsersCreate: (request: TypesAdminCreateUserRequest, params: RequestParams = {}) =>
      this.request<TypesUser, any>({
        path: `/api/v1/users`,
        method: "POST",
        body: request,
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
     * @description Returns an overview of a user's activity: projects owned, spec tasks created, per-model inference usage, and an effective last-active timestamp combining tracked auth activity with usage-metric data.
     *
     * @tags users
     * @name V1UsersStatsDetail
     * @summary Get user stats (admin only)
     * @request GET:/api/v1/users/{id}/stats
     * @secure
     */
    v1UsersStatsDetail: (id: string, params: RequestParams = {}) =>
      this.request<TypesUserStatsResponse, any>({
        path: `/api/v1/users/${id}/stats`,
        method: "GET",
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the current user's default chat settings (applied when chatting without an app)
     *
     * @tags Users
     * @name V1UsersMeChatSettingsList
     * @summary Get user chat settings
     * @request GET:/api/v1/users/me/chat-settings
     * @secure
     */
    v1UsersMeChatSettingsList: (params: RequestParams = {}) =>
      this.request<TypesUserChatSettings, SystemHTTPError>({
        path: `/api/v1/users/me/chat-settings`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update the current user's default chat settings (system prompt + LLM parameters used when chatting without an app)
     *
     * @tags Users
     * @name V1UsersMeChatSettingsUpdate
     * @summary Update user chat settings
     * @request PUT:/api/v1/users/me/chat-settings
     * @secure
     */
    v1UsersMeChatSettingsUpdate: (request: TypesUserChatSettings, params: RequestParams = {}) =>
      this.request<TypesUserChatSettings, SystemHTTPError>({
        path: `/api/v1/users/me/chat-settings`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Set the user's UI color scheme. Propagates instantly to GNOME and Zed in any spec-task sessions owned by this user.
     *
     * @tags Users
     * @name V1UsersMeColorSchemeUpdate
     * @summary Update user color scheme preference
     * @request PUT:/api/v1/users/me/color-scheme
     * @secure
     */
    v1UsersMeColorSchemeUpdate: (request: TypesUpdateUserColorSchemeRequest, params: RequestParams = {}) =>
      this.request<TypesUpdateUserColorSchemeRequest, SystemHTTPError>({
        path: `/api/v1/users/me/color-scheme`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the current user's personal workspace guidelines
     *
     * @tags Users
     * @name V1UsersMeGuidelinesList
     * @summary Get user guidelines
     * @request GET:/api/v1/users/me/guidelines
     * @secure
     */
    v1UsersMeGuidelinesList: (params: RequestParams = {}) =>
      this.request<TypesUserGuidelinesResponse, SystemHTTPError>({
        path: `/api/v1/users/me/guidelines`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Update the current user's personal workspace guidelines
     *
     * @tags Users
     * @name V1UsersMeGuidelinesUpdate
     * @summary Update user guidelines
     * @request PUT:/api/v1/users/me/guidelines
     * @secure
     */
    v1UsersMeGuidelinesUpdate: (request: TypesUpdateUserGuidelinesRequest, params: RequestParams = {}) =>
      this.request<TypesUserGuidelinesResponse, SystemHTTPError>({
        path: `/api/v1/users/me/guidelines`,
        method: "PUT",
        body: request,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the version history of the current user's personal workspace guidelines
     *
     * @tags Users
     * @name V1UsersMeGuidelinesHistoryList
     * @summary Get user guidelines history
     * @request GET:/api/v1/users/me/guidelines-history
     * @secure
     */
    v1UsersMeGuidelinesHistoryList: (params: RequestParams = {}) =>
      this.request<TypesGuidelinesHistory[], SystemHTTPError>({
        path: `/api/v1/users/me/guidelines-history`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Mark onboarding as completed for the current user
     *
     * @tags Users
     * @name V1UsersMeOnboardingCreate
     * @summary Complete onboarding
     * @request POST:/api/v1/users/me/onboarding
     * @secure
     */
    v1UsersMeOnboardingCreate: (params: RequestParams = {}) =>
      this.request<TypesUser, SystemHTTPError>({
        path: `/api/v1/users/me/onboarding`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Get the list of project IDs pinned by the current user
     *
     * @tags Users
     * @name V1UsersMePinnedProjectsList
     * @summary Get pinned project IDs
     * @request GET:/api/v1/users/me/pinned-projects
     * @secure
     */
    v1UsersMePinnedProjectsList: (params: RequestParams = {}) =>
      this.request<ServerPinnedProjectsResponse, SystemHTTPError>({
        path: `/api/v1/users/me/pinned-projects`,
        method: "GET",
        secure: true,
        format: "json",
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
     * @description List models from a specific provider, or aggregate from all providers if none specified. If the request includes an anthropic-version header, proxies to the upstream Anthropic provider.
     *
     * @tags models
     * @name ModelsList
     * @summary List models
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
