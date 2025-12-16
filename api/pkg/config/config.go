package config

import (
	"fmt"
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
	TURN               TURN
	ExternalAgents     ExternalAgents

	DisableLLMCallLogging bool `envconfig:"DISABLE_LLM_CALL_LOGGING" default:"false"`
	DisableUsageLogging   bool `envconfig:"DISABLE_USAGE_LOGGING" default:"false"`
	DisableVersionPing    bool `envconfig:"DISABLE_VERSION_PING" default:"false"`

	// AI Providers management - controls if users can add their own AI provider API keys
	// Disabled by default for enterprise customers who don't want users adding external API keys
	ProvidersManagementEnabled bool `envconfig:"PROVIDERS_MANAGEMENT_ENABLED" default:"false"`

	// License key for deployment identification
	LicenseKey string `envconfig:"LICENSE_KEY"`
	// Launchpad URL for version pings
	LaunchpadURL string `envconfig:"LAUNCHPAD_URL" default:"https://deploy.helix.ml"`

	SBMessage string `envconfig:"SB_MESSAGE" default:""`
}

func LoadServerConfig() (ServerConfig, error) {
	var cfg ServerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return ServerConfig{}, err
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
	BaseURL string `envconfig:"KODIT_BASE_URL" default:"http://kodit:8632"`
	APIKey  string `envconfig:"KODIT_API_KEY" default:"dev-key"`
	Enabled bool   `envconfig:"KODIT_ENABLED" default:"true"`
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
}

type Helix struct {
	OwnerID            string        `envconfig:"TOOLS_PROVIDER_HELIX_OWNER_ID" default:"helix-internal"` // Will be used for sesions
	OwnerType          string        `envconfig:"TOOLS_PROVIDER_HELIX_OWNER_TYPE" default:"system"`       // Will be used for sesions
	ModelTTL           time.Duration `envconfig:"HELIX_MODEL_TTL" default:"10s"`                          // How long to keep models warm before allowing other work to be scheduled
	SlotTTL            time.Duration `envconfig:"HELIX_SLOT_TTL" default:"600s"`                          // How long to wait for work to complete before slots are considered dead
	RunnerTTL          time.Duration `envconfig:"HELIX_RUNNER_TTL" default:"30s"`                         // How long before runners are considered dead
	SchedulingStrategy string        `envconfig:"HELIX_SCHEDULING_STRATEGY" default:"max_spread" description:"The strategy to use for scheduling workloads."`
	QueueSize          int           `envconfig:"HELIX_QUEUE_SIZE" default:"100" description:"The size of the queue when buffering workloads."`
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
	Keycloak            Keycloak
	OIDC                OIDC
	Regular             Regular
}

type Regular struct {
	Enabled       bool          `envconfig:"REGULAR_AUTH_ENABLED" default:"true"`
	TokenValidity time.Duration `envconfig:"REGULAR_AUTH_TOKEN_VALIDITY" default:"168h"` // 7 days
	JWTSecret     string        `envconfig:"REGULAR_AUTH_JWT_SECRET" default:"helix-default-jwt-secret"`
}

// Keycloak is used for authentication. You can find keycloak documentation
// at https://www.keycloak.org/guides
type Keycloak struct {
	KeycloakEnabled     bool   `envconfig:"KEYCLOAK_ENABLED" default:"false"`
	KeycloakURL         string `envconfig:"KEYCLOAK_URL" default:"http://keycloak:8080/auth"`
	KeycloakFrontEndURL string `envconfig:"KEYCLOAK_FRONTEND_URL" default:"http://localhost:8080/auth"`
	ServerURL           string `envconfig:"SERVER_URL" description:"The URL the api server is listening on."`
	APIClientID         string `envconfig:"KEYCLOAK_CLIENT_ID" default:"api"`
	ClientSecret        string `envconfig:"KEYCLOAK_CLIENT_SECRET"` // If not set, will be looked up using admin API
	AdminRealm          string `envconfig:"KEYCLOAK_ADMIN_REALM" default:"master"`
	Realm               string `envconfig:"KEYCLOAK_REALM" default:"helix"`
	Username            string `envconfig:"KEYCLOAK_USER" default:"admin"`
	Password            string `envconfig:"KEYCLOAK_PASSWORD"`
}

