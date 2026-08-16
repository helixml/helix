package types

import (
	"slices"
	"time"
)

// OrgCodeAgentProvider records whether one coding-agent runtime is available to
// an organization, and how it authenticates when it is.
//
// This is the org's *allow list*, not a credential store. Deliberately, it
// never names a subscription: Claude and Codex subscriptions belong to
// individual users and carry their own consent model (see
// ClaudeSubscription.DelegatedOrgIDs and ResolveClaudeCredentialOwner). Pinning
// one subscription here would make every member spend one person's quota, which
// is exactly what that model exists to prevent. Enabling a runtime with
// CredentialType "subscription" means "members may use this runtime with their
// own subscription" — resolution still happens per session owner at run time.
//
// One row per (OrganizationID, Runtime, Name). Name is empty for the built-in
// row every supported harness gets; additional named rows are "flavours" — the
// same harness pointed at a different provider or model, e.g. one opencode on
// qwen and another on deepseek. Absence of a row means the harness has never
// been configured, which reads as disabled.
type OrgCodeAgentProvider struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`

	OrganizationID string           `json:"organization_id" gorm:"index:idx_org_code_agent_provider_named,unique,priority:1"`
	Runtime        CodeAgentRuntime `json:"runtime" gorm:"index:idx_org_code_agent_provider_named,unique,priority:2"`

	// Name distinguishes flavours of the same harness. Empty is the built-in
	// row, which is why it is part of the key rather than a separate flag: the
	// default and its flavours are the same kind of thing and are configured,
	// listed and selected identically.
	Name string `json:"name,omitempty" gorm:"index:idx_org_code_agent_provider_named,unique,priority:3"`

	// Enabled controls whether members may pick this runtime for a task.
	Enabled bool `json:"enabled"`

	// CredentialType is how this runtime authenticates for the org. For
	// "subscription" the acting user's own subscription is resolved at run time;
	// for "api_key" requests route through ProviderEndpointID.
	CredentialType CodeAgentCredentialType `json:"credential_type,omitempty"`

	// ProviderEndpointID pins the LLM provider used when CredentialType is
	// "api_key". Naming it here is what lets the task picker drop its provider
	// selector and offer only this provider's models.
	ProviderEndpointID string `json:"provider_endpoint_id,omitempty"`

	// DefaultModel is the model offered first for this runtime. Optional; empty
	// means the picker offers the provider's full model list with no preselection.
	DefaultModel string `json:"default_model,omitempty"`
}

func (OrgCodeAgentProvider) TableName() string {
	return "org_code_agent_providers"
}

// OrgCodeAgentProviderUpdate is one entry of a settings save. Runtime selects
// the row; the remaining fields replace it wholesale.
type OrgCodeAgentProviderUpdate struct {
	Runtime            CodeAgentRuntime        `json:"runtime"`
	Name               string                  `json:"name,omitempty"`
	Enabled            bool                    `json:"enabled"`
	CredentialType     CodeAgentCredentialType `json:"credential_type,omitempty"`
	ProviderEndpointID string                  `json:"provider_endpoint_id,omitempty"`
	DefaultModel       string                  `json:"default_model,omitempty"`
}

// OrgCodeAgentProvidersUpdateRequest replaces the named rows' settings. Rows
// absent from Providers are left untouched, so a client can save one row
// without having to send the whole set. Delete names flavours to remove; a
// built-in row (empty name) cannot be deleted, only disabled.
type OrgCodeAgentProvidersUpdateRequest struct {
	Providers []OrgCodeAgentProviderUpdate `json:"providers"`
	Delete    []OrgCodeAgentProviderRef    `json:"delete,omitempty"`
}

// OrgCodeAgentProviderRef identifies one row.
type OrgCodeAgentProviderRef struct {
	Runtime CodeAgentRuntime `json:"runtime"`
	Name    string           `json:"name"`
}

// OrgCodeAgentProviderStatus is one row as the settings UI needs to render it:
// the stored configuration plus the live availability facts the UI would
// otherwise have to assemble from three separate endpoints.
type OrgCodeAgentProviderStatus struct {
	Runtime CodeAgentRuntime `json:"runtime"`
	// Name is empty for the built-in row and set for an added flavour.
	Name string `json:"name,omitempty"`
	// IsFlavour marks a row the org added on top of the built-in harness list,
	// which is what the UI allows deleting. The built-in rows are permanent.
	IsFlavour          bool                    `json:"is_flavour"`
	Enabled            bool                    `json:"enabled"`
	CredentialType     CodeAgentCredentialType `json:"credential_type,omitempty"`
	ProviderEndpointID string                  `json:"provider_endpoint_id,omitempty"`
	DefaultModel       string                  `json:"default_model,omitempty"`

	// SupportsSubscription is true for runtimes that can authenticate with a
	// personal subscription (Claude Code, Codex). Everything else is API-key only.
	SupportsSubscription bool `json:"supports_subscription"`

	// ViewerHasSubscription reports whether *the requesting user* has a
	// subscription for this runtime. It is deliberately viewer-scoped: with
	// per-user resolution, whether the runtime actually works differs per member,
	// and an org-wide "connected" flag would be a lie for everyone else.
	ViewerHasSubscription bool `json:"viewer_has_subscription"`

	// Available is whether this viewer can actually run this runtime right now —
	// enabled, and either holding a subscription or having a usable API-key
	// provider. The task picker offers exactly the runtimes where this is true.
	Available bool `json:"available"`

	// UnavailableReason explains a false Available so the UI can say why instead
	// of silently hiding the row.
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// SubscriptionCodeAgentRuntimes are the runtimes that can authenticate with a
// personal subscription instead of an API key.
var SubscriptionCodeAgentRuntimes = map[CodeAgentRuntime]bool{
	CodeAgentRuntimeClaudeCode: true,
	CodeAgentRuntimeCodexCLI:   true,
}

// SupportsSubscriptionCredentials reports whether a runtime can run on a
// personal subscription.
func (r CodeAgentRuntime) SupportsSubscriptionCredentials() bool {
	return SubscriptionCodeAgentRuntimes[r]
}

// SelectableCodeAgentRuntimes is the set an organization can enable, in the
// order the settings UI lists them.
//
// Every entry is always returned by the list endpoint, configured or not, so
// the settings page can show the full set of supported harnesses with an off
// switch rather than an empty page an owner has no way to act on.
var SelectableCodeAgentRuntimes = []CodeAgentRuntime{
	CodeAgentRuntimeClaudeCode,
	CodeAgentRuntimeCodexCLI,
	CodeAgentRuntimeGooseCode,
	CodeAgentRuntimeZedAgent,
	CodeAgentRuntimeOpenCode,
}

// IsSelectableCodeAgentRuntime reports whether a runtime is one an org can
// enable, so a handler can reject an unknown or retired value rather than
// writing a row nothing will ever read.
func IsSelectableCodeAgentRuntime(runtime CodeAgentRuntime) bool {
	return slices.Contains(SelectableCodeAgentRuntimes, runtime)
}
