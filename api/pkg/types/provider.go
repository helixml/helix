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
