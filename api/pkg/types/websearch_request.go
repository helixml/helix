package types

import "time"

// WebSearchSource is who made a web search call.
type WebSearchSource string

const (
	// WebSearchSourceAPI is a call to the web search API with a user's
	// credential (an API key, a session).
	WebSearchSourceAPI WebSearchSource = "api"
	// WebSearchSourceAgent is a call a Helix agent made through its web
	// search or browser skill.
	WebSearchSourceAgent WebSearchSource = "agent"
)

// WebSearchEndpoint is the web search API endpoint a call was made to, named
// as in its URL path.
type WebSearchEndpoint string

const (
	WebSearchEndpointSearch      WebSearchEndpoint = "search"
	WebSearchEndpointContents    WebSearchEndpoint = "contents"
	WebSearchEndpointFindSimilar WebSearchEndpoint = "findSimilar"
	WebSearchEndpointAnswer      WebSearchEndpoint = "answer"
)

// WebSearchRequest is one web search call, successful or not, as the usage
// log records it. ID is the requestId the client saw.
type WebSearchRequest struct {
	ID      string    `json:"id" gorm:"primaryKey"`
	Created time.Time `json:"created" gorm:"index:idx_web_search_requests_org_created,priority:2"`
	// OrganizationID is empty for a call billed to a user's own wallet (a
	// personal API key, an agent session outside an organization).
	OrganizationID string            `json:"organizationId" gorm:"index:idx_web_search_requests_org_created,priority:1"`
	UserID         string            `json:"userId"`
	APIKeyName     string            `json:"apiKeyName"`
	Source         WebSearchSource   `json:"source"`
	Endpoint       WebSearchEndpoint `json:"endpoint"`
	// Query is the search or answer query, the findSimilar URL, or the first
	// contents URL with how many more were asked for.
	Query      string `json:"query"`
	SearchType string `json:"searchType"`
	Stream     bool   `json:"stream"`
	// Status is the HTTP status the call ended with; an error event after a
	// stream started counts with the error's status.
	Status      int     `json:"status"`
	ErrorTag    string  `json:"errorTag"`
	DurationMs  int64   `json:"durationMs"`
	Results     int     `json:"results"`
	CostDollars float64 `json:"costDollars"`
}

// WebSearchRequestList is a page of an organization's web search calls,
// newest first.
type WebSearchRequestList struct {
	Requests []*WebSearchRequest `json:"requests"`
	Total    int64               `json:"total"`
}

// WebSearchUsageCounts summarises a set of web search calls. CostDollars is
// what was charged; succeeded, outOfCredits (402) and failed (any other
// error) add up to requests.
type WebSearchUsageCounts struct {
	Requests     int64   `json:"requests"`
	CostDollars  float64 `json:"costDollars"`
	Succeeded    int64   `json:"succeeded"`
	OutOfCredits int64   `json:"outOfCredits"`
	Failed       int64   `json:"failed"`
}

// WebSearchUsageDay is one endpoint's calls on one UTC day.
type WebSearchUsageDay struct {
	Date     string            `json:"date"` // YYYY-MM-DD
	Endpoint WebSearchEndpoint `json:"endpoint"`
	WebSearchUsageCounts
}

// WebSearchEndpointUsage is one endpoint's calls over the whole range.
type WebSearchEndpointUsage struct {
	Endpoint WebSearchEndpoint `json:"endpoint"`
	WebSearchUsageCounts
}

// WebSearchUsage is an organization's web search usage over a date range.
// Days lists only the days and endpoints that had calls.
type WebSearchUsage struct {
	Days       []WebSearchUsageDay      `json:"days"`
	Totals     WebSearchUsageCounts     `json:"totals"`
	ByEndpoint []WebSearchEndpointUsage `json:"byEndpoint"`
}
