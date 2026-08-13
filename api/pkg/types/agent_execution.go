package types

// Execution configuration shared by every surface that runs an external coding
// agent. A SpecTask and a plain chat session (org bot chat, project chat) both
// run the same Zed/ACP machinery, so both describe their coding identity with
// AgentExecutionConfig and both customize it with CodeAgentOverrides.

// CodeAgentOverrides customizes the coding model for a single execution surface
// — one SpecTask or one session — without mutating the reusable Agent
// configuration it was created from.
type CodeAgentOverrides struct {
	ProviderRef     string `json:"provider_ref,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
}

// AgentExecutionConfig describes a surface's current coding identity without
// exposing the reusable Agent or its secrets. AgentAvailable is false when the
// surface points at an Agent that has since been deleted.
type AgentExecutionConfig struct {
	AgentID         string                  `json:"agent_id,omitempty"`
	AgentName       string                  `json:"agent_name,omitempty"`
	AgentAvailable  bool                    `json:"agent_available"`
	Runtime         CodeAgentRuntime        `json:"runtime,omitempty"`
	CredentialType  CodeAgentCredentialType `json:"credential_type,omitempty"`
	ProviderRef     string                  `json:"provider_ref,omitempty"`
	Model           string                  `json:"model,omitempty"`
	ReasoningEffort string                  `json:"reasoning_effort,omitempty"`
	ServiceTier     string                  `json:"service_tier,omitempty"`
	// CodeAgentOverrides is the override set the fields above were resolved
	// with, so a caller can round-trip an edit without having to know which
	// record (task or session) stores it.
	CodeAgentOverrides *CodeAgentOverrides `json:"code_agent_overrides,omitempty"`
}

// SessionExecutionConfigUpdateRequest replaces a session's coding identity.
// CodeAgentOverrides is always the full replacement set; AgentID switches the
// session to a different Agent (and therefore possibly a different runtime).
type SessionExecutionConfigUpdateRequest struct {
	AgentID            string              `json:"agent_id,omitempty"`
	CodeAgentOverrides *CodeAgentOverrides `json:"code_agent_overrides,omitempty"`
}

// SessionExecutionConfigUpdateResponse reports where the change landed.
// SpecTaskID is set when the session belongs to a SpecTask, because the task —
// not the session — owns the overrides in that case.
type SessionExecutionConfigUpdateResponse struct {
	SessionID            string              `json:"session_id"`
	SpecTaskID           string              `json:"spec_task_id,omitempty"`
	AgentID              string              `json:"agent_id,omitempty"`
	CodeAgentOverrides   *CodeAgentOverrides `json:"code_agent_overrides,omitempty"`
	AgentThreadRestarted bool                `json:"agent_thread_restarted"`
}
