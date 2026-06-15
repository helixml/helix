package config

import (
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
)

type ServerConfig struct {
	Inference          Inference
	Providers          Providers
	Tools              Tools
	Auth               Auth
	Notifications      Notifications
	Janitor            Janitor
	Stripe             Stripe
	DataPrepText       DataPrepText
	TextExtractor      TextExtractor
	RAG                RAG
	Controller         Controller
	FileStore          FileStore
	Store              Store
	PGVectorStore      PGVectorStore
	PubSub             PubSub
	WebServer          WebServer
	SubscriptionQuotas SubscriptionQuotas
	GitHub             GitHub
	FineTuning         FineTuning
	Apps               Apps
	Triggers           Triggers
	Search             Search
	Kodit              Kodit
	SSL                SSL
	Organizations      Organizations
	Sandboxes          Sandboxes
	Compute            Compute

	// DesktopIdleTimeout is how long a desktop can be inactive before it is automatically shut down.
	// Inactivity is measured as the time since the last interaction was created or updated
	// across all sessions belonging to the desktop.
	DesktopIdleTimeout time.Duration `envconfig:"HELIX_DESKTOP_IDLE_TIMEOUT" default:"1h"`

	// DesktopIdleCheckInterval controls how often the idle checker scans for desktops to shut down.
	DesktopIdleCheckInterval time.Duration `envconfig:"HELIX_DESKTOP_IDLE_CHECK_INTERVAL" default:"5m"`

	// SandboxReaperInterval is how often the sandbox-instance reaper scans
	// the sandbox_instances table for stale rows and flips their status to
	// offline. Used in conjunction with SandboxStaleThreshold and
	// SandboxDispatchStaleThreshold.
	SandboxReaperInterval time.Duration `envconfig:"HELIX_SANDBOX_REAPER_INTERVAL" default:"1m"`

	// SandboxStaleThreshold is the inactivity threshold after which a
	// sandbox row is flipped to status="offline" by the reaper. Should
	// comfortably exceed the heartbeat cadence so transient lag doesn't
	// reap a healthy Runner.
	SandboxStaleThreshold time.Duration `envconfig:"HELIX_SANDBOX_STALE_THRESHOLD" default:"5m"`

	// SandboxDispatchStaleThreshold is the tighter freshness filter
	// applied by FindAvailableSandboxInstance when selecting a Runner
	// for new work. Set lower than SandboxStaleThreshold so freshly-dead
	// Runners are excluded from dispatch well before the reaper flips
	// their DB row.
	SandboxDispatchStaleThreshold time.Duration `envconfig:"HELIX_SANDBOX_DISPATCH_STALE_THRESHOLD" default:"90s"`

	DisableLLMCallLogging bool `envconfig:"DISABLE_LLM_CALL_LOGGING" default:"false"`
	DisableUsageLogging   bool `envconfig:"DISABLE_USAGE_LOGGING" default:"false"`
	DisableVersionPing    bool `envconfig:"DISABLE_VERSION_PING" default:"false"`

	// License key for deployment identification
	LicenseKey string `envconfig:"LICENSE_KEY"`
	// Launchpad URL for version pings
	LaunchpadURL string `envconfig:"LAUNCHPAD_URL" default:"https://deploy.helix.ml"`
	// Edition identifies the deployment type (e.g., "mac-desktop", "server", "cloud")
	Edition string `envconfig:"HELIX_EDITION" default:""`

	SBMessage string `envconfig:"SB_MESSAGE" default:""`

	// HelixOrgEnabled is the deployment-wide kill switch for the
	// embedded helix-org alpha. When false (the default), none of
	// the helix-org init runs and none of its HTTP surfaces
	// (/api/v1/orgs/{org}/, /api/v1/mcp/helix-org/) are mounted —
	// the per-user alpha feature flag in the DB has no effect. Set
	// HELIX_ORG_ENABLED=true to opt in.
	HelixOrgEnabled bool `envconfig:"HELIX_ORG_ENABLED" default:"false"`
}

// Sandboxes configures the user-facing Sandboxes API.
//
// Each entry in Runtimes maps a runtime name (the value users put in
// `runtime` on POST /sandboxes) to a container image and the command that
// keeps the container alive. Operators can add new runtimes (e.g. node22,
// python3.13) by extending HELIX_SANDBOX_RUNTIMES without changing code.
//
// AllowCustomImage controls whether end users may pass an arbitrary `image`
// in CreateSandboxRequest, which bypasses the curated runtime list. Off by
// default — turn on for trusted single-tenant deployments.
type Sandboxes struct {
	// Runtimes is a comma-separated list of runtime specs, each of the form
	//   <name>=<image>[|<entrypoint and cmd shell expression>]
	// Examples:
	//   headless-ubuntu=ubuntu:22.04|sleep infinity
	//   node22=node:22-bookworm-slim|tail -f /dev/null
	//   python313=python:3.13-slim|tail -f /dev/null
	// If the trailing `|...` is omitted the default keep-alive command
	// (`tail -f /dev/null`) is used.
	Runtimes string `envconfig:"HELIX_SANDBOX_RUNTIMES" default:"headless-ubuntu=ubuntu:22.04|sleep infinity,node22=node:22-bookworm-slim|tail -f /dev/null,python313=python:3.13-slim|tail -f /dev/null"`

	// AllowCustomImage lets API callers pass an arbitrary image name in the
	// create request. False blocks anything outside the configured runtimes.
	AllowCustomImage bool `envconfig:"HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE" default:"false"`

	// DefaultRuntime is the runtime applied when the create request omits
	// both `runtime` and `image`. Must match one of the names in Runtimes.
	DefaultRuntime string `envconfig:"HELIX_SANDBOX_DEFAULT_RUNTIME" default:"headless-ubuntu"`
}

