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

			err := server.validateOrgCodeAgentHarness(
				context.Background(), "org_1", types.CodeAgentRuntimeClaudeCode,
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
		context.Background(), "", types.CodeAgentRuntimeClaudeCode,
	))
}

func TestBuildOrgCodeAgentHarnessStatuses(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().ListOrgCodeAgentHarnesses(gomock.Any(), "org_1").Return(
		[]*types.OrgCodeAgentHarness{{Runtime: types.CodeAgentRuntimeClaudeCode, Enabled: true}}, nil,
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
	assert.True(t, byRuntime[types.CodeAgentRuntimeClaudeCode].ViewerHasSubscription)
	assert.False(t, byRuntime[types.CodeAgentRuntimeCodexCLI].Enabled)
	assert.False(t, byRuntime[types.CodeAgentRuntimeCodexCLI].ViewerHasSubscription)
}
