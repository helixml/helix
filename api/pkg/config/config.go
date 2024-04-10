package config

import "github.com/kelseyhightower/envconfig"

type ServerConfig struct {
	Providers     Providers
	Tools         Tools
	Keycloak      Keycloak
	Notifications Notifications
	Janitor       Janitor
	Stripe        Stripe
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
