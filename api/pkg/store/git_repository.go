package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/helixml/helix/api/pkg/services"
	"gorm.io/gorm"
)

// DBGitRepository represents a git repository stored in the database
type DBGitRepository struct {
	ID            string    `gorm:"type:varchar(255);primaryKey"`
	Name          string    `gorm:"type:varchar(255);not null;index"`
	Description   string    `gorm:"type:text"`
	OwnerID       string    `gorm:"type:varchar(255);not null;index"`
	ProjectID     string    `gorm:"type:varchar(255);index"`
	SpecTaskID    string    `gorm:"type:varchar(255);index"`
	RepoType      string    `gorm:"type:varchar(50);not null;index"`
	Status        string    `gorm:"type:varchar(50);not null"`
	CloneURL      string    `gorm:"type:text"`
	LocalPath     string    `gorm:"type:text"`
	DefaultBranch string    `gorm:"type:varchar(100)"`
	LastActivity  time.Time `gorm:"index"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
	MetadataJSON  string    `gorm:"type:jsonb"` // Stores Metadata as JSON
}

// TableName overrides the table name
func (DBGitRepository) TableName() string {
	return "git_repositories"
}

// CreateGitRepository creates a new git repository record
func (s *PostgresStore) CreateGitRepository(ctx context.Context, repo *services.GitRepository) error {
	// Marshal metadata to JSON
	metadataJSON := "{}"
	if repo.Metadata != nil {
		data, err := json.Marshal(repo.Metadata)
		if err == nil {
			metadataJSON = string(data)
		}
	}

	dbRepo := &DBGitRepository{
		ID:            repo.ID,
		Name:          repo.Name,
		Description:   repo.Description,
		OwnerID:       repo.OwnerID,
		ProjectID:     repo.ProjectID,
		SpecTaskID:    repo.SpecTaskID,
		RepoType:      string(repo.RepoType),
		Status:        string(repo.Status),
		CloneURL:      repo.CloneURL,
		LocalPath:     repo.LocalPath,
		DefaultBranch: repo.DefaultBranch,
		LastActivity:  repo.LastActivity,
		CreatedAt:     repo.CreatedAt,
		UpdatedAt:     repo.UpdatedAt,
		MetadataJSON:  metadataJSON,
	}

	return s.gdb.WithContext(ctx).Create(dbRepo).Error
}

// GetGitRepository retrieves a git repository by ID
func (s *PostgresStore) GetGitRepository(ctx context.Context, id string) (*services.GitRepository, error) {
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

	return &services.GitRepository{
		ID:            dbRepo.ID,
		Name:          dbRepo.Name,
		Description:   dbRepo.Description,
		OwnerID:       dbRepo.OwnerID,
		ProjectID:     dbRepo.ProjectID,
		SpecTaskID:    dbRepo.SpecTaskID,
		RepoType:      services.GitRepositoryType(dbRepo.RepoType),
		Status:        services.GitRepositoryStatus(dbRepo.Status),
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
func (s *PostgresStore) ListGitRepositories(ctx context.Context, ownerID string) ([]*services.GitRepository, error) {
	var dbRepos []DBGitRepository

	query := s.gdb.WithContext(ctx)
	if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	}

	err := query.Order("created_at DESC").Find(&dbRepos).Error
	if err != nil {
		return nil, err
	}

	repos := make([]*services.GitRepository, len(dbRepos))
	for i, dbRepo := range dbRepos {
		var metadata map[string]interface{}
		if dbRepo.MetadataJSON != "" && dbRepo.MetadataJSON != "{}" {
			json.Unmarshal([]byte(dbRepo.MetadataJSON), &metadata)
		}

		repos[i] = &services.GitRepository{
			ID:            dbRepo.ID,
			Name:          dbRepo.Name,
			Description:   dbRepo.Description,
			OwnerID:       dbRepo.OwnerID,
			ProjectID:     dbRepo.ProjectID,
			SpecTaskID:    dbRepo.SpecTaskID,
			RepoType:      services.GitRepositoryType(dbRepo.RepoType),
			Status:        services.GitRepositoryStatus(dbRepo.Status),
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
func (s *PostgresStore) UpdateGitRepository(ctx context.Context, repo *services.GitRepository) error {
	metadataJSON := "{}"
	if repo.Metadata != nil {
		data, err := json.Marshal(repo.Metadata)
		if err == nil {
			metadataJSON = string(data)
		}
	}

	dbRepo := &DBGitRepository{
		ID:            repo.ID,
		Name:          repo.Name,
		Description:   repo.Description,
		OwnerID:       repo.OwnerID,
		ProjectID:     repo.ProjectID,
		SpecTaskID:    repo.SpecTaskID,
		RepoType:      string(repo.RepoType),
		Status:        string(repo.Status),
		CloneURL:      repo.CloneURL,
		LocalPath:     repo.LocalPath,
		DefaultBranch: repo.DefaultBranch,
		LastActivity:  repo.LastActivity,
		UpdatedAt:     time.Now(),
		MetadataJSON:  metadataJSON,
	}

	return s.gdb.WithContext(ctx).Model(&DBGitRepository{}).Where("id = ?", repo.ID).Updates(dbRepo).Error
}

// DeleteGitRepository deletes a git repository record
func (s *PostgresStore) DeleteGitRepository(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&DBGitRepository{}).Error
}
