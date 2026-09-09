package gorm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	gormio "gorm.io/gorm"
)

func TestRemoveLegacyHumanBots(t *testing.T) {
	db, err := gormio.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gormio.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE org_bots (
		id text NOT NULL,
		org_id text NOT NULL,
		content text NOT NULL,
		kind text,
		helix_user_id text,
		identity text,
		PRIMARY KEY (id, org_id)
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO org_bots (id, org_id, content, kind) VALUES
		('b-engineer', 'org-1', 'Build things', ''),
		('h-owner', 'org-1', 'Owner', 'human')`).Error)

	require.NoError(t, removeLegacyHumanBots(db))

	var ids []string
	require.NoError(t, db.Table("org_bots").Order("id").Pluck("id", &ids).Error)
	require.Equal(t, []string{"b-engineer"}, ids)
	for _, column := range []string{"identity", "helix_user_id", "kind"} {
		require.False(t, db.Migrator().HasColumn("org_bots", column), column)
	}
	require.NoError(t, removeLegacyHumanBots(db), "migration must be idempotent")
}