// Compute configures the cloud-provisioning side of Helix's sandbox
// host management. When Provider is empty (the default) the entire
// subsystem is disabled - no Provider is constructed, no reconcile
// loop runs, no SandboxInstance rows are auto-created. Self-registered
// hosts (the legacy path) continue to work unchanged.
//
// When Provider is set, Helix constructs the named compute.Provider
// from the rest of this section + the provider-specific config block
// (e.g. Yellowdog) and starts a compute.Manager reconcile loop.
type Compute struct {
	// Provider selects which compute.Provider implementation Helix
	// uses to bring sandbox hosts into existence. Empty (default)
	// disables the whole subsystem. Currently the only supported
	// value is "yellowdog".
	Provider string `envconfig:"HELIX_COMPUTE_PROVIDER" default:""`

	// DeploymentTag distinguishes work requirements created by this
	// Helix install from WRs created by other tooling (e.g. someone
	// running yd-submit directly against the same YD account) or by
	// another Helix install that happens to share the YD namespace.
	// It is the primary filter applied to YD's List endpoint.
	//
	// Auto-derived from the provider-specific namespace at boot when
	// unset (e.g. "helix-<namespace>"). For the common deployment
	// (one Helix install per YD namespace) the default is sufficient.
	// Operators running multiple Helix installs in the same YD
	// namespace MUST set this explicitly per install or the two
	// Managers will see each other's WRs as their own.
	//
	// Also forms the suffix of the value written to
	// SandboxInstance.Provider (e.g. "yellowdog-prod"). The two
	// purposes share one knob.
	DeploymentTag string `envconfig:"HELIX_COMPUTE_DEPLOYMENT_TAG" default:""`

	// Floor is the minimum number of provisioned hosts the Manager
	// keeps available at all times. The reconcile loop kicks off
	// Provision calls until (Ready + Provisioning) reaches this count.
	// Zero (the default) disables pre-warming - the Manager exists
	// but does no work in floor-only mode. On-demand scaling lands
	// in a follow-up; for now, Floor=0 means "Manager is a no-op".
	Floor int `envconfig:"HELIX_COMPUTE_FLOOR" default:"0"`

	// ReconcileInterval is how often the Manager's reconcile loop
	// runs. Lower values respond faster to drift; higher values
	// reduce pressure on Helix and the upstream Provider API. 30s
	// is a reasonable default matching the existing sandbox
	// heartbeat cadence.
	ReconcileInterval time.Duration `envconfig:"HELIX_COMPUTE_RECONCILE_INTERVAL" default:"30s"`

	// HealthCheckTimeout caps how long one Provider.HealthCheck call
	// can take per provisioning row before the loop moves on.
	HealthCheckTimeout time.Duration `envconfig:"HELIX_COMPUTE_HEALTHCHECK_TIMEOUT" default:"10s"`

	// MaxConcurrentProvisions caps how many Provision calls the
	// Manager fires per reconcile cycle when below Floor. Default 1.
	// Raise this when bringing up a large Floor on cold boot;
	// MaxConcurrentProvisions=5 with ReconcileInterval=30s reaches
	// Floor=5 in one cycle instead of five.
	MaxConcurrentProvisions int `envconfig:"HELIX_COMPUTE_MAX_CONCURRENT_PROVISIONS" default:"1"`

	// MaxProvisioningAge bounds how long a row may sit in
	// ComputeState=provisioning before the Manager rolls it back.
	// Default 30m - covers the YD g5.xlarge happy path (~10m) plus
	// headroom for cross-region fallback and slow NVIDIA image pulls.
	MaxProvisioningAge time.Duration `envconfig:"HELIX_COMPUTE_MAX_PROVISIONING_AGE" default:"30m"`

	// Max is the hard ceiling on Manager-owned hosts (Ready +
	// Provisioning combined). Zero (the default) disables on-demand
	// scale-up - the Manager only maintains Floor and ignores demand
	// pressure. Set Max > Floor to allow the Manager to provision
	// extra hosts when sandbox-session demand exhausts the headroom
	// on existing hosts. Must be >= Floor when non-zero.
	Max int `envconfig:"HELIX_COMPUTE_MAX" default:"0"`

	// ScaleUpHeadroomMin is the minimum number of free sandbox slots
	// the Manager tries to keep available across all Ready hosts.
	// When (sum(MaxSandboxes) - sum(ActiveSandboxes)) drops below this
	// value AND total owned is below Max, the Manager provisions
	// an additional host. Default 1 (provision when 0 slots remain)
	// when Max > Floor; ignored when Max = 0 (D3 disabled).
	//
	// Operators serving bursty workloads can raise this (e.g. to 2-3)
	// to provision the next host before the last slot is claimed,
	// hiding the ~90s cold-start latency from the user.
	ScaleUpHeadroomMin int `envconfig:"HELIX_COMPUTE_SCALEUP_HEADROOM_MIN" default:"0"`

	// Yellowdog is the provider-specific config block. Only consulted
	// when Provider="yellowdog".
	Yellowdog Yellowdog
}

// Yellowdog is the YellowDog-provider-specific configuration block.
// All fields are required when Compute.Provider="yellowdog"; an empty
// value at boot causes Helix to fail fast rather than start with a
// half-configured Manager.
type Yellowdog struct {
	// APIKeyID and APISecret are the YD account credentials. Generate
	// them in the YD portal under Applications. Treat as secrets.
	APIKeyID  string `envconfig:"HELIX_YD_KEY"`
	APISecret string `envconfig:"HELIX_YD_SECRET"`

	// BaseURL overrides the production API endpoint. Leave unset for
	// the public portal at https://portal.yellowdog.co/api.
	BaseURL string `envconfig:"HELIX_YD_BASE_URL" default:""`

	// Namespace is the YD namespace work requirements live in. Match
	// the namespace your YD account administrator allocated for this
	// Helix install.
	Namespace string `envconfig:"HELIX_YD_NAMESPACE" default:""`

	// WorkerTag is the tag the operator-provisioned YD worker pool
	// advertises. Tasks include this in their RunSpecification so
	// the YD scheduler only assigns them to matching workers.
	//
	// Auto-derived from Namespace when unset:
	//   WorkerTag = "worker-" + Namespace
	//
	// Matches the yd-provision POC convention (`worker_tag =
	// "worker-{{tag}}"`), so an operator who set up their pool
	// per the POC docs gets working defaults. Override via
	// HELIX_YD_WORKER_TAG when the pool was created with a
	// different naming scheme.
	//
	// Mismatch between this value and the pool's advertised tag
	// produces silent "tasks starved" failures rather than a
	// clear error - the YD scheduler simply finds no eligible
	// workers and leaves the task pending. Boot logs the resolved
	// tag so the operator can spot a mismatch quickly.
	WorkerTag string `envconfig:"HELIX_YD_WORKER_TAG" default:""`

	// TaskTimeout bounds individual task runtime upstream-side. The
	// platform aborts the task and records TaskError type=TIMED_OUT
	// when exceeded. 4h matches the POC's safety circuit-breaker.
	TaskTimeout time.Duration `envconfig:"HELIX_YD_TASK_TIMEOUT" default:"4h"`

	// MaxRetries caps retry attempts for idempotent YD API requests
	// (GET, PUT, DELETE). POST is never retried.
	MaxRetries int `envconfig:"HELIX_YD_MAX_RETRIES" default:"3"`
}

func LoadServerConfig() (ServerConfig, error) {
	var cfg ServerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return ServerConfig{}, err
	}
	if cfg.Notifications.AppURL == "" {
		cfg.Notifications.AppURL = cfg.WebServer.URL
	}
	return cfg, nil
}

type Inference struct {
	Provider string `envconfig:"INFERENCE_PROVIDER" default:"helix" description:"One of helix, openai, or togetherai"`

	DefaultContextLimit int `envconfig:"INFERENCE_DEFAULT_CONTEXT_LIMIT" default:"10" description:"The default context limit for inference."`
}

type Search struct {
	SearXNGBaseURL string `envconfig:"SEARCH_SEARXNG_BASE_URL" default:"http://searxng:8080"`
}

