package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

// A Teams trigger without an App ID would make the Bot Framework SDK treat
// authentication as disabled, so such a trigger must not be saveable as
// enabled. Disabled blocks may be partial (the editor autosaves).
func TestValidateTriggers_TeamsRequiresCredentials(t *testing.T) {
	s := &HelixAPIServer{}
	for name, tc := range map[string]struct {
		teams   *types.TeamsTrigger
		wantErr bool
	}{
		"absent":                    {teams: nil},
		"empty block":               {teams: &types.TeamsTrigger{}},
		"enabled without app id":    {teams: &types.TeamsTrigger{Enabled: true, AppPassword: "pw"}, wantErr: true},
		"enabled without password":  {teams: &types.TeamsTrigger{Enabled: true, AppID: "app"}, wantErr: true},
		"enabled with blank app id": {teams: &types.TeamsTrigger{Enabled: true, AppID: "  ", AppPassword: "pw"}, wantErr: true},
		// The editor autosaves while the operator types, so a disabled block
		// may be partial and must still save.
		"disabled partial app id":     {teams: &types.TeamsTrigger{AppID: "app"}},
		"disabled partial password":   {teams: &types.TeamsTrigger{AppPassword: "pw"}},
		"disabled partial tenant":     {teams: &types.TeamsTrigger{TenantID: "tenant"}},
		"disabled explicitly partial": {teams: &types.TeamsTrigger{Enabled: false, AppID: "app", TenantID: "t"}},
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
