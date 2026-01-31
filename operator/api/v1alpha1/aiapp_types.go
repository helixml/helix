/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AssistantConfig is a local version of the Helix assistant configuration.
type AssistantConfig struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
	Image       string `json:"image,omitempty"`
	Provider    string `json:"provider,omitempty"` // openai, togetherai, helix, etc.
	Model       string `json:"model"`
	Type        string `json:"type,omitempty"` // TODO: remove

	SystemPrompt string `json:"system_prompt,omitempty"`

	// The data entity ID that we have created as the RAG source
	RAGSourceID string `json:"rag_source_id,omitempty"`

	// The data entity ID that we have created for the lora fine tune
	LoraID string `json:"lora_id,omitempty"`

	// Knowledge available to the assistant
	Knowledge []AssistantKnowledge `json:"knowledge,omitempty"`

	// Template for determining if the request is actionable or informative
	IsActionableTemplate string `json:"is_actionable_template,omitempty"`

	// The list of API tools this assistant will use
	APIs []AssistantAPI `json:"apis,omitempty"`

	// The list of GPT scripts this assistant will use
	GPTScripts []AssistantGPTScript `json:"gptscripts,omitempty"`

	// Zapier integration configuration
	Zapier []AssistantZapier `json:"zapier,omitempty"`

	// Tools is populated from the APIs and GPTScripts on create and update
	Tools []Tool `json:"tools,omitempty"`

	// Test configurations
	Tests []AssistantTest `json:"tests,omitempty"`
}

// AssistantAPI represents an API tool configuration
type AssistantAPI struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Schema      string            `json:"schema"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	Query       map[string]string `json:"query,omitempty"`

	RequestPrepTemplate     string `json:"request_prep_template,omitempty"`
	ResponseSuccessTemplate string `json:"response_success_template,omitempty"`
	ResponseErrorTemplate   string `json:"response_error_template,omitempty"`
}

// AssistantGPTScript represents a GPTScript tool configuration
type AssistantGPTScript struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	File        string `json:"file"`
	Content     string `json:"content"`
}

// AssistantZapier represents Zapier integration configuration
type AssistantZapier struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	MaxIterations int    `json:"max_iterations"`
}

// Tool represents a tool available to the assistant
type Tool struct {
	ID          string      `json:"id"`
	Created     metav1.Time `json:"created"`
	Updated     metav1.Time `json:"updated"`
	Owner       string      `json:"owner"`
	OwnerType   string      `json:"owner_type"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	ToolType    string      `json:"tool_type"`
	Global      bool        `json:"global"`
	Config      ToolConfig  `json:"config"`
}

// AssistantTest represents test configuration for an assistant
type AssistantTest struct {
	Name  string     `json:"name"`
	Steps []TestStep `json:"steps"`
}

// TestStep represents a single test step
type TestStep struct {
	Prompt         string `json:"prompt"`
	ExpectedOutput string `json:"expected_output"`
}

// RAGSettings contains settings for RAG processing
type RAGSettings struct {
	Threshold          int  `json:"threshold"`
	ResultsCount       int  `json:"results_count"`
	ChunkSize          int  `json:"chunk_size"`
	ChunkOverflow      int  `json:"chunk_overflow"`
	DisableChunking    bool `json:"disable_chunking"`
	DisableDownloading bool `json:"disable_downloading"`
}

// FilestoreSource represents filestore configuration
type FilestoreSource struct {
	Path string `json:"path"`
}

// WebCrawlerConfig contains web crawler settings
type WebCrawlerConfig struct {
	Enabled     bool `json:"enabled"`
	MaxDepth    int  `json:"max_depth"`
	MaxPages    int  `json:"max_pages"`
	Readability bool `json:"readability"`
}

// WebSource represents web source configuration
type WebSource struct {
	URLs    []string         `json:"urls"`
	Crawler WebCrawlerConfig `json:"crawler"`
}

// KnowledgeSource contains source configuration for knowledge
type KnowledgeSource struct {
	Filestore FilestoreSource `json:"filestore"`
	Web       WebSource       `json:"web"`
}

// AssistantKnowledge represents knowledge configuration for an assistant
type AssistantKnowledge struct {
	Name            string          `json:"name"`
	RAGSettings     RAGSettings     `json:"rag_settings"`
	Source          KnowledgeSource `json:"source"`
	RefreshEnabled  bool            `json:"refresh_enabled"`
	RefreshSchedule string          `json:"refresh_schedule,omitempty"`
}

// ToolAPIConfig represents API tool configuration.
type ToolAPIConfig struct {
	URL     string            `json:"url"`
	Schema  string            `json:"schema"`
	Actions []ToolAPIAction   `json:"actions"`
	Headers map[string]string `json:"headers"`
	Query   map[string]string `json:"query"`

	RequestPrepTemplate     string `json:"request_prep_template"`
	ResponseSuccessTemplate string `json:"response_success_template"`
	ResponseErrorTemplate   string `json:"response_error_template"`

	Model string `json:"model"`
}

// ToolAPIAction represents an API action configuration.
type ToolAPIAction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Method      string `json:"method"`
	Path        string `json:"path"`
}

// ToolGPTScriptConfig represents GPTScript tool configuration
type ToolGPTScriptConfig struct {
	Script    string `json:"script"`
	ScriptURL string `json:"script_url"`
}

// ToolZapierConfig represents Zapier tool configuration
type ToolZapierConfig struct {
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	MaxIterations int    `json:"max_iterations"`
}

// ToolConfig represents tool configuration
type ToolConfig struct {
	API       *ToolAPIConfig       `json:"api,omitempty"`
	GPTScript *ToolGPTScriptConfig `json:"gptscript,omitempty"`
	Zapier    *ToolZapierConfig    `json:"zapier,omitempty"`
}

// AIAppSpec defines the desired state of AIApp
type AIAppSpec struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Avatar      string            `json:"avatar,omitempty"`
	Image       string            `json:"image,omitempty"`
	Assistants  []AssistantConfig `json:"assistants,omitempty"`
}

// AIAppStatus defines the observed state of AIApp
type AIAppStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AIApp is the Schema for the aiapps API
type AIApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIAppSpec   `json:"spec,omitempty"`
	Status AIAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIAppList contains a list of AIApp
type AIAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIApp{}, &AIAppList{})
}
