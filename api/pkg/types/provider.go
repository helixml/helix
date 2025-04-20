package types

import (
	"time"

	"github.com/lib/pq"
)

type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderTogetherAI Provider = "togetherai"
	ProviderHelix      Provider = "helix"
	ProviderVLLM       Provider = "vllm"
)

type ProviderEndpointType string

const (
	ProviderEndpointTypeGlobal ProviderEndpointType = "global"
	ProviderEndpointTypeUser   ProviderEndpointType = "user"
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
	Models         pq.StringArray       `json:"models" gorm:"type:text[]"` // Optional
	EndpointType   ProviderEndpointType `json:"endpoint_type"`             // global, user (TODO: orgs, teams)
	Owner          string               `json:"owner"`
	OwnerType      OwnerType            `json:"owner_type"` // user, system, org
	BaseURL        string               `json:"base_url"`
	APIKey         string               `json:"api_key"`
	APIKeyFromFile string               `json:"api_key_file"`     // Must be mounted to the container
	Default        bool                 `json:"default" gorm:"-"` // Set from environment variable

	AvailableModels []OpenAIModel          `json:"available_models" gorm:"-"`
	Status          ProviderEndpointStatus `json:"status" gorm:"-"` // If we can't fetch models
	Error           string                 `json:"error" gorm:"-"`
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
}

// UpdateProviderEndpoint used for updating a provider endpoint through the API
type UpdateProviderEndpoint struct {
	Description    string               `json:"description"`
	Models         []string             `json:"models"`
	EndpointType   ProviderEndpointType `json:"endpoint_type"` // global, user (TODO: orgs, teams)
	BaseURL        string               `json:"base_url"`
	APIKey         *string              `json:"api_key,omitempty"`
	APIKeyFromFile *string              `json:"api_key_file,omitempty"` // Must be mounted to the container
}
