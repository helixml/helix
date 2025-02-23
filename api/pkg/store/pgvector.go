package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func NewPGVectorStore(
	serverCfg *config.ServerConfig,
) (*PGVectorStore, error) {

	cfg := serverCfg.PGVectorStore

	// Waiting for connection
	gormDB, err := connect(context.Background(), connectConfig{
		host:            cfg.Host,
		port:            cfg.Port,
		schemaName:      cfg.Schema,
		database:        cfg.Database,
		username:        cfg.Username,
		password:        cfg.Password,
		ssl:             cfg.SSL,
		idleConns:       cfg.IdleConns,
		maxConns:        cfg.MaxConns,
		maxConnIdleTime: cfg.MaxConnIdleTime,
		maxConnLifetime: cfg.MaxConnLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PGVector: %w", err)
	}

	store := &PGVectorStore{
		cfg: serverCfg.Store,
		gdb: gormDB,
	}

	err = store.autoMigratePGVector()
	if err != nil {
		return nil, err
	}

	return store, nil
}

type PGVectorStore struct {
	cfg config.Store

	gdb *gorm.DB
}

func (s *PGVectorStore) Close() error {
	sqlDB, err := s.gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
func (s *PGVectorStore) autoMigratePGVector() error {
	err := s.gdb.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
	if err != nil {
		return fmt.Errorf("failed to create vector extension: %w. Install it manually or disable PGVector RAG (RAG_PGVECTOR_ENABLED env variable)", err)
	}

	err = s.gdb.WithContext(context.Background()).AutoMigrate(
		&types.KnowledgeEmbeddingItem{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto migrate PGVector table: %w", err)
	}

	err = s.createIndex("embedding384", "embedding_384_index")
	if err != nil {
		return fmt.Errorf("failed to create embedding384 index: %w", err)
	}

	err = s.createIndex("embedding512", "embedding_512_index")
	if err != nil {
		return fmt.Errorf("failed to create embedding512 index: %w", err)
	}

	err = s.createIndex("embedding1024", "embedding_1024_index")
	if err != nil {
		return fmt.Errorf("failed to create embedding1024 index: %w", err)
	}

	err = s.createIndex("embedding1536", "embedding_1536_index")
	if err != nil {
		return fmt.Errorf("failed to create embedding1536 index: %w", err)
	}

	// Note: column cannot have more than 2000 dimensions for
	// hnsw index hence skipping index creation for 3584

	return nil
}

func (s *PGVectorStore) createIndex(columnName, _ string) error {
	// Get the schema name from config, default to "public" if not set
	schemaName := "public"
	if cfg := s.cfg; cfg.Schema != "" {
		schemaName = cfg.Schema
	}
	// Create index with schema name
	err := s.gdb.Exec(fmt.Sprintf("CREATE INDEX ON %s.knowledge_embedding_items USING hnsw (%s vector_l2_ops)", schemaName, columnName)).Error
	if err != nil {
		return fmt.Errorf("failed to create hnsw index: %w", err)
	}

	return nil
}
