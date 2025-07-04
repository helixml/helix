package types

import (
	"time"
)

// YAMLSkill represents a skill definition loaded from YAML files
type YAMLSkill struct {
	APIVersion string        `yaml:"apiVersion" json:"apiVersion"`
	Kind       string        `yaml:"kind" json:"kind"`
	Metadata   SkillMetadata `yaml:"metadata" json:"metadata"`
	Spec       SkillSpec     `yaml:"spec" json:"spec"`
}

// SkillMetadata contains metadata about the skill
type SkillMetadata struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"displayName" json:"displayName"`
	Provider    string `yaml:"provider" json:"provider"`
	Category    string `yaml:"category" json:"category"`
}

// SkillSpec contains the skill specification
type SkillSpec struct {
	Description  string     `yaml:"description" json:"description"`
	SystemPrompt string     `yaml:"systemPrompt" json:"systemPrompt"`
	Icon         SkillIcon  `yaml:"icon" json:"icon"`
	OAuth        SkillOAuth `yaml:"oauth" json:"oauth"`
	API          SkillAPI   `yaml:"api" json:"api"`
	Configurable bool       `yaml:"configurable" json:"configurable"`
}

// SkillIcon defines how the skill icon should be displayed
type SkillIcon struct {
	Type string `yaml:"type" json:"type"` // e.g., "material-ui", "custom"
	Name string `yaml:"name" json:"name"` // e.g., "GitHub", "Google"
}

// SkillOAuth defines OAuth configuration for the skill
type SkillOAuth struct {
	Provider string   `yaml:"provider" json:"provider"`
	Scopes   []string `yaml:"scopes" json:"scopes"`
}

// SkillAPI defines the API configuration for the skill
type SkillAPI struct {
	BaseURL string            `yaml:"baseUrl" json:"baseUrl"`
	Headers map[string]string `yaml:"headers" json:"headers"`
	Schema  string            `yaml:"schema" json:"schema"` // OpenAPI schema as YAML string
}

// SkillDefinition is the internal representation used by the backend
type SkillDefinition struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"displayName"`
	Description  string    `json:"description"`
	SystemPrompt string    `json:"systemPrompt"`
	Category     string    `json:"category"`
	Provider     string    `json:"provider"`
	Icon         SkillIcon `json:"icon"`

	// OAuth configuration
	OAuthProvider string   `json:"oauthProvider"`
	OAuthScopes   []string `json:"oauthScopes"`

	// API configuration
	BaseURL string            `json:"baseUrl"`
	Headers map[string]string `json:"headers"`
	Schema  string            `json:"schema"`

	// Metadata
	Configurable bool      `json:"configurable"`
	LoadedAt     time.Time `json:"loadedAt"`
	FilePath     string    `json:"filePath"`
}

// SkillsListResponse represents the response for listing skills
type SkillsListResponse struct {
	Skills []SkillDefinition `json:"skills"`
	Count  int               `json:"count"`
}

// SkillTestRequest represents a request to test a skill
type SkillTestRequest struct {
	SkillID    string                 `json:"skillId"`
	Operation  string                 `json:"operation"`
	Parameters map[string]interface{} `json:"parameters"`
}

// SkillTestResponse represents the response from testing a skill
type SkillTestResponse struct {
	Success    bool                   `json:"success"`
	StatusCode int                    `json:"statusCode"`
	Response   map[string]interface{} `json:"response"`
	Error      string                 `json:"error,omitempty"`
	Duration   time.Duration          `json:"duration"`
}
