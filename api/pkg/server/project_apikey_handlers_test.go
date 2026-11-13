package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Project API keys: who may mint, list and revoke them. Tenant A owns
// prj_a; tenant B is another organization. Every refusal also asserts the
// store saw no write.

const (
	pkOrgA        = "org_pk_a"
	pkOrgB        = "org_pk_b"
	pkOwnerA      = "usr_pk_owner_a"  // org A member, owner of prj_a
	pkMemberA     = "usr_pk_member_a" // org A member with no grant on prj_a
	pkOwnerB      = "usr_pk_owner_b"  // org B owner
	pkProjectA    = "prj_pk_a"
	pkProjectB    = "prj_pk_b"
	pkPersonalPrj = "prj_pk_personal" // ownerA's project outside any org
)

type ProjectAPIKeySuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
	audits *sync.WaitGroup

	mu      sync.Mutex
	keys    []*types.ApiKey
	created []*types.ApiKey
	deleted []string
	logged  []*types.ProjectAuditLog
}

func TestProjectAPIKeySuite(t *testing.T) { suite.Run(t, new(ProjectAPIKeySuite)) }

func (s *ProjectAPIKeySuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.keys, s.created, s.deleted, s.logged = nil, nil, nil, nil

	projects := map[string]*types.Project{
		pkProjectA:    {ID: pkProjectA, Name: "A", OrganizationID: pkOrgA, UserID: pkOwnerA},
		pkProjectB:    {ID: pkProjectB, Name: "B", OrganizationID: pkOrgB, UserID: pkOwnerB},
		pkPersonalPrj: {ID: pkPersonalPrj, Name: "personal", UserID: pkOwnerA},
	}
	memberships := map[string]map[string]types.OrganizationRole{
		pkOwnerA:  {pkOrgA: types.OrganizationRoleMember},
		pkMemberA: {pkOrgA: types.OrganizationRoleMember},
		pkOwnerB:  {pkOrgB: types.OrganizationRoleOwner},
	}

	s.store.EXPECT().GetProject(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Project, error) {
			if p, ok := projects[id]; ok {
				copied := *p
				return &copied, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			if role, ok := memberships[q.UserID][q.OrganizationID]; ok {
				return &types.OrganizationMembership{UserID: q.UserID, OrganizationID: q.OrganizationID, Role: role}, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	s.store.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	s.store.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.created = append(s.created, key)
			s.keys = append(s.keys, key)
			return key, nil
		}).AnyTimes()
	s.store.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListAPIKeysQuery) ([]*types.ApiKey, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			// The handler must scope the listing in the query itself.
			s.Require().NotEmpty(q.Owner, "listing must be scoped to the caller")
			s.Require().NotEmpty(q.ProjectID, "listing must be scoped to the project")
			s.Require().True(q.UserManaged, "listing must exclude session keys")
			var out []*types.ApiKey
			for _, k := range s.keys {
				if k.Owner == q.Owner && k.ProjectID == q.ProjectID && k.OrganizationID == q.OrganizationID &&
					k.SessionID == "" && k.SpecTaskID == "" {
					out = append(out, k)
				}
			}
			return out, nil
		}).AnyTimes()
	s.store.EXPECT().DeleteAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.deleted = append(s.deleted, key)
			return nil
		}).AnyTimes()
	s.store.EXPECT().CreateProjectAuditLog(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, entry *types.ProjectAuditLog) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.logged = append(s.logged, entry)
			return nil
		}).AnyTimes()

	audit := services.NewAuditLogService(s.store)
	s.audits = &sync.WaitGroup{}
	audit.SetWaitGroup(s.audits)
	s.server = &HelixAPIServer{
		Store:           s.store,
		Controller:      &controller.Controller{Options: controller.Options{Store: s.store}},
		auditLogService: audit,
	}
}

func sessionCaller(id string) *types.User { return &types.User{ID: id, Type: types.OwnerTypeUser} }

func keyCaller(id string, key types.ApiKey) *types.User {
	u := &types.User{
		ID: id, Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey, Token: "hl-caller",
		APIKeyType: types.APIkeytypeAPI, OrganizationID: key.OrganizationID, ProjectID: key.ProjectID,
		SessionID: key.SessionID, SpecTaskID: key.SpecTaskID,
	}
	if key.AppID != nil {
		u.AppID = key.AppID.String
	}
	return u
}

