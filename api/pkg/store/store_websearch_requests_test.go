package store

import (
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_WebSearchRequests() {
	org := "org_" + system.GenerateUUID()
	day1 := time.Date(2026, 11, 2, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 11, 3, 0, 1, 0, 0, time.UTC)
	rows := []*types.WebSearchRequest{
		{Created: day1, Endpoint: types.WebSearchEndpointSearch, Status: 200, CostDollars: 0.007},
		{Created: day1.Add(-time.Hour), Endpoint: types.WebSearchEndpointSearch, Status: 200, CostDollars: 0.0001},
		{Created: day1.Add(-2 * time.Hour), Endpoint: types.WebSearchEndpointSearch, Status: 402, ErrorTag: "NO_MORE_CREDITS"},
		{Created: day2, Endpoint: types.WebSearchEndpointAnswer, Status: 200, CostDollars: 0.005},
		{Created: day2.Add(time.Minute), Endpoint: types.WebSearchEndpointAnswer, Status: 400, ErrorTag: "INVALID_REQUEST"},
		// Outside the range, or another org's.
		{Created: day2.AddDate(0, 0, 1), Endpoint: types.WebSearchEndpointSearch, Status: 200, CostDollars: 1},
	}
	var ids []string
	for _, row := range rows {
		row.ID = "req_" + system.GenerateUUID()
		row.OrganizationID = org
		row.Source = types.WebSearchSourceAPI
		suite.Require().NoError(suite.db.CreateWebSearchRequest(suite.ctx, row))
		ids = append(ids, row.ID)
	}
	other := &types.WebSearchRequest{ID: "req_" + system.GenerateUUID(), OrganizationID: "org_" + system.GenerateUUID(), Created: day1, Endpoint: types.WebSearchEndpointSearch, Status: 200}
	suite.Require().NoError(suite.db.CreateWebSearchRequest(suite.ctx, other))
	ids = append(ids, other.ID)
	suite.T().Cleanup(func() {
		_ = suite.db.gdb.WithContext(suite.ctx).Where("id IN ?", ids).Delete(&types.WebSearchRequest{}).Error
	})
	q := &WebSearchRequestsQuery{OrganizationID: org, From: day1.Truncate(24 * time.Hour), To: day2.Truncate(24 * time.Hour).AddDate(0, 0, 1)}

	usage, err := suite.db.GetWebSearchUsage(suite.ctx, q)
	suite.Require().NoError(err)
	suite.Equal(types.WebSearchUsageCounts{Requests: 5, CostDollars: 0.0121, Succeeded: 3, OutOfCredits: 1, Failed: 1}, usage.Totals)
	suite.Equal([]types.WebSearchUsageDay{
		{Date: "2026-11-02", Endpoint: types.WebSearchEndpointSearch, WebSearchUsageCounts: types.WebSearchUsageCounts{Requests: 3, CostDollars: 0.0071, Succeeded: 2, OutOfCredits: 1}},
		{Date: "2026-11-03", Endpoint: types.WebSearchEndpointAnswer, WebSearchUsageCounts: types.WebSearchUsageCounts{Requests: 2, CostDollars: 0.005, Succeeded: 1, Failed: 1}},
	}, usage.Days)
	suite.Equal([]types.WebSearchEndpointUsage{
		{Endpoint: types.WebSearchEndpointAnswer, WebSearchUsageCounts: types.WebSearchUsageCounts{Requests: 2, CostDollars: 0.005, Succeeded: 1, Failed: 1}},
		{Endpoint: types.WebSearchEndpointSearch, WebSearchUsageCounts: types.WebSearchUsageCounts{Requests: 3, CostDollars: 0.0071, Succeeded: 2, OutOfCredits: 1}},
	}, usage.ByEndpoint)

	empty, err := suite.db.GetWebSearchUsage(suite.ctx, &WebSearchRequestsQuery{OrganizationID: "org_" + system.GenerateUUID(), From: q.From, To: q.To})
	suite.Require().NoError(err)
	suite.Equal(&types.WebSearchUsage{Days: []types.WebSearchUsageDay{}, ByEndpoint: []types.WebSearchEndpointUsage{}}, empty)

	q.Limit = 2
	page, err := suite.db.ListWebSearchRequests(suite.ctx, q)
	suite.Require().NoError(err)
	suite.Equal(int64(5), page.Total)
	suite.Require().Len(page.Requests, 2)
	suite.Equal(rows[4].ID, page.Requests[0].ID, "newest first")
	suite.Equal(rows[3].ID, page.Requests[1].ID)

	q.Offset, q.Endpoint = 1, types.WebSearchEndpointSearch
	page, err = suite.db.ListWebSearchRequests(suite.ctx, q)
	suite.Require().NoError(err)
	suite.Equal(int64(3), page.Total)
	suite.Require().Len(page.Requests, 2)
	suite.Equal(rows[1].ID, page.Requests[0].ID)
	suite.Equal(rows[2].ID, page.Requests[1].ID)
}