type Kodit struct {
	Enabled      bool   `envconfig:"KODIT_ENABLED" default:"false"`
	DatabaseURL  string `envconfig:"KODIT_DB_URL" default:""`                    // PostgreSQL+VectorChord DSN (required)
	DataDir      string `envconfig:"KODIT_DATA_DIR" default:""`                  // For cloned repos, defaults to {filestore}/kodit
	ModelDir     string `envconfig:"KODIT_MODEL_DIR" default:""`                 // ONNX embedding model directory, defaults to {dataDir}/models
	WorkerCount  int    `envconfig:"KODIT_WORKER_COUNT" default:"1"`             // Number of background enrichment workers
	LLMBaseURL   string `envconfig:"KODIT_LLM_BASE_URL" default:""`              // OpenAI-compatible endpoint for enrichments
	LLMAPIKey    string `envconfig:"KODIT_LLM_API_KEY" default:""`               // API key for LLM endpoint
	LLMChatModel string `envconfig:"KODIT_LLM_CHAT_MODEL" default:"kodit-model"` // LLM model name
	// Text embedding (proxied through Helix via the "kodit-text-embedding" special model name).
	// When BaseURL is set (or falls back to LLMBaseURL), kodit uses an external OpenAI-compatible
	// embedding provider instead of the local ONNX model.
	TextEmbeddingBaseURL string `envconfig:"KODIT_TEXT_EMBEDDING_BASE_URL" default:""`                  // Defaults to LLMBaseURL
	TextEmbeddingAPIKey  string `envconfig:"KODIT_TEXT_EMBEDDING_API_KEY" default:""`                   // Defaults to LLMAPIKey
	TextEmbeddingModel   string `envconfig:"KODIT_TEXT_EMBEDDING_MODEL" default:"kodit-text-embedding"` // Placeholder model name sent to Helix
	// Vision embedding (proxied through Helix via the "kodit-vision-embedding" special model name).
	// When BaseURL is set (or falls back to LLMBaseURL), kodit uses an external vision embedding
	// provider (e.g. Qwen3-VL-Embedding) instead of the local SigLIP2 model.
	VisionEmbeddingBaseURL string `envconfig:"KODIT_VISION_EMBEDDING_BASE_URL" default:""`                    // Defaults to LLMBaseURL
	VisionEmbeddingAPIKey  string `envconfig:"KODIT_VISION_EMBEDDING_API_KEY" default:""`                     // Defaults to LLMAPIKey
	VisionEmbeddingModel   string `envconfig:"KODIT_VISION_EMBEDDING_MODEL" default:"kodit-vision-embedding"` // Placeholder model name sent to Helix
	// GitURL is the URL Kodit uses to access the git server (for cloning local repos)
	// Defaults to http://api:8080 for Docker Compose, but may differ in Kubernetes or local dev
	GitURL string `envconfig:"KODIT_GIT_URL" default:"http://api:8080"`
}

// Providers is used to configure the various AI providers that we use
type Providers struct {
	OpenAI                    OpenAI
	TogetherAI                TogetherAI
	Anthropic                 Anthropic
	Helix                     Helix
	VLLM                      VLLM
	EnableCustomUserProviders bool   `envconfig:"ENABLE_CUSTOM_USER_PROVIDERS" default:"false"` // Allow users to configure their own providers, if "false" then only admins can add them
	DynamicProviders          string `envconfig:"DYNAMIC_PROVIDERS"`                            // Format: "provider1:api_key1:base_url1,provider2:api_key2:base_url2"
	BillingEnabled            bool   `envconfig:"PROVIDERS_BILLING_ENABLED" default:"false"`    // Enable usage tracking/billing for built-in providers (from env vars)
}

type OpenAI struct {
	BaseURL               string        `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`
	APIKey                string        `envconfig:"OPENAI_API_KEY"`
	APIKeyFromFile        string        `envconfig:"OPENAI_API_KEY_FILE"` // i.e. /run/secrets/openai-api-key
	APIKeyRefreshInterval time.Duration `envconfig:"OPENAI_API_KEY_REFRESH_INTERVAL" default:"3s"`
	Models                []string      `envconfig:"OPENAI_MODELS"` // If set, only these models will be used
}

type VLLM struct {
	BaseURL               string        `envconfig:"VLLM_BASE_URL"`
	APIKey                string        `envconfig:"VLLM_API_KEY"`
	APIKeyFromFile        string        `envconfig:"VLLM_API_KEY_FILE"` // i.e. /run/secrets/vllm-api-key
	APIKeyRefreshInterval time.Duration `envconfig:"VLLM_API_KEY_REFRESH_INTERVAL" default:"3s"`
	Models                []string      `envconfig:"VLLM_MODELS"` // If set, only these models will be used
}

type TogetherAI struct {
	BaseURL               string        `envconfig:"TOGETHER_BASE_URL" default:"https://api.together.xyz/v1"`
	APIKey                string        `envconfig:"TOGETHER_API_KEY"`
	APIKeyFromFile        string        `envconfig:"TOGETHER_API_KEY_FILE"` // i.e. /run/secrets/together-api-key
	APIKeyRefreshInterval time.Duration `envconfig:"TOGETHER_API_KEY_REFRESH_INTERVAL" default:"3s"`
	Models                []string      `envconfig:"TOGETHER_MODELS"` // If set, only these models will be used
}

type Anthropic struct {
	BaseURL               string        `envconfig:"ANTHROPIC_BASE_URL" default:"https://api.anthropic.com/v1"`
	APIKey                string        `envconfig:"ANTHROPIC_API_KEY"`
	APIKeyFromFile        string        `envconfig:"ANTHROPIC_API_KEY_FILE"` // i.e. /run/secrets/anthropic-api-key
	APIKeyRefreshInterval time.Duration `envconfig:"ANTHROPIC_API_KEY_REFRESH_INTERVAL" default:"3s"`
	Models                []string      `envconfig:"ANTHROPIC_MODELS"` // If set, only these models will be used

	// Google Vertex AI configuration for Anthropic models
	// When VertexProjectID is set, all Anthropic inference traffic routes through Vertex AI
	// instead of the direct Anthropic API. Vertex wins unconditionally — ANTHROPIC_API_KEY
	// is only used for model listing if also set.
	VertexProjectID       string `envconfig:"ANTHROPIC_VERTEX_PROJECT_ID"`
	VertexRegion          string `envconfig:"ANTHROPIC_VERTEX_REGION" default:"global"`
	VertexCredentialsJSON string `envconfig:"ANTHROPIC_VERTEX_CREDENTIALS_JSON"` // service account JSON string; takes precedence over VertexCredentialsFile
	VertexCredentialsFile string `envconfig:"ANTHROPIC_VERTEX_CREDENTIALS_FILE"` // path to service account JSON; empty = Application Default Credentials

	// OAuth configuration for Claude subscription login flow
	// Users can connect their Claude Pro/Max subscription via OAuth
	OAuthClientID     string `envconfig:"ANTHROPIC_OAUTH_CLIENT_ID"`
	OAuthClientSecret string `envconfig:"ANTHROPIC_OAUTH_CLIENT_SECRET"`
}

