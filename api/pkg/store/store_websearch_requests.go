package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// WebSearchRequestsQuery selects an organization's web search calls created
// in [From, To).
type WebSearchRequestsQuery struct {
	OrganizationID string
	From           time.Time
	To             time.Time
	// Endpoint, when set, narrows a list to one endpoint.
	Endpoint types.WebSearchEndpoint
	Limit    int
	Offset   int
}

func (s *PostgresStore) CreateWebSearchRequest(ctx context.Context, request *types.WebSearchRequest) error {
	if err := s.gdb.WithContext(ctx).Create(request).Error; err != nil {
		return fmt.Errorf("create web search request %s: %w", request.ID, err)
	}
	return nil
}

// ListWebSearchRequests returns a page of the calls, newest first, and how
// many there are in all.
func (s *PostgresStore) ListWebSearchRequests(ctx context.Context, q *WebSearchRequestsQuery) (*types.WebSearchRequestList, error) {
	db := s.gdb.WithContext(ctx).Model(&types.WebSearchRequest{}).
		Where("organization_id = ? AND created >= ? AND created < ?", q.OrganizationID, q.From, q.To)
	if q.Endpoint != "" {
		db = db.Where("endpoint = ?", q.Endpoint)
	}
	list := &types.WebSearchRequestList{Requests: []*types.WebSearchRequest{}}
	if err := db.Count(&list.Total).Error; err != nil {
		return nil, fmt.Errorf("count web search requests: %w", err)
	}
	if err := db.Order("created DESC, id DESC").Limit(q.Limit).Offset(q.Offset).Find(&list.Requests).Error; err != nil {
		return nil, fmt.Errorf("list web search requests: %w", err)
	}
	return list, nil
}

// webSearchUsageRow is one row of the usage rollup: a day and endpoint, an
// endpoint over the range (Date NULL), or the total (both NULL).
type webSearchUsageRow struct {
	Date     *string
	Endpoint *types.WebSearchEndpoint
	types.WebSearchUsageCounts
}

const webSearchDay = `to_char(created AT TIME ZONE 'UTC', 'YYYY-MM-DD')`

// GetWebSearchUsage summarises the calls per UTC day and endpoint, per
// endpoint, and in total, in one rollup query. Endpoint is ignored.
func (s *PostgresStore) GetWebSearchUsage(ctx context.Context, q *WebSearchRequestsQuery) (*types.WebSearchUsage, error) {
	var rows []webSearchUsageRow
	err := s.gdb.WithContext(ctx).Raw(`
		SELECT
			`+webSearchDay+` AS date,
			endpoint,
			count(*) AS requests,
			coalesce(round(sum(cost_dollars)::numeric, 6), 0)::float8 AS cost_dollars,
			count(*) FILTER (WHERE status < 400) AS succeeded,
			count(*) FILTER (WHERE status = 402) AS out_of_credits,
			count(*) FILTER (WHERE status >= 400 AND status <> 402) AS failed
		FROM web_search_requests
		WHERE organization_id = ? AND created >= ? AND created < ?
		GROUP BY GROUPING SETS ((`+webSearchDay+`, endpoint), (endpoint), ())
		ORDER BY date NULLS FIRST, endpoint NULLS FIRST`,
		q.OrganizationID, q.From, q.To).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("summarise web search requests: %w", err)
	}

	usage := &types.WebSearchUsage{Days: []types.WebSearchUsageDay{}, ByEndpoint: []types.WebSearchEndpointUsage{}}
	for _, row := range rows {
		switch {
		case row.Endpoint == nil:
			usage.Totals = row.WebSearchUsageCounts
		case row.Date == nil:
			usage.ByEndpoint = append(usage.ByEndpoint, types.WebSearchEndpointUsage{Endpoint: *row.Endpoint, WebSearchUsageCounts: row.WebSearchUsageCounts})
		default:
			usage.Days = append(usage.Days, types.WebSearchUsageDay{Date: *row.Date, Endpoint: *row.Endpoint, WebSearchUsageCounts: row.WebSearchUsageCounts})
		}
	}
	return usage, nil
}
