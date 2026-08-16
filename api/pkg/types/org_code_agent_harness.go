package types

import (
	"slices"
	"time"
)

// OrgCodeAgentHarness records whether an organization permits one coding
// harness. Provider endpoints and models deliberately do not live here:
// provider endpoints are managed independently, and a task selects a model
// from their current model lists when it starts.
type OrgCodeAgentHarness struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`

	OrganizationID string           `json:"organization_id" gorm:"uniqueIndex:idx_org_code_agent_harness"`
	Runtime        CodeAgentRuntime `json:"runtime" gorm:"uniqueIndex:idx_org_code_agent_harness"`
	Enabled        bool             `json:"enabled"`
}

func (OrgCodeAgentHarness) TableName() string {
	return "org_code_agent_harnesses"
}

// OrgCodeAgentHarnessUpdate replaces one harness's enabled state.
type OrgCodeAgentHarnessUpdate struct {
	Runtime CodeAgentRuntime `json:"runtime"`
	Enabled bool             `json:"enabled"`
}

type OrgCodeAgentHarnessesUpdateRequest struct {
	Harnesses []OrgCodeAgentHarnessUpdate `json:"harnesses"`
}

// OrgCodeAgentHarnessStatus is the organization policy plus viewer-scoped
// subscription availability. Subscription availability is informational; it
// never changes whether the organization enabled the harness, because the
// task may instead use any configured API provider.
type OrgCodeAgentHarnessStatus struct {
	Runtime               CodeAgentRuntime `json:"runtime"`
	Enabled               bool             `json:"enabled"`
	SupportsSubscription  bool             `json:"supports_subscription"`
	ViewerHasSubscription bool             `json:"viewer_has_subscription"`
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
