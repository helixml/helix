package store

import (
	"context"
	"testing"

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
	suite.db = GetTestDB()
}

func (suite *PostgresStoreTestSuite) TearDownTestSuite() {
	// No need to close the database connection here as it's managed by TestMain
}