func (s *ProjectAPIKeySuite) do(caller *types.User, method, path string, vars map[string]string, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = mux.SetURLVars(req, vars)
	req = req.WithContext(setTestRequestUser(req.Context(), caller))
	rr := httptest.NewRecorder()
	handler(rr, req)
	s.audits.Wait()
	return rr
}

func (s *ProjectAPIKeySuite) mint(caller *types.User, projectID string) *httptest.ResponseRecorder {
	body := `{"name":"portal","type":"api","project_id":"` + projectID + `"}`
	return s.do(caller, http.MethodPost, "/api/v1/api_keys", nil, body, system.DefaultWrapper(s.server.createAPIKey))
}

func (s *ProjectAPIKeySuite) list(caller *types.User, projectID string) *httptest.ResponseRecorder {
	return s.do(caller, http.MethodGet, "/api/v1/projects/"+projectID+"/api_keys",
		map[string]string{"id": projectID}, "", system.Wrapper(s.server.listProjectAPIKeys))
}

func (s *ProjectAPIKeySuite) revoke(caller *types.User, projectID, keyID string) *httptest.ResponseRecorder {
	return s.do(caller, http.MethodDelete, "/api/v1/projects/"+projectID+"/api_keys/"+keyID,
		map[string]string{"id": projectID, "key_id": keyID}, "", system.Wrapper(s.server.deleteProjectAPIKey))
}

// seedKey stores a project key directly, as if minted earlier.
func (s *ProjectAPIKeySuite) seedKey(owner, orgID, projectID, secret string) *types.ApiKey {
	key := &types.ApiKey{Key: secret, Name: "seed", Owner: owner, OwnerType: types.OwnerTypeUser,
		Type: types.APIkeytypeAPI, OrganizationID: orgID, ProjectID: projectID}
	s.keys = append(s.keys, key)
	return key
}

func (s *ProjectAPIKeySuite) requireNoWrites() {
	s.Empty(s.created, "no key may be created")
	s.Empty(s.deleted, "no key may be deleted")
	s.Empty(s.logged, "no audit row for a refused request")
}

func (s *ProjectAPIKeySuite) TestMintByOwnerSessionAndOrgKey() {
	for name, caller := range map[string]*types.User{
		"session":      sessionCaller(pkOwnerA),
		"org A key":    keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA}),
		"personal key": keyCaller(pkOwnerA, types.ApiKey{}),
	} {
		s.Run(name, func() {
			s.SetupTest()
			rr := s.mint(caller, pkProjectA)
			s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())

			var secret string
			s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &secret))
			s.Require().Len(s.created, 1)
			key := s.created[0]
			s.Equal(secret, key.Key)
			s.Equal(pkOwnerA, key.Owner)
			s.Equal(pkOrgA, key.OrganizationID)
			s.Equal(pkProjectA, key.ProjectID)
			s.Equal(types.APIkeytypeAPI, key.Type)
			s.Empty(key.SessionID)
			s.Empty(key.SpecTaskID)
			s.Nil(key.AppID)

			s.Require().Len(s.logged, 1)
			s.Equal(types.AuditEventAPIKeyCreated, s.logged[0].EventType)
			s.Equal(pkProjectA, s.logged[0].ProjectID)
			s.Equal(pkOwnerA, s.logged[0].UserID)
			s.Equal(types.APIKeyID(secret), s.logged[0].Metadata.APIKeyID)
			s.NotContains(s.logged[0].PromptText+s.logged[0].Metadata.APIKeyName, secret)
		})
	}
}

// Owner, organization and scope come from the caller and the project. A body
// naming any other field is refused, not silently narrowed.
func (s *ProjectAPIKeySuite) TestMintRejectsUnknownFields() {
	for _, field := range []string{
		`"owner":"` + pkOwnerB + `"`,
		`"organization_id":"` + pkOrgB + `"`,
		`"session_id":"ses_x"`,
		`"expires_at":"2030-01-01T00:00:00Z"`,
	} {
		s.Run(field, func() {
			s.SetupTest()
			body := `{"name":"portal","type":"api","project_id":"` + pkProjectA + `",` + field + `}`
			rr := s.do(sessionCaller(pkOwnerA), http.MethodPost, "/api/v1/api_keys", nil, body, system.DefaultWrapper(s.server.createAPIKey))
			s.Equal(http.StatusBadRequest, rr.Code, rr.Body.String())
			s.requireNoWrites()
		})
	}
}

