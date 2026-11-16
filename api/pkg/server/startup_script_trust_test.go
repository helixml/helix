package server

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// A startup script saved through the UI/API is trusted only when a person
// saved it; an agent-bound key (which may call the same endpoints) never
// moves the trusted startup commit.
func TestTrustStartupScriptWriteOnlyForHumans(t *testing.T) {
	const sha = "1111111111111111111111111111111111111111"
	for _, tc := range []struct {
		name  string
		user  *types.User
		trust bool
	}{
		{"browser session", &types.User{ID: "u", TokenType: types.TokenTypeSession}, true},
		{"oidc", &types.User{ID: "u", TokenType: types.TokenTypeOIDC}, true},
		{"personal key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI}, true},
		{"org key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: "org_1"}, true},
		{"spec task key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "ses_1", ProjectID: "prj_1", SpecTaskID: "spt_1"}, false},
		{"project key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, ProjectID: "prj_1"}, false},
		{"sandbox key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "sbx_1"}, false},
		{"app key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeApp, AppID: "app_1"}, false},
		{"bot instance key", &types.User{ID: "u", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeBotInstance}, false},
		{"runner", &types.User{ID: "u", TokenType: types.TokenTypeRunner}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			if tc.trust {
				st.EXPECT().SetGitRepositoryTrustedStartupSHA(gomock.Any(), "repo_1", sha).Return(nil)
			}
			s := &HelixAPIServer{Store: st}
			if err := s.trustStartupScriptWrite(context.Background(), tc.user, "repo_1", sha); err != nil {
				t.Fatalf("trustStartupScriptWrite: %v", err)
			}
		})
	}
}

func TestTrustStartupScriptWriteSkipsWhenNothingWritten(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := &HelixAPIServer{Store: store.NewMockStore(ctrl)}
	user := &types.User{ID: "u", TokenType: types.TokenTypeSession}
	if err := s.trustStartupScriptWrite(context.Background(), user, "repo_1", ""); err != nil {
		t.Fatalf("trustStartupScriptWrite: %v", err)
	}
}
