package store

import (
	"strings"
	"testing"
	"time"
)

// TestMigrationFailsFastBehindALock reproduces 2026-11-18: a pg_dump held
// ACCESS SHARE on every table and a boot-time ALTER queued behind it until the
// dump was cancelled. Migrations must instead give up within a bounded time,
// name the blocking session, and succeed once the lock is gone.
func TestMigrationFailsFastBehindALock(t *testing.T) {
	db := GetTestDB()

	saved := lockTimeoutRetryDelays
	lockTimeoutRetryDelays = []time.Duration{10 * time.Millisecond}
	defer func() { lockTimeoutRetryDelays = saved }()

	// The interrupt default ALTER needs ACCESS EXCLUSIVE; make it pending.
	if err := db.gdb.Exec("ALTER TABLE prompt_history_entries ALTER COLUMN interrupt SET DEFAULT true").Error; err != nil {
		t.Fatal(err)
	}

	dump := db.gdb.Begin()
	if err := dump.Exec("LOCK TABLE prompt_history_entries IN ACCESS SHARE MODE").Error; err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err := db.autoMigrate()
	elapsed := time.Since(start)
	dump.Rollback()

	if err == nil {
		t.Fatal("migration succeeded while an ACCESS SHARE lock blocked its ALTER")
	}
	if !strings.Contains(err.Error(), "blocked by a lock after 2 attempts") || !strings.Contains(err.Error(), "LOCK TABLE prompt_history_entries") {
		t.Fatalf("error doesn't report the lock and its holder: %v", err)
	}
	// Two attempts of MigrationLockTimeout each, plus slack.
	if elapsed > 2*MigrationLockTimeout+10*time.Second {
		t.Fatalf("migration took %s to give up", elapsed)
	}

	if err := db.autoMigrate(); err != nil {
		t.Fatalf("migration after the lock was released: %v", err)
	}
	def, err := db.columnDefault(t.Context(), "prompt_history_entries", "interrupt")
	if err != nil || def != "false" {
		t.Fatalf("interrupt default = %q, %v; want false", def, err)
	}
}
