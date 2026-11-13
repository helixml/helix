package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

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

	// testDBAdmin is connected to the configured database and owns the
	// lifecycle of the throwaway database testDB runs against.
	testDBAdmin *PostgresStore
	testDBName  string
)

// GetTestDB returns the store shared by every test in this process. It runs
// against a database created for this process on the Postgres server that
// POSTGRES_* configures, never against the configured database itself: tests
// delete and rewrite install-wide rows such as system_settings, which wiped a
// dev install's settings and pinned its opencode_version to a test value when
// POSTGRES_* pointed at the dev stack. Call DropTestDB from TestMain.
func GetTestDB() *PostgresStore {
	testDBOnce.Do(func() {
		testDB, testDBErr = newIsolatedTestDB()
	})

	if testDBErr != nil {
		// log.Fatal exits before TestMain's DropTestDB runs.
		DropTestDB()
		log.Fatal().Err(testDBErr).Msg("failed to initialize test database")
	}

	return testDB
}

func newIsolatedTestDB() (*PostgresStore, error) {
	var cfg config.Store
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load store configuration: %w", err)
	}
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		return nil, fmt.Errorf("create in-memory pubsub: %w", err)
	}

	admin := cfg
	admin.AutoMigrate = false
	admin.SeedModels = false
	testDBAdmin, err = NewPostgresStore(admin, ps)
	if err != nil {
		return nil, fmt.Errorf("connect to Postgres: %w", err)
	}
	name := fmt.Sprintf("helix_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	if err := testDBAdmin.gdb.Exec("CREATE DATABASE " + name).Error; err != nil {
		return nil, fmt.Errorf("create test database: %w", err)
	}
	testDBName = name

	isolated := cfg
	isolated.Database = testDBName
	isolated.Schema = ""
	isolated.AutoMigrate = false
	isolated.SeedModels = false
	db, err := NewPostgresStore(isolated, ps)
	if err != nil {
		return nil, fmt.Errorf("connect to test database: %w", err)
	}
	// Migrate directly: HELIX_SKIP_AUTOMIGRATE describes the configured
	// database, which CI migrates up front, not this fresh one.
	if err := db.runMigrations(); err != nil {
		return nil, fmt.Errorf("migrate test database: %w", err)
	}
	if cfg.SeedModels {
		if err := db.seedModels(context.Background()); err != nil {
			return nil, fmt.Errorf("seed test database: %w", err)
		}
	}
	return db, nil
}

// DropTestDB drops the database GetTestDB created, if any, including one left
// half-initialised by a failed GetTestDB.
func DropTestDB() {
	if testDBAdmin == nil {
		return
	}
	if testDB != nil {
		_ = testDB.Close()
	}
	if testDBName == "" {
		_ = testDBAdmin.Close()
		return
	}
	if err := testDBAdmin.gdb.Exec("DROP DATABASE " + testDBName + " WITH (FORCE)").Error; err != nil {
		log.Error().Err(err).Str("database", testDBName).Msg("failed to drop test database")
	}
	_ = testDBAdmin.Close()
}
