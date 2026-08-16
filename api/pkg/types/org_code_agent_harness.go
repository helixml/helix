package types

import (
	"slices"
	"time"
)

// OrgCodeAgentHarness records whether an organization permits one coding
// harness and which provider endpoints that harness may use. Models
// deliberately do not live here: a task selects a model from the allowed
// providers' current model lists when it starts.
type OrgCodeAgentHarness struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`

	OrganizationID string           `json:"organization_id" gorm:"uniqueIndex:idx_org_code_agent_harness"`
	Runtime        CodeAgentRuntime `json:"runtime" gorm:"uniqueIndex:idx_org_code_agent_harness"`
	Enabled        bool             `json:"enabled"`
	// Nil preserves the pre-source-policy behaviour where subscription mode is
	// allowed for runtimes that support it. An explicit false disables it.
	SubscriptionEnabled *bool `json:"subscription_enabled"`
	// Nil preserves the pre-allow-list behaviour where every provider visible
	// to the organization is allowed. A non-nil empty list allows none. Once an
	// owner changes a provider switch, the UI writes an explicit full list so
	// newly connected providers do not become exposed automatically.
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
	return h.ProviderRefs == nil || slices.Contains(h.ProviderRefs, providerRef)
}

func (h *OrgCodeAgentHarness) AllowsSubscription() bool {
	return h.SubscriptionEnabled == nil || *h.SubscriptionEnabled
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
	CodeAgentRuntimeZedAgent,
	CodeAgentRuntimeOpenCode,
}

func IsSelectableCodeAgentRuntime(runtime CodeAgentRuntime) bool {
	return slices.Contains(SelectableCodeAgentRuntimes, runtime)
}