// Clients send an empty app_id with every key; that is not an app key.
func (s *ProjectAPIKeySuite) TestMintAcceptsEmptyAppID() {
	body := `{"name":"portal","type":"api","app_id":"","project_id":"` + pkProjectA + `"}`
	rr := s.do(sessionCaller(pkOwnerA), http.MethodPost, "/api/v1/api_keys", nil, body, system.DefaultWrapper(s.server.createAPIKey))
	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	s.Require().Len(s.created, 1)
	s.Equal(pkProjectA, s.created[0].ProjectID)
}

// The ?name= form mints a personal key and never read the body, so a body
// asking for a project key silently got an unscoped one.
func (s *ProjectAPIKeySuite) TestLegacyNameQueryRefusesBody() {
	body := `{"type":"api","project_id":"` + pkProjectA + `"}`
	rr := s.do(sessionCaller(pkOwnerA), http.MethodPost, "/api/v1/api_keys?name=portal", nil, body, system.DefaultWrapper(s.server.createAPIKey))
	s.Equal(http.StatusBadRequest, rr.Code, rr.Body.String())
	s.requireNoWrites()

	rr = s.do(sessionCaller(pkOwnerA), http.MethodPost, "/api/v1/api_keys?name=portal", nil, "", system.DefaultWrapper(s.server.createAPIKey))
	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	s.Require().Len(s.created, 1)
	s.Empty(s.created[0].ProjectID, "the query form mints a personal key")
	s.Empty(s.created[0].OrganizationID)
}

func (s *ProjectAPIKeySuite) TestMintRefused() {
	appKey := keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA})
	appKey.AppID = "app_x"
	appKey.APIKeyType = types.APIkeytypeApp

	cases := []struct {
		name    string
		caller  *types.User
		project string
		status  int
	}{
		{"tenant B session, project A", sessionCaller(pkOwnerB), pkProjectA, http.StatusNotFound},
		{"tenant B org key, project A", keyCaller(pkOwnerB, types.ApiKey{OrganizationID: pkOrgB}), pkProjectA, http.StatusNotFound},
		{"org A key, personal project", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA}), pkPersonalPrj, http.StatusNotFound},
		{"tenant B, personal project of A", sessionCaller(pkOwnerB), pkPersonalPrj, http.StatusNotFound},
		{"unknown project", sessionCaller(pkOwnerA), "prj_missing", http.StatusNotFound},
		{"member without grant", sessionCaller(pkMemberA), pkProjectA, http.StatusForbidden},
		{"project key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, ProjectID: pkProjectA}), pkProjectA, http.StatusForbidden},
		{"session key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, ProjectID: pkProjectA, SessionID: "ses_1"}), pkProjectA, http.StatusForbidden},
		{"spec task key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, ProjectID: pkProjectA, SpecTaskID: "spt_1", SessionID: "ses_1"}), pkProjectA, http.StatusForbidden},
		{"projectless session key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, SessionID: "ses_1"}), pkProjectA, http.StatusForbidden},
		{"app key", appKey, pkProjectA, http.StatusForbidden},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.SetupTest()
			rr := s.mint(tc.caller, tc.project)
			s.Equal(tc.status, rr.Code, rr.Body.String())
			s.requireNoWrites()
		})
	}
}

func (s *ProjectAPIKeySuite) TestMintRejectsBadBodies() {
	for name, body := range map[string]string{
		"app key type":   `{"name":"x","type":"app","project_id":"` + pkProjectA + `"}`,
		"with an app_id": `{"name":"x","type":"api","app_id":"app_1","project_id":"` + pkProjectA + `"}`,
		"empty project":  `{"name":"x","type":"api","project_id":""}`,
	} {
		s.Run(name, func() {
			s.SetupTest()
			rr := s.do(sessionCaller(pkOwnerA), http.MethodPost, "/api/v1/api_keys", nil, body, system.DefaultWrapper(s.server.createAPIKey))
			s.Equal(http.StatusBadRequest, rr.Code, rr.Body.String())
			s.requireNoWrites()
		})
	}
}

func (s *ProjectAPIKeySuite) TestListReturnsOnlyOwnKeysWithoutSecrets() {
	own := s.seedKey(pkOwnerA, pkOrgA, pkProjectA, "hl-own-project-key-secret-000000")
	s.seedKey(pkMemberA, pkOrgA, pkProjectA, "hl-other-members-key-secret-0000")
	s.seedKey(pkOwnerA, pkOrgA, "prj_other", "hl-own-other-project-secret-0000")
	session := s.seedKey(pkOwnerA, pkOrgA, pkProjectA, "hl-own-session-key-secret-000000")
	session.SessionID = "ses_1"

	for name, caller := range map[string]*types.User{
		"session":   sessionCaller(pkOwnerA),
		"org A key": keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA}),
	} {
		s.Run(name, func() {
			rr := s.list(caller, pkProjectA)
			s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
			s.NotContains(rr.Body.String(), own.Key)
			var got []types.APIKeySummary
			s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &got))
			s.Require().Len(got, 1)
			s.Equal(types.APIKeyID(own.Key), got[0].ID)
			s.Equal(pkProjectA, got[0].ProjectID)
		})
	}
}

