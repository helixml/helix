package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestSelectableCodeAgentRuntimes(t *testing.T) {
	for _, runtime := range types.SelectableCodeAgentRuntimes {
		assert.True(t, types.IsSelectableCodeAgentRuntime(runtime))
	}
	for _, runtime := range []types.CodeAgentRuntime{"", "gemini_cli", "qwen_code", "unknown"} {
		assert.False(t, types.IsSelectableCodeAgentRuntime(runtime))
	}
}

func TestValidateOrgCodeAgentHarness(t *testing.T) {
	tests := []struct {
		name     string
		row      *types.OrgCodeAgentHarness
		storeErr error
		wantErr  string
	}{
		{name: "enabled", row: &types.OrgCodeAgentHarness{Enabled: true}},
		{name: "provider allowed", row: &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{"provider-1"}}},
		{name: "provider denied", row: &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{"provider-2"}}, wantErr: "is not enabled"},
		{name: "subscription allowed", row: &types.OrgCodeAgentHarness{Enabled: true}},
		{name: "subscription denied", row: &types.OrgCodeAgentHarness{Enabled: true, SubscriptionEnabled: boolPointer(false)}, wantErr: "subscription credentials are not enabled"},
		{name: "disabled", row: &types.OrgCodeAgentHarness{Enabled: false}, wantErr: "not enabled"},
		{name: "missing", storeErr: store.ErrNotFound, wantErr: "not enabled"},
		{name: "store failure", storeErr: fmt.Errorf("database unavailable"), wantErr: "failed to load"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Store: mockStore}
			mockStore.EXPECT().GetOrgCodeAgentHarness(
				gomock.Any(), "org_1", types.CodeAgentRuntimeClaudeCode,
			).Return(tc.row, tc.storeErr)

			credentialType := types.CodeAgentCredentialTypeAPIKey
			providerRef := "provider-1"
			if tc.name == "subscription allowed" || tc.name == "subscription denied" {
				credentialType = types.CodeAgentCredentialTypeSubscription
				providerRef = ""
			}
			err := server.validateOrgCodeAgentHarness(
				context.Background(), "org_1", types.CodeAgentRuntimeClaudeCode, credentialType, providerRef,
			)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestValidateOrgCodeAgentHarnessSkipsPersonalWorkspace(t *testing.T) {
	server := &HelixAPIServer{}
	require.NoError(t, server.validateOrgCodeAgentHarness(
		context.Background(), "", types.CodeAgentRuntimeClaudeCode, types.CodeAgentCredentialTypeAPIKey, "provider-1",
	))
}

func TestNormalizeHarnessProviderRefs(t *testing.T) {
	refs, err := normalizeHarnessProviderRefs([]string{" provider-2 ", "provider-1", "provider-2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"provider-1", "provider-2"}, refs)

	refs, err = normalizeHarnessProviderRefs([]string{})
	require.NoError(t, err)
	require.NotNil(t, refs)
	assert.Empty(t, refs)

	_, err = normalizeHarnessProviderRefs([]string{""})
	require.ErrorContains(t, err, "empty reference")
}

func TestFilterProviderEndpointsByRefs(t *testing.T) {
	endpoints := []*types.ProviderEndpoint{
		{ID: "provider-1", Name: "renamed"},
		{ID: "", Name: "openai"},
		{ID: "provider-2", Name: "other"},
	}
	filtered := filterProviderEndpointsByRefs(endpoints, []string{"provider-1", "openai"})
	require.Len(t, filtered, 2)
	assert.Equal(t, "provider-1", filtered[0].ID)
	assert.Equal(t, "openai", filtered[1].Name)

	filtered = filterProviderEndpointsByRefs(endpoints, []string{})
	require.NotNil(t, filtered)
	assert.Empty(t, filtered)
}

func boolPointer(value bool) *bool {
	return &value
}

func TestBuildOrgCodeAgentHarnessStatuses(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().ListOrgCodeAgentHarnesses(gomock.Any(), "org_1").Return(
		[]*types.OrgCodeAgentHarness{{
			Runtime:             types.CodeAgentRuntimeClaudeCode,
			Enabled:             true,
			SubscriptionEnabled: boolPointer(false),
			ProviderRefs:        []string{"provider-1"},
		}}, nil,
	)
	mockStore.EXPECT().GetEffectiveClaudeSubscription(gomock.Any(), "user_1", "org_1").Return(
		&types.ClaudeSubscription{}, nil,
	)
	mockStore.EXPECT().GetEffectiveCodexSubscription(gomock.Any(), "user_1", "org_1").Return(
		nil, store.ErrNotFound,
	)

	statuses, err := server.buildOrgCodeAgentHarnessStatuses(
		context.Background(), "org_1", "user_1",
	)
	require.NoError(t, err)
	require.Len(t, statuses, len(types.SelectableCodeAgentRuntimes))

	byRuntime := make(map[types.CodeAgentRuntime]*types.OrgCodeAgentHarnessStatus, len(statuses))
	for _, status := range statuses {
		byRuntime[status.Runtime] = status
	}
	assert.True(t, byRuntime[types.CodeAgentRuntimeClaudeCode].Enabled)
	require.NotNil(t, byRuntime[types.CodeAgentRuntimeClaudeCode].SubscriptionEnabled)
	assert.False(t, *byRuntime[types.CodeAgentRuntimeClaudeCode].SubscriptionEnabled)
	assert.Equal(t, []string{"provider-1"}, byRuntime[types.CodeAgentRuntimeClaudeCode].ProviderRefs)
	assert.True(t, byRuntime[types.CodeAgentRuntimeClaudeCode].ViewerHasSubscription)
	assert.False(t, byRuntime[types.CodeAgentRuntimeCodexCLI].Enabled)
	assert.False(t, byRuntime[types.CodeAgentRuntimeCodexCLI].ViewerHasSubscription)
}
