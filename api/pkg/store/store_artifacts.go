package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ListArtifactsQuery struct {
	ProjectID      string
	OrganizationID string
}

func (s *PostgresStore) CreateArtifact(ctx context.Context, artifact *types.Artifact, version *types.ArtifactVersion) error {
	if artifact == nil || version == nil {
		return errors.New("artifact and version are required")
	}
	if artifact.ID == "" || artifact.ProjectID == "" || version.ID == "" {
		return errors.New("artifact ID, project ID, and version ID are required")
	}
	if artifact.ActiveVersionID != version.ID || version.ArtifactID != artifact.ID {
		return errors.New("artifact active version and version artifact do not match")
	}
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(artifact).Error; err != nil {
			return fmt.Errorf("create artifact: %w", err)
		}
		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("create artifact version: %w", err)
		}
		return nil
	})
}

func (s *PostgresStore) GetArtifact(ctx context.Context, artifactID string) (*types.Artifact, error) {
	if artifactID == "" {
		return nil, errors.New("artifact ID is required")
	}
	var artifact types.Artifact
	if err := s.gdb.WithContext(ctx).Where("id = ?", artifactID).First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	if err := s.loadArtifactActiveVersion(ctx, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (s *PostgresStore) ListArtifacts(ctx context.Context, query *ListArtifactsQuery) ([]*types.Artifact, error) {
	if query == nil || (query.ProjectID == "" && query.OrganizationID == "") {
		return nil, errors.New("project ID or organization ID is required")
	}
	var artifacts []*types.Artifact
	db := s.gdb.WithContext(ctx)
	if query.ProjectID != "" {
		db = db.Where("project_id = ?", query.ProjectID)
	}
	if query.OrganizationID != "" {
		db = db.Where("organization_id = ?", query.OrganizationID)
	}
	if err := db.Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		if err := s.loadArtifactActiveVersion(ctx, artifact); err != nil {
			return nil, err
		}
	}
	return artifacts, nil
}

func (s *PostgresStore) loadArtifactActiveVersion(ctx context.Context, artifact *types.Artifact) error {
	var version types.ArtifactVersion
	if err := s.gdb.WithContext(ctx).Where("id = ?", artifact.ActiveVersionID).First(&version).Error; err != nil {
		return fmt.Errorf("get active version for artifact %s: %w", artifact.ID, err)
	}
	artifact.ActiveVersion = &version
	return nil
}

func (s *PostgresStore) UpdateArtifact(ctx context.Context, artifact *types.Artifact, version *types.ArtifactVersion) error {
	if artifact == nil || artifact.ID == "" {
		return errors.New("artifact ID is required")
	}
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current types.Artifact
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", artifact.ID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock artifact: %w", err)
		}
		entrypointFiles := []types.ArtifactFile(nil)
		if version != nil {
			if version.ID == "" {
				return errors.New("version ID is required")
			}
			var lastVersion int
			if err := tx.Model(&types.ArtifactVersion{}).
				Where("artifact_id = ?", artifact.ID).
				Select("COALESCE(MAX(version), 0)").
				Scan(&lastVersion).Error; err != nil {
				return fmt.Errorf("get latest artifact version: %w", err)
			}
			version.ArtifactID = artifact.ID
			version.Version = lastVersion + 1
			entrypointFiles = version.Files
			if err := tx.Create(version).Error; err != nil {
				return fmt.Errorf("create artifact version: %w", err)
			}
			artifact.ActiveVersionID = version.ID
			artifact.Kind = artifactKindForFileCount(version.FileCount)
		} else {
			var activeVersion types.ArtifactVersion
			if err := tx.Where("id = ?", current.ActiveVersionID).First(&activeVersion).Error; err != nil {
				return fmt.Errorf("get current artifact version: %w", err)
			}
			entrypointFiles = activeVersion.Files
		}
		if !artifactFilesContain(entrypointFiles, artifact.Entrypoint) {
			return fmt.Errorf("artifact entrypoint does not exist in active version: %s", artifact.Entrypoint)
		}
		artifact.UpdatedAt = time.Now()
		fields := []string{"name", "description", "entrypoint", "visibility", "updated_by", "updated_at"}
		if version != nil {
			fields = append(fields, "active_version_id", "kind")
		}
		if err := tx.Model(&current).Select(fields).Updates(artifact).Error; err != nil {
			return fmt.Errorf("update artifact: %w", err)
		}
		return nil
	})
}

func artifactFilesContain(files []types.ArtifactFile, filename string) bool {
	for _, file := range files {
		if file.Path == filename {
			return true
		}
	}
	return false
}

func artifactKindForFileCount(fileCount int) types.ArtifactKind {
	if fileCount == 1 {
		return types.ArtifactKindSingleFile
	}
	return types.ArtifactKindSPA
}

func (s *PostgresStore) ListArtifactVersions(ctx context.Context, artifactID string) ([]*types.ArtifactVersion, error) {
	if artifactID == "" {
		return nil, errors.New("artifact ID is required")
	}
	var versions []*types.ArtifactVersion
	if err := s.gdb.WithContext(ctx).
		Where("artifact_id = ?", artifactID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("list artifact versions: %w", err)
	}
	return versions, nil
}

func (s *PostgresStore) DeleteArtifact(ctx context.Context, artifactID string) error {
	if artifactID == "" {
		return errors.New("artifact ID is required")
	}
	result := s.gdb.WithContext(ctx).Delete(&types.Artifact{}, "id = ?", artifactID)
	if result.Error != nil {
		return fmt.Errorf("delete artifact: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
