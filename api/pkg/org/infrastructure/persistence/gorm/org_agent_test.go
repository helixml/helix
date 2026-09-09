package gorm_test

import (
	"testing"
	"time"

	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	gormio "gorm.io/gorm"
)

func TestOrgBotsIsPublicPersistentModel(t *testing.T) {
	t.Parallel()

	db, err := gormio.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gormio.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&orggorm.OrgBots{}))
	require.True(t, db.Migrator().HasTable("org_bots"))

	legacyAppID := "app-legacy"
	want := orggorm.OrgBots{
		ID:             "b-engineer",
		OrganizationID: "org-acme",
		LegacyAppID:    &legacyAppID,
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeCodexCLI,
			CredentialType: types.CodeAgentCredentialTypeSubscription,
			Model:          "gpt-5.6",
		},
		Name:            "Engineer",
		Content:         "Build and maintain the product.",
		Tools:           []string{"list_agents", "create_spectask"},
		ProjectIDs:      []string{"prj-runtime"},
		PreserveContext: true,
		SandboxRuntime:  "headless-ubuntu",
		SandboxVCPUs:    4,
		SandboxMemoryMB: 8192,
		CreatedAt:       time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 9, 9, 13, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&want).Error)

	var got orggorm.OrgBots
	require.NoError(t, db.Where("org_id = ? AND id = ?", want.OrganizationID, want.ID).First(&got).Error)
	require.Equal(t, want, got)
}
