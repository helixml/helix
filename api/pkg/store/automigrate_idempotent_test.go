package store

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder is a GORM logger that records every statement it traces.
type sqlRecorder struct {
	logger.Interface
	mu   sync.Mutex
	stmt []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.stmt = append(r.stmt, sql)
	r.mu.Unlock()
}

// writeStatement matches statements that change schema or data. Catalog reads
// (HasTable, ColumnTypes, the migration_scripts ledger lookup) are allowed.
var writeStatement = regexp.MustCompile(`(?i)^\s*(ALTER|CREATE|DROP|UPDATE|INSERT|DELETE|TRUNCATE|COMMENT)\b`)

// TestRunMigrationsIsIdempotent fails when a boot against an already-migrated
// database issues DDL or writes. Every such statement takes locks on a live
// database; an ALTER that waited behind a pg_dump took prod down on
// 2026-11-18. A model tag GORM can't match against the catalog makes
// AutoMigrate ALTER the column on every boot; fix the tag, or guard the
// hand-written statement so it is a read-only no-op once applied.
func TestRunMigrationsIsIdempotent(t *testing.T) {
	db := GetTestDB()

	rec := &sqlRecorder{Interface: logger.Discard}
	quiet := db.withGormDB(db.gdb.Session(&gorm.Session{Logger: rec}))
	if err := quiet.runMigrations(); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	var writes []string
	for _, s := range rec.stmt {
		if writeStatement.MatchString(s) {
			writes = append(writes, strings.Join(strings.Fields(s), " "))
		}
	}
	if len(writes) > 0 {
		t.Fatalf("second migration run issued %d write statement(s):\n%s", len(writes), strings.Join(writes, "\n"))
	}
}
