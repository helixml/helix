package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// DeleteUser removes everything that belongs to the user alone and their
// access to organization resources, but keeps the organization's resources.
func TestDeleteUserRemovesPersonalDataAndKeepsOrgData(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	gdb := db.gdb.WithContext(ctx)
	suffix := system.GenerateID()
	userID := "usr_del_" + suffix
	otherID := "usr_keep_" + suffix
	orgID := "org_del_" + suffix

	_, err := db.CreateUser(ctx, &types.User{ID: userID, Email: userID + "@example.com"})
	require.NoError(t, err)
	_, err = db.CreateUser(ctx, &types.User{ID: otherID, Email: otherID + "@example.com"})
	require.NoError(t, err)
	_, err = db.CreateOrganization(ctx, &types.Organization{ID: orgID, Name: "del-" + suffix, Owner: otherID})
	require.NoError(t, err)

	personalApp := &types.App{ID: "app_p_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser}
	orgApp := &types.App{ID: "app_o_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, OrganizationID: orgID}
	role := &types.Role{ID: system.GenerateUUID(), OrganizationID: orgID, Name: "r-" + suffix}
	provider := &types.OAuthProvider{ID: system.GenerateUUID(), Name: "p-" + suffix, Type: "github", ClientID: "c",
		CreatorID: otherID, CreatorType: types.OwnerTypeUser, Enabled: true}
	grant := &types.AccessGrant{ID: "ag_" + suffix, OrganizationID: orgID, ResourceID: "prj_" + suffix, UserID: userID}
	rows := []any{
		provider, personalApp, orgApp, role, grant,
		&types.ApiKey{Key: "hl-del-personal-" + suffix, Name: "k", Owner: userID, OwnerType: types.OwnerTypeUser},
		&types.ApiKey{Key: "hl-del-org-" + suffix, Name: "k", Owner: userID, OwnerType: types.OwnerTypeUser, OrganizationID: orgID,
			AppID: &sql.NullString{String: orgApp.ID, Valid: true}},
		&types.ApiKey{Key: "hl-keep-" + suffix, Name: "k", Owner: otherID, OwnerType: types.OwnerTypeUser},
		&types.UserSession{ID: "us_" + suffix, UserID: userID, ExpiresAt: time.Now().Add(time.Hour)},
		&types.OAuthConnection{ID: system.GenerateUUID(), UserID: userID, ProviderID: provider.ID},
		&types.TriggerConfiguration{ID: "trg_p_" + suffix, AppID: personalApp.ID, Owner: userID, OwnerType: types.OwnerTypeUser},
		&types.TriggerConfiguration{ID: "trg_o_" + suffix, AppID: orgApp.ID, Owner: userID, OwnerType: types.OwnerTypeUser, OrganizationID: orgID},
		&types.Secret{ID: "sec_p_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, Name: "personal"},
		&types.Secret{ID: "sec_proj_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, Name: "project", ProjectID: "prj_" + suffix},
		&types.Secret{ID: "sec_orgapp_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, Name: "orgapp", AppID: orgApp.ID},
		&types.ProviderEndpoint{ID: "pe_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, Name: "pe"},
		&types.Memory{ID: "mem_" + suffix, UserID: userID, AppID: personalApp.ID, Contents: "x"},
		&types.Wallet{ID: "wal_" + suffix, UserID: userID},
		&types.OrganizationMembership{OrganizationID: orgID, UserID: userID, Role: types.OrganizationRoleMember},
		&types.OrganizationMembership{OrganizationID: orgID, UserID: otherID, Role: types.OrganizationRoleOwner},
		&types.AccessGrantRoleBinding{AccessGrantID: grant.ID, RoleID: role.ID, OrganizationID: orgID},
	}
	for _, row := range rows {
		require.NoError(t, gdb.Create(row).Error, "%T", row)
	}

	require.NoError(t, db.DeleteUser(ctx, userID))

	count := func(model any, where string, args ...any) int64 {
		var n int64
		require.NoError(t, gdb.Unscoped().Model(model).Where(where, args...).Count(&n).Error)
		return n
	}
	gone := map[string]int64{
		"api keys":        count(&types.ApiKey{}, "owner = ?", userID),
		"login sessions":  count(&types.UserSession{}, "user_id = ?", userID),
		"oauth":           count(&types.OAuthConnection{}, "user_id = ?", userID),
		"personal agent":  count(&types.App{}, "id = ?", personalApp.ID),
		"personal trig":   count(&types.TriggerConfiguration{}, "id = ?", "trg_p_"+suffix),
		"personal secret": count(&types.Secret{}, "id = ?", "sec_p_"+suffix),
		"endpoint":        count(&types.ProviderEndpoint{}, "id = ?", "pe_"+suffix),
		"memory":          count(&types.Memory{}, "user_id = ?", userID),
		"wallet":          count(&types.Wallet{}, "user_id = ?", userID),
		"membership":      count(&types.OrganizationMembership{}, "user_id = ?", userID),
		"grant":           count(&types.AccessGrant{}, "user_id = ?", userID),
		"grant binding":   count(&types.AccessGrantRoleBinding{}, "access_grant_id = ?", grant.ID),
	}
	for what, n := range gone {
		require.Zero(t, n, what)
	}

	kept := map[string]int64{
		"org agent":          count(&types.App{}, "id = ?", orgApp.ID),
		"org trigger":        count(&types.TriggerConfiguration{}, "id = ?", "trg_o_"+suffix),
		"project secret":     count(&types.Secret{}, "id = ?", "sec_proj_"+suffix),
		"org agent secret":   count(&types.Secret{}, "id = ?", "sec_orgapp_"+suffix),
		"other user key":     count(&types.ApiKey{}, "owner = ?", otherID),
		"other owner member": count(&types.OrganizationMembership{}, "user_id = ?", otherID),
	}
	for what, n := range kept {
		require.Equal(t, int64(1), n, what)
	}

	_, err = db.GetUser(ctx, &GetUserQuery{ID: userID})
	require.ErrorIs(t, err, ErrNotFound)
}
