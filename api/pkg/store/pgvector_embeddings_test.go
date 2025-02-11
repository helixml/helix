package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/suite"
)

func TestPGVectorStoreSuite(t *testing.T) {
	suite.Run(t, new(PGVectorStoreTestSuite))
}

type PGVectorStoreTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PGVectorStore
}

func (suite *PGVectorStoreTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var serverCfg config.ServerConfig

	err := envconfig.Process("", &serverCfg)
	suite.NoError(err)

	store, err := NewPGVectorStore(&serverCfg)
	suite.NoError(err)

	suite.db = store
}

func (suite *PGVectorStoreTestSuite) TestCreateKnowledgeEmbedding() {

	id := system.GenerateUUID()

	// Generate 384 dimension embedding
	embedding384 := pgvector.NewVector(make([]float32, 384))

	err := suite.db.CreateKnowledgeEmbedding(suite.ctx, &types.KnowledgeEmbeddingItem{
		KnowledgeID:     id,
		DocumentGroupID: "test-document-group-id",
		DocumentID:      "test-document-id",
		Embedding384:    embedding384,
	})
	suite.NoError(err)

	// Query the embedding
	items, err := suite.db.QueryKnowledgeEmbeddings(suite.ctx, &types.KnowledgeEmbeddingQuery{
		KnowledgeID: id,
	})
	suite.NoError(err)
	suite.Equal(1, len(items))
}
