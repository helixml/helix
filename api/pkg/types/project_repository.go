package types

import "time"

// ProjectRepository represents the many-to-many relationship between projects and repositories.
// A repository can be attached to multiple projects, and a project can have multiple repositories.
// This replaces the deprecated project_id field on GitRepository.
type ProjectRepository struct {
	ProjectID      string `json:"project_id" gorm:"primaryKey"`      // composite key
	RepositoryID   string `json:"repository_id" gorm:"primaryKey;index"`
	OrganizationID string `json:"organization_id" gorm:"index"` // For access control queries

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName overrides the table name
func (ProjectRepository) TableName() string {
	return "project_repositories"
}

// ListProjectRepositoriesQuery defines filters for listing project-repository relationships
type ListProjectRepositoriesQuery struct {
	ProjectID      string
	RepositoryID   string
	OrganizationID string
}
