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

	// Sandbox API concurrency. Distinct from ActiveConcurrentDesktops above —
	// that one counts external_agent sessions (the spec-task desktop stack),
	// while these count rows in the sandboxes table by runtime category.
	// "Active" = pending|running|stopping (matches ensureSandboxLimits).
	ActiveDesktopSandboxes  int `json:"active_desktop_sandboxes"`
	MaxDesktopSandboxes     int `json:"max_desktop_sandboxes"`
	ActiveHeadlessSandboxes int `json:"active_headless_sandboxes"`
	MaxHeadlessSandboxes    int `json:"max_headless_sandboxes"`

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