type OIDC struct {
	Enabled bool `envconfig:"OIDC_ENABLED" default:"false"`
	// SecureCookies forces the Secure flag on auth cookies when set to true.
	// When false (default), secure cookies are auto-detected from SERVER_URL protocol.
	// Set to true to force secure cookies even when SERVER_URL is HTTP (e.g., behind HTTPS proxy).
	SecureCookies bool   `envconfig:"OIDC_SECURE_COOKIES" default:"false"`
	URL           string `envconfig:"OIDC_URL" default:"http://localhost:8080/auth/realms/helix"`
	ClientID      string `envconfig:"OIDC_CLIENT_ID" default:"api"`
	ClientSecret  string `envconfig:"OIDC_CLIENT_SECRET"`
	Audience      string `envconfig:"OIDC_AUDIENCE"`
	Scopes        string `envconfig:"OIDC_SCOPES" default:"openid,profile,email"`
}

// Notifications is used for sending notifications to users when certain events happen
// such as finetuning starting or completing.
type Notifications struct {
	AppURL string `envconfig:"APP_URL" default:"https://app.helix.ml"`
	Email  EmailConfig
	// TODO: Slack, Discord, etc.
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
	BillingEnabled          bool    `envconfig:"STRIPE_BILLING_ENABLED" default:"false" description:"Whether to enable billing."`
	MinimumInferenceBalance float64 `envconfig:"STRIPE_MINIMUM_INFERENCE_BALANCE" default:"0.01" description:"Minimum balance required for an inference call."`
	InitialBalance          float64 `envconfig:"STRIPE_INITIAL_BALANCE" default:"10" description:"The initial balance for the wallet"`

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
	Provider types.Extractor `envconfig:"TEXT_EXTRACTION_PROVIDER" default:"tika"`
	// the URL we post documents to so we can get the text back from them
	Unstructured struct {
		URL string `envconfig:"TEXT_EXTRACTION_URL" default:"http://llamaindex:5000/api/v1/extract" description:"The URL to extract text from a document."`
	}

	Tika struct {
		URL string `envconfig:"TEXT_EXTRACTION_TIKA_URL" default:"http://tika:9998" description:"The URL to extract text from a document."`
	}
}

type RAGProvider string

const (
	RAGProviderTypesense  RAGProvider = "typesense"
	RAGProviderLlamaindex RAGProvider = "llamaindex"
	RAGProviderHaystack   RAGProvider = "haystack"
)

