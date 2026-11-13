package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Cross-project isolation within one organization and one owner.
//
// The PTaaS portal's service user owns both projects of an engagement and
// mints every key: project M (main, finder lanes) and project V (verify,
// verifier lanes, task-confined). Ownership therefore says nothing; only a
// key's scope keeps M's lanes out of V. The owner is also an owner of the
// organization, so every owner and org-owner shortcut is open, which is the
// worst case for the scope checks.
//
// Each kind of key M hands out is sent at every V surface through the real
// router and handlers. Every request must be refused (401/403/404) and must
// write nothing: the store records any call that is not a read, and the test
// fails on one.

const (
	isoOrgID      = "org_portal"
	isoOwnerID    = "usr_portal"
	isoMainID     = "prj_main"
	isoVerifyID   = "prj_verify"
	isoMainTask   = "spt_main"
	isoVerifyTask = "spt_verify"
	isoMainSes    = "ses_main"
	isoVerifySes  = "ses_verify"
	isoVerifyRepo = "repo_verify"
	isoVerifyArt  = "art_verify"
	isoSandboxID  = "sbx_main"
)

// isoKeys are the keys project M hands out, by kind.
var isoKeys = map[string]*types.ApiKey{
	"task-confined session key": {Key: types.APIKeyPrefix + "isoconfined", ProjectID: isoMainID, SpecTaskID: isoMainTask, SessionID: isoMainSes, TaskConfined: true},
	"unconfined session key":    {Key: types.APIKeyPrefix + "isosession", ProjectID: isoMainID, SpecTaskID: isoMainTask, SessionID: isoMainSes},
	"sandbox key":               {Key: types.APIKeyPrefix + "isosandbox", ProjectID: isoMainID, SessionID: isoSandboxID},
	"project-scoped user key":   {Key: types.APIKeyPrefix + "isoproject", ProjectID: isoMainID},
}

// isoOrgKey is the portal's organization key, which keeps reading V's
// engagement files.
var isoOrgKey = &types.ApiKey{Key: types.APIKeyPrefix + "isoorg"}

// isoReadCall matches store methods that only read, plus the auth
// bookkeeping every authenticated request does.
var isoReadCall = regexp.MustCompile(`^(Get|List|Find|Count|Has|Is|Exists|Search|Lookup|EnsureUserMeta|TouchUserLastSeen)`)

type isoStore struct {
	mu     sync.Mutex
	writes []string
}

func (s *isoStore) takeWrites() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.writes
	s.writes = nil
	return out
}

func newIsolationStore(t *testing.T) (*store.MockStore, *isoStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mock := store.NewMockStore(ctrl)
	rec := &isoStore{}
	iface := reflect.TypeOf((*store.Store)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		name, mt := m.Name, m.Type
		matchers := make([]any, mt.NumIn())
		for j := range matchers {
			matchers[j] = gomock.Any()
		}
		fn := reflect.MakeFunc(mt, func(args []reflect.Value) []reflect.Value {
			if !isoReadCall.MatchString(name) {
				rec.mu.Lock()
				rec.writes = append(rec.writes, name)
				rec.mu.Unlock()
			}
			return isoStoreResult(name, mt, args)
		})
		reflect.ValueOf(mock.EXPECT()).MethodByName(name).Call(reflectValues(matchers))[0].
			MethodByName("DoAndReturn").Call([]reflect.Value{fn})[0].
			MethodByName("AnyTimes").Call(nil)
	}
	return mock, rec
}

func reflectValues(in []any) []reflect.Value {
	out := make([]reflect.Value, len(in))
	for i, v := range in {
		out[i] = reflect.ValueOf(v)
	}
	return out
}

func isoKeyRow(token string) *types.ApiKey {
	for _, k := range append(mapValues(isoKeys), isoOrgKey) {
		if k.Key == token {
			row := *k
			row.Owner, row.OwnerType, row.Type, row.OrganizationID = isoOwnerID, types.OwnerTypeUser, types.APIkeytypeAPI, isoOrgID
			return &row
		}
	}
	return nil
}

