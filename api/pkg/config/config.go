package config

import (
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
)

type ServerConfig struct {
	Providers     Providers
	Tools         Tools
	Keycloak      Keycloak
	Notifications Notifications
	Janitor       Janitor
	Stripe        Stripe
	Widget        Widget
	DataPrepText  DataPrepText
	Controller    Controller
	FileStore     FileStore
	Store         Store
}

func LoadServerConfig() (ServerConfig, error) {
	var cfg ServerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return ServerConfig{}, err
	}
	return cfg, nil
}

// Providers is used to configure the various AI providers that we use
type Providers struct {
	OpenAI     OpenAI
	TogetherAI TogetherAI
}

type OpenAI struct {
	APIKey  string `envconfig:"OPENAI_API_KEY"`
	BaseURL string `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`
}

type TogetherAI struct {
	APIKey  string `envconfig:"TOGETHER_API_KEY"`
	BaseURL string `envconfig:"TOGETHER_BASE_URL" default:"https://api.together.xyz/v1"`
}

type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderTogetherAI Provider = "togetherai"
)

type Tools struct {
	Enabled  bool     `envconfig:"TOOLS_ENABLED" default:"true"` // Enable/disable tools for the server
	Provider Provider `envconfig:"TOOLS_PROVIDER" default:"togetherai"`
	Model    string   `envconfig:"TOOLS_MODEL" default:"mistralai/Mixtral-8x7B-Instruct-v0.1"` // gpt-4-1106-preview
}

// Keycloak is used for authentication. You can find keycloak documentation
// at https://www.keycloak.org/guides
type Keycloak struct {
	URL          string `envconfig:"KEYCLOAK_URL" default:"http://keycloak:8080/auth"`
	ClientID     string `envconfig:"KEYCLOAK_CLIENT_ID" default:"api"`
	ClientSecret string `envconfig:"KEYCLOAK_CLIENT_SECRET"` // If not set, will be looked up using admin API
	AdminRealm   string `envconfig:"KEYCLOAK_ADMIN_REALM" default:"master"`
	Realm        string `envconfig:"KEYCLOAK_REALM" default:"helix"`
	Username     string `envconfig:"KEYCLOAK_USER"`
	Password     string `envconfig:"KEYCLOAK_PASSWORD"`
}

// Notifications is used for sending notifications to users when certain events happen
// such as finetuning starting or completing.
type Notifications struct {
	AppURL string `envconfig:"APP_URL" default:"https://app.tryhelix.ai"`
	Email  EmailConfig
	// TODO: Slack, Discord, etc.
}

type EmailConfig struct {
	SenderAddress string `envconfig:"EMAIL_SENDER_ADDRESS" default:"chris@helix.ml"`

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
	AppURL                  string   `envconfig:"APP_URL"`
	SentryDsnAPI            string   `envconfig:"SENTRY_DSN_API"`
	SentryDsnFrontend       string   `envconfig:"SENTRY_DSN_FRONTEND"`
	GoogleAnalyticsFrontend string   `envconfig:"GOOGLE_ANALYTICS_FRONTEND"`
	SlackWebhookURL         string   `envconfig:"JANITOR_SLACK_WEBHOOK_URL"`
	SlackIgnoreUser         []string `envconfig:"JANITOR_SLACK_IGNORE_USERS"`
}

type Stripe struct {
	SecretKey            string `envconfig:"STRIPE_SECRET_KEY"`
	WebhookSigningSecret string `envconfig:"STRIPE_WEBHOOK_SIGNING_SECRET"`
	PriceLookupKey       string `envconfig:"STRIPE_PRICE_LOOKUP_KEY"`
}

type Widget struct {
	Enabled  bool   `envconfig:"WIDGET_ENABLED" default:"true"` // Enable/disable the embedded widget
	FilePath string `envconfig:"WIDGET_FILE_PATH" default:"/www/helix-embed.iife.js"`
}

type DataPrepText struct {
	Module            types.DataPrepModule `envconfig:"DATA_PREP_TEXT_MODULE" default:"dynamic"`
	OverflowSize      int                  `envconfig:"DATA_PREP_TEXT_OVERFLOW_SIZE" default:"256"`
	QuestionsPerChunk int                  `envconfig:"DATA_PREP_TEXT_QUESTIONS_PER_CHUNK" default:"30"`
	Temperature       float32              `envconfig:"DATA_PREP_TEXT_TEMPERATURE" default:"0.5"`
}

type Controller struct {
	FilestorePresignSecret string `envconfig:"FILESTORE_PRESIGN_SECRET"`
	// this is an "env" prefix like "dev"
	// the user prefix is handled inside the controller
	// (see getFilestorePath)
	FilePrefixGlobal string `envconfig:"FILE_PREFIX_GLOBAL" default:"dev"`
	// this is a golang template that is used to prefix the user
	// path in the filestore - it is passed Owner and OwnerType values
	// write me an example FilePrefixUser as a go template
	// e.g. "users/{{.Owner}}"
	FilePrefixUser string `envconfig:"FILE_PREFIX_USER" default:"users/{{.Owner}}"`

	// a static path used to denote what sub-folder job results live in
	FilePrefixResults string `envconfig:"FILE_PREFIX_RESULTS" default:"results"`

	// the URL we post documents to so we can get the text back from them
	TextExtractionURL string `envconfig:"TEXT_EXTRACTION_URL" default:"http://llamaindex:5000/api/v1/extract"`

	// the URL we can post a chunk of text to for RAG indexing
	RAGIndexingURL string `envconfig:"RAG_INDEX_URL" default:"http://llamaindex:5000/api/v1/rag/chunk"`

	// the URL we can post a prompt to to match RAG records
	RAGQueryURL string `envconfig:"RAG_QUERY_URL" default:"http://llamaindex:5000/api/v1/rag/query"`

	// how many scheduler decisions to buffer before we start dropping them
	SchedulingDecisionBufferSize int `envconfig:"SCHEDULING_DECISION_BUFFER_SIZE" default:"10"`
}

type FileStore struct {
	Type         types.FileStoreType `envconfig:"FILESTORE_TYPE" default:"fs"`
	LocalFSPath  string              `envconfig:"FILESTORE_LOCALFS_PATH" default:"/tmp/helix/filestore"`
	GCSKeyBase64 string              `envconfig:"FILESTORE_GCS_KEY_BASE64"`
	GCSKeyFile   string              `envconfig:"FILESTORE_GCS_KEY_FILE"`
	GCSBucket    string              `envconfig:"FILESTORE_GCS_BUCKET"`
}

type Store struct {
	Host     string `envconfig:"POSTGRES_HOST"`
	Port     int    `envconfig:"POSTGRES_PORT" default:"5432"`
	Database string `envconfig:"POSTGRES_DATABASE" default:"helix"`
	Username string `envconfig:"POSTGRES_USER"`
	Password string `envconfig:"POSTGRES_PASSWORD"`

	AutoMigrate     bool          `envconfig:"DATABASE_AUTO_MIGRATE" default:"true"`
	MaxConns        int           `envconfig:"DATABASE_MAX_CONNS" default:"50"`
	IdleConns       int           `envconfig:"DATABASE_IDLE_CONNS" default:"25"`
	MaxConnLifetime time.Duration `envconfig:"DATABASE_MAX_CONN_LIFETIME" default:"1h"`
	MaxConnIdleTime time.Duration `envconfig:"DATABASE_MAX_CONN_IDLE_TIME" default:"1m"`
}
