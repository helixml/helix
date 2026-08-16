package types

// Execution configuration shared by every surface that runs an external coding
// agent. SpecTasks and plain chat sessions own a complete
// CodeAgentExecutionConfig; general sessions retain ParentApp separately for
// Agent instructions and tools.

// CodeAgentOverrides is retained for historical sessions and SpecTasks until a
// complete execution config is materialized.
type CodeAgentOverrides struct {
	ProviderRef     string `json:"provider_ref,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
}

// CodeAgentExecutionConfig is the complete, durable configuration needed to
// run a coding agent. Unlike CodeAgentOverrides it has no dependency on a
// reusable Helix App. Projects store the default for new SpecTasks and each
// SpecTask stores its own copy so later project changes cannot alter a run.
type CodeAgentExecutionConfig struct {
	Runtime         CodeAgentRuntime        `json:"runtime"`
	CredentialType  CodeAgentCredentialType `json:"credential_type"`
	ProviderRef     string                  `json:"provider_ref,omitempty"`
	Model           string                  `json:"model,omitempty"`
	ReasoningEffort string                  `json:"reasoning_effort,omitempty"`
	ServiceTier     string                  `json:"service_tier,omitempty"`

	// Goose declarations are execution inputs, not Agent identity. They are
	// copied while migrating legacy coding Apps so existing Goose tasks keep
	// their project recipe catalogue after the App link is cleared.
	GooseRecipeRepoURL string                 `json:"goose_recipe_repo_url,omitempty"`
	GooseRecipes       []AssistantGooseRecipe `json:"goose_recipes,omitempty"`
}

// AgentExecutionConfig describes a surface's current coding identity without
// exposing the reusable Agent or its secrets. AgentAvailable is false when the
// surface points at an Agent that has since been deleted.
type AgentExecutionConfig struct {
	AgentID         string                    `json:"agent_id,omitempty"`
	AgentName       string                    `json:"agent_name,omitempty"`
	AgentAvailable  bool                      `json:"agent_available"`
	Runtime         CodeAgentRuntime          `json:"runtime,omitempty"`
	CredentialType  CodeAgentCredentialType   `json:"credential_type,omitempty"`
	ProviderRef     string                    `json:"provider_ref,omitempty"`
	Model           string                    `json:"model,omitempty"`
	ReasoningEffort string                    `json:"reasoning_effort,omitempty"`
	ServiceTier     string                    `json:"service_tier,omitempty"`
	CodeAgentConfig *CodeAgentExecutionConfig `json:"code_agent_config,omitempty"`
	// CodeAgentOverrides is the override set the fields above were resolved
	// with, so a caller can round-trip an edit without having to know which
	// record (task or session) stores it.
	CodeAgentOverrides *CodeAgentOverrides `json:"code_agent_overrides,omitempty"`
}

// SessionExecutionConfigUpdateRequest replaces a session's coding identity.
// New callers send CodeAgentConfig. AgentID and CodeAgentOverrides remain for
// historical general-session clients.
type SessionExecutionConfigUpdateRequest struct {
	AgentID            string                    `json:"agent_id,omitempty"`
	CodeAgentConfig    *CodeAgentExecutionConfig `json:"code_agent_config,omitempty"`
	CodeAgentOverrides *CodeAgentOverrides       `json:"code_agent_overrides,omitempty"`
}

// SessionExecutionConfigUpdateResponse reports where the change landed.
// SpecTaskID is set when the session belongs to a SpecTask, because the task —
// not the session — owns CodeAgentConfig in that case.
type SessionExecutionConfigUpdateResponse struct {
	SessionID            string                    `json:"session_id"`
	SpecTaskID           string                    `json:"spec_task_id,omitempty"`
	AgentID              string                    `json:"agent_id,omitempty"`
	CodeAgentConfig      *CodeAgentExecutionConfig `json:"code_agent_config,omitempty"`
	CodeAgentOverrides   *CodeAgentOverrides       `json:"code_agent_overrides,omitempty"`
	AgentThreadRestarted bool                      `json:"agent_thread_restarted"`
}
