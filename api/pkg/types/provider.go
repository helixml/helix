package types

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Provider string

const OrganizationProviderUnavailableMessage = "This agent's provider is not available to the organization. Personal providers are no longer supported for organization agents. Configure the provider for the organization, then select it again in the agent settings."

const (
	ProviderOpenAI     Provider = "openai"
	ProviderTogetherAI Provider = "togetherai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderHelix      Provider = "helix"
	ProviderVLLM       Provider = "vllm"
)

var GlobalProviders = []string{
	string(ProviderOpenAI),
	string(ProviderTogetherAI),
	string(ProviderAnthropic),
	string(ProviderHelix),
	string(ProviderVLLM),
}

func IsGlobalProvider(provider string) bool {
	for _, p := range GlobalProviders {
		if p == provider {
			return true
		}
	}
	return false
}

func CanonicalProviderName(provider string) string {
	name := strings.ToLower(provider)
	for _, prefix := range []string{"user/", "global/"} {
		if strings.HasPrefix(name, prefix) && IsGlobalProvider(strings.TrimPrefix(name, prefix)) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

func GlobalProviderID(provider string) string {
	return "global/" + CanonicalProviderName(provider)
}

func IsGlobalProviderID(provider string) bool {
	return strings.HasPrefix(strings.ToLower(provider), "global/") && IsGlobalProvider(CanonicalProviderName(provider))
}

type ProviderEndpointType string

const (
	ProviderEndpointTypeGlobal ProviderEndpointType = "global"
	ProviderEndpointTypeUser   ProviderEndpointType = "user"
	ProviderEndpointTypeOrg    ProviderEndpointType = "org"
	ProviderEndpointTypeTeam   ProviderEndpointType = "team"
)

type ProviderEndpointStatus string

const (
	ProviderEndpointStatusOK       ProviderEndpointStatus = "ok"
	ProviderEndpointStatusError    ProviderEndpointStatus = "error"
	ProviderEndpointStatusLoading  ProviderEndpointStatus = "loading"
	ProviderEndpointStatusDisabled ProviderEndpointStatus = "disabled"
)

type ProviderEndpoint struct {
	ID             string               `json:"id" gorm:"primaryKey"`
	Created        time.Time            `json:"created"`
	Updated        time.Time            `json:"updated"`
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Icon           string               `json:"icon"`
	Models         pq.StringArray       `json:"models" gorm:"type:text[]"` // Optional
	EndpointType   ProviderEndpointType `json:"endpoint_type"`             // global, user (TODO: orgs, teams)
	Owner          string               `json:"owner"`
	OwnerType      OwnerType            `json:"owner_type"` // user, system, org
	BaseURL        string               `json:"base_url"`
	APIKey         string               `json:"api_key"`
	APIKeyFromFile string               `json:"api_key_file"`     // Must be mounted to the container
	Default        bool                 `json:"default" gorm:"-"` // Set from environment variable
	BillingEnabled bool                 `json:"billing_enabled"`
	Headers        map[string]string    `json:"headers" gorm:"type:jsonb;serializer:json"` // If for example anthropic expects x-api-key and anthropic-version

	// Google Vertex AI fields — when VertexProjectID is set, this endpoint routes through Vertex
	VertexProjectID       string `json:"vertex_project_id,omitempty" gorm:"column:vertex_project_id"`
	VertexRegion          string `json:"vertex_region,omitempty" gorm:"column:vertex_region"`
	VertexCredentialsJSON string `json:"vertex_credentials_json,omitempty" gorm:"column:vertex_credentials_json"` // Service account JSON string; takes precedence over file
	VertexCredentialsFile string `json:"vertex_credentials_file,omitempty" gorm:"column:vertex_credentials_file"`

	AvailableModels []OpenAIModel          `json:"available_models" gorm:"-"`
	Status          ProviderEndpointStatus `json:"status" gorm:"-"` // If we can't fetch models
	Error           string                 `json:"error" gorm:"-"`
}

// ProviderEndpointModels is the model picker payload for one provider endpoint:
// everything the upstream offers, plus the subset the operator has enabled.
type ProviderEndpointModels struct {
	// Models is the provider's full upstream catalogue, unfiltered.
	Models []OpenAIModel `json:"models"`
	// EnabledModels is the operator's whitelist. Empty means every model in
	// Models is available — the default for a newly added provider.
	EnabledModels []string `json:"enabled_models"`
}

// UpdateProviderEndpointModels sets the enabled-models whitelist for an
// endpoint. An empty list re-enables the provider's whole catalogue.
type UpdateProviderEndpointModels struct {
	Models []string `json:"models"`
}

// ModelsList is a list of models, including those that belong to the user or organization.
type OpenAIModelsList struct {
	Models []OpenAIModel `json:"data"`
}

// Permission struct represents an OpenAPI permission.
type OpenAIPermission struct {
	CreatedAt          int64       `json:"created"`
	ID                 string      `json:"id"`
	Object             string      `json:"object"`
	AllowCreateEngine  bool        `json:"allow_create_engine"`
	AllowSampling      bool        `json:"allow_sampling"`
	AllowLogprobs      bool        `json:"allow_logprobs"`
	AllowSearchIndices bool        `json:"allow_search_indices"`
	AllowView          bool        `json:"allow_view"`
	AllowFineTuning    bool        `json:"allow_fine_tuning"`
	Organization       string      `json:"organization"`
	Group              interface{} `json:"group"`
	IsBlocking         bool        `json:"is_blocking"`
}

// Model struct represents an OpenAPI model.
type OpenAIModel struct {
	CreatedAt     int64              `json:"created"`
	ID            string             `json:"id"`
	Object        string             `json:"object"`
	OwnedBy       string             `json:"owned_by"`
	Permission    []OpenAIPermission `json:"permission"`
	Root          string             `json:"root"`
	Parent        string             `json:"parent"`
	Name          string             `json:"name,omitempty"`
	Description   string             `json:"description,omitempty"`
	Hide          bool               `json:"hide,omitempty"`
	Type          string             `json:"type,omitempty"`
	ContextLength int                `json:"context_length,omitempty"`
	Enabled       bool               `json:"enabled,omitempty"`
	ModelInfo     *ModelInfo         `json:"model_info,omitempty"`
	// ReasoningEfforts is the curated set of reasoning-effort values this model
	// accepts. It is a separate field from ModelInfo on purpose: ModelInfo being
	// non-nil is what the billing path reads as "this model is priceable", so
	// effort capability — which is known for models that have no pricing entry,
	// e.g. self-hosted vLLM deployments — must not be smuggled in through it.
	// Nil means Helix does not know; a UI must not offer a guessed effort list,
	// because sending a value the provider rejects aborts the whole turn.
	ReasoningEfforts *ReasoningEffortProfile `json:"reasoning_efforts,omitempty"`

	// SupportedParameters is the set of request parameters the model accepts,
	// as reported by aggregators that publish it (OpenRouter's /v1/models does;
	// plain OpenAI-compatible servers don't). Used by the model picker to
	// filter a several-hundred-model catalogue down to, for example, the models
	// that can actually call tools. Nil means the provider didn't say.
	SupportedParameters []string `json:"supported_parameters,omitempty"`
	// InputModalities is the model's accepted input types ("text", "image",
	// "file", ...). Same provenance and same nil-means-unknown rule as
	// SupportedParameters.
	InputModalities []string `json:"input_modalities,omitempty"`
}

// UnmarshalJSON accepts the context-window fields commonly returned by
// OpenAI-compatible servers. OpenAI itself does not define a standard context
// field on /v1/models, while vLLM uses max_model_len and some compatible
// servers use max_context_length. Normalize all of them into ContextLength so
// downstream code has one authoritative field to consume.
func (m *OpenAIModel) UnmarshalJSON(data []byte) error {
	type openAIModelAlias OpenAIModel

	var decoded openAIModelAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var compatibleFields struct {
		MaxModelLen      int `json:"max_model_len"`
		MaxContextLength int `json:"max_context_length"`
	}
	if err := json.Unmarshal(data, &compatibleFields); err != nil {
		return err
	}

	// OpenRouter nests the modality list under architecture. Decoded
	// best-effort and separately: a provider that uses that key for some other
	// shape must not fail the whole model list.
	var nested struct {
		Architecture struct {
			InputModalities []string `json:"input_modalities"`
		} `json:"architecture"`
	}
	_ = json.Unmarshal(data, &nested)

	*m = OpenAIModel(decoded)
	if len(m.InputModalities) == 0 {
		m.InputModalities = nested.Architecture.InputModalities
	}
	if m.ContextLength > 0 {
		return nil
	}
	if compatibleFields.MaxModelLen > 0 {
		m.ContextLength = compatibleFields.MaxModelLen
		return nil
	}
	if compatibleFields.MaxContextLength > 0 {
		m.ContextLength = compatibleFields.MaxContextLength
		return nil
	}
	if m.ModelInfo != nil && m.ModelInfo.ContextLength > 0 {
		m.ContextLength = m.ModelInfo.ContextLength
	}

	return nil
}

// UpdateProviderEndpoint used for updating a provider endpoint through the API
type UpdateProviderEndpoint struct {
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Icon           *string              `json:"icon,omitempty"`
	Models         []string             `json:"models"`
	EndpointType   ProviderEndpointType `json:"endpoint_type"` // global, user (TODO: orgs, teams)
	BillingEnabled *bool                `json:"billing_enabled,omitempty"`

	BaseURL        string            `json:"base_url"`
	APIKey         *string           `json:"api_key,omitempty"`
	APIKeyFromFile *string           `json:"api_key_file,omitempty"` // Must be mounted to the container
	Headers        map[string]string `json:"headers,omitempty"`      // Custom headers for the endpoint

	// Google Vertex AI fields
	VertexProjectID       *string `json:"vertex_project_id,omitempty"`
	VertexRegion          *string `json:"vertex_region,omitempty"`
	VertexCredentialsJSON *string `json:"vertex_credentials_json,omitempty"`
	VertexCredentialsFile *string `json:"vertex_credentials_file,omitempty"`
}