type Helix struct {
	OwnerID   string `envconfig:"TOOLS_PROVIDER_HELIX_OWNER_ID" default:"helix-internal"` // Will be used for sesions
	OwnerType string `envconfig:"TOOLS_PROVIDER_HELIX_OWNER_TYPE" default:"system"`       // Will be used for sesions
}

type Tools struct {
	Enabled bool `envconfig:"TOOLS_ENABLED" default:"true"` // Enable/disable tools for the server

	TLSSkipVerify bool `envconfig:"TOOLS_TLS_SKIP_VERIFY" default:"false"`

	// Suggestions based on provider (now set by INFERENCE_PROVIDER):
	// - OpenAI: gpt-4-1106-preview
	// - Together AI: openai/gpt-oss-20b
	// - Helix: llama3:instruct
	Model string `envconfig:"TOOLS_MODEL" default:"llama3:instruct"`

	// IsActionableTemplate is used to determine whether Helix should
	// use a tool or not. Leave empty for default
	IsActionableTemplate      string `envconfig:"TOOLS_IS_ACTIONABLE_TEMPLATE"`                   // Either plain text, base64 or path to a file
	IsActionableHistoryLength int    `envconfig:"TOOLS_IS_ACTIONABLE_HISTORY_LENGTH" default:"4"` // 2 assistant messages, 2 user messages
}

type Auth struct {
	Provider            types.AuthProvider `envconfig:"AUTH_PROVIDER" default:"regular"`
	RegistrationEnabled bool               `envconfig:"AUTH_REGISTRATION_ENABLED" default:"true"`
	OIDC                OIDC
	Regular             Regular
	Waitlist            bool `envconfig:"AUTH_WAITLIST_ENABLED" default:"false"`

	// DesktopAutoLoginSecret is a shared secret that enables automatic admin login
	// for the Helix Desktop app. When set, GET /api/v1/auth/desktop-callback?token=<secret>
	// creates an admin session and redirects to /.
	DesktopAutoLoginSecret string `envconfig:"DESKTOP_AUTO_LOGIN_SECRET"`
}

type Regular struct {
	Enabled       bool          `envconfig:"REGULAR_AUTH_ENABLED" default:"true"`
	TokenValidity time.Duration `envconfig:"REGULAR_AUTH_TOKEN_VALIDITY" default:"168h"` // 7 days
	JWTSecret     string        `envconfig:"REGULAR_AUTH_JWT_SECRET" default:"helix-default-jwt-secret"`
}

type OIDC struct {
	// SecureCookies forces the Secure flag on auth cookies when set to true.
	// When false (default), secure cookies are auto-detected from SERVER_URL protocol.
	// Set to true to force secure cookies even when SERVER_URL is HTTP (e.g., behind HTTPS proxy).
	SecureCookies bool   `envconfig:"OIDC_SECURE_COOKIES" default:"false"`
	URL           string `envconfig:"OIDC_URL" default:"http://localhost:8080/auth/realms/helix"`
	ClientID      string `envconfig:"OIDC_CLIENT_ID" default:"api"`
	ClientSecret  string `envconfig:"OIDC_CLIENT_SECRET"`
	Audience      string `envconfig:"OIDC_AUDIENCE"`
	Scopes        string `envconfig:"OIDC_SCOPES" default:"openid,profile,email"`
	// ExpectedIssuer allows using a different issuer than the OIDC_URL.
	// Useful when the OIDC provider returns a browser-accessible issuer (e.g., localhost:8180)
	// that differs from the discovery URL.
	ExpectedIssuer string `envconfig:"OIDC_EXPECTED_ISSUER"`
	// TokenURL overrides the token endpoint from OIDC discovery.
	// Useful when the API needs to reach the token endpoint via an internal URL
	// (e.g., http://keycloak:8080/auth/realms/helix/protocol/openid-connect/token)
	// while the discovery response contains a browser-accessible URL (localhost:8180).
	TokenURL string `envconfig:"OIDC_TOKEN_URL"`
	// OfflineAccess requests offline access for refresh tokens (access_type=offline).
	// Required for Google OIDC to return refresh tokens, which allow sessions to persist
	// beyond the 1-hour access token lifetime. Enabled by default since it's harmless for
	// providers that don't need it (like Keycloak) and required for providers that do (like Google).
	OfflineAccess bool `envconfig:"OIDC_OFFLINE_ACCESS" default:"true"`
	// CookieMaxAge sets the max age for auth cookies in seconds.
	// Default is 0 (session cookie - expires when browser closes).
	// Set to a positive value (e.g., 604800 = 7 days) to persist sessions across browser restarts.
	// This is especially useful with Google OIDC + OIDC_OFFLINE_ACCESS=true, as the refresh token
	// will be preserved and can obtain new access tokens even after the browser is closed.
	CookieMaxAge int `envconfig:"OIDC_COOKIE_MAX_AGE" default:"0"`
}

// Notifications is used for sending notifications to users when certain events happen
// such as finetuning starting or completing.
type Notifications struct {
	// AppURL is the public-facing base URL used in user-visible links (Slack/email/etc).
	// When unset, falls back to WebServer.URL (SERVER_URL) — see LoadServerConfig.
	AppURL string `envconfig:"APP_URL"`
	Email  EmailConfig
	// Agent progress notifications (Slack/Teams threads with screenshots)
	AgentNotifications AgentNotificationsConfig
}

// AgentNotificationsConfig configures agent status notifications to Slack/Teams
type AgentNotificationsConfig struct {
	// Slack configuration for agent progress updates
	Slack SlackNotificationsConfig
	// Teams configuration for agent progress updates
	Teams TeamsNotificationsConfig
}

type SlackNotificationsConfig struct {
	Enabled    bool   `envconfig:"AGENT_NOTIFICATIONS_SLACK_ENABLED" default:"false"`
	WebhookURL string `envconfig:"AGENT_NOTIFICATIONS_SLACK_WEBHOOK_URL" description:"Slack incoming webhook URL for agent notifications"`
	BotToken   string `envconfig:"AGENT_NOTIFICATIONS_SLACK_BOT_TOKEN" description:"Slack bot token for interactive messages (optional)"`
	Channel    string `envconfig:"AGENT_NOTIFICATIONS_SLACK_CHANNEL" description:"Default Slack channel for agent notifications"`
}

type TeamsNotificationsConfig struct {
	Enabled    bool   `envconfig:"AGENT_NOTIFICATIONS_TEAMS_ENABLED" default:"false"`
	WebhookURL string `envconfig:"AGENT_NOTIFICATIONS_TEAMS_WEBHOOK_URL" description:"Teams incoming webhook URL for agent notifications"`
}

