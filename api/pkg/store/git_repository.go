package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// GitRepository represents a git repository
// Supports both Helix-hosted repositories and external repositories (GitHub, GitLab, ADO, etc.)
type GitRepository struct {
	ID             string    `gorm:"type:varchar(255);primaryKey"`
	Name           string    `gorm:"type:varchar(255);not null;index"`
	Description    string    `gorm:"type:text"`
	OwnerID        string    `gorm:"type:varchar(255);not null;index"`
	OrganizationID string    `gorm:"type:varchar(255);index"` // Organization ID - will be backfilled for existing repos
	ProjectID      string    `gorm:"type:varchar(255);index"`
	SpecTaskID     string    `gorm:"type:varchar(255);index"`
	RepoType       string    `gorm:"type:varchar(50);not null;index"`
	Status         string    `gorm:"type:varchar(50);not null"`
	CloneURL      string    `gorm:"type:text"` // For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git
	LocalPath     string    `gorm:"type:text"` // Local filesystem path for Helix-hosted repos (empty for external)
	DefaultBranch string    `gorm:"type:varchar(100)"`
	LastActivity  time.Time `gorm:"index"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
	MetadataJSON  string    `gorm:"type:jsonb"` // Stores Metadata as JSON
	Metadata      map[string]interface{} `gorm:"-"` // Transient field, not persisted (used by services)

	// External repository fields
	IsExternal     bool   `gorm:"type:boolean;default:false;index"` // True for GitHub/GitLab/ADO, false for Helix-hosted
	ExternalURL    string `gorm:"type:text"`                        // Full URL to external repo (e.g., https://github.com/org/repo)
	ExternalType   string `gorm:"type:varchar(50)"`                 // "github", "gitlab", "ado", "bitbucket", etc.
	ExternalRepoID string `gorm:"type:varchar(255)"`                // External platform's repository ID
	CredentialRef  string `gorm:"type:varchar(255)"`                // Reference to stored credentials (SSH key, OAuth token, etc.)

	// Code intelligence fields
	KoditIndexing bool `gorm:"type:boolean;default:false;index"` // Enable Kodit indexing for code intelligence (MCP server for snippets/architecture)
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
