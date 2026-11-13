package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/attestation"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	attOrgA      = "org_att_a"
	attOrgB      = "org_att_b"
	attOwnerA    = "usr_att_owner_a"
	attMemberA   = "usr_att_member_a"
	attOwnerB    = "usr_att_owner_b"
	attProjectA  = "prj_att_a"
	attProjectB  = "prj_att_b"
	attAudience  = "ptaas.example.com"
	attNonce     = "nonce-0123456789abcdef"
	attFinderSes = "ses_att_finder"
	attFinderTsk = "spt_att_finder"
	attVerifSes  = "ses_att_verifier"
	attVerifTsk  = "spt_att_verifier"
	attArchSes   = "ses_att_archived"
	attArchTsk   = "spt_att_archived"
	attGoneSes   = "ses_att_gone"
	attGoneTsk   = "spt_att_gone"
	attSandbox   = "sbx_att_a"
)

type IdentityAttestationSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
	signer *attestation.Signer

	mu       sync.Mutex
	redeemed map[string]bool
	audit    []*types.ProjectAuditLog
}

func TestIdentityAttestationSuite(t *testing.T) { suite.Run(t, new(IdentityAttestationSuite)) }

func (s *IdentityAttestationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	signer, err := attestation.NewSigner("test-secret-test-secret-test-sec")
	s.Require().NoError(err)
	s.signer = signer
	s.server = &HelixAPIServer{Store: s.store, attestations: signer}
	s.redeemed = map[string]bool{}
	s.audit = nil

	memberships := map[string]map[string]types.OrganizationRole{
		attOwnerA:  {attOrgA: types.OrganizationRoleMember},
		attMemberA: {attOrgA: types.OrganizationRoleMember},
		attOwnerB:  {attOrgB: types.OrganizationRoleOwner},
	}
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			role, ok := memberships[q.UserID][q.OrganizationID]
			if !ok {
				return nil, store.ErrNotFound
			}
			return &types.OrganizationMembership{UserID: q.UserID, OrganizationID: q.OrganizationID, Role: role}, nil
		}).AnyTimes()
	s.store.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	projects := map[string]*types.Project{
		attProjectA: {ID: attProjectA, OrganizationID: attOrgA, UserID: attOwnerA},
		attProjectB: {ID: attProjectB, OrganizationID: attOrgB, UserID: attOwnerB},
	}
	s.store.EXPECT().GetProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.Project, error) {
			if p, ok := projects[id]; ok {
				copied := *p
				return &copied, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	session := func(id, task string) *types.Session {
		return &types.Session{ID: id, Owner: attOwnerA, OrganizationID: attOrgA, ProjectID: attProjectA,
			Metadata: types.SessionMetadata{SpecTaskID: task, ProjectID: attProjectA}}
	}
	sessions := map[string]*types.Session{
		attFinderSes: session(attFinderSes, attFinderTsk),
		attVerifSes:  session(attVerifSes, attVerifTsk),
		attArchSes:   session(attArchSes, attArchTsk),
		attGoneSes:   session(attGoneSes, attGoneTsk),
	}
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.Session, error) {
			if v, ok := sessions[id]; ok {
				copied := *v
				return &copied, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	tasks := map[string]*types.SpecTask{
		attFinderTsk: {ID: attFinderTsk, ProjectID: attProjectA, OrganizationID: attOrgA},
		attVerifTsk:  {ID: attVerifTsk, ProjectID: attProjectA, OrganizationID: attOrgA},
		attArchTsk:   {ID: attArchTsk, ProjectID: attProjectA, OrganizationID: attOrgA, Archived: true},
	}
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.SpecTask, error) {
			if v, ok := tasks[id]; ok {
				copied := *v
				return &copied, nil
			}
			// Spec tasks are hard-deleted; the store reports a plain error.
			return nil, fmt.Errorf("spec task not found: %s", id)
		}).AnyTimes()

	s.store.EXPECT().GetSandbox(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.Sandbox, error) {
			if id == attSandbox {
				return &types.Sandbox{ID: attSandbox, OrganizationID: attOrgA, ProjectID: attProjectA, Owner: attOwnerA}, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	s.store.EXPECT().RedeemIdentityAttestation(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, jti string, _ time.Time) (bool, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.redeemed[jti] {
				return false, nil
			}
			s.redeemed[jti] = true
			return true, nil
		}).AnyTimes()
	s.store.EXPECT().CreateProjectAuditLog(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, entry *types.ProjectAuditLog) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.audit = append(s.audit, entry)
			return nil
		}).AnyTimes()
}

