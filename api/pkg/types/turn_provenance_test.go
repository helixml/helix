package types

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testSecret = "hl-0123456789abcdefghijklmnopqrstuvwxyz"

func TestRequestTurnProvenance(t *testing.T) {
	keyID := APIKeyID(testSecret)
	cases := []struct {
		name string
		user *User
		want TurnProvenance
	}{
		{"oidc browser session", &User{ID: "u1", TokenType: TokenTypeOIDC},
			TurnProvenance{Origin: TurnOriginUser, UserID: "u1"}},
		{"password browser session", &User{ID: "u1", TokenType: TokenTypeSession},
			TurnProvenance{Origin: TurnOriginUser, UserID: "u1"}},
		{"personal key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeAPI},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopePersonal, APIKeyID: keyID}},
		{"org key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeAPI, OrganizationID: "org_1"},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeOrg, APIKeyID: keyID, APIKeyOrgID: "org_1"}},
		{"project key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeAPI, OrganizationID: "org_1", ProjectID: "prj_1"},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeProject, APIKeyID: keyID, APIKeyOrgID: "org_1", APIKeyProjectID: "prj_1"}},
		{"spec task key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeAPI, ProjectID: "prj_1", SpecTaskID: "spt_1"},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeSpecTask, APIKeyID: keyID, APIKeyProjectID: "prj_1", APIKeySpecTaskID: "spt_1"}},
		{"session key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeAPI, SessionID: "ses_1", SpecTaskID: "spt_1"},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeSession, APIKeyID: keyID, APIKeySpecTaskID: "spt_1", APIKeySessionID: "ses_1"}},
		{"sandbox key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeAPI, SessionID: SandboxIDPrefix + "01abc"},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeSandbox, APIKeyID: keyID, APIKeySessionID: SandboxIDPrefix + "01abc"}},
		{"app key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeApp},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeApp, APIKeyID: keyID}},
		{"embed key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeEmbed, SessionID: "ses_1"},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeEmbed, APIKeyID: keyID, APIKeySessionID: "ses_1"}},
		{"bot instance key", &User{ID: "u1", TokenType: TokenTypeAPIKey, Token: testSecret, APIKeyType: APIkeytypeBotInstance},
			TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeBotInstance, APIKeyID: keyID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RequestTurnProvenance(tc.user)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			b, err := json.Marshal(got)
			require.NoError(t, err)
			require.NotContains(t, string(b), testSecret, "the key secret must never be recorded")
		})
	}
}

func TestRequestTurnProvenanceRefusesCredentialsThatCannotSendTurns(t *testing.T) {
	for _, u := range []*User{
		nil,
		{ID: "u1"},
		{ID: "runner", TokenType: TokenTypeRunner},
		{ID: "socket", TokenType: TokenTypeSocket},
		{ID: "u1", TokenType: TokenTypeAPIKey},
	} {
		_, err := RequestTurnProvenance(u)
		require.Error(t, err)
	}
}

func TestTurnProvenanceWithoutOriginReadsAsUnknown(t *testing.T) {
	var fromNull TurnProvenance
	require.NoError(t, fromNull.Scan(nil))
	b, err := json.Marshal(Interaction{ID: "int_old"})
	require.NoError(t, err)
	require.Contains(t, string(b), `"provenance":{"origin":"unknown"}`)

	b, err = json.Marshal(fromNull)
	require.NoError(t, err)
	require.JSONEq(t, `{"origin":"unknown"}`, string(b))
	require.Error(t, fromNull.Validate(), "a creation path must not write unknown")
}

func TestTurnProvenanceRoundTripsThroughTheDatabaseValue(t *testing.T) {
	want := TriggeredBySystemTurn(SystemTurnRevisionRequested,
		TurnProvenance{Origin: TurnOriginAPI, APIKeyOwner: "u1", APIKeyType: APIKeyScopeOrg, APIKeyID: APIKeyID(testSecret)})
	v, err := want.Value()
	require.NoError(t, err)
	var got TurnProvenance
	require.NoError(t, got.Scan([]byte(v.(string))))
	require.Equal(t, want, got)
	require.True(t, strings.Contains(v.(string), `"system_kind":"revision_requested"`))
}

func TestTurnProvenanceValidate(t *testing.T) {
	require.NoError(t, SystemTurn(SystemTurnCIStatus).Validate())
	require.NoError(t, TurnProvenance{Origin: TurnOriginExternalAgent}.Validate())
	require.Error(t, TurnProvenance{Origin: TurnOriginSystem}.Validate(), "system needs a kind")
	require.Error(t, TurnProvenance{Origin: TurnOriginUnknown}.Validate())
	require.Error(t, TurnProvenance{Origin: "admin"}.Validate())
}