func mapValues(m map[string]*types.ApiKey) []*types.ApiKey {
	out := make([]*types.ApiKey, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func isoSession(id, projectID, taskID string) *types.Session {
	return &types.Session{ID: id, Owner: isoOwnerID, OwnerType: types.OwnerTypeUser, OrganizationID: isoOrgID, ProjectID: projectID,
		Metadata: types.SessionMetadata{SpecTaskID: taskID, ProjectID: projectID}}
}

func isoStoreResult(name string, mt reflect.Type, args []reflect.Value) []reflect.Value {
	arg := func(i int) any { return args[i].Interface() }
	switch name {
	case "GetAPIKey":
		if row := isoKeyRow(arg(1).(*types.ApiKey).Key); row != nil {
			return values(row, nil)
		}
		return values((*types.ApiKey)(nil), store.ErrNotFound)
	case "GetUser":
		return values(&types.User{ID: isoOwnerID, Type: types.OwnerTypeUser}, nil)
	case "GetOrganizationMembership":
		return values(&types.OrganizationMembership{OrganizationID: isoOrgID, UserID: isoOwnerID, Role: types.OrganizationRoleOwner}, nil)
	case "GetOrganization":
		return values(&types.Organization{ID: isoOrgID, Name: "portal", Owner: isoOwnerID}, nil)
	case "GetProject":
		id := arg(1).(string)
		return values(&types.Project{ID: id, Name: id, OrganizationID: isoOrgID, UserID: isoOwnerID}, nil)
	case "GetSession":
		switch arg(1).(string) {
		case isoVerifySes:
			return values(isoSession(isoVerifySes, isoVerifyID, isoVerifyTask), nil)
		case isoMainSes:
			return values(isoSession(isoMainSes, isoMainID, isoMainTask), nil)
		}
		return values((*types.Session)(nil), store.ErrNotFound)
	case "GetSpecTask":
		switch arg(1).(string) {
		case isoVerifyTask:
			return values(&types.SpecTask{ID: isoVerifyTask, ProjectID: isoVerifyID, OrganizationID: isoOrgID, UserID: isoOwnerID, CreatedBy: isoOwnerID, Status: types.TaskStatusBacklog}, nil)
		case isoMainTask:
			return values(&types.SpecTask{ID: isoMainTask, ProjectID: isoMainID, OrganizationID: isoOrgID, UserID: isoOwnerID, CreatedBy: isoOwnerID, Status: types.TaskStatusBacklog}, nil)
		}
		return values((*types.SpecTask)(nil), store.ErrNotFound)
	case "GetGitRepository":
		return values(&types.GitRepository{ID: arg(1).(string), OrganizationID: isoOrgID, OwnerID: isoOwnerID, LocalPath: os.TempDir()}, nil)
	case "ListProjectRepositories":
		q := arg(1).(*types.ListProjectRepositoriesQuery)
		if q.ProjectID == isoVerifyID && (q.RepositoryID == "" || q.RepositoryID == isoVerifyRepo) {
			return values([]*types.ProjectRepository{{ProjectID: isoVerifyID, RepositoryID: isoVerifyRepo}}, nil)
		}
		return values([]*types.ProjectRepository(nil), nil)
	case "GetArtifact":
		return values(&types.Artifact{ID: isoVerifyArt, ProjectID: isoVerifyID, OrganizationID: isoOrgID, CreatedBy: isoOwnerID}, nil)
	case "GetSandbox":
		return values(&types.Sandbox{ID: isoSandboxID, ProjectID: isoMainID, OrganizationID: isoOrgID, Owner: isoOwnerID}, nil)
	}
	outs := make([]reflect.Value, mt.NumOut())
	for i := range outs {
		ot := mt.Out(i)
		switch {
		case ot == errType && strings.HasPrefix(name, "Get"):
			outs[i] = reflect.ValueOf(&store.ErrNotFound).Elem()
		default:
			outs[i] = reflect.Zero(ot)
		}
	}
	return outs
}

// isoFilestore answers every filestore call with an item, so a request that
// gets past its authorization is served rather than failing on its own.
func isoFilestore(t *testing.T) *filestore.MockFileStore {
	fs := filestore.NewMockFileStore(gomock.NewController(t))
	fs.EXPECT().List(gomock.Any(), gomock.Any()).Return([]filestore.Item{{Name: "src.tar"}}, nil).AnyTimes()
	fs.EXPECT().Get(gomock.Any(), gomock.Any()).Return(filestore.Item{Name: "src.tar"}, nil).AnyTimes()
	fs.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).Return(filestore.Item{}, nil).AnyTimes()
	fs.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	fs.EXPECT().Rename(gomock.Any(), gomock.Any(), gomock.Any()).Return(filestore.Item{}, nil).AnyTimes()
	return fs
}

type isoProbe struct {
	surface, method, target, body string
	git                           bool
}

var isoProbes = []isoProbe{
	{"project", http.MethodGet, "/api/v1/projects/" + isoVerifyID, "", false},
	{"project", http.MethodPut, "/api/v1/projects/" + isoVerifyID, `{"name":"x"}`, false},
	{"project guidelines", http.MethodGet, "/api/v1/projects/" + isoVerifyID + "/guidelines-history", "", false},
	{"project secrets", http.MethodGet, "/api/v1/projects/" + isoVerifyID + "/secrets", "", false},
	{"project secrets", http.MethodPost, "/api/v1/projects/" + isoVerifyID + "/secrets", `{"name":"CRED_X","value":"v"}`, false},
	{"spec task", http.MethodGet, "/api/v1/spec-tasks/" + isoVerifyTask, "", false},
	{"spec task", http.MethodPut, "/api/v1/spec-tasks/" + isoVerifyTask, `{"status":"done"}`, false},
	{"spec task list", http.MethodGet, "/api/v1/spec-tasks?project_id=" + isoVerifyID, "", false},
	{"spec task create", http.MethodPost, "/api/v1/spec-tasks/from-prompt", `{"project_id":"` + isoVerifyID + `","prompt":"x"}`, false},
	{"session read", http.MethodGet, "/api/v1/sessions/" + isoVerifySes, "", false},
	{"session chat", http.MethodPost, "/api/v1/sessions/" + isoVerifySes + "/messages", `{"content":"x"}`, false},
	{"session interactions", http.MethodGet, "/api/v1/sessions/" + isoVerifySes + "/interactions", "", false},
	{"session websocket", http.MethodGet, "/api/v1/ws/user?session_id=" + isoVerifySes, "", false},
	{"session agent sync websocket", http.MethodGet, "/api/v1/external-agents/sync?session_id=" + isoVerifySes, "", false},
	{"session env (zed config)", http.MethodGet, "/api/v1/sessions/" + isoVerifySes + "/zed-config", "", false},
	{"repository", http.MethodGet, "/api/v1/git/repositories/" + isoVerifyRepo, "", false},
	{"artifact list", http.MethodGet, "/api/v1/projects/" + isoVerifyID + "/artifacts", "", false},
	{"artifact", http.MethodGet, "/api/v1/artifacts/" + isoVerifyArt, "", false},
	{"artifact", http.MethodDelete, "/api/v1/artifacts/" + isoVerifyArt, "", false},
	{"git clone/fetch", http.MethodGet, "/git/" + isoVerifyRepo + "/info/refs?service=git-upload-pack", "", true},
	{"git clone/fetch", http.MethodPost, "/git/" + isoVerifyRepo + "/git-upload-pack", "", true},
	{"git push", http.MethodGet, "/git/" + isoVerifyRepo + "/info/refs?service=git-receive-pack", "", true},
	{"git push", http.MethodPost, "/git/" + isoVerifyRepo + "/git-receive-pack", "", true},
	{"filestore", http.MethodGet, "/api/v1/filestore/list?path=engagements/" + isoVerifyID, "", false},
	{"filestore", http.MethodGet, "/api/v1/filestore/list?path=", "", false},
	{"filestore", http.MethodGet, "/api/v1/filestore/get?path=engagements/" + isoVerifyID + "/src.tar", "", false},
	{"filestore", http.MethodGet, "/api/v1/filestore/viewer/users/" + isoOwnerID + "/engagements/" + isoVerifyID + "/src.tar", "", false},
	{"filestore", http.MethodPost, "/api/v1/filestore/folder", `{"path":"engagements/` + isoVerifyID + `/x"}`, false},
	{"filestore", http.MethodPut, "/api/v1/filestore/rename?path=engagements/" + isoVerifyID + "/src.tar&new_path=projects/" + isoMainID + "/src.tar", "", false},
	{"filestore", http.MethodDelete, "/api/v1/filestore/delete?path=engagements/" + isoVerifyID + "/src.tar", "", false},
}

// isoGitRouter serves the git HTTP server with a repository service, which
// the shared route test server leaves out.
func isoGitRouter(t *testing.T, srv *HelixAPIServer) *mux.Router {
	t.Helper()
	repos := services.NewGitRepositoryService(srv.Store, t.TempDir(), srv.Cfg.WebServer.URL, "test", "test@example.com")
	gs := services.NewGitHTTPServer(srv.Store, repos, services.GitHTTPServerConfig{
		ServerBaseURL: srv.Cfg.WebServer.URL, AuthTokenHeader: "Authorization", EnablePush: true, EnablePull: true,
	}, srv.authorizeUserToRepository, nil)
	router := mux.NewRouter()
	gs.RegisterRoutes(router)
	return router
}

func isoServe(srv *HelixAPIServer, gitRouter *mux.Router, p isoProbe, token string) (code int, body string) {
	var req *http.Request
	if p.body == "" {
		req = httptest.NewRequest(p.method, p.target, http.NoBody)
	} else {
		req = httptest.NewRequest(p.method, p.target, strings.NewReader(p.body))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if r := recover(); r != nil {
				rec.Code = http.StatusInternalServerError
				rec.Body.WriteString(fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
			}
		}()
		if p.git {
			gitRouter.ServeHTTP(rec, req)
			return
		}
		srv.router.ServeHTTP(rec, req)
	}()
	return rec.Code, rec.Body.String()
}

func TestCrossProjectIsolation_MainKeysCannotReachVerify(t *testing.T) {
	st, rec := newIsolationStore(t)
	srv := newRouteTestServer(t, st)
	srv.Controller.Options.Filestore = isoFilestore(t)
	gitRouter := isoGitRouter(t, srv)

	for kind, key := range isoKeys {
		for _, p := range isoProbes {
			t.Run(kind+"/"+p.surface+"/"+p.method+" "+p.target, func(t *testing.T) {
				rec.takeWrites()
				code, body := isoServe(srv, gitRouter, p, key.Key)
				t.Logf("%d %s", code, strings.SplitN(body, "\n", 2)[0])
				assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}, code,
					"must be refused, got %d: %s", code, body)
				assert.Empty(t, rec.takeWrites(), "a refused request must not write")
			})
		}
	}
}