// Credentials. A session key is what Helix injects into a spec task desktop.
func sessionKey(sessionID, taskID string) *types.User {
	return &types.User{ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
		OrganizationID: attOrgA, ProjectID: attProjectA, SessionID: sessionID, SpecTaskID: taskID}
}

func attestationPersonalKey(userID string) *types.User {
	return &types.User{ID: userID, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI}
}

func attestationOrgKey(userID, orgID string) *types.User {
	return &types.User{ID: userID, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: orgID}
}

func browserSession(userID string) *types.User {
	return &types.User{ID: userID, TokenType: types.TokenTypeSession}
}

func (s *IdentityAttestationSuite) post(path string, caller *types.User, body any, handler http.HandlerFunc) *httptest.ResponseRecorder {
	raw, err := json.Marshal(body)
	s.Require().NoError(err)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setTestRequestUser(req.Context(), caller))
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func (s *IdentityAttestationSuite) mint(caller *types.User, body any) *httptest.ResponseRecorder {
	return s.post("/api/v1/identity/attestations", caller, body, system.Wrapper(s.server.createIdentityAttestation))
}

func (s *IdentityAttestationSuite) verify(caller *types.User, body any) *httptest.ResponseRecorder {
	return s.post("/api/v1/identity/attestations/verify", caller, body, s.server.verifyIdentityAttestation)
}

func (s *IdentityAttestationSuite) mintToken(caller *types.User) string {
	rr := s.mint(caller, types.IdentityAttestationRequest{Audience: attAudience, Nonce: attNonce})
	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	var resp types.IdentityAttestationResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))
	s.Require().NotEmpty(resp.Token)
	return resp.Token
}

func (s *IdentityAttestationSuite) verifyRequest(token string) types.IdentityAttestationVerifyRequest {
	return types.IdentityAttestationVerifyRequest{Token: token, Audience: attAudience, Nonce: attNonce}
}

func (s *IdentityAttestationSuite) requireNotValid(rr *httptest.ResponseRecorder) {
	s.Require().Equal(http.StatusNotFound, rr.Code, rr.Body.String())
	s.Require().JSONEq(`{"error":"attestation not valid"}`, rr.Body.String())
}

func (s *IdentityAttestationSuite) auditEvents(event types.AuditEventType) []*types.ProjectAuditLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*types.ProjectAuditLog
	for _, e := range s.audit {
		if e.EventType == event {
			out = append(out, e)
		}
	}
	return out
}

func (s *IdentityAttestationSuite) TestFinderMintsFinderNeverVerifier() {
	// A finder lane asking to be the verifier in its body is ignored: claims
	// come from the key only.
	rr := s.mint(sessionKey(attFinderSes, attFinderTsk), map[string]string{
		"audience": attAudience, "nonce": attNonce,
		"spec_task_id": attVerifTsk, "session_id": attVerifSes, "owner": attOwnerB,
	})
	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	var resp types.IdentityAttestationResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))

	vr := s.verify(attestationOrgKey(attOwnerA, attOrgA), s.verifyRequest(resp.Token))
	s.Require().Equal(http.StatusOK, vr.Code, vr.Body.String())
	var claims types.IdentityAttestationClaims
	s.Require().NoError(json.Unmarshal(vr.Body.Bytes(), &claims))
	s.Equal(attFinderTsk, claims.SpecTaskID)
	s.Equal(attFinderSes, claims.SessionID)
	s.Equal(attProjectA, claims.ProjectID)
	s.Equal(attOrgA, claims.OrganizationID)
	s.Equal(attOwnerA, claims.Owner)
	s.Equal(attAudience, claims.Audience)
	s.Equal(attNonce, claims.Nonce)
	s.Equal(attestation.TTL, claims.ExpiresAt.Sub(claims.IssuedAt))
	s.NotEmpty(claims.JTI)
	s.Empty(claims.SandboxID)

	minted := s.auditEvents(types.AuditEventIdentityAttestationMinted)
	s.Require().Len(minted, 1)
	s.Equal(attFinderTsk, minted[0].SpecTaskID)
	s.Equal(attOwnerA, minted[0].UserID)
	s.Equal(claims.JTI, minted[0].Metadata.AttestationJTI)
	verified := s.auditEvents(types.AuditEventIdentityAttestationVerified)
	s.Require().Len(verified, 1)
	s.Equal(claims.JTI, verified[0].Metadata.AttestationJTI)
	s.Equal(attProjectA, verified[0].ProjectID)
}

