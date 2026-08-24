package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/openai/manager"
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
		name           string
		row            *types.OrgCodeAgentHarness
		storeErr       error
		credentialType types.CodeAgentCredentialType
		providerRef    string
		endpoints      []*types.ProviderEndpoint
		wantErr        string
	}{
		{name: "nil inherits visible provider", row: &types.OrgCodeAgentHarness{Enabled: true}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", endpoints: []*types.ProviderEndpoint{{ID: "provider-1", Name: "anthropic"}}},
		{name: "explicit provider allowed", row: &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{"provider-1"}}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", endpoints: []*types.ProviderEndpoint{{ID: "provider-1", Name: "anthropic"}}},
		{name: "explicit empty denies provider", row: &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{}}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", endpoints: []*types.ProviderEndpoint{{ID: "provider-1", Name: "anthropic"}}, wantErr: "is not enabled"},
		{name: "provider not visible to org", row: &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{"provider-1"}}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", endpoints: []*types.ProviderEndpoint{}, wantErr: "is not enabled"},
		{name: "native runtime rejects incompatible provider", row: &types.OrgCodeAgentHarness{Enabled: true}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", endpoints: []*types.ProviderEndpoint{{ID: "provider-1", Name: "togetherai"}}, wantErr: "is not enabled"},
		{name: "provider denied in subscription mode", row: &types.OrgCodeAgentHarness{Enabled: true, SubscriptionEnabled: boolPointer(true), ProviderRefs: []string{"provider-1"}}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", wantErr: "is not enabled"},
		{name: "subscription ignores usage provider", row: &types.OrgCodeAgentHarness{Enabled: true, SubscriptionEnabled: boolPointer(true), ProviderRefs: []string{}}, credentialType: types.CodeAgentCredentialTypeSubscription, providerRef: "openai"},
		{name: "subscription denied by legacy nil", row: &types.OrgCodeAgentHarness{Enabled: true}, credentialType: types.CodeAgentCredentialTypeSubscription, wantErr: "subscription credentials are not enabled"},
		{name: "subscription explicitly disabled", row: &types.OrgCodeAgentHarness{Enabled: true, SubscriptionEnabled: boolPointer(false)}, credentialType: types.CodeAgentCredentialTypeSubscription, wantErr: "subscription credentials are not enabled"},
		{name: "disabled", row: &types.OrgCodeAgentHarness{Enabled: false}, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", wantErr: "not enabled"},
		{name: "missing policy inherits visible provider", storeErr: store.ErrNotFound, credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", endpoints: []*types.ProviderEndpoint{{ID: "provider-1", Name: "anthropic"}}},
		{name: "store failure", storeErr: fmt.Errorf("database unavailable"), credentialType: types.CodeAgentCredentialTypeAPIKey, providerRef: "provider-1", wantErr: "failed to load"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Store: mockStore}
			if tc.endpoints != nil {
				providerManager := manager.NewMockProviderManager(ctrl)
				providerManager.EXPECT().
					ListProviderEndpointsForOwner(gomock.Any(), "org_1", types.OwnerTypeOrg).
					Return(tc.endpoints, nil)
				server.providerManager = providerManager
			}
			mockStore.EXPECT().GetOrgCodeAgentHarness(
				gomock.Any(), "org_1", types.CodeAgentRuntimeClaudeCode,
			).Return(tc.row, tc.storeErr)

			err := server.validateOrgCodeAgentHarness(
				context.Background(), "org_1", types.CodeAgentRuntimeClaudeCode, tc.credentialType, tc.providerRef,
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

func TestEnableSubscriptionCodeAgentHarness(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().UpsertOrgCodeAgentHarnesses(
		gomock.Any(),
		"org_1",
		"user_1",
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _, _ string, updates []types.OrgCodeAgentHarnessUpdate) ([]*types.OrgCodeAgentHarness, error) {
		require.Len(t, updates, 1)
		assert.Equal(t, types.CodeAgentRuntimeCodexCLI, updates[0].Runtime)
		assert.True(t, updates[0].Enabled)
		require.NotNil(t, updates[0].SubscriptionEnabled)
		assert.True(t, *updates[0].SubscriptionEnabled)
		require.NotNil(t, updates[0].ProviderRefs)
		assert.Empty(t, updates[0].ProviderRefs)
		return nil, nil
	})

	require.NoError(t, server.enableSubscriptionCodeAgentHarness(
		context.Background(), "org_1", "user_1", types.CodeAgentRuntimeCodexCLI,
	))
}

func TestEnableSubscriptionCodeAgentHarnessSkipsPersonalWorkspace(t *testing.T) {
	server := &HelixAPIServer{}
	require.NoError(t, server.enableSubscriptionCodeAgentHarness(
		context.Background(), "", "user_1", types.CodeAgentRuntimeClaudeCode,
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

func TestNormalizeHarnessCredentialSources(t *testing.T) {
	t.Run("subscription clears API providers", func(t *testing.T) {
		update := types.OrgCodeAgentHarnessUpdate{
			Runtime:             types.CodeAgentRuntimeClaudeCode,
			SubscriptionEnabled: boolPointer(true),
		}
		require.NoError(t, normalizeHarnessCredentialSources(&update))
		require.NotNil(t, update.ProviderRefs)
		assert.Empty(t, update.ProviderRefs)
	})

	t.Run("API providers disable subscription", func(t *testing.T) {
		update := types.OrgCodeAgentHarnessUpdate{
			Runtime:      types.CodeAgentRuntimeCodexCLI,
			ProviderRefs: []string{"openai"},
		}
		require.NoError(t, normalizeHarnessCredentialSources(&update))
		require.NotNil(t, update.SubscriptionEnabled)
		assert.False(t, *update.SubscriptionEnabled)
	})

	t.Run("rejects both modes", func(t *testing.T) {
		update := types.OrgCodeAgentHarnessUpdate{
			Runtime:             types.CodeAgentRuntimeClaudeCode,
			SubscriptionEnabled: boolPointer(true),
			ProviderRefs:        []string{"anthropic"},
		}
		require.ErrorContains(t, normalizeHarnessCredentialSources(&update), "cannot both be enabled")
	})

	t.Run("rejects subscriptions for unsupported harness", func(t *testing.T) {
		update := types.OrgCodeAgentHarnessUpdate{
			Runtime:             types.CodeAgentRuntimeOpenCode,
			SubscriptionEnabled: boolPointer(true),
		}
		require.ErrorContains(t, normalizeHarnessCredentialSources(&update), "does not support")
	})
}

func TestFilterProviderEndpointsByRefs(t *testing.T) {
	endpoints := []*types.ProviderEndpoint{
		{ID: "provider-1", Name: "renamed"},
		{ID: "global/openai", Name: "openai", EndpointType: types.ProviderEndpointTypeGlobal},
		{ID: "provider-2", Name: "other"},
	}
	filtered := filterProviderEndpointsByRefs(endpoints, []string{"provider-1"})
	require.Len(t, filtered, 1)
	assert.Equal(t, "provider-1", filtered[0].ID)

	filtered = filterProviderEndpointsByRefs(endpoints, []string{})
	require.NotNil(t, filtered)
	assert.Empty(t, filtered)

	endpoints = []*types.ProviderEndpoint{
		{ID: "global/openai", Name: "openai", EndpointType: types.ProviderEndpointTypeGlobal},
		{ID: "pe_global", Name: "openai", EndpointType: types.ProviderEndpointTypeGlobal},
		{ID: "pe_org", Name: "user/openai", EndpointType: types.ProviderEndpointTypeOrg},
	}
	filtered = filterProviderEndpointsByRefs(endpoints, []string{"openai"})
	require.Len(t, filtered, 1)
	assert.Equal(t, "pe_org", filtered[0].ID)
	filtered = filterProviderEndpointsByRefs(endpoints, []string{"global/openai"})
	require.Len(t, filtered, 1)
	assert.Equal(t, "global/openai", filtered[0].ID)
}

func TestFilterProviderEndpointsForHarness(t *testing.T) {
	endpoints := []*types.ProviderEndpoint{
		{ID: "pe_anthropic", Name: "anthropic"},
		{ID: "pe_together", Name: "togetherai"},
	}

	inherited := filterProviderEndpointsForHarness(endpoints, &types.OrgCodeAgentHarness{Enabled: true}, types.CodeAgentRuntimeClaudeCode)
	require.Len(t, inherited, 1)
	assert.Equal(t, "pe_anthropic", inherited[0].ID)

	explicitNone := filterProviderEndpointsForHarness(endpoints, &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{}}, types.CodeAgentRuntimeZedAgent)
	assert.Empty(t, explicitNone)

	allowlisted := filterProviderEndpointsForHarness(endpoints, &types.OrgCodeAgentHarness{Enabled: true, ProviderRefs: []string{"pe_together"}}, types.CodeAgentRuntimeZedAgent)
	require.Len(t, allowlisted, 1)
	assert.Equal(t, "pe_together", allowlisted[0].ID)
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
	assert.True(t, byRuntime[types.CodeAgentRuntimeCodexCLI].Enabled)
	assert.Nil(t, byRuntime[types.CodeAgentRuntimeCodexCLI].ProviderRefs)
	assert.Nil(t, byRuntime[types.CodeAgentRuntimeCodexCLI].SubscriptionEnabled)
	assert.False(t, byRuntime[types.CodeAgentRuntimeCodexCLI].ViewerHasSubscription)
}
