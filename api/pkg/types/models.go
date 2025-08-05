package types

import "time"

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

	// User modification tracking - system defaults are automatically updated if this is false
	UserModified bool `json:"user_modified,omitempty" yaml:"user_modified,omitempty"` // Whether user has modified system defaults
}

type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityFile  Modality = "file"
)

type ModelInfo struct { //nolint:revive
	ProviderSlug        string     `json:"provider_slug"`
	ProviderModelID     string     `json:"provider_model_id"`
	Slug                string     `json:"slug"`
	Name                string     `json:"name"`
	Author              string     `json:"author"`
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
