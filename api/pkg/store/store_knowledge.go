package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

const (
	DefaultKnowledgeDistanceFunction = "cosine"
	DefaultKnowledgeChunkOverflow    = 20
	DefaultKnowledgeResultsCount     = 3
	DefaultKnowledgeThreshold        = 0.4
	DefaultKnowledgeChunkSize        = 2000
)

func (s *PostgresStore) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	if knowledge.ID == "" {
		knowledge.ID = system.GenerateKnowledgeID()
	}

	if knowledge.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	knowledge.Created = time.Now()
	knowledge.Updated = time.Now()

	setDefaultKnowledgeRAGSettings(knowledge)

	err := s.gdb.WithContext(ctx).Create(knowledge).Error
	if err != nil {
		return nil, err
	}
	return s.GetKnowledge(ctx, knowledge.ID)
}

func setDefaultKnowledgeRAGSettings(knowledge *types.Knowledge) {
	if knowledge.RAGSettings.ChunkSize == 0 {
		knowledge.RAGSettings.ChunkSize = DefaultKnowledgeChunkSize
	}
	if knowledge.RAGSettings.DistanceFunction == "" {
		knowledge.RAGSettings.DistanceFunction = DefaultKnowledgeDistanceFunction
	}
	if knowledge.RAGSettings.ChunkOverflow == 0 {
		knowledge.RAGSettings.ChunkOverflow = DefaultKnowledgeChunkOverflow
	}
	if knowledge.RAGSettings.ResultsCount == 0 {
		knowledge.RAGSettings.ResultsCount = DefaultKnowledgeResultsCount
	}
	if knowledge.RAGSettings.Threshold == 0 {
		knowledge.RAGSettings.Threshold = DefaultKnowledgeThreshold
	}
	// Only disable chunking if Haystack is the RAG provider
	// XXX factor this properly into the config (or set it from somewhere else)
	if os.Getenv("RAG_DEFAULT_PROVIDER") == "haystack" {
		knowledge.RAGSettings.DisableChunking = true
	}
}

func (s *PostgresStore) GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var knowledge types.Knowledge
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&knowledge).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &knowledge, nil
}

type LookupKnowledgeQuery struct {
	AppID string `json:"app_id"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

func (s *PostgresStore) LookupKnowledge(ctx context.Context, q *LookupKnowledgeQuery) (*types.Knowledge, error) {
	var knowledge types.Knowledge
	err := s.gdb.WithContext(ctx).Where(&types.Knowledge{
		AppID: q.AppID,
		ID:    q.ID,
		Name:  q.Name,
		Owner: q.Owner,
	}).First(&knowledge).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &knowledge, nil
}

func (s *PostgresStore) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	if knowledge.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if knowledge.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	knowledge.Updated = time.Now()

	setDefaultKnowledgeRAGSettings(knowledge)

	err := s.gdb.WithContext(ctx).Save(knowledge).Error
	if err != nil {
		return nil, err
	}
	return s.GetKnowledge(ctx, knowledge.ID)
}

func (s *PostgresStore) UpdateKnowledgeState(ctx context.Context, id string, state types.KnowledgeState, message string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	return s.gdb.WithContext(ctx).Model(&types.Knowledge{}).Where("id = ?", id).Updates(&types.Knowledge{
		State:   state,
		Message: message,
	}).Error
}

type ListKnowledgeQuery struct {
	Owner     string
	OwnerType types.OwnerType
	State     types.KnowledgeState
	ID        string // Knowledge ID to search for
	AppID     string
}

func (s *PostgresStore) ListKnowledge(ctx context.Context, q *ListKnowledgeQuery) ([]*types.Knowledge, error) {
	query := s.gdb.WithContext(ctx)

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}
	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}
	if q.State != "" {
		query = query.Where("state = ?", q.State)
	}

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}

	var knowledgeList []*types.Knowledge

	err := query.Order("id DESC").Find(&knowledgeList).Error
	if err != nil {
		return nil, err
	}

	return knowledgeList, nil
}

func (s *PostgresStore) DeleteKnowledge(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete all knowledge versions
		if err := tx.Where("knowledge_id = ?", id).Delete(&types.KnowledgeVersion{}).Error; err != nil {
			return err
		}

		// Delete the knowledge
		if err := tx.Delete(&types.Knowledge{ID: id}).Error; err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *PostgresStore) CreateKnowledgeVersion(ctx context.Context, version *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
	if version.ID == "" {
		version.ID = system.GenerateKnowledgeVersionID()
	}

	if version.KnowledgeID == "" {
		return nil, fmt.Errorf("knowledge_id not specified")
	}

	version.Created = time.Now()
	version.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Create(version).Error
	if err != nil {
		return nil, err
	}
	return s.GetKnowledgeVersion(ctx, version.ID)
}

func (s *PostgresStore) GetKnowledgeVersion(ctx context.Context, id string) (*types.KnowledgeVersion, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var version types.KnowledgeVersion
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &version, nil
}

type ListKnowledgeVersionQuery struct {
	KnowledgeID string
	State       types.KnowledgeState
}

func (s *PostgresStore) ListKnowledgeVersions(ctx context.Context, q *ListKnowledgeVersionQuery) ([]*types.KnowledgeVersion, error) {
	query := s.gdb.WithContext(ctx)

	if q.KnowledgeID != "" {
		query = query.Where("knowledge_id = ?", q.KnowledgeID)
	}
	if q.State != "" {
		query = query.Where("state = ?", q.State)
	}

	var versionList []*types.KnowledgeVersion

	err := query.Order("created DESC").Find(&versionList).Error
	if err != nil {
		return nil, err
	}

	return versionList, nil
}

func (s *PostgresStore) DeleteKnowledgeVersion(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.KnowledgeVersion{ID: id}).Error
	if err != nil {
		return err
	}
	return nil
}
