package types

type ResourceSearchRequest struct {
	Query          string     `json:"query"`
	Types          []Resource `json:"types,omitempty"`
	Limit          int        `json:"limit,omitempty"`
	OrganizationID string     `json:"organization_id,omitempty"`
	UserID         string     `json:"user_id,omitempty"`
}

type ResourceSearchResponse struct {
	Results []ResourceSearchResult `json:"results"`
	Total   int                    `json:"total"`
}

type ResourceSearchResult struct {
	ResourceType        Resource `json:"type"`
	ResourceID          string   `json:"id"`
	ResourceName        string   `json:"name"`
	ResourceDescription string   `json:"description"`
	Contents            string   `json:"contents"`
}
