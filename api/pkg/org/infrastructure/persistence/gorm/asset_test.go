package gorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/stretchr/testify/require"
)

func newAssetRepo(t *testing.T) *assetsRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&asset.Asset{}))
	return newAssetsRepo(db)
}

func testAsset(t *testing.T, orgID, id, name string) asset.Asset {
	t.Helper()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	a, err := asset.New(id, orgID, name, "server", "agent notes", asset.KindServer, asset.Config{
		Server: &asset.Server{
			Address: "10.0.0.8", Port: 22, User: "ubuntu", AuthType: asset.AuthSSHKey,
			PublicKey: "ssh-ed25519 public", EncryptedPrivateKey: "encrypted-private",
		},
	}, now)
	require.NoError(t, err)
	return a
}

func TestAssetServerConfigRoundTripAndUpdate(t *testing.T) {
	repo := newAssetRepo(t)
	ctx := context.Background()
	a := testAsset(t, "org-one", "a-server", "production")
	require.NoError(t, repo.Create(ctx, a))

	got, err := repo.Get(ctx, "org-one", "a-server")
	require.NoError(t, err)
	require.Equal(t, a.Config.Server, got.Config.Server)

	server := *got.Config.Server
	server.Address = "server.internal"
	server.Port = 2222
	server.User = "deploy"
	server.HostKey = "ssh-ed25519 host-key"
	got.Config.Server = &server
	got.Description = ""
	got.UpdatedAt = got.UpdatedAt.Add(time.Minute)
	require.NoError(t, repo.Update(ctx, got))

	updated, err := repo.Get(ctx, "org-one", "a-server")
	require.NoError(t, err)
	require.Equal(t, "server.internal", updated.Config.Server.Address)
	require.Equal(t, uint16(2222), updated.Config.Server.Port)
	require.Equal(t, "deploy", updated.Config.Server.User)
	require.Equal(t, "ssh-ed25519 host-key", updated.Config.Server.HostKey)
	require.Equal(t, "encrypted-private", updated.Config.Server.EncryptedPrivateKey)
	require.Empty(t, updated.Description)
}

func TestAssetNameConflictIsScopedToOrganization(t *testing.T) {
	repo := newAssetRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, testAsset(t, "org-one", "a-one", "production")))

	err := repo.Create(ctx, testAsset(t, "org-one", "a-two", "production"))
	require.Error(t, err)
	require.True(t, errors.Is(err, store.ErrConflict))

	require.NoError(t, repo.Create(ctx, testAsset(t, "org-two", "a-one", "production")))
	_, err = repo.Get(ctx, "org-one", "a-one")
	require.NoError(t, err)
	_, err = repo.Get(ctx, "org-two", "a-one")
	require.NoError(t, err)
	_, err = repo.Get(ctx, "org-three", "a-one")
	require.True(t, errors.Is(err, store.ErrNotFound))
}

func TestRenameLegacyAssetLinkBotColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE "org_asset_links" (
		"org_id" text NOT NULL,
		"asset_id" text NOT NULL,
		"bot_id" text NOT NULL,
		"agent_id" text,
		"created_at" datetime,
		PRIMARY KEY ("org_id", "asset_id", "bot_id")
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO org_asset_links (org_id, asset_id, bot_id, created_at) VALUES ('org-1', 'a-1', 'agent-1', CURRENT_TIMESTAMP)`).Error)

	require.NoError(t, renameLegacyAssetLinkAgentColumn(db))
	require.False(t, db.Migrator().HasColumn("org_asset_links", "bot_id"))
	require.True(t, db.Migrator().HasColumn("org_asset_links", "agent_id"))
	var link asset.Link
	require.NoError(t, db.Table("org_asset_links").First(&link).Error)
	require.Equal(t, "agent-1", link.AgentID)
}