type EmailConfig struct {
	SenderAddress string `envconfig:"EMAIL_SENDER_ADDRESS" default:"no-reply@helix.ml"`

	AgentSkillSenderAddress string `envconfig:"EMAIL_AGENT_SKILL_SENDER_ADDRESS" default:"no-reply@helix.ml"`

	SMTP struct {
		Host     string `envconfig:"EMAIL_SMTP_HOST"`
		Port     string `envconfig:"EMAIL_SMTP_PORT"`
		Identity string `envconfig:"EMAIL_SMTP_IDENTITY"`
		Username string `envconfig:"EMAIL_SMTP_USERNAME"`
		Password string `envconfig:"EMAIL_SMTP_PASSWORD"`
	}

	Mailgun struct {
		Domain string `envconfig:"EMAIL_MAILGUN_DOMAIN"`
		APIKey string `envconfig:"EMAIL_MAILGUN_API_KEY"`
		Europe bool   `envconfig:"EMAIL_MAILGUN_EUROPE" default:"false"` // use EU region
	}
}

type Janitor struct {
	AppURL                  string
	SentryDsnAPI            string   `envconfig:"SENTRY_DSN_API" description:"The api sentry DSN."`
	SentryDsnFrontend       string   `envconfig:"SENTRY_DSN_FRONTEND" description:"The frontend sentry DSN."`
	GoogleAnalyticsFrontend string   `envconfig:"GOOGLE_ANALYTICS_FRONTEND" description:"The frontend Google analytics id."`
	SlackWebhookURL         string   `envconfig:"JANITOR_SLACK_WEBHOOK_URL" description:"The slack webhook URL to ping messages to."`
	SlackIgnoreUser         []string `envconfig:"JANITOR_SLACK_IGNORE_USERS" description:"Ignore keycloak user ids for slack messages."`
	RudderStackWriteKey     string   `envconfig:"RUDDERSTACK_WRITE_KEY" description:"The write key for rudderstack."`
	RudderStackDataPlaneURL string   `envconfig:"RUDDERSTACK_DATA_PLANE_URL" description:"The data plane URL for rudderstack."`
}

type Stripe struct {
	BillingEnabled bool `envconfig:"STRIPE_BILLING_ENABLED" default:"false" description:"Whether to enable billing."`

	RequireActiveSubscription bool `envconfig:"STRIPE_BILLING_REQUIRE_ACTIVE_SUBSCRIPTION" default:"false" description:"Whether require an active subscription before allowing to use the product"` //

	MinimumInferenceBalance float64 `envconfig:"STRIPE_MINIMUM_INFERENCE_BALANCE" default:"0.01" description:"Minimum balance required for an inference call."`
	InitialBalance          float64 `envconfig:"STRIPE_INITIAL_BALANCE" default:"0" description:"The initial balance seeded into a newly created wallet. Defaults to 0 so new signups arrive with an empty wallet; on-prem deployments that want to hand out free trial credits can set this to a positive value."`

	AppURL               string
	SecretKey            string `envconfig:"STRIPE_SECRET_KEY" description:"The secret key for stripe."`
	WebhookSigningSecret string `envconfig:"STRIPE_WEBHOOK_SIGNING_SECRET" description:"The webhook signing secret for stripe."`
	PriceLookupKey       string `envconfig:"STRIPE_PRICE_LOOKUP_KEY" default:"helix-subscription" description:"The lookup key for the stripe price."`
	OrgPriceLookupKey    string `envconfig:"STRIPE_ORG_PRICE_LOOKUP_KEY" default:"helix-org-subscription" description:"The lookup key for the stripe price."`
}

type DataPrepText struct {
	Module            types.DataPrepModule `envconfig:"DATA_PREP_TEXT_MODULE" default:"dynamic" description:"Which module to use for text data prep."`
	OverflowSize      int                  `envconfig:"DATA_PREP_TEXT_OVERFLOW_SIZE" default:"256" description:"The overflow size for the text data prep."`
	QuestionsPerChunk int                  `envconfig:"DATA_PREP_TEXT_QUESTIONS_PER_CHUNK" default:"30" description:"The questions per chunk for the text data prep."`
	Temperature       float32              `envconfig:"DATA_PREP_TEXT_TEMPERATURE" default:"0.5" description:"The temperature for the text data prep prompt."`
}

type TextExtractor struct {
	URL string `envconfig:"TEXT_EXTRACTION_URL" default:"http://llamaindex:5000/api/v1/extract" description:"The URL to extract text from a document."`
}

// RAGProviderName is the string stamped into KnowledgeVersion records to
// identify which backend indexed them. Kodit is the only RAG backend.
const RAGProviderName = "kodit"

// Sandbox reaper defaults. Use when ServerConfig values are zero (e.g.
// the field was never explicitly configured and envconfig left it
// unparsed). Keep in sync with the `default:` tags above.
var (
	DefaultSandboxReaperInterval         = time.Minute
	DefaultSandboxStaleThreshold         = 5 * time.Minute
	DefaultSandboxDispatchStaleThreshold = 90 * time.Second
)

type RAG struct {
	IndexingConcurrency int `envconfig:"RAG_INDEXING_CONCURRENCY" default:"1" description:"The number of concurrent indexing tasks."`

	MaxVersions int `envconfig:"RAG_MAX_VERSIONS" default:"3" description:"The maximum number of versions to keep for a knowledge."`

	// EmbeddingsProvider is the default provider used by the /v1/embeddings
	// proxy when the caller sends a raw model name (not a placeholder like
	// "kodit-text-embedding" or "kodit-vision-embedding"). Placeholder-model
	// requests resolve the provider from SystemSettings instead.
	EmbeddingsProvider string `envconfig:"RAG_EMBEDDINGS_PROVIDER" default:"openai" description:"Default provider for direct /v1/embeddings calls with raw model names. One of openai, togetherai, vllm, helix."`

	Llamaindex struct {
		// the URL we can post a chunk of text to for RAG indexing
		RAGIndexingURL string `envconfig:"RAG_INDEX_URL" default:"http://llamaindex:5000/api/v1/rag/chunk" description:"The URL to index text with RAG."`
		// the URL we can post a prompt to to match RAG records
		RAGQueryURL string `envconfig:"RAG_QUERY_URL" default:"http://llamaindex:5000/api/v1/rag/query" description:"The URL to query RAG records."`
		// the URL we can post a delete request to for RAG records,
		// this is a prefix, full path is http://llamaindex:5000/api/v1/rag/<data_entity_id>
		RAGDeleteURL string `envconfig:"RAG_DELETE_URL" default:"http://llamaindex:5000/api/v1/rag" description:"The URL to delete RAG records."`
	}

	Crawler struct {
		ChromeURL       string `envconfig:"RAG_CRAWLER_CHROME_URL" default:"http://chrome:9222" description:"The URL to the Chrome instance."`
		LauncherEnabled bool   `envconfig:"RAG_CRAWLER_LAUNCHER_ENABLED" default:"true" description:"Whether to use the Launcher to start the browser."`
		LauncherURL     string `envconfig:"RAG_CRAWLER_LAUNCHER_URL" default:"http://chrome:7317" description:"The URL to the Launcher instance."`
		BrowserPoolSize int    `envconfig:"RAG_CRAWLER_BROWSER_POOL_SIZE" default:"5" description:"The number of browsers to keep in the pool."`
		PagePoolSize    int    `envconfig:"RAG_CRAWLER_PAGE_POOL_SIZE" default:"50" description:"The number of pages to keep in the pool."`

		// Limits
		MaxFrequency time.Duration `envconfig:"RAG_CRAWLER_MAX_FREQUENCY" default:"60m" description:"The maximum frequency to crawl."`
		MaxDepth     int           `envconfig:"RAG_CRAWLER_MAX_DEPTH" default:"100" description:"The maximum depth to crawl."`
	}
}

