package gorm

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kelseyhightower/envconfig"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/org/store"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// schemaCounter feeds a process-unique suffix into per-test schema
// names so two parallel tests in the same package never collide.
var schemaCounter uint64

// GetOrgTestDB returns an *orgstore.Store backed by helix's shared
// Postgres test database, isolated inside a freshly-created
// Postgres schema dedicated to this test. The schema is dropped via
// t.Cleanup so leakage is bounded by the test process lifetime.
//
// Why schema-per-test rather than the unique-IDs-per-test pattern
// used by helix's main store suite: the org-graph tests were written
// against an in-memory SQLite per test (hard-coded fixture IDs like
// "r-ceo"/"p-root"/"w-ceo", t.Parallel everywhere), and rewriting
// ~7000 lines of test setup to thread unique IDs through every
// assertion is out of scope. Postgres schemas give us the same
// "fresh empty DB per test" semantic without touching the test
// bodies — parallel-safe.
//
// We bootstrap the schema on the shared connection from
// store.GetTestDB(), then open a fresh, pool-of-one *gorm.DB whose
// session's search_path is pinned to that schema via DSN options.
// A single connection avoids the GORM/PG pool reissuing connections
// whose search_path defaults to public — every query lands in the
// per-test schema. The row types' TableName() returns unqualified
// names ("roles", "workers", …), so the search_path is the only
// thing that keeps the data isolated; setting TablePrefix would not
// have worked because TableName() takes precedence.
func GetOrgTestDB(t *testing.T) *store.Store {
	t.Helper()

	// Use the shared connection just to create + drop the schema.
	bootstrapDB := helixstore.GetTestDB().GormDB()

	raw := strings.ToLower(t.Name())
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, raw)
	if len(safe) > 32 {
		safe = safe[:32]
	}
	schemaName := fmt.Sprintf("helix_org_test_%s_%d", safe, atomic.AddUint64(&schemaCounter, 1))

	if err := bootstrapDB.Exec(fmt.Sprintf("CREATE SCHEMA %q", schemaName)).Error; err != nil {
		t.Fatalf("create schema %s: %v", schemaName, err)
	}
	t.Cleanup(func() {
		if err := bootstrapDB.Exec(fmt.Sprintf("DROP SCHEMA %q CASCADE", schemaName)).Error; err != nil {
			t.Logf("drop schema %s: %v", schemaName, err)
		}
	})

	// Build the DSN from the same env vars helix uses for its main
	// test connection, but pin search_path to this test's schema and
	// cap the pool to one connection so the setting cannot leak
	// between checked-out connections.
	var cfg config.Store
	if err := envconfig.Process("", &cfg); err != nil {
		t.Fatalf("load store config: %v", err)
	}
	sslMode := "sslmode=disable"
	if cfg.SSL {
		sslMode = "sslmode=require"
	}
	dsn := fmt.Sprintf(
		"user=%s password=%s host=%s port=%d dbname=%s %s options='-csearch_path=%s'",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, sslMode, schemaName,
	)

	scoped, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open scoped gorm.DB for schema %s: %v", schemaName, err)
	}
	sqlDB, err := scoped.DB()
	if err != nil {
		t.Fatalf("scoped sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	st, err := OpenWithDB(scoped)
	if err != nil {
		t.Fatalf("open org store on schema %s: %v", schemaName, err)
	}
	return st
}
