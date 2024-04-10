package store

import (
	"context"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
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

	// TODO: move server options to envconfig
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}

	store, err := NewPostgresStore(config.Store{
		Host:        host,
		Port:        5432,
		Username:    "postgres",
		Password:    "postgres",
		Database:    "postgres",
		AutoMigrate: true,
	})
	suite.NoError(err)

	suite.db = store
}
