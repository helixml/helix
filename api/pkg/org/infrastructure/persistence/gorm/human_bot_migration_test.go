package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
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

func TestUpdateCodeAgentConfigPreservesOtherBotFields(t *testing.T) {
	db, err := gormio.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gormio.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&OrgBot{}))
	repo := newNodesRepo(db)
	createdAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	node, err := orgchart.NewNode("b-engineer", "original instructions", []string{"chat"}, createdAt, "org-test")
	require.NoError(t, err)
	node = node.WithAgentID("app-legacy")
	require.NoError(t, repo.Create(context.Background(), node))

	interleaved := node.WithContent("concurrent instructions").WithTools([]string{"chat", "create_bot"})
	require.NoError(t, repo.Update(context.Background(), interleaved))
	updatedAt := createdAt.Add(time.Hour)
	config := &types.CodeAgentExecutionConfig{Runtime: types.CodeAgentRuntimeCodexCLI, Model: "gpt-5.6"}
	require.NoError(t, repo.UpdateCodeAgentConfig(context.Background(), node.OrganizationID, node.ID, config, updatedAt))

	got, err := repo.Get(context.Background(), node.OrganizationID, node.ID)
	require.NoError(t, err)
	require.Equal(t, "concurrent instructions", got.Content)
	require.Equal(t, []string{"chat", "create_bot"}, got.Tools)
	require.Equal(t, config, got.CodeAgentConfig)
	require.Equal(t, updatedAt, got.UpdatedAt)
}
