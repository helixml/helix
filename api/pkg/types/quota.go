package types

type QuotaRequest struct {
	UserID         string
	OrganizationID string // Optional
}

type QuotaResponse struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"` // If applicable

	ActiveConcurrentDesktops int `json:"active_concurrent_desktops"`
	MaxConcurrentDesktops    int `json:"max_concurrent_desktops"`

	Projects    int `json:"projects"`
	MaxProjects int `json:"max_projects"`

	Repositories    int `json:"repositories"`
	MaxRepositories int `json:"max_repositories"`

	SpecTasks    int `json:"spec_tasks"`
	MaxSpecTasks int `json:"max_spec_tasks"`
}

type QuotaLimitReachedRequest struct {
	UserID         string
	OrganizationID string // Optional
	Resource       Resource
}

type QuotaLimitReachedResponse struct {
	LimitReached bool
	Limit        int
}
