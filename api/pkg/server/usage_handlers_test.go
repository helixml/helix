package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestMergeSandboxUsageCostsAddsSandboxSpend(t *testing.T) {
	date := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	metrics := []*types.AggregatedUsageMetric{
		{
			Date:           date,
			PromptCost:     1,
			CompletionCost: 2,
			TotalCost:      3,
			TotalRequests:  4,
		},
	}
	sandboxMetrics := []*types.AggregatedUsageMetric{
		{
			Date:          date,
			SandboxCost:   2.5,
			TotalCost:     2.5,
			TotalRequests: 1,
		},
	}

	mergeSandboxUsageCosts(metrics, sandboxMetrics)

	require.Equal(t, 2.5, metrics[0].SandboxCost)
	require.Equal(t, 5.5, metrics[0].TotalCost)
	require.Equal(t, 5, metrics[0].TotalRequests)
}

func TestGetOrgUsageSummaryParsesFiltersAndPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	org := &types.Organization{
		ID:   "org_123",
		Name: "koala-bunny-corp",
	}
	user := types.User{ID: "user_admin", Admin: true}
	from := "2026-01-01T00:00:00Z"
	to := "2026-02-28T23:59:59Z"
	fromTime, err := time.Parse(time.RFC3339, from)
	require.NoError(t, err)
	toTime, err := time.Parse(time.RFC3339, to)
	require.NoError(t, err)

	mockStore.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: org.Name}).
		Return(org, nil)
	mockStore.EXPECT().
		GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)
	mockStore.EXPECT().
		GetOrgUsageSummary(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, q *store.GetOrgUsageSummaryQuery) (*types.OrgUsageSummaryResponse, error) {
			require.Equal(t, org.ID, q.OrganizationID)
			require.Equal(t, fromTime, q.From)
			require.Equal(t, toTime, q.To)
			require.Equal(t, "user_456", q.UserID)
			require.Equal(t, "prj_456", q.ProjectID)
			require.Equal(t, "app_456", q.AppID)
			require.Equal(t, "ses_456", q.SessionID)
			require.Equal(t, "anthropic", q.Provider)
			require.Equal(t, "claude-sonnet-4", q.Model)
			require.Equal(t, "alice@example.com", q.UserSearch)
			require.Equal(t, 25, q.UserLimit)
			require.Equal(t, 50, q.UserOffset)
			require.Equal(t, 10, q.ProjectLimit)
			require.Equal(t, 20, q.ProjectOffset)
			require.Equal(t, 10, q.TaskLimit)
			require.Equal(t, 30, q.TaskOffset)
			require.Equal(t, 25, q.SessionLimit)
			require.Equal(t, 75, q.SessionOffset)
			return &types.OrgUsageSummaryResponse{}, nil
		})

	req := httptest.NewRequest("GET", "/api/v1/usage/org-summary?org_id=koala-bunny-corp&from="+from+"&to="+to+"&user_id=user_456&project_id=prj_456&app_id=app_456&session_id=ses_456&provider=anthropic&model=claude-sonnet-4&user_search=alice@example.com&user_limit=25&user_offset=50&project_limit=10&project_offset=20&task_limit=10&task_offset=30&session_limit=25&session_offset=75", nil)
	req = req.WithContext(setRequestUser(req.Context(), user))

	resp, httpErr := server.getOrgUsageSummary(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
	require.NotNil(t, resp)
}
