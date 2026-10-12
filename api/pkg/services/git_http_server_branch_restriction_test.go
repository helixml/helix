package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestBranchRestrictionIncludesApprovedProposalBranches(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hl-agent"}).Return(&types.ApiKey{Key: "hl-agent", SpecTaskID: "spt_1"}, nil)
	st.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{ID: "spt_1", BranchName: "feature/000001-task"}, nil)
	st.EXPECT().ListSpecTaskPRProposals(gomock.Any(), gomock.Any()).Return([]*types.SpecTaskPRProposal{
		{HeadBranch: "feature/000001-slice", Status: types.PRProposalStatusOpened},
		{HeadBranch: "feature/000001-rejected", Status: types.PRProposalStatusRejected},
	}, nil)
	s := &GitHTTPServer{store: st, prProposals: NewPRProposalService(st, nil, nil, "")}

	r, err := s.getBranchRestrictionForAPIKey(context.Background(), "Bearer hl-agent", "repo_1")
	require.NoError(t, err)
	require.Empty(t, r.ErrorMessage)
	require.Equal(t, []string{SpecsBranchName, "feature/000001-task", "feature/000001-slice"}, r.AllowedBranches)
}

// The receive-pack caller treats an error as "no restriction", so a failed
// proposal lookup must deny the push rather than let the agent push anywhere.
func TestBranchRestrictionFailsClosedWhenProposalsCannotBeRead(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(&types.ApiKey{Key: "hl-agent", SpecTaskID: "spt_1"}, nil)
	st.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{ID: "spt_1", BranchName: "feature/000001-task"}, nil)
	st.EXPECT().ListSpecTaskPRProposals(gomock.Any(), gomock.Any()).Return(nil, errors.New("connection reset"))
	s := &GitHTTPServer{store: st, prProposals: NewPRProposalService(st, nil, nil, "")}

	r, err := s.getBranchRestrictionForAPIKey(context.Background(), "hl-agent", "repo_1")
	require.NoError(t, err)
	require.NotNil(t, r)
	require.True(t, r.IsAgentKey)
	require.NotEmpty(t, r.ErrorMessage)
	require.Empty(t, r.AllowedBranches)
}
