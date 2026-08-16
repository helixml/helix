package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/helixml/helix/api/pkg/types"
)

// TestCodeAgentAvailabilityIsViewerScoped is the core of the per-user
// subscription rule. An org can enable Claude Code for everyone, but a member
// who has not connected their own subscription must not be told it is ready —
// otherwise they pick it, the run resolves to no credential, and the failure
// surfaces mid-task instead of in the picker.
func TestCodeAgentAvailabilityIsViewerScoped(t *testing.T) {
	tests := []struct {
		name               string
		status             types.OrgCodeAgentProviderStatus
		wantAvailable      bool
		wantReasonNotables string
	}{
		{
			name: "subscription runtime with the viewer's own subscription",
			status: types.OrgCodeAgentProviderStatus{
				Runtime:               types.CodeAgentRuntimeClaudeCode,
				Enabled:               true,
				CredentialType:        types.CodeAgentCredentialTypeSubscription,
				ViewerHasSubscription: true,
			},
			wantAvailable: true,
		},
		{
			name: "subscription runtime, viewer has no subscription of their own",
			status: types.OrgCodeAgentProviderStatus{
				Runtime:               types.CodeAgentRuntimeClaudeCode,
				Enabled:               true,
				CredentialType:        types.CodeAgentCredentialTypeSubscription,
				ViewerHasSubscription: false,
			},
			wantAvailable:      false,
			wantReasonNotables: "your own subscription",
		},
		{
			name: "api key runtime with a pinned provider",
			status: types.OrgCodeAgentProviderStatus{
				Runtime:            types.CodeAgentRuntimeOpenCode,
				Enabled:            true,
				CredentialType:     types.CodeAgentCredentialTypeAPIKey,
				ProviderEndpointID: "pe_123",
			},
			wantAvailable: true,
		},
		{
			name: "api key runtime with no provider pinned",
			status: types.OrgCodeAgentProviderStatus{
				Runtime:        types.CodeAgentRuntimeOpenCode,
				Enabled:        true,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			wantAvailable:      false,
			wantReasonNotables: "No provider configured",
		},
		{
			name: "disabled runtime is never available even with a subscription",
			status: types.OrgCodeAgentProviderStatus{
				Runtime:               types.CodeAgentRuntimeClaudeCode,
				Enabled:               false,
				CredentialType:        types.CodeAgentCredentialTypeSubscription,
				ViewerHasSubscription: true,
			},
			wantAvailable:      false,
			wantReasonNotables: "Not enabled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			available, reason := codeAgentAvailability(&tc.status)
			assert.Equal(t, tc.wantAvailable, available)
			if tc.wantAvailable {
				assert.Empty(t, reason, "an available runtime must not carry a reason")
			} else {
				assert.Contains(t, reason, tc.wantReasonNotables)
			}
		})
	}
}

// TestSubscriptionRuntimes pins which runtimes may be configured for
// subscription credentials. Enabling subscription mode on any other runtime is
// rejected at the handler, so this list is load-bearing.
func TestSubscriptionRuntimes(t *testing.T) {
	assert.True(t, types.CodeAgentRuntimeClaudeCode.SupportsSubscriptionCredentials())
	assert.True(t, types.CodeAgentRuntimeCodexCLI.SupportsSubscriptionCredentials())

	for _, runtime := range []types.CodeAgentRuntime{
		types.CodeAgentRuntimeOpenCode,
		types.CodeAgentRuntimeGooseCode,
		types.CodeAgentRuntimeZedAgent,
	} {
		assert.False(t, runtime.SupportsSubscriptionCredentials(),
			"%s must not accept subscription credentials", runtime)
	}
}

// TestSelectableRuntimesAreRecognised guards the handler's reject-unknown gate.
func TestSelectableRuntimesAreRecognised(t *testing.T) {
	for _, runtime := range types.SelectableCodeAgentRuntimes {
		assert.True(t, types.IsSelectableCodeAgentRuntime(runtime))
	}
	for _, runtime := range []types.CodeAgentRuntime{"gemini_cli", "qwen_code"} {
		assert.False(t, types.IsSelectableCodeAgentRuntime(runtime),
			"%s is not offered in settings and must be rejected", runtime)
	}
	assert.False(t, types.IsSelectableCodeAgentRuntime(""))
	assert.False(t, types.IsSelectableCodeAgentRuntime("not_a_runtime"))
}

// TestBuiltInRowsAreKeyedByEmptyName pins the read-side half of the
// "could not turn Codex off" bug. Rows written before the name column existed
// carried NULL, which Postgres treats as distinct from ” in the unique index,
// so a stale row survived alongside its replacement. Both scan into Go as "",
// so both landed in the built-in lookup and the later one won — reporting the
// old enabled value no matter what was saved. The migration collapses NULL to
// ” and makes the column NOT NULL; this asserts the lookup rule the migration
// exists to keep honest.
func TestBuiltInRowsAreKeyedByEmptyName(t *testing.T) {
	rows := []*types.OrgCodeAgentProvider{
		{Runtime: types.CodeAgentRuntimeCodexCLI, Name: "", Enabled: false},
		{Runtime: types.CodeAgentRuntimeCodexCLI, Name: "qwen", Enabled: true},
	}

	builtIn := map[types.CodeAgentRuntime]*types.OrgCodeAgentProvider{}
	var flavours []*types.OrgCodeAgentProvider
	for _, row := range rows {
		if row.Name == "" {
			builtIn[row.Runtime] = row
			continue
		}
		flavours = append(flavours, row)
	}

	assert.False(t, builtIn[types.CodeAgentRuntimeCodexCLI].Enabled,
		"the built-in row owns the harness's enabled state; a flavour must not override it")
	assert.Len(t, flavours, 1)
	assert.Equal(t, "qwen", flavours[0].Name)
}
