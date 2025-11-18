package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// GitRepository represents a git repository
// Supports both Helix-hosted repositories and external repositories (GitHub, GitLab, ADO, etc.)
type GitRepository struct {
	ID             string                 `gorm:"type:varchar(255);primaryKey" json:"id"`
	Name           string                 `gorm:"type:varchar(255);not null;index" json:"name"`
	Description    string                 `gorm:"type:text" json:"description"`
	OwnerID        string                 `gorm:"type:varchar(255);not null;index" json:"owner_id"`
	OrganizationID string                 `gorm:"type:varchar(255);index" json:"organization_id"` // Organization ID - will be backfilled for existing repos
	ProjectID      string                 `gorm:"type:varchar(255);index" json:"project_id"`
	SpecTaskID     string                 `gorm:"type:varchar(255);index" json:"spec_task_id"`
	RepoType       string                 `gorm:"type:varchar(50);not null;index" json:"repo_type"`
	Status         string                 `gorm:"type:varchar(50);not null" json:"status"`
	CloneURL       string                 `gorm:"type:text" json:"clone_url"`  // For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git
	LocalPath      string                 `gorm:"type:text" json:"local_path"` // Local filesystem path for Helix-hosted repos (empty for external)
	DefaultBranch  string                 `gorm:"type:varchar(100)" json:"default_branch"`
	LastActivity   time.Time              `gorm:"index" json:"last_activity"`
	CreatedAt      time.Time              `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt      time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
	Metadata       map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"` // Stores Metadata as JSON

	// External repository fields
	IsExternal     bool   `gorm:"type:boolean;default:false;index" json:"is_external"` // True for GitHub/GitLab/ADO, false for Helix-hosted
	ExternalURL    string `gorm:"type:text" json:"external_url"`                       // Full URL to external repo (e.g., https://github.com/org/repo)
	ExternalType   string `gorm:"type:varchar(50)" json:"external_type"`               // "github", "gitlab", "ado", "bitbucket", etc.
	ExternalRepoID string `gorm:"type:varchar(255)" json:"external_repo_id"`           // External platform's repository ID

	AuthToken string `gorm:"type:text" json:"auth_token"` // Authentication token for the repository

	// Code intelligence fields
	KoditIndexing bool `gorm:"type:boolean;default:false;index" json:"kodit_indexing"` // Enable Kodit indexing for code intelligence (MCP server for snippets/architecture)
}

// TableName overrides the table name
func (GitRepository) TableName() string {
	return "git_repositories"
}

// CreateGitRepository creates a new git repository record
func (s *PostgresStore) CreateGitRepository(ctx context.Context, repo *GitRepository) error {
	return s.gdb.WithContext(ctx).Create(repo).Error
}

// GetGitRepository retrieves a git repository by ID
func (s *PostgresStore) GetGitRepository(ctx context.Context, id string) (*GitRepository, error) {
	var repo GitRepository
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&repo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &repo, nil
}

// ListGitRepositories lists all git repositories, optionally filtered by owner
func (s *PostgresStore) ListGitRepositories(ctx context.Context, ownerID string) ([]*GitRepository, error) {
	var repos []*GitRepository

	query := s.gdb.WithContext(ctx)
	if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	}

	err := query.Order("created_at DESC").Find(&repos).Error
	if err != nil {
		return nil, err
	}

	return repos, nil
}

// UpdateGitRepository updates a git repository record
func (s *PostgresStore) UpdateGitRepository(ctx context.Context, repo *GitRepository) error {
	repo.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Model(&GitRepository{}).Where("id = ?", repo.ID).Updates(repo).Error
}

// DeleteGitRepository deletes a git repository record
func (s *PostgresStore) DeleteGitRepository(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&GitRepository{}).Error
}