type Controller struct {
	FilestorePresignSecret string `envconfig:"FILESTORE_PRESIGN_SECRET" description:""`
	// this is an "env" prefix like "dev"
	// the user prefix is handled inside the controller
	// (see getFilestorePath)
	FilePrefixGlobal string `envconfig:"FILE_PREFIX_GLOBAL" default:"dev" description:"The global prefix path for the filestore."`
	// this is a golang template that is used to prefix the user
	// path in the filestore - it is passed Owner and OwnerType values
	// write me an example FilePrefixUser as a go template
	// e.g. "users/{{.Owner}}"
	// FilePrefixUser string `envconfig:"FILE_PREFIX_USER" default:"users/{{.Owner}}" description:"The go template that produces the prefix path for a user."`

	// a static path used to denote what sub-folder job results live in
	FilePrefixResults string `envconfig:"FILE_PREFIX_RESULTS" default:"results" description:"The go template that produces the prefix path for a user."`

	// how many scheduler decisions to buffer before we start dropping them
	SchedulingDecisionBufferSize int `envconfig:"SCHEDULING_DECISION_BUFFER_SIZE" default:"10" description:"How many scheduling decisions to buffer before we start dropping them."`
}

type FileStore struct {
	Type         types.FileStoreType `envconfig:"FILESTORE_TYPE" default:"fs" description:"What type of filestore should we use (fs | gcs)."`
	LocalFSPath  string              `envconfig:"FILESTORE_LOCALFS_PATH" default:"/tmp/helix/filestore" description:"The local path that is the root for the local fs filestore."`
	AvatarsPath  string              `envconfig:"FILESTORE_AVATARS_PATH" default:"/filestore/avatars" description:"The local path that is the root for the avatars filestore."`
	GCSKeyBase64 string              `envconfig:"FILESTORE_GCS_KEY_BASE64" description:"The base64 encoded service account json file for GCS."`
	GCSKeyFile   string              `envconfig:"FILESTORE_GCS_KEY_FILE" description:"The local path to the service account json file for GCS."`
	GCSBucket    string              `envconfig:"FILESTORE_GCS_BUCKET" description:"The bucket we are storing things in GCS."`
}

type PubSub struct {
	StoreDir string `envconfig:"NATS_STORE_DIR" default:"/filestore/nats" description:"The directory to store nats data."`
	Provider string `envconfig:"PUBSUB_PROVIDER" default:"nats" description:"The pubsub provider to use (nats or inmemory)."`
	Server   struct {
		EmbeddedNatsServerEnabled bool   `envconfig:"NATS_SERVER_EMBEDDED_ENABLED" default:"true" description:"Whether to enable the embedded NATS server."`
		Host                      string `envconfig:"NATS_SERVER_HOST" default:"127.0.0.1" description:"The host to bind the NATS server to."`
		Port                      int    `envconfig:"NATS_SERVER_PORT" default:"4222" description:"The port to bind the NATS server to."`
		WebsocketPort             int    `envconfig:"NATS_SERVER_WEBSOCKET_PORT" default:"8433" description:"The websocket port used as a proxy to the NATS server."`
		Token                     string `envconfig:"NATS_SERVER_TOKEN" description:"The authentication token for the NATS server."`
		MaxPayload                int    `envconfig:"NATS_SERVER_MAX_PAYLOAD" default:"33554432" description:"The maximum payload size in bytes (default 32MB)."`
		JetStream                 bool   `envconfig:"NATS_SERVER_JETSTREAM" default:"true" description:"Whether to enable JetStream."`
	}
}

type Store struct {
	Host     string `envconfig:"POSTGRES_HOST" description:"The host to connect to the postgres server."`
	Port     int    `envconfig:"POSTGRES_PORT" default:"5432" description:"The port to connect to the postgres server."`
	Database string `envconfig:"POSTGRES_DATABASE" default:"helix" description:"The database to connect to the postgres server."`
	Username string `envconfig:"POSTGRES_USER" description:"The username to connect to the postgres server."`
	Password string `envconfig:"POSTGRES_PASSWORD" description:"The password to connect to the postgres server."`
	SSL      bool   `envconfig:"POSTGRES_SSL" default:"false"`
	Schema   string `envconfig:"POSTGRES_SCHEMA"` // Defaults to public

	AutoMigrate     bool          `envconfig:"DATABASE_AUTO_MIGRATE" default:"true" description:"Should we automatically run the migrations?"`
	MaxConns        int           `envconfig:"DATABASE_MAX_CONNS" default:"50"`
	IdleConns       int           `envconfig:"DATABASE_IDLE_CONNS" default:"25"`
	MaxConnLifetime time.Duration `envconfig:"DATABASE_MAX_CONN_LIFETIME" default:"1h"`
	MaxConnIdleTime time.Duration `envconfig:"DATABASE_MAX_CONN_IDLE_TIME" default:"1m"`

	SeedModels bool `envconfig:"DATABASE_SEED_MODELS" default:"true" description:"Should we seed the models?"`
}

type PGVectorStore struct {
	Host     string `envconfig:"PGVECTOR_HOST" default:"pgvector" description:"The host to connect to the postgres server."`
	Port     int    `envconfig:"PGVECTOR_PORT" default:"5432" description:"The port to connect to the postgres server."`
	Database string `envconfig:"PGVECTOR_DATABASE" default:"postgres" description:"The database to connect to the postgres server."`
	Username string `envconfig:"PGVECTOR_USER" default:"postgres" description:"The username to connect to the postgres server."`
	Password string `envconfig:"PGVECTOR_PASSWORD" default:"postgres" description:"The password to connect to the postgres server."`
	SSL      bool   `envconfig:"PGVECTOR_SSL" default:"false"`
	Schema   string `envconfig:"PGVECTOR_SCHEMA"` // Defaults to public

	AutoMigrate     bool          `envconfig:"PGVECTOR_AUTO_MIGRATE" default:"true" description:"Should we automatically run the migrations?"`
	MaxConns        int           `envconfig:"PGVECTOR_MAX_CONNS" default:"50"`
	IdleConns       int           `envconfig:"PGVECTOR_IDLE_CONNS" default:"25"`
	MaxConnLifetime time.Duration `envconfig:"PGVECTOR_MAX_CONN_LIFETIME" default:"1h"`
	MaxConnIdleTime time.Duration `envconfig:"PGVECTOR_MAX_CONN_IDLE_TIME" default:"1m"`
}

