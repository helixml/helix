package rag

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	gomock "go.uber.org/mock/gomock"
)

type PGVectorTestSuite struct {
	suite.Suite
	ctx  context.Context
	ctrl *gomock.Controller
	pg   *PGVector
}

func TestPGVectorTestSuite(t *testing.T) {
	suite.Run(t, new(PGVectorTestSuite))
}

func (suite *PGVectorTestSuite) SetupTest() {
	suite.ctx = context.Background()

	ctrl := gomock.NewController(suite.T())

	suite.ctrl = ctrl

	mockProviderManager := manager.NewMockProviderManager(ctrl)

	cfg := &config.ServerConfig{}
	cfg.PGVectorStore.Host = "localhost"
	if os.Getenv("PGVECTOR_HOST") != "" {
		cfg.PGVectorStore.Host = os.Getenv("PGVECTOR_HOST")
	}

	cfg.PGVectorStore.Port = 5433
	if os.Getenv("PGVECTOR_PORT") != "" {
		port, err := strconv.Atoi(os.Getenv("PGVECTOR_PORT"))
		suite.Require().NoError(err)
		cfg.PGVectorStore.Port = port
	}

	cfg.PGVectorStore.Username = "postgres"
	if os.Getenv("PGVECTOR_USERNAME") != "" {
		cfg.PGVectorStore.Username = os.Getenv("PGVECTOR_USERNAME")
	}

	cfg.PGVectorStore.Password = "postgres"
	if os.Getenv("PGVECTOR_PASSWORD") != "" {
		cfg.PGVectorStore.Password = os.Getenv("PGVECTOR_PASSWORD")
	}

	pgVectorStore, err := store.NewPGVectorStore(cfg)
	suite.Require().NoError(err)

	pg := NewPGVector(cfg, mockProviderManager, pgVectorStore)

	suite.pg = pg
}

func (suite *PGVectorTestSuite) TestIndex() {
	err := suite.pg.Index(suite.ctx, &types.SessionRAGIndexChunk{
		DataEntityID: "test-data-entity-id",
		Content:      "test-content",
	})
	suite.Require().NoError(err)
}
