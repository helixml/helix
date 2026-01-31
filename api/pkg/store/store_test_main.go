package store

import (
	"sync"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

var (
	// Global test database connection shared across all test suites
	testDB     *PostgresStore
	testDBOnce sync.Once
	testDBErr  error
)

// GetTestDB returns the shared test database connection, initializing it once if needed
func GetTestDB() *PostgresStore {
	testDBOnce.Do(func() {
		// Load store configuration from environment
		var storeCfg config.Store
		err := envconfig.Process("", &storeCfg)
		if err != nil {
			testDBErr = err
			log.Fatal().Err(err).Msg("failed to load store configuration")
			return
		}

		ps, err := pubsub.NewInMemoryNats()
		if err != nil {
			testDBErr = err
			log.Fatal().Err(err).Msg("failed to create in-memory pubsub")
			return
		}

		// Create a single database connection for all tests
		testDB, err = NewPostgresStore(storeCfg, ps)
		if err != nil {
			testDBErr = err
			log.Fatal().Err(err).Msg("failed to create test database connection")
			return
		}

	})

	if testDBErr != nil {
		log.Fatal().Err(testDBErr).Msg("failed to initialize test database")
	}

	return testDB
}
