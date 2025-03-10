package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestPostgresStoreSuite(t *testing.T) {
	suite.Run(t, new(PostgresStoreTestSuite))
}

type PostgresStoreTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *PostgresStoreTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store

	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_ = store.Close()
	})

	suite.db = store
}
