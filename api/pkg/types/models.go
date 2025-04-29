package types

import "time"

type ModelType string

const (
	ModelTypeChat  ModelType = "chat"
	ModelTypeImage ModelType = "image"
	ModelTypeEmbed ModelType = "embed"
)

type ModelRuntimeType string

const (
	ModelRuntimeTypeOllama ModelRuntimeType = "ollama"
	ModelRuntimeTypeVLLM   ModelRuntimeType = "vllm"
)

type Model struct {
	ID            string           `json:"id,omitempty" yaml:"id,omitempty"` // for example 'phi3.5:3.8b-mini-instruct-q8_0'
	Created       time.Time        `json:"created,omitempty" yaml:"created,omitempty"`
	Updated       time.Time        `json:"updated,omitempty" yaml:"updated,omitempty"`
	Type          ModelType        `json:"type,omitempty" yaml:"type,omitempty"`
	Runtime       ModelRuntimeType `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Name          string           `json:"name,omitempty" yaml:"name,omitempty"`
	Memory        uint64           `json:"memory,omitempty" yaml:"memory,omitempty"` // in bytes, required
	ContextLength int64            `json:"context_length,omitempty" yaml:"context_length,omitempty"`
	Description   string           `json:"description,omitempty" yaml:"description,omitempty"`
	Hide          bool             `json:"hide,omitempty" yaml:"hide,omitempty"`
	Enabled       bool             `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}
