package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

// A Teams trigger without an App ID would make the Bot Framework SDK treat
// authentication as disabled, so such a trigger must not be saveable as
// enabled or partially configured.
func TestValidateTriggers_TeamsRequiresCredentials(t *testing.T) {
	s := &HelixAPIServer{}
	for name, tc := range map[string]struct {
		teams   *types.TeamsTrigger
		wantErr bool
	}{
		"absent":                      {teams: nil},
		"empty block":                 {teams: &types.TeamsTrigger{}},
		"enabled without app id":      {teams: &types.TeamsTrigger{Enabled: true, AppPassword: "pw"}, wantErr: true},
		"enabled without password":    {teams: &types.TeamsTrigger{Enabled: true, AppID: "app"}, wantErr: true},
		"enabled with blank app id":   {teams: &types.TeamsTrigger{Enabled: true, AppID: "  ", AppPassword: "pw"}, wantErr: true},
		"password set without app id": {teams: &types.TeamsTrigger{AppPassword: "pw"}, wantErr: true},
		"tenant set without app id":   {teams: &types.TeamsTrigger{TenantID: "tenant"}, wantErr: true},
		"complete":                    {teams: &types.TeamsTrigger{Enabled: true, AppID: "app", AppPassword: "pw"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := s.validateTriggers([]types.Trigger{{Teams: tc.teams}})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