type WebServer struct {
	URL  string `envconfig:"SERVER_URL" description:"The URL the api server is listening on."`
	Host string `envconfig:"SERVER_HOST" default:"0.0.0.0" description:"The host to bind the api server to."`
	Port int    `envconfig:"SERVER_PORT" default:"80" description:""`
	// Can either be a URL to frontend (for dev proxy) or a path to static files (for prod)
	// Default is dev proxy; Dockerfile sets FRONTEND_URL=/www for production
	FrontendURL string `envconfig:"FRONTEND_URL" default:"http://frontend:8081" description:"URL to proxy to or filesystem path to serve from"`

	RunnerToken string `envconfig:"RUNNER_TOKEN" description:"The token for runner auth."`
	// Comma-separated list of user IDs that should be admins, or "all" for dev mode.
	// If empty, admin status is determined by the user's admin field in the database.
	// Examples: "all", "user-123,user-456", ""
	AdminUserIDs []string `envconfig:"ADMIN_USER_IDS" description:"Comma-separated list of admin user IDs, or 'all' for dev mode. Empty uses database admin field."`
	// if this is specified then we provide the option to clone entire
	// sessions into this user without having to logout and login
	EvalUserID string `envconfig:"EVAL_USER_ID" description:""`
	// this is for when we are running localfs filesystem
	// and we need to add a route to view files based on their path
	// we are assuming all file storage is open right now
	// so we just deep link to the object path and don't apply auth
	// (this is so helix nodes can see files)
	// later, we might add a token to the URLs
	// LocalFilestorePath string `envconfig:"LOCAL_FILESTORE_PATH"`

	// Path to UNIX socket for serving embeddings without auth
	// TODO: naming
	EmbeddingsSocket       string `envconfig:"HELIX_EMBEDDINGS_SOCKET" description:"Path to UNIX socket for serving embeddings without auth. If set, a UNIX socket server will be started."`
	EmbeddingsSocketUserID string `envconfig:"HELIX_EMBEDDINGS_SOCKET_USER_ID" description:"The user ID to use for the UNIX socket server."`

	ModelsCacheTTL time.Duration `envconfig:"MODELS_CACHE_TTL" default:"1m" description:"The TTL for the models cache."`

	// DevSubdomain enables subdomain-based virtual hosting for dev container ports.
	// Format: "dev.helix.example.com" (full domain) or "dev" (uses SERVER_URL domain).
	// When enabled, requests to p{port}-{session_id}.dev.domain.com are proxied to the session's port.
	// Example: p8080-ses_abc123.dev.helix.example.com → session ses_abc123, port 8080
	DevSubdomain string `envconfig:"DEV_SUBDOMAIN" description:"Subdomain prefix for dev container port proxying. Format: 'dev' or 'dev.helix.example.com'"`

	// SandboxAPIURL is the URL that sandbox containers use to connect back to the API.
	// This is needed when the main SERVER_URL goes through a reverse proxy that doesn't
	// support HTTP hijacking (used by RevDial). If not set, defaults to SERVER_URL.
	// Example: http://api-internal.example.com:8080 (direct HTTP, bypassing Caddy)
	SandboxAPIURL string `envconfig:"SANDBOX_API_URL" description:"Direct API URL for sandbox containers (bypasses reverse proxy). Defaults to SERVER_URL if not set."`

	// VHostTLSMode controls embedded TLS termination for project web
	// services and sandbox preview tokens. "off" (default) means Helix
	// listens HTTP only and a reverse proxy in front terminates TLS.
	// "auto" enables certmagic — Helix binds :443 + :80 and issues
	// per-hostname Let's Encrypt certs on demand for any hostname
	// registered in vhost_routes or matching SERVER_URL.
	VHostTLSMode string `envconfig:"HELIX_VHOST_TLS_MODE" default:"off" description:"TLS termination mode for vhost-routed traffic: 'off' (rely on upstream) or 'auto' (embed certmagic + Let's Encrypt)."`

	// VHostLetsEncryptEmail is the ACME registration email used by
	// certmagic when VHostTLSMode=auto. Required in that mode.
	VHostLetsEncryptEmail string `envconfig:"HELIX_VHOST_LETSENCRYPT_EMAIL" description:"ACME registration email used by certmagic when HELIX_VHOST_TLS_MODE=auto."`

	// VHostACMEDNSProvider selects a DNS-01 challenge provider for
	// certmagic when VHostTLSMode=auto. Empty (the default) uses the
	// network challenges (HTTP-01 + TLS-ALPN-01). Set to "cloudflare"
	// when running behind a Cloudflare proxy (orange-cloud DNS), where
	// the network challenges cannot reach Helix.
	VHostACMEDNSProvider string `envconfig:"HELIX_VHOST_ACME_DNS_PROVIDER" description:"DNS-01 challenge provider for certmagic when HELIX_VHOST_TLS_MODE=auto. Empty=use HTTP-01+TLS-ALPN-01. Supported: cloudflare."`

	// VHostCloudflareAPIToken is the Cloudflare API token used when
	// VHostACMEDNSProvider=cloudflare. Must be an API token (not a
	// legacy global API key) with Zone:Zone:Read + Zone:DNS:Edit
	// permissions on the zones Helix issues certs for.
	VHostCloudflareAPIToken string `envconfig:"HELIX_VHOST_CLOUDFLARE_API_TOKEN" description:"Cloudflare API token (Zone:Zone:Read + Zone:DNS:Edit) used when HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare."`
}

// AdminAllUsers is the special value for ADMIN_USER_IDS that makes all users admins
const AdminAllUsers = "all"