// The portal's organization key keeps reading V's engagement folder: the
// filestore rule binds keys with a project only.
func TestCrossProjectIsolation_OrgKeyKeepsEngagementFilestore(t *testing.T) {
	st, rec := newIsolationStore(t)
	srv := newRouteTestServer(t, st)
	srv.Controller.Options.Filestore = isoFilestore(t)
	for _, target := range []string{
		"/api/v1/filestore/list?path=engagements/" + isoVerifyID,
		"/api/v1/filestore/get?path=engagements/" + isoVerifyID + "/src.tar",
	} {
		code, body := isoServe(srv, nil, isoProbe{method: http.MethodGet, target: target}, isoOrgKey.Key)
		require.Equal(t, http.StatusOK, code, "%s: %s", target, body)
	}
	assert.Empty(t, rec.takeWrites())
}

// The LLM proxy attributes a call to the key's project, whatever project
// the request names.
func TestCrossProjectIsolation_LLMProxyAttributesToKeyProject(t *testing.T) {
	for kind, key := range isoKeys {
		t.Run(kind, func(t *testing.T) {
			upstream := newFakeOpenAI(t)
			server, mockStore, capture := codeAgentTestServer(t, upstream)
			mockStore.EXPECT().GetUserMeta(gomock.Any(), isoOwnerID).Return(&types.UserMeta{ID: isoOwnerID}, nil).AnyTimes()
			mockStore.EXPECT().GetSession(gomock.Any(), isoMainSes).Return(isoSession(isoMainSes, isoMainID, isoMainTask), nil).AnyTimes()
			mockStore.EXPECT().GetSpecTask(gomock.Any(), isoMainTask).Return(&types.SpecTask{ID: isoMainTask, ProjectID: isoMainID}, nil).AnyTimes()
			req := codeAgentChatRequest(t, types.User{
				ID: isoOwnerID, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: isoOrgID,
				ProjectID: key.ProjectID, SpecTaskID: key.SpecTaskID, SessionID: key.SessionID, TaskConfined: key.TaskConfined,
			})
			q := req.URL.Query()
			q.Set("project_id", isoVerifyID)
			req.URL.RawQuery = q.Encode()
			req.Header.Set("X-Helix-Project-ID", isoVerifyID)
			rec := httptest.NewRecorder()
			server.createChatCompletion(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Eventually(t, func() bool { return capture.get() != nil }, 5e9, 2e7)
			assert.Equal(t, isoMainID, capture.get().ProjectID)
		})
	}
}
