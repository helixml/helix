package types

import (
	"slices"
	"time"
)

// OrgCodeAgentHarness records whether an organization permits one coding
// harness and which provider endpoints that harness may use. Models
// deliberately do not live here: a task selects a model from the allowed
// providers' current model lists when it starts. API providers and subscription
// credentials are both opt-in.
type OrgCodeAgentHarness struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`

	OrganizationID string           `json:"organization_id" gorm:"uniqueIndex:idx_org_code_agent_harness"`
	Runtime        CodeAgentRuntime `json:"runtime" gorm:"uniqueIndex:idx_org_code_agent_harness"`
	Enabled        bool             `json:"enabled"`
	// Subscription mode is opt-in. Nil is treated as disabled so legacy rows
	// remain in API-provider mode until an owner explicitly connects or selects
	// a subscription.
	SubscriptionEnabled *bool `json:"subscription_enabled"`
	// Nil and an empty list allow no providers. Providers must be explicitly
	// enabled for each harness.
	ProviderRefs []string `json:"provider_refs" gorm:"type:jsonb;serializer:json"`
}

func (OrgCodeAgentHarness) TableName() string {
	return "org_code_agent_harnesses"
}

// OrgCodeAgentHarnessUpdate updates one harness. Nil source fields preserve
// their existing values.
type OrgCodeAgentHarnessUpdate struct {
	Runtime             CodeAgentRuntime `json:"runtime"`
	Enabled             bool             `json:"enabled"`
	SubscriptionEnabled *bool            `json:"subscription_enabled"`
	ProviderRefs        []string         `json:"provider_refs"`
}

type OrgCodeAgentHarnessesUpdateRequest struct {
	Harnesses []OrgCodeAgentHarnessUpdate `json:"harnesses"`
}

// OrgCodeAgentHarnessStatus is the organization policy plus viewer-scoped
// subscription availability. ViewerHasSubscription is informational and is
// combined with SubscriptionEnabled to decide whether subscription models are
// available to the requesting member.
type OrgCodeAgentHarnessStatus struct {
	Runtime               CodeAgentRuntime `json:"runtime"`
	Enabled               bool             `json:"enabled"`
	SubscriptionEnabled   *bool            `json:"subscription_enabled"`
	ProviderRefs          []string         `json:"provider_refs"`
	SupportsSubscription  bool             `json:"supports_subscription"`
	ViewerHasSubscription bool             `json:"viewer_has_subscription"`
}

func (h *OrgCodeAgentHarness) AllowsProvider(providerRef string) bool {
	return !h.AllowsSubscription() && slices.Contains(h.ProviderRefs, providerRef)
}

func (h *OrgCodeAgentHarness) AllowsSubscription() bool {
	return h.SubscriptionEnabled != nil && *h.SubscriptionEnabled
}

var SubscriptionCodeAgentRuntimes = map[CodeAgentRuntime]bool{
	CodeAgentRuntimeClaudeCode: true,
	CodeAgentRuntimeCodexCLI:   true,
}

func (r CodeAgentRuntime) SupportsSubscriptionCredentials() bool {
	return SubscriptionCodeAgentRuntimes[r]
}

// SelectableCodeAgentRuntimes is the canonical ordered list exposed by the
// settings API and task picker.
var SelectableCodeAgentRuntimes = []CodeAgentRuntime{
	CodeAgentRuntimeClaudeCode,
	CodeAgentRuntimeCodexCLI,
	CodeAgentRuntimeGooseCode,
	CodeAgentRuntimeQwenCode,
	CodeAgentRuntimeZedAgent,
	CodeAgentRuntimeOpenCode,
	CodeAgentRuntimeDeepSeekHarness,
}

func IsSelectableCodeAgentRuntime(runtime CodeAgentRuntime) bool {
	return slices.Contains(SelectableCodeAgentRuntimes, runtime)
}