type SubscriptionQuotas struct {
	Enabled    bool `envconfig:"SUBSCRIPTION_QUOTAS_ENABLED" default:"true"`
	Finetuning struct {
		Free struct {
			Strict        bool `envconfig:"SUBSCRIPTION_QUOTAS_FINETUNING_FREE_STRICT" default:"true"` // If set, will now allow any finetuning if the user is over quota
			MaxConcurrent int  `envconfig:"SUBSCRIPTION_QUOTAS_FINETUNING_FREE_MAX_CONCURRENT" default:"1"`
			MaxChunks     int  `envconfig:"SUBSCRIPTION_QUOTAS_FINETUNING_FREE_MAX_CHUNKS" default:"5"`
		}
		Pro struct {
			Strict        bool `envconfig:"SUBSCRIPTION_QUOTAS_FINETUNING_PRO_STRICT" default:"false"` // If set, will now allow any finetuning if the user is over quota
			MaxConcurrent int  `envconfig:"SUBSCRIPTION_QUOTAS_FINETUNING_PRO_MAX_CONCURRENT" default:"3"`
			MaxChunks     int  `envconfig:"SUBSCRIPTION_QUOTAS_FINETUNING_PRO_MAX_CHUNKS" default:"100"`
		}
	}
	Inference struct {
		Enabled bool `envconfig:"SUBSCRIPTION_QUOTAS_INFERENCE_ENABLED" default:"false"` // Must be explicitly enabled
		Free    struct {
			MaxMonthlyTokens int  `envconfig:"SUBSCRIPTION_QUOTAS_INFERENCE_FREE_MAX_MONTHLY_TOKENS" default:"50000"` // 50K tokens/month for free users
			Strict           bool `envconfig:"SUBSCRIPTION_QUOTAS_INFERENCE_FREE_STRICT" default:"true"`
		}
		Pro struct {
			MaxMonthlyTokens int  `envconfig:"SUBSCRIPTION_QUOTAS_INFERENCE_PRO_MAX_MONTHLY_TOKENS" default:"2500000"` // 2.5M tokens/month for pro users
			Strict           bool `envconfig:"SUBSCRIPTION_QUOTAS_INFERENCE_PRO_STRICT" default:"true"`
		}
	}
	Projects struct {
		Enabled bool `envconfig:"PROJECTS_ENABLED" default:"true" description:"Enable project quotas"`
		Free    struct {
			// MaxConcurrentDesktops: cap on concurrent desktop sessions for users
			// without an active Stripe subscription. Enforced per organisation
			// when the session has an org, per user otherwise. -1 = unlimited.
			MaxConcurrentDesktops int `envconfig:"PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS" default:"2"`
			MaxProjects           int `envconfig:"PROJECTS_FREE_MAX_PROJECTS" default:"3"`
			MaxRepositories       int `envconfig:"PROJECTS_FREE_MAX_REPOSITORIES" default:"3"`
			MaxSpecTasks          int `envconfig:"PROJECTS_FREE_MAX_SPEC_TASKS" default:"500"` // Non-archived/done
		}
		Pro struct {
			// MaxConcurrentDesktops: cap on concurrent desktop sessions for users
			// with an active Stripe subscription. Enforced per organisation when
			// the session has an org, per user otherwise. -1 = unlimited.
			MaxConcurrentDesktops int `envconfig:"PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS" default:"30"`
			MaxProjects           int `envconfig:"PROJECTS_PRO_MAX_PROJECTS" default:"50"`
			MaxRepositories       int `envconfig:"PROJECTS_PRO_MAX_REPOSITORIES" default:"100"`
			MaxSpecTasks          int `envconfig:"PROJECTS_PRO_MAX_SPEC_TASKS" default:"50000"` // Non-archived/done
		}
	}
}

type GitHub struct {
	Enabled      bool   `envconfig:"GITHUB_INTEGRATION_ENABLED" default:"false" description:"Enable github integration."`
	ClientID     string `envconfig:"GITHUB_INTEGRATION_CLIENT_ID" description:"The github app client id."`
	ClientSecret string `envconfig:"GITHUB_INTEGRATION_CLIENT_SECRET" description:"The github app client secret."`
	RepoFolder   string `envconfig:"GITHUB_INTEGRATION_REPO_FOLDER" default:"/filestore/github/repos" description:"What folder do we use to clone github repos."`
	WebhookURL   string `envconfig:"GITHUB_INTEGRATION_WEBHOOK_URL" description:"The URL to receive github webhooks."`
	// AppSlug is the public URL slug of this deployment's Helix GitHub App
	// (e.g. "helix-agent" → https://github.com/apps/helix-agent). NOT a
	// secret — just the public app handle used to build the install URL the
	// New Stream "Install Helix" gate opens. The app's private key lives
	// encrypted in a ServiceConnection, never here.
	AppSlug string `envconfig:"GITHUB_APP_SLUG" description:"Public slug of the Helix GitHub App used to build the install URL."`
	// URL is the base web URL of the GitHub instance: https://github.com
	// (default) or a GitHub Enterprise Server origin (e.g.
	// https://github.acme.com). It drives the app create/install/manage links
	// and points the API client at the right host for GHES customers.
	URL string `envconfig:"GITHUB_URL" default:"https://github.com" description:"Base web URL of the GitHub instance (https://github.com or a GitHub Enterprise Server origin)."`
}

// WebURL returns the GitHub web origin (no trailing slash), defaulting to
// github.com. Used to build the app create / install / manage links.
func (g GitHub) WebURL() string {
	u := strings.TrimRight(strings.TrimSpace(g.URL), "/")
	if u == "" {
		return "https://github.com"
	}
	return u
}

// APIBaseURL returns the value to hand the github client/transport: empty for
// public github.com (the client special-cases it to api.github.com), or the
// GHES origin otherwise. This is also what gets stored on a github_app
// ServiceConnection so later API calls target the right host.
func (g GitHub) APIBaseURL() string {
	if g.WebURL() == "https://github.com" {
		return ""
	}
	return g.WebURL()
}

type FineTuning struct {
	Enabled  bool           `envconfig:"FINETUNING_ENABLED" default:"true" description:"Enable QA pairs."` // Enable/disable QA pairs for the server
	Provider types.Provider `envconfig:"FINETUNING_PROVIDER" default:"togetherai" description:"Which LLM provider to use for QA pairs."`
	// Suggestions based on provider:
	// - Together AI: openai/gpt-oss-20b
	// - Helix: llama3:instruct
	QAPairGenModel string `envconfig:"FINETUNING_QA_PAIR_GEN_MODEL" default:"mistralai/Mixtral-8x7B-Instruct-v0.1" description:"Which LLM model to use for QA pairs."`
}

type Apps struct {
	Enabled  bool           `envconfig:"APPS_ENABLED" default:"true" description:"Enable apps."` // Enable/disable apps for the server
	Provider types.Provider `envconfig:"APPS_PROVIDER" default:"togetherai" description:"Which LLM provider to use for apps."`
	Model    string         `envconfig:"APPS_MODEL" default:"mistralai/Mixtral-8x7B-Instruct-v0.1" description:"Which LLM model to use for apps."` // gpt-4-1106-preview
}

type Triggers struct {
	Discord Discord
	Cron    Cron
	Slack   Slack
	Teams   Teams
	Crisp   Crisp
}

type Discord struct {
	Enabled  bool   `envconfig:"DISCORD_ENABLED" default:"false"`
	BotToken string `envconfig:"DISCORD_BOT_TOKEN"`
}

type Slack struct {
	// Optional way to disable slack triggers across all apps/agents
	Enabled bool `envconfig:"SLACK_ENABLED" default:"true"`
}

type Teams struct {
	// Optional way to disable teams triggers across all apps/agents
	Enabled bool `envconfig:"TEAMS_ENABLED" default:"true"`
}

type Crisp struct {
	Enabled bool `envconfig:"CRISP_ENABLED" default:"true"`
}

type Cron struct {
	Enabled bool `envconfig:"CRON_ENABLED" default:"true"`
}

type SSL struct {
	// certFileEnv is the environment variable which identifies where to locate
	// the SSL certificate file. If set this overrides the system default.
	SSLCertFile string `envconfig:"SSL_CERT_FILE"`

	// certDirEnv is the environment variable which identifies which directory
	// to check for SSL certificate files. If set this overrides the system default.
	// It is a colon separated list of directories.
	// See https://www.openssl.org/docs/man1.0.2/man1/c_rehash.html.
	SSLCertDir string `envconfig:"SSL_CERT_DIR"`
}

type Organizations struct {
	CreateEnabledForNonAdmins bool `envconfig:"ORGANIZATIONS_CREATE_ENABLED_FOR_NON_ADMINS" default:"true"`
}
