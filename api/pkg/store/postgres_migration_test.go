package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

func TestSkipAutoMigrate(t *testing.T) {
	for _, tc := range []struct {
		env    string
		schema string
		skip   bool
	}{
		{env: "1", schema: "", skip: true},
		{env: "1", schema: "public", skip: true},
		{env: "1", schema: "test_schema", skip: false},
		{env: "", schema: "", skip: false},
		{env: "true", schema: "", skip: false},
	} {
		t.Run(fmt.Sprintf("env=%q schema=%q", tc.env, tc.schema), func(t *testing.T) {
			t.Setenv("HELIX_SKIP_AUTOMIGRATE", tc.env)
			require.Equal(t, tc.skip, skipAutoMigrate(tc.schema))
		})
	}
}

// freshDatabaseConfig creates an empty database on the configured Postgres
// server and returns a store config pointing at it. The database is dropped
// when the test ends, so the test never migrates or writes a shared database.
func freshDatabaseConfig(t *testing.T) config.Store {
	t.Helper()
	var cfg config.Store
	require.NoError(t, envconfig.Process("", &cfg))

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	t.Cleanup(func() { ps.Close() })

	admin := cfg
	admin.AutoMigrate = false
	admin.SeedModels = false
	adminStore, err := NewPostgresStore(admin, ps)
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminStore.Close() })

	name := fmt.Sprintf("helix_migrate_test_%d", time.Now().UnixNano())
	require.NoError(t, adminStore.gdb.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() {
		require.NoError(t, adminStore.gdb.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error)
	})

	cfg.Database = name
	cfg.Schema = ""
	cfg.AutoMigrate = true
	cfg.SeedModels = false
	return cfg
}

func openStores(t *testing.T, cfg config.Store, n int) []error {
	t.Helper()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	t.Cleanup(func() { ps.Close() })

	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			s, err := NewPostgresStore(cfg, ps)
			errs[i] = err
			if err == nil {
				t.Cleanup(func() { _ = s.Close() })
			}
		})
	}
	close(start)
	wg.Wait()
	return errs
}

func tableExists(t *testing.T, cfg config.Store, table string) bool {
	t.Helper()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	defer ps.Close()
	cfg.AutoMigrate = false
	s, err := NewPostgresStore(cfg, ps)
	require.NoError(t, err)
	defer s.Close()
	return s.gdb.Migrator().HasTable(table)
}

// API replicas starting together on one empty database must all come up:
// without the migration lock they race on CREATE TABLE and one fails with a
// duplicate key on pg_type_typname_nsp_index.
func TestAutoMigrateConcurrentStarts(t *testing.T) {
	t.Setenv("HELIX_SKIP_AUTOMIGRATE", "")
	cfg := freshDatabaseConfig(t)

	for i, err := range openStores(t, cfg, 4) {
		require.NoError(t, err, "store %d", i)
	}
	require.True(t, tableExists(t, cfg, "organizations"))
	require.True(t, tableExists(t, cfg, "org_bots"))
}

func TestAutoMigrateHonoursSkipFlag(t *testing.T) {
	t.Setenv("HELIX_SKIP_AUTOMIGRATE", "1")
	cfg := freshDatabaseConfig(t)

	for i, err := range openStores(t, cfg, 1) {
		require.NoError(t, err, "store %d", i)
	}
	require.False(t, tableExists(t, cfg, "organizations"))
	require.False(t, tableExists(t, cfg, "org_bots"))
}

// Store tests rewrite install-wide rows (system_settings, the JWT signing
// secret), so they must never run against the database POSTGRES_* names: that
// is how a test pinned a dev install's opencode_version to 1.2.3.
func TestGetTestDBUsesThrowawayDatabase(t *testing.T) {
	var cfg config.Store
	require.NoError(t, envconfig.Process("", &cfg))
	db := GetTestDB()

	var current string
	require.NoError(t, db.gdb.Raw("SELECT current_database()").Scan(&current).Error)
	require.NotEqual(t, cfg.Database, current)
	require.Equal(t, testDBName, current)

	version := "0.0.0-isolation-check"
	_, err := db.UpdateSystemSettings(t.Context(), &types.SystemSettingsRequest{OpenCodeVersion: &version})
	require.NoError(t, err)
	if testDBAdmin.gdb.Migrator().HasTable("system_settings") {
		var count int64
		require.NoError(t, testDBAdmin.gdb.Table("system_settings").Where("opencode_version = ?", version).Count(&count).Error)
		require.Zero(t, count)
	}
}
