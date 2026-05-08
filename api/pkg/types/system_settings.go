package types

import (
	"time"
)

// SystemSettings represents global system configuration
// This serves as the fallback/default for all users and organizations
// Future enhancement: Add HuggingFaceToken to Organization and User tables
// with resolution hierarchy: User -> Organization -> System (global)
type SystemSettings struct {
	ID      string    `json:"id" gorm:"primaryKey"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Global Hugging Face configuration (fallback for all users/orgs)
	// Future: This will be the lowest priority in token resolution hierarchy
	HuggingFaceToken string `json:"huggingface_token,omitempty" gorm:"column:huggingface_token"`

	// Kodit enrichment model configuration
	// Used when Kodit sends requests with model "kodit-model" - Helix substitutes with these values
	KoditEnrichmentProvider string `json:"kodit_enrichment_provider,omitempty" gorm:"column:kodit_enrichment_provider"` // e.g., "together_ai", "openai", "helix"
	KoditEnrichmentModel    string `json:"kodit_enrichment_model,omitempty" gorm:"column:kodit_enrichment_model"`       // e.g., "Qwen/Qwen3-8B", "gpt-4o", "llama3:instruct"

	// Kodit text embedding model configuration
	// Used when Kodit sends requests with model "kodit-text-embedding" - Helix substitutes with these values
	KoditTextEmbeddingProvider string `json:"kodit_text_embedding_provider,omitempty" gorm:"column:kodit_text_embedding_provider"`
	KoditTextEmbeddingModel    string `json:"kodit_text_embedding_model,omitempty" gorm:"column:kodit_text_embedding_model"`

	// Kodit vision embedding model configuration
	// Used when Kodit sends requests with model "kodit-vision-embedding" - Helix substitutes with these values
	KoditVisionEmbeddingProvider string `json:"kodit_vision_embedding_provider,omitempty" gorm:"column:kodit_vision_embedding_provider"`
	KoditVisionEmbeddingModel    string `json:"kodit_vision_embedding_model,omitempty" gorm:"column:kodit_vision_embedding_model"`

	EnforceQuotas bool `json:"enforce_quotas,omitempty" gorm:"column:enforce_quotas"`

	MaxConcurrentDesktops int `json:"max_concurrent_desktops,omitempty"` // Per user

	ProvidersManagementEnabled bool `json:"providers_management_enabled,omitempty"`

	SandboxBillingEnabled                bool    `json:"sandbox_billing_enabled,omitempty" gorm:"column:sandbox_billing_enabled"`
	SandboxHeadlessPriceCreditsPerSecond float64 `json:"sandbox_headless_price_credits_per_second,omitempty" gorm:"column:sandbox_headless_price_credits_per_second"`
	SandboxDesktopPriceCreditsPerSecond  float64 `json:"sandbox_desktop_price_credits_per_second,omitempty" gorm:"column:sandbox_desktop_price_credits_per_second"`
	MaxConcurrentHeadlessSandboxes       int     `json:"max_concurrent_headless_sandboxes,omitempty" gorm:"column:max_concurrent_headless_sandboxes"`
	MaxConcurrentDesktopSandboxes        int     `json:"max_concurrent_desktop_sandboxes,omitempty" gorm:"column:max_concurrent_desktop_sandboxes"`

	// Optimus configuration
	OptimusReasoningModelProvider string `json:"optimus_reasoning_model_provider" yaml:"optimus_reasoning_model_provider"`
	OptimusReasoningModel         string `json:"optimus_reasoning_model" yaml:"optimus_reasoning_model"`
	OptimusReasoningModelEffort   string `json:"optimus_reasoning_model_effort" yaml:"optimus_reasoning_model_effort"`

	OptimusGenerationModelProvider string `json:"optimus_generation_model_provider" yaml:"optimus_generation_model_provider"`
	OptimusGenerationModel         string `json:"optimus_generation_model" yaml:"optimus_generation_model"`

	OptimusSmallReasoningModelProvider string `json:"optimus_small_reasoning_model_provider" yaml:"optimus_small_reasoning_model_provider"`
	OptimusSmallReasoningModel         string `json:"optimus_small_reasoning_model" yaml:"optimus_small_reasoning_model"`
	OptimusSmallReasoningModelEffort   string `json:"optimus_small_reasoning_model_effort" yaml:"optimus_small_reasoning_model_effort"`

	OptimusSmallGenerationModelProvider string `json:"optimus_small_generation_model_provider" yaml:"optimus_small_generation_model_provider"`
	OptimusSmallGenerationModel         string `json:"optimus_small_generation_model" yaml:"optimus_small_generation_model"`
}

// SystemSettingsRequest represents the request payload for updating system settings
type SystemSettingsRequest struct {
	HuggingFaceToken *string `json:"huggingface_token,omitempty"`

	// Kodit enrichment model configuration
	KoditEnrichmentProvider *string `json:"kodit_enrichment_provider,omitempty"`
	KoditEnrichmentModel    *string `json:"kodit_enrichment_model,omitempty"`

	// Kodit text embedding model configuration
	KoditTextEmbeddingProvider *string `json:"kodit_text_embedding_provider,omitempty"`
	KoditTextEmbeddingModel    *string `json:"kodit_text_embedding_model,omitempty"`

	// Kodit vision embedding model configuration
	KoditVisionEmbeddingProvider *string `json:"kodit_vision_embedding_provider,omitempty"`
	KoditVisionEmbeddingModel    *string `json:"kodit_vision_embedding_model,omitempty"`

	MaxConcurrentDesktops *int `json:"max_concurrent_desktops"`

	ProvidersManagementEnabled *bool `json:"providers_management_enabled"`

	EnforceQuotas *bool `json:"enforce_quotas"`

	SandboxBillingEnabled                *bool    `json:"sandbox_billing_enabled"`
	SandboxHeadlessPriceCreditsPerSecond *float64 `json:"sandbox_headless_price_credits_per_second"`
	SandboxDesktopPriceCreditsPerSecond  *float64 `json:"sandbox_desktop_price_credits_per_second"`
	MaxConcurrentHeadlessSandboxes       *int     `json:"max_concurrent_headless_sandboxes"`
	MaxConcurrentDesktopSandboxes        *int     `json:"max_concurrent_desktop_sandboxes"`

	OptimusReasoningModelProvider *string `json:"optimus_reasoning_model_provider"`
	OptimusReasoningModel         *string `json:"optimus_reasoning_model"`
	OptimusReasoningModelEffort   *string `json:"optimus_reasoning_model_effort"`

	OptimusGenerationModelProvider *string `json:"optimus_generation_model_provider"`
	OptimusGenerationModel         *string `json:"optimus_generation_model"`

	OptimusSmallReasoningModelProvider *string `json:"optimus_small_reasoning_model_provider"`
	OptimusSmallReasoningModel         *string `json:"optimus_small_reasoning_model"`
	OptimusSmallReasoningModelEffort   *string `json:"optimus_small_reasoning_model_effort"`

	OptimusSmallGenerationModelProvider *string `json:"optimus_small_generation_model_provider"`
	OptimusSmallGenerationModel         *string `json:"optimus_small_generation_model"`
}

// SystemSettingsResponse represents the response payload for system settings (without sensitive data)
type SystemSettingsResponse struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Sensitive fields are masked
	HuggingFaceTokenSet    bool   `json:"huggingface_token_set"`
	HuggingFaceTokenSource string `json:"huggingface_token_source"` // "database", "environment", or "none"

	// Kodit enrichment model configuration (not sensitive, returned as-is)
	KoditEnrichmentProvider string `json:"kodit_enrichment_provider"`
	KoditEnrichmentModel    string `json:"kodit_enrichment_model"`
	KoditEnrichmentModelSet bool   `json:"kodit_enrichment_model_set"` // true if both provider and model are configured

	// Kodit text embedding model configuration
	KoditTextEmbeddingProvider string `json:"kodit_text_embedding_provider"`
	KoditTextEmbeddingModel    string `json:"kodit_text_embedding_model"`
	KoditTextEmbeddingModelSet bool   `json:"kodit_text_embedding_model_set"`

	// Kodit vision embedding model configuration
	KoditVisionEmbeddingProvider string `json:"kodit_vision_embedding_provider"`
	KoditVisionEmbeddingModel    string `json:"kodit_vision_embedding_model"`
	KoditVisionEmbeddingModelSet bool   `json:"kodit_vision_embedding_model_set"`

	MaxConcurrentDesktops int `json:"max_concurrent_desktops"` // Per user

	ProvidersManagementEnabled bool `json:"providers_management_enabled"`

	EnforceQuotas bool `json:"enforce_quotas"`

	SandboxBillingEnabled                bool    `json:"sandbox_billing_enabled"`
	SandboxHeadlessPriceCreditsPerSecond float64 `json:"sandbox_headless_price_credits_per_second"`
	SandboxDesktopPriceCreditsPerSecond  float64 `json:"sandbox_desktop_price_credits_per_second"`
	MaxConcurrentHeadlessSandboxes       int     `json:"max_concurrent_headless_sandboxes"`
	MaxConcurrentDesktopSandboxes        int     `json:"max_concurrent_desktop_sandboxes"`

	// Optimus configuration
	OptimusReasoningModelProvider string `json:"optimus_reasoning_model_provider"`
	OptimusReasoningModel         string `json:"optimus_reasoning_model"`
	OptimusReasoningModelEffort   string `json:"optimus_reasoning_model_effort"`

	OptimusGenerationModelProvider string `json:"optimus_generation_model_provider"`
	OptimusGenerationModel         string `json:"optimus_generation_model"`

	OptimusSmallReasoningModelProvider string `json:"optimus_small_reasoning_model_provider"`
	OptimusSmallReasoningModel         string `json:"optimus_small_reasoning_model"`
	OptimusSmallReasoningModelEffort   string `json:"optimus_small_reasoning_model_effort"`

	OptimusSmallGenerationModelProvider string `json:"optimus_small_generation_model_provider"`
	OptimusSmallGenerationModel         string `json:"optimus_small_generation_model"`
}

// ToResponseWithSource converts SystemSettings to SystemSettingsResponse with source information
func (s *SystemSettings) ToResponseWithSource(dbToken, envToken string) *SystemSettingsResponse {
	var source string
	var hasToken bool

	if dbToken != "" {
		source = "database"
		hasToken = true
	} else if envToken != "" {
		source = "environment"
		hasToken = true
	} else {
		source = "none"
		hasToken = false
	}

	return &SystemSettingsResponse{
		ID:                                   s.ID,
		Created:                              s.Created,
		Updated:                              s.Updated,
		HuggingFaceTokenSet:                  hasToken,
		HuggingFaceTokenSource:               source,
		KoditEnrichmentProvider:              s.KoditEnrichmentProvider,
		KoditEnrichmentModel:                 s.KoditEnrichmentModel,
		KoditEnrichmentModelSet:              s.KoditEnrichmentProvider != "" && s.KoditEnrichmentModel != "",
		KoditTextEmbeddingProvider:           s.KoditTextEmbeddingProvider,
		KoditTextEmbeddingModel:              s.KoditTextEmbeddingModel,
		KoditTextEmbeddingModelSet:           s.KoditTextEmbeddingProvider != "" && s.KoditTextEmbeddingModel != "",
		KoditVisionEmbeddingProvider:         s.KoditVisionEmbeddingProvider,
		KoditVisionEmbeddingModel:            s.KoditVisionEmbeddingModel,
		KoditVisionEmbeddingModelSet:         s.KoditVisionEmbeddingProvider != "" && s.KoditVisionEmbeddingModel != "",
		MaxConcurrentDesktops:                s.MaxConcurrentDesktops,
		ProvidersManagementEnabled:           s.ProvidersManagementEnabled,
		EnforceQuotas:                        s.EnforceQuotas,
		SandboxBillingEnabled:                s.SandboxBillingEnabled,
		SandboxHeadlessPriceCreditsPerSecond: s.SandboxHeadlessPriceCreditsPerSecond,
		SandboxDesktopPriceCreditsPerSecond:  s.SandboxDesktopPriceCreditsPerSecond,
		MaxConcurrentHeadlessSandboxes:       s.EffectiveMaxConcurrentHeadlessSandboxes(),
		MaxConcurrentDesktopSandboxes:        s.EffectiveMaxConcurrentDesktopSandboxes(),
		OptimusReasoningModelProvider:        s.OptimusReasoningModelProvider,
		OptimusReasoningModel:                s.OptimusReasoningModel,
		OptimusReasoningModelEffort:          s.OptimusReasoningModelEffort,
		OptimusGenerationModelProvider:       s.OptimusGenerationModelProvider,
		OptimusGenerationModel:               s.OptimusGenerationModel,
		OptimusSmallReasoningModelProvider:   s.OptimusSmallReasoningModelProvider,
		OptimusSmallReasoningModel:           s.OptimusSmallReasoningModel,
		OptimusSmallReasoningModelEffort:     s.OptimusSmallReasoningModelEffort,
		OptimusSmallGenerationModelProvider:  s.OptimusSmallGenerationModelProvider,
		OptimusSmallGenerationModel:          s.OptimusSmallGenerationModel,
	}
}

const (
	// SystemSettingsID is the fixed ID for the single system settings record
	SystemSettingsID = "system"

	DefaultMaxConcurrentHeadlessSandboxes = 10
	DefaultMaxConcurrentDesktopSandboxes  = 10
)

func (s *SystemSettings) EffectiveMaxConcurrentHeadlessSandboxes() int {
	if s.MaxConcurrentHeadlessSandboxes <= 0 {
		return DefaultMaxConcurrentHeadlessSandboxes
	}
	return s.MaxConcurrentHeadlessSandboxes
}

func (s *SystemSettings) EffectiveMaxConcurrentDesktopSandboxes() int {
	if s.MaxConcurrentDesktopSandboxes <= 0 {
		return DefaultMaxConcurrentDesktopSandboxes
	}
	return s.MaxConcurrentDesktopSandboxes
}