func (s *ProjectAPIKeySuite) TestListRefused() {
	cases := []struct {
		name   string
		caller *types.User
		status int
	}{
		{"tenant B session", sessionCaller(pkOwnerB), http.StatusNotFound},
		{"tenant B org key", keyCaller(pkOwnerB, types.ApiKey{OrganizationID: pkOrgB}), http.StatusNotFound},
		{"member without grant", sessionCaller(pkMemberA), http.StatusForbidden},
		{"project key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, ProjectID: pkProjectA}), http.StatusForbidden},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.SetupTest()
			key := s.seedKey(pkOwnerA, pkOrgA, pkProjectA, "hl-own-project-key-secret-000000")
			rr := s.list(tc.caller, pkProjectA)
			s.Equal(tc.status, rr.Code, rr.Body.String())
			s.NotContains(rr.Body.String(), types.APIKeyID(key.Key))
		})
	}
}

func (s *ProjectAPIKeySuite) TestRevokeOwnKey() {
	for name, caller := range map[string]*types.User{
		"session":   sessionCaller(pkOwnerA),
		"org A key": keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA}),
	} {
		s.Run(name, func() {
			s.SetupTest()
			key := s.seedKey(pkOwnerA, pkOrgA, pkProjectA, "hl-own-project-key-secret-000000")
			rr := s.revoke(caller, pkProjectA, types.APIKeyID(key.Key))
			s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
			s.NotContains(rr.Body.String(), key.Key)
			s.Equal([]string{key.Key}, s.deleted)
			s.Require().Len(s.logged, 1)
			s.Equal(types.AuditEventAPIKeyRevoked, s.logged[0].EventType)
			s.Equal(types.APIKeyID(key.Key), s.logged[0].Metadata.APIKeyID)
		})
	}
}

func (s *ProjectAPIKeySuite) TestRevokeRefused() {
	cases := []struct {
		name    string
		caller  *types.User
		project string
		status  int
	}{
		{"tenant B session", sessionCaller(pkOwnerB), pkProjectA, http.StatusNotFound},
		{"tenant B org key", keyCaller(pkOwnerB, types.ApiKey{OrganizationID: pkOrgB}), pkProjectA, http.StatusNotFound},
		// Tenant B names A's key under its own project: the key is not there.
		{"tenant B via own project", sessionCaller(pkOwnerB), pkProjectB, http.StatusNotFound},
		{"member without grant", sessionCaller(pkMemberA), pkProjectA, http.StatusForbidden},
		{"project key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, ProjectID: pkProjectA}), pkProjectA, http.StatusForbidden},
		{"session key", keyCaller(pkOwnerA, types.ApiKey{OrganizationID: pkOrgA, ProjectID: pkProjectA, SessionID: "ses_1"}), pkProjectA, http.StatusForbidden},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.SetupTest()
			key := s.seedKey(pkOwnerA, pkOrgA, pkProjectA, "hl-own-project-key-secret-000000")
			rr := s.revoke(tc.caller, tc.project, types.APIKeyID(key.Key))
			s.Equal(tc.status, rr.Code, rr.Body.String())
			s.requireNoWrites()
		})
	}
}

// An org owner is authorized to every project, but still only revokes their
// own keys here (the org key listing manages members' keys).
func (s *ProjectAPIKeySuite) TestRevokeAnotherUsersKeyIsNotFound() {
	key := s.seedKey(pkMemberA, pkOrgA, pkProjectA, "hl-members-project-key-secret-00")
	rr := s.revoke(sessionCaller(pkOwnerA), pkProjectA, types.APIKeyID(key.Key))
	s.Equal(http.StatusNotFound, rr.Code, rr.Body.String())
	s.requireNoWrites()

	rr = s.revoke(sessionCaller(pkOwnerA), pkProjectA, "not-a-key-id")
	s.Equal(http.StatusNotFound, rr.Code, rr.Body.String())
	s.requireNoWrites()
}
