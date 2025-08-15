package types

import (
	"encoding/json"
	"time"
)

type ModelType string

const (
	ModelTypeChat  ModelType = "chat"
	ModelTypeImage ModelType = "image"
	ModelTypeEmbed ModelType = "embed"
)

func (t ModelType) String() string {
	return string(t)
}

type Model struct {
	ID            string    `json:"id,omitempty" yaml:"id,omitempty"` // for example 'phi3.5:3.8b-mini-instruct-q8_0'
	Created       time.Time `json:"created,omitempty" yaml:"created,omitempty"`
	Updated       time.Time `json:"updated,omitempty" yaml:"updated,omitempty"`
	Type          ModelType `json:"type,omitempty" yaml:"type,omitempty"`
	Runtime       Runtime   `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Name          string    `json:"name,omitempty" yaml:"name,omitempty"`
	Memory        uint64    `json:"memory,omitempty" yaml:"memory,omitempty"` // in bytes, required
	ContextLength int64     `json:"context_length,omitempty" yaml:"context_length,omitempty"`
	Description   string    `json:"description,omitempty" yaml:"description,omitempty"`
	Hide          bool      `json:"hide,omitempty" yaml:"hide,omitempty"`
	Enabled       bool      `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	AutoPull      bool      `json:"auto_pull,omitempty" yaml:"auto_pull,omitempty"`   // Whether to automatically pull the model if missing in the runner
	SortOrder     int       `json:"sort_order,omitempty" yaml:"sort_order,omitempty"` // Order for sorting models in UI (lower numbers appear first)
	Prewarm       bool      `json:"prewarm,omitempty" yaml:"prewarm,omitempty"`       // Whether to prewarm this model to fill free GPU memory on runners

	// Runtime-specific arguments (e.g., VLLM command line args)
	RuntimeArgs map[string]interface{} `json:"runtime_args,omitempty" yaml:"runtime_args,omitempty" gorm:"type:jsonb;serializer:json"`

	// User modification tracking - system defaults are automatically updated if this is false
	UserModified bool `json:"user_modified,omitempty" yaml:"user_modified,omitempty"` // Whether user has modified system defaults
}

// UnmarshalJSON handles both flattened array format and nested object format for RuntimeArgs
func (m *Model) UnmarshalJSON(data []byte) error {
	// Define a temporary struct that matches Model but with RuntimeArgs as interface{}
	type TempModel struct {
		ID            string      `json:"id,omitempty"`
		Created       time.Time   `json:"created,omitempty"`
		Updated       time.Time   `json:"updated,omitempty"`
		Type          ModelType   `json:"type,omitempty"`
		Runtime       Runtime     `json:"runtime,omitempty"`
		Name          string      `json:"name,omitempty"`
		Memory        uint64      `json:"memory,omitempty"`
		ContextLength int64       `json:"context_length,omitempty"`
		Description   string      `json:"description,omitempty"`
		Hide          bool        `json:"hide,omitempty"`
		Enabled       bool        `json:"enabled,omitempty"`
		AutoPull      bool        `json:"auto_pull,omitempty"`
		SortOrder     int         `json:"sort_order,omitempty"`
		Prewarm       bool        `json:"prewarm,omitempty"`
		RuntimeArgs   interface{} `json:"runtime_args,omitempty"`
		UserModified  bool        `json:"user_modified,omitempty"`
	}

	var temp TempModel
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy all fields except RuntimeArgs
	m.ID = temp.ID
	m.Created = temp.Created
	m.Updated = temp.Updated
	m.Type = temp.Type
	m.Runtime = temp.Runtime
	m.Name = temp.Name
	m.Memory = temp.Memory
	m.ContextLength = temp.ContextLength
	m.Description = temp.Description
	m.Hide = temp.Hide
	m.Enabled = temp.Enabled
	m.AutoPull = temp.AutoPull
	m.SortOrder = temp.SortOrder
	m.Prewarm = temp.Prewarm
	m.UserModified = temp.UserModified

	// Handle RuntimeArgs - convert flattened array to nested format
	if temp.RuntimeArgs != nil {
		if argsArray, ok := temp.RuntimeArgs.([]interface{}); ok {
			// Flattened format - wrap in "args" key
			m.RuntimeArgs = map[string]interface{}{
				"args": argsArray,
			}
		} else if argsMap, ok := temp.RuntimeArgs.(map[string]interface{}); ok {
			// Already nested format - use as-is
			m.RuntimeArgs = argsMap
		}
	}

	return nil
}

type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityFile  Modality = "file"
)

type DynamicModelInfo struct {
	ID       string    `json:"id" gorm:"primaryKey"`
	Created  time.Time `json:"created,omitempty"`
	Updated  time.Time `json:"updated,omitempty"`
	Provider string    `json:"provider"` // helix, openai, etc. (Helix internal information)
	Name     string    `json:"name"`     // Model name

	ModelInfo ModelInfo `json:"model_info" gorm:"type:jsonb;serializer:json"`
}

type ModelInfo struct { //nolint:revive
	ProviderSlug        string     `json:"provider_slug"`
	ProviderModelID     string     `json:"provider_model_id"`
	Slug                string     `json:"slug"`
	Name                string     `json:"name"`
	Author              string     `json:"author"`
	SupportedParameters []string   `json:"supported_parameters"`
	Description         string     `json:"description"`
	InputModalities     []Modality `json:"input_modalities"`
	OutputModalities    []Modality `json:"output_modalities"`
	SupportsReasoning   bool       `json:"supports_reasoning"`
	ContextLength       int        `json:"context_length"`
	MaxCompletionTokens int        `json:"max_completion_tokens"`
	Pricing             Pricing    `json:"pricing"`
}

type Pricing struct {
	Prompt            string `json:"prompt"`
	Completion        string `json:"completion"`
	Image             string `json:"image"`
	Audio             string `json:"audio"`
	Request           string `json:"request"`
	WebSearch         string `json:"web_search"`
	InternalReasoning string `json:"internal_reasoning"`
}

type ListDynamicModelInfosQuery struct {
	Provider string
	Name     string
}

// Model CRD structures following the same pattern as App CRD
type ModelMetadata struct {
	Name string `json:"name" yaml:"name"`
}

type ModelCRD struct {
	APIVersion string        `json:"apiVersion" yaml:"apiVersion"`
	Kind       string        `json:"kind" yaml:"kind"`
	Metadata   ModelMetadata `json:"metadata" yaml:"metadata"`
	Spec       Model         `json:"spec" yaml:"spec"`
}