type RAG struct {
	IndexingConcurrency int `envconfig:"RAG_INDEXING_CONCURRENCY" default:"1" description:"The number of concurrent indexing tasks."`

	// DefaultRagProvider is the default RAG provider to use if not specified
	DefaultRagProvider RAGProvider `envconfig:"RAG_DEFAULT_PROVIDER" default:"typesense" description:"The default RAG provider to use if not specified."`

	MaxVersions int `envconfig:"RAG_MAX_VERSIONS" default:"3" description:"The maximum number of versions to keep for a knowledge."`

	// Typesense is used to store RAG records in a Typesense index
	Typesense struct {
		URL    string `envconfig:"RAG_TYPESENSE_URL" default:"http://typesense:8108" description:"The URL to the Typesense server."`
		APIKey string `envconfig:"RAG_TYPESENSE_API_KEY" default:"typesense" description:"The API key to the Typesense server."`
	}

	PGVector struct {
		Provider              string           `envconfig:"RAG_PGVECTOR_PROVIDER" default:"openai" description:"One of openai, togetherai, vllm, helix"`
		EmbeddingsModel       string           `envconfig:"RAG_PGVECTOR_EMBEDDINGS_MODEL" default:"text-embedding-3-small" description:"The model to use for embeddings."`
		EmbeddingsConcurrency int              `envconfig:"RAG_PGVECTOR_EMBEDDINGS_CONCURRENCY" default:"10" description:"The number of concurrent embeddings to create."`
		Dimensions            types.Dimensions `envconfig:"RAG_PGVECTOR_DIMENSIONS" description:"The dimensions to use for embeddings, only set for custom models. Available options are 384, 512, 1024, 3584."` // Set this if you are using custom model
	}

	Llamaindex struct {
		// the URL we can post a chunk of text to for RAG indexing
		RAGIndexingURL string `envconfig:"RAG_INDEX_URL" default:"http://llamaindex:5000/api/v1/rag/chunk" description:"The URL to index text with RAG."`
		// the URL we can post a prompt to to match RAG records
		RAGQueryURL string `envconfig:"RAG_QUERY_URL" default:"http://llamaindex:5000/api/v1/rag/query" description:"The URL to query RAG records."`
		// the URL we can post a delete request to for RAG records,
		// this is a prefix, full path is http://llamaindex:5000/api/v1/rag/<data_entity_id>
		RAGDeleteURL string `envconfig:"RAG_DELETE_URL" default:"http://llamaindex:5000/api/v1/rag" description:"The URL to delete RAG records."`
	}

	Haystack struct {
		Enabled bool   `envconfig:"RAG_HAYSTACK_ENABLED" default:"false" description:"Whether to enable Haystack RAG."`
		URL     string `envconfig:"RAG_HAYSTACK_URL" default:"http://localhost:8000" description:"The URL to the Haystack service."`
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
	// Can either be a URL to frontend or a path to static files
	FrontendURL string `envconfig:"FRONTEND_URL" default:"http://frontend:8081" description:""`

	RunnerToken string `envconfig:"RUNNER_TOKEN" description:"The token for runner auth."`
	// a list of keycloak ids that are considered admins
	// if the string 'all' is included it means ALL users
	AdminIDs []string `envconfig:"ADMIN_USER_IDS" description:"Keycloak admin IDs."`
	// Specifies the source of the Admin user IDs.
	// By default AdminSrc is set to env.
	AdminSrc AdminSrcType `envconfig:"ADMIN_USER_SOURCE" default:"env" description:"Source of admin IDs (env or jwt)"`
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

	// SandboxAPIURL is the URL that sandbox containers use to connect back to the API.
	// This is needed when the main SERVER_URL goes through a reverse proxy that doesn't
	// support HTTP hijacking (used by RevDial). If not set, defaults to SERVER_URL.
	// Example: http://api-internal.example.com:8080 (direct HTTP, bypassing Caddy)
	SandboxAPIURL string `envconfig:"SANDBOX_API_URL" description:"Direct API URL for sandbox containers (bypasses reverse proxy). Defaults to SERVER_URL if not set."`
}

// AdminSrcType is an enum specifyin the type of Admin ID source.
// It currently supports only two sources:
// * env: ADMIN_USER_IDS env var
// * jwt: admin JWT token claim
type AdminSrcType string

const (
	AdminSrcTypeEnv AdminSrcType = "env"
	AdminSrcTypeJWT AdminSrcType = "jwt"
)

// String implements fmt.Stringer
func (a AdminSrcType) String() string {
	return string(a)
}

// Decode implements envconfig.Decoder for value validation.
func (a *AdminSrcType) Decode(value string) error {
	if value == "" {
		*a = AdminSrcTypeEnv
		return nil
	}
	switch value {
	case string(AdminSrcTypeEnv), string(AdminSrcTypeJWT):
		*a = AdminSrcType(value)
		return nil
	default:
		return fmt.Errorf("invalid source of admin IDs: %q", value)
	}
}

func (a *AdminSrcType) UnmarshalText(text []byte) error {
	return a.Decode(string(text))
}

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
}

type GitHub struct {
	Enabled      bool   `envconfig:"GITHUB_INTEGRATION_ENABLED" default:"false" description:"Enable github integration."`
	ClientID     string `envconfig:"GITHUB_INTEGRATION_CLIENT_ID" description:"The github app client id."`
	ClientSecret string `envconfig:"GITHUB_INTEGRATION_CLIENT_SECRET" description:"The github app client secret."`
	RepoFolder   string `envconfig:"GITHUB_INTEGRATION_REPO_FOLDER" default:"/filestore/github/repos" description:"What folder do we use to clone github repos."`
	WebhookURL   string `envconfig:"GITHUB_INTEGRATION_WEBHOOK_URL" description:"The URL to receive github webhooks."`
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

type TURN struct {
	Enabled  bool   `envconfig:"TURN_ENABLED" default:"true" description:"Enable TURN server for WebRTC NAT traversal."`
	PublicIP string `envconfig:"TURN_PUBLIC_IP" default:"127.0.0.1" description:"Public IP address for TURN server."`
	Port     int    `envconfig:"TURN_PORT" default:"3478" description:"UDP port for TURN server."`
	Realm    string `envconfig:"TURN_REALM" default:"helix.ai" description:"Authentication realm for TURN server."`
	Username string `envconfig:"TURN_USERNAME" default:"helix" description:"Username for TURN authentication."`
	Password string `envconfig:"TURN_PASSWORD" default:"helix-turn-secret" description:"Password for TURN authentication."`
}

type ExternalAgents struct {
	// MaxConcurrentLobbies is the maximum number of Wolf lobbies that can be created concurrently.
	// Each lobby uses GPU resources (VRAM for video encoding).
	MaxConcurrentLobbies int `envconfig:"EXTERNAL_AGENTS_MAX_CONCURRENT_LOBBIES" default:"10" description:"Maximum number of concurrent Wolf lobbies (GPU streaming sessions)."`
}
