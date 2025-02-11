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

func (s *PGVectorStore) autoMigratePGVector() error {
	err := s.gdb.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
	if err != nil {
		return fmt.Errorf("failed to create vector extension: %w. Install it manually or disable PGVector RAG (RAG_PGVECTOR_ENABLED env variable)", err)
	}

	err = s.gdb.Exec("CREATE INDEX ON knowledge_embedding_items USING hnsw (embedding vector_l2_ops)").Error
	if err != nil {
		return fmt.Errorf("failed to create hnsw index: %w", err)
	}

	err = s.gdb.WithContext(context.Background()).AutoMigrate(
		&types.KnowledgeEmbeddingItem{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto migrate PGVector table: %w", err)
	}

	return nil
}