func (s *IdentityAttestationSuite) TestVerifierLaneMintsVerifier() {
	token := s.mintToken(sessionKey(attVerifSes, attVerifTsk))
	vr := s.verify(browserSession(attOwnerA), s.verifyRequest(token))
	s.Require().Equal(http.StatusOK, vr.Code, vr.Body.String())
	var claims types.IdentityAttestationClaims
	s.Require().NoError(json.Unmarshal(vr.Body.Bytes(), &claims))
	s.Equal(attVerifTsk, claims.SpecTaskID)
}

func (s *IdentityAttestationSuite) TestSandboxKeyMints() {
	key := &types.User{ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
		OrganizationID: attOrgA, ProjectID: attProjectA, SessionID: attSandbox}
	token := s.mintToken(key)
	vr := s.verify(attestationOrgKey(attOwnerA, attOrgA), s.verifyRequest(token))
	s.Require().Equal(http.StatusOK, vr.Code, vr.Body.String())
	var claims types.IdentityAttestationClaims
	s.Require().NoError(json.Unmarshal(vr.Body.Bytes(), &claims))
	s.Equal(attSandbox, claims.SandboxID)
	s.Empty(claims.SpecTaskID)
	s.Empty(claims.SessionID)
}

func (s *IdentityAttestationSuite) TestMintRefusesOtherCredentials() {
	embed := sessionKey(attFinderSes, attFinderTsk)
	embed.APIKeyType = types.APIkeytypeEmbed
	botInstance := sessionKey(attFinderSes, attFinderTsk)
	botInstance.APIKeyType = types.APIkeytypeBotInstance
	appKey := sessionKey(attFinderSes, attFinderTsk)
	appKey.APIKeyType = types.APIkeytypeApp
	appKey.AppID = "app_1"
	projectless := sessionKey(attFinderSes, attFinderTsk)
	projectless.ProjectID = ""

	cases := map[string]*types.User{
		"personal key":         attestationPersonalKey(attOwnerA),
		"org key":              attestationOrgKey(attOwnerA, attOrgA),
		"project key":          {ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: attOrgA, ProjectID: attProjectA},
		"app key":              appKey,
		"browser session":      browserSession(attOwnerA),
		"oidc":                 {ID: attOwnerA, TokenType: types.TokenTypeOIDC},
		"embed key":            embed,
		"bot instance key":     botInstance,
		"session key, no proj": projectless,
		"runner":               {ID: "runner", TokenType: types.TokenTypeRunner, SessionID: attFinderSes, ProjectID: attProjectA},
	}
	for name, caller := range cases {
		s.Run(name, func() {
			rr := s.mint(caller, types.IdentityAttestationRequest{Audience: attAudience, Nonce: attNonce})
			s.Equal(http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
	s.Empty(s.auditEvents(types.AuditEventIdentityAttestationMinted))
}

func (s *IdentityAttestationSuite) TestMintRefusesUnboundKeys() {
	wrongTask := sessionKey(attFinderSes, attVerifTsk) // key says verifier, its session is the finder's
	otherOwner := sessionKey(attFinderSes, attFinderTsk)
	otherOwner.ID = attMemberA
	otherProject := sessionKey(attFinderSes, attFinderTsk)
	otherProject.ProjectID = attProjectB
	otherOrg := sessionKey(attFinderSes, attFinderTsk)
	otherOrg.OrganizationID = attOrgB
	foreignSandbox := &types.User{ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
		OrganizationID: attOrgA, ProjectID: attProjectB, SessionID: attSandbox}

	cases := map[string]*types.User{
		"deleted spec task":         sessionKey(attGoneSes, attGoneTsk),
		"archived spec task":        sessionKey(attArchSes, attArchTsk),
		"deleted session":           sessionKey("ses_att_missing", attFinderTsk),
		"key/session task mismatch": wrongTask,
		"session of another owner":  otherOwner,
		"key of another project":    otherProject,
		"key of another org":        otherOrg,
		"sandbox of another proj":   foreignSandbox,
		"deleted sandbox":           {ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: attOrgA, ProjectID: attProjectA, SessionID: "sbx_att_gone"},
	}
	for name, caller := range cases {
		s.Run(name, func() {
			rr := s.mint(caller, types.IdentityAttestationRequest{Audience: attAudience, Nonce: attNonce})
			s.Equal(http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
	s.Empty(s.auditEvents(types.AuditEventIdentityAttestationMinted))
}

func (s *IdentityAttestationSuite) TestMintValidatesBody() {
	key := sessionKey(attFinderSes, attFinderTsk)
	for name, body := range map[string]any{
		"missing audience": types.IdentityAttestationRequest{Nonce: attNonce},
		"missing nonce":    types.IdentityAttestationRequest{Audience: attAudience},
		"short nonce":      types.IdentityAttestationRequest{Audience: attAudience, Nonce: "short"},
		"long nonce":       types.IdentityAttestationRequest{Audience: attAudience, Nonce: strings.Repeat("n", 129)},
		"bad audience":     types.IdentityAttestationRequest{Audience: "evil audience\n", Nonce: attNonce},
		"long audience":    types.IdentityAttestationRequest{Audience: strings.Repeat("a", 129), Nonce: attNonce},
	} {
		s.Run(name, func() {
			rr := s.mint(key, body)
			s.Equal(http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
}

func (s *IdentityAttestationSuite) TestVerifyRefusesForeignCallers() {
	token := s.mintToken(sessionKey(attFinderSes, attFinderTsk))

	cases := map[string]*types.User{
		"org B key":                     attestationOrgKey(attOwnerB, attOrgB),
		"org B owner signed in":         browserSession(attOwnerB),
		"org A member, no project":      attestationOrgKey(attMemberA, attOrgA),
		"org A member signed in":        browserSession(attMemberA),
		"project owner's key for org B": attestationOrgKey(attOwnerA, attOrgB),
	}
	for name, caller := range cases {
		s.Run(name, func() {
			s.requireNotValid(s.verify(caller, s.verifyRequest(token)))
		})
	}
	s.Empty(s.redeemed, "a refused verifier must not burn the token")
	s.Empty(s.auditEvents(types.AuditEventIdentityAttestationVerified))

	// The token is still good for an authorized verifier.
	s.Equal(http.StatusOK, s.verify(attestationOrgKey(attOwnerA, attOrgA), s.verifyRequest(token)).Code)
}

func (s *IdentityAttestationSuite) TestVerifyRefusesAgentKeys() {
	token := s.mintToken(sessionKey(attFinderSes, attFinderTsk))
	embed := sessionKey(attFinderSes, attFinderTsk)
	embed.APIKeyType = types.APIkeytypeEmbed
	cases := map[string]*types.User{
		"finder session key":   sessionKey(attFinderSes, attFinderTsk),
		"verifier session key": sessionKey(attVerifSes, attVerifTsk),
		"project key":          {ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: attOrgA, ProjectID: attProjectA},
		"sandbox key":          {ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: attOrgA, SessionID: attSandbox},
		"app key":              {ID: attOwnerA, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeApp, AppID: "app_1"},
		"embed key":            embed,
		"runner":               {ID: "runner", TokenType: types.TokenTypeRunner},
		"socket":               {ID: "socket", TokenType: types.TokenTypeSocket},
	}
	for name, caller := range cases {
		s.Run(name, func() {
			rr := s.verify(caller, s.verifyRequest(token))
			s.Equal(http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
	s.Empty(s.redeemed)
}

func (s *IdentityAttestationSuite) TestVerifyRefusesBadTokens() {
	verifier := attestationOrgKey(attOwnerA, attOrgA)
	token := s.mintToken(sessionKey(attFinderSes, attFinderTsk))

	past := time.Now().Add(-attestation.TTL - attestation.ClockSkew - time.Minute)
	s.server.attestations = s.signer.WithClock(func() time.Time { return past })
	expired := s.mintToken(sessionKey(attFinderSes, attFinderTsk))
	s.server.attestations = s.signer

	otherSigner, err := attestation.NewSigner("another-secret-another-secret-xx")
	s.Require().NoError(err)
	forged, _, err := otherSigner.Mint(attestation.Subject{SpecTaskID: attVerifTsk, ProjectID: attProjectA, OrganizationID: attOrgA, Owner: attOwnerA}, attAudience, attNonce)
	s.Require().NoError(err)

	parts := strings.Split(token, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]

	cases := map[string]types.IdentityAttestationVerifyRequest{
		"wrong audience": {Token: token, Audience: "other.example.com", Nonce: attNonce},
		"wrong nonce":    {Token: token, Audience: attAudience, Nonce: "nonce-ffffffffffffffff"},
		"expired":        s.verifyRequest(expired),
		"tampered":       s.verifyRequest(tampered),
		"forged":         s.verifyRequest(forged),
		"empty":          s.verifyRequest(""),
	}
	for name, req := range cases {
		s.Run(name, func() {
			s.requireNotValid(s.verify(verifier, req))
		})
	}
	s.Empty(s.redeemed)

	s.Equal(http.StatusOK, s.verify(verifier, s.verifyRequest(token)).Code)
	s.requireNotValid(s.verify(verifier, s.verifyRequest(token)))
	s.requireNotValid(s.verify(browserSession(attOwnerA), s.verifyRequest(token)))
	s.Len(s.auditEvents(types.AuditEventIdentityAttestationVerified), 1)
}
