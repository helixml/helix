package store

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// GitRepository is a simple DTO for git repository data to avoid circular imports
type GitRepository struct {
	ID            string
	Name          string
	Description   string
	OwnerID       string
	ProjectID     string
	SpecTaskID    string
	RepoType      string
	Status        string
	CloneURL      string
	LocalPath     string
	DefaultBranch string
	LastActivity  time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Metadata      map[string]interface{}
}

// DBGitRepository represents a git repository stored in the database
// Supports both Helix-hosted repositories and external repositories (GitHub, GitLab, ADO, etc.)
type DBGitRepository struct {
	ID            string    `gorm:"type:varchar(255);primaryKey"`
	Name          string    `gorm:"type:varchar(255);not null;index"`
	Description   string    `gorm:"type:text"`
	OwnerID       string    `gorm:"type:varchar(255);not null;index"`
	ProjectID     string    `gorm:"type:varchar(255);index"`
	SpecTaskID    string    `gorm:"type:varchar(255);index"`
	RepoType      string    `gorm:"type:varchar(50);not null;index"`
	Status        string    `gorm:"type:varchar(50);not null"`
	CloneURL      string    `gorm:"type:text"` // For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git
	LocalPath     string    `gorm:"type:text"` // Local filesystem path for Helix-hosted repos (empty for external)
	DefaultBranch string    `gorm:"type:varchar(100)"`
	LastActivity  time.Time `gorm:"index"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
	MetadataJSON  string    `gorm:"type:jsonb"` // Stores Metadata as JSON

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
func (DBGitRepository) TableName() string {
	return "git_repositories"
}

// CreateGitRepository creates a new git repository record
func (s *PostgresStore) CreateGitRepository(ctx context.Context, repo *GitRepository) error {
	// Marshal metadata to JSON
	metadataJSON := "{}"
	if repo.Metadata != nil {
		data, err := json.Marshal(repo.Metadata)
		if err == nil {
			metadataJSON = string(data)
		}
	}

	// Extract external repository fields from metadata if present
	isExternal := false
	externalURL := ""
	externalType := ""
	externalRepoID := ""
	credentialRef := ""
	koditIndexing := false

	if repo.Metadata != nil {
		if val, ok := repo.Metadata["is_external"].(bool); ok {
			isExternal = val
		}
		if val, ok := repo.Metadata["external_url"].(string); ok {
			externalURL = val
		}
		if val, ok := repo.Metadata["external_type"].(string); ok {
			externalType = val
		}
		if val, ok := repo.Metadata["external_repo_id"].(string); ok {
			externalRepoID = val
		}
		if val, ok := repo.Metadata["credential_ref"].(string); ok {
			credentialRef = val
		}
		if val, ok := repo.Metadata["kodit_indexing"].(bool); ok {
			koditIndexing = val
		}
	}

	dbRepo := &DBGitRepository{
		ID:             repo.ID,
		Name:           repo.Name,
		Description:    repo.Description,
		OwnerID:        repo.OwnerID,
		ProjectID:      repo.ProjectID,
		SpecTaskID:     repo.SpecTaskID,
		RepoType:       repo.RepoType,
		Status:         repo.Status,
		CloneURL:       repo.CloneURL,
		LocalPath:      repo.LocalPath,
		DefaultBranch:  repo.DefaultBranch,
		LastActivity:   repo.LastActivity,
		CreatedAt:      repo.CreatedAt,
		UpdatedAt:      repo.UpdatedAt,
		MetadataJSON:   metadataJSON,
		IsExternal:     isExternal,
		ExternalURL:    externalURL,
		ExternalType:   externalType,
		ExternalRepoID: externalRepoID,
		CredentialRef:  credentialRef,
		KoditIndexing:  koditIndexing,
	}

	return s.gdb.WithContext(ctx).Create(dbRepo).Error
}

// GetGitRepository retrieves a git repository by ID
func (s *PostgresStore) GetGitRepository(ctx context.Context, id string) (*GitRepository, error) {
	var dbRepo DBGitRepository
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&dbRepo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Unmarshal metadata
	var metadata map[string]interface{}
	if dbRepo.MetadataJSON != "" && dbRepo.MetadataJSON != "{}" {
		json.Unmarshal([]byte(dbRepo.MetadataJSON), &metadata)
	}

	// Ensure metadata map exists
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	// Add external repository fields to metadata for API compatibility
	if dbRepo.IsExternal {
		metadata["is_external"] = true
		metadata["external_url"] = dbRepo.ExternalURL
		metadata["external_type"] = dbRepo.ExternalType
		metadata["external_repo_id"] = dbRepo.ExternalRepoID
		if dbRepo.CredentialRef != "" {
			metadata["credential_ref"] = dbRepo.CredentialRef
		}
	}

	// Add code intelligence fields to metadata
	metadata["kodit_indexing"] = dbRepo.KoditIndexing

	return &GitRepository{
		ID:            dbRepo.ID,
		Name:          dbRepo.Name,
		Description:   dbRepo.Description,
		OwnerID:       dbRepo.OwnerID,
		ProjectID:     dbRepo.ProjectID,
		SpecTaskID:    dbRepo.SpecTaskID,
		RepoType:      dbRepo.RepoType,
		Status:        dbRepo.Status,
		CloneURL:      dbRepo.CloneURL,
		LocalPath:     dbRepo.LocalPath,
		DefaultBranch: dbRepo.DefaultBranch,
		LastActivity:  dbRepo.LastActivity,
		CreatedAt:     dbRepo.CreatedAt,
		UpdatedAt:     dbRepo.UpdatedAt,
		Metadata:      metadata,
	}, nil
}

// ListGitRepositories lists all git repositories, optionally filtered by owner
func (s *PostgresStore) ListGitRepositories(ctx context.Context, ownerID string) ([]*GitRepository, error) {
	var dbRepos []DBGitRepository

	query := s.gdb.WithContext(ctx)
	if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	}

	err := query.Order("created_at DESC").Find(&dbRepos).Error
	if err != nil {
		return nil, err
	}

	repos := make([]*GitRepository, len(dbRepos))
	for i, dbRepo := range dbRepos {
		var metadata map[string]interface{}
		if dbRepo.MetadataJSON != "" && dbRepo.MetadataJSON != "{}" {
			json.Unmarshal([]byte(dbRepo.MetadataJSON), &metadata)
		}

		// Ensure metadata map exists
		if metadata == nil {
			metadata = make(map[string]interface{})
		}

		// Add external repository fields to metadata for API compatibility
		if dbRepo.IsExternal {
			metadata["is_external"] = true
			metadata["external_url"] = dbRepo.ExternalURL
			metadata["external_type"] = dbRepo.ExternalType
			metadata["external_repo_id"] = dbRepo.ExternalRepoID
			if dbRepo.CredentialRef != "" {
				metadata["credential_ref"] = dbRepo.CredentialRef
			}
		}

		// Add code intelligence fields to metadata
		metadata["kodit_indexing"] = dbRepo.KoditIndexing

		repos[i] = &GitRepository{
			ID:            dbRepo.ID,
			Name:          dbRepo.Name,
			Description:   dbRepo.Description,
			OwnerID:       dbRepo.OwnerID,
			ProjectID:     dbRepo.ProjectID,
			SpecTaskID:    dbRepo.SpecTaskID,
			RepoType:      dbRepo.RepoType,
			Status:        dbRepo.Status,
			CloneURL:      dbRepo.CloneURL,
			LocalPath:     dbRepo.LocalPath,
			DefaultBranch: dbRepo.DefaultBranch,
			LastActivity:  dbRepo.LastActivity,
			CreatedAt:     dbRepo.CreatedAt,
			UpdatedAt:     dbRepo.UpdatedAt,
			Metadata:      metadata,
		}
	}

	return repos, nil
}

// UpdateGitRepository updates a git repository record
func (s *PostgresStore) UpdateGitRepository(ctx context.Context, repo *GitRepository) error {
	metadataJSON := "{}"
	if repo.Metadata != nil {
		data, err := json.Marshal(repo.Metadata)
		if err == nil {
			metadataJSON = string(data)
		}
	}

	// Extract external repository fields from metadata if present
	isExternal := false
	externalURL := ""
	externalType := ""
	externalRepoID := ""
	credentialRef := ""
	koditIndexing := false

	if repo.Metadata != nil {
		if val, ok := repo.Metadata["is_external"].(bool); ok {
			isExternal = val
		}
		if val, ok := repo.Metadata["external_url"].(string); ok {
			externalURL = val
		}
		if val, ok := repo.Metadata["external_type"].(string); ok {
			externalType = val
		}
		if val, ok := repo.Metadata["external_repo_id"].(string); ok {
			externalRepoID = val
		}
		if val, ok := repo.Metadata["credential_ref"].(string); ok {
			credentialRef = val
		}
		if val, ok := repo.Metadata["kodit_indexing"].(bool); ok {
			koditIndexing = val
		}
	}

	dbRepo := &DBGitRepository{
		ID:             repo.ID,
		Name:           repo.Name,
		Description:    repo.Description,
		OwnerID:        repo.OwnerID,
		ProjectID:      repo.ProjectID,
		SpecTaskID:     repo.SpecTaskID,
		RepoType:       repo.RepoType,
		Status:         repo.Status,
		CloneURL:       repo.CloneURL,
		LocalPath:      repo.LocalPath,
		DefaultBranch:  repo.DefaultBranch,
		LastActivity:   repo.LastActivity,
		UpdatedAt:      time.Now(),
		MetadataJSON:   metadataJSON,
		IsExternal:     isExternal,
		ExternalURL:    externalURL,
		ExternalType:   externalType,
		ExternalRepoID: externalRepoID,
		CredentialRef:  credentialRef,
		KoditIndexing:  koditIndexing,
	}

	return s.gdb.WithContext(ctx).Model(&DBGitRepository{}).Where("id = ?", repo.ID).Updates(dbRepo).Error
}

// DeleteGitRepository deletes a git repository record
func (s *PostgresStore) DeleteGitRepository(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&DBGitRepository{}).Error
}
