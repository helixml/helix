package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
)

// A key bound to a project uses exactly two folders of its owner's
// filestore: projects/<own id>/ and engagements/<own id>/. These tests send
// M's keys at both folders of V (refused, nothing written) and of M
// (served), and check the org key keeps the whole namespace.

// fsWrites records filestore calls that change something.
type fsWrites struct {
	mu    sync.Mutex
	calls []string
}

func (w *fsWrites) add(call string) {
	w.mu.Lock()
	w.calls = append(w.calls, call)
	w.mu.Unlock()
}

func (w *fsWrites) take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.calls
	w.calls = nil
	return out
}

func recordingFilestore(t *testing.T, writes *fsWrites) *filestore.MockFileStore {
	fs := filestore.NewMockFileStore(gomock.NewController(t))
	fs.EXPECT().List(gomock.Any(), gomock.Any()).Return([]filestore.Item{{Name: "findings.json"}}, nil).AnyTimes()
	fs.EXPECT().Get(gomock.Any(), gomock.Any()).Return(filestore.Item{Name: "findings.json"}, nil).AnyTimes()
	fs.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, p string) (filestore.Item, error) {
		writes.add("CreateFolder " + p)
		return filestore.Item{}, nil
	}).AnyTimes()
	fs.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, p string) error {
		writes.add("Delete " + p)
		return nil
	}).AnyTimes()
	fs.EXPECT().Rename(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, from, to string) (filestore.Item, error) {
		writes.add("Rename " + from + " " + to)
		return filestore.Item{}, nil
	}).AnyTimes()
	fs.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, p string, r io.Reader) (filestore.Item, error) {
		writes.add("WriteFile " + p)
		return filestore.Item{}, nil
	}).AnyTimes()
	return fs
}

type fsProbe struct {
	name, method, target string
	upload               bool
	body                 string
}

// fsProbes are reads and writes of one folder (and a sibling inside it).
func fsProbes(folder string) []fsProbe {
	return []fsProbe{
		{"list", http.MethodGet, "/api/v1/filestore/list?path=" + folder, false, ""},
		{"get", http.MethodGet, "/api/v1/filestore/get?path=" + folder + "/findings.json", false, ""},
		{"viewer", http.MethodGet, "/api/v1/filestore/viewer/users/" + isoOwnerID + "/" + folder + "/findings.json", false, ""},
		{"upload", http.MethodPost, "/api/v1/filestore/upload?path=" + folder + "/collected/", true, ""},
		{"mkdir", http.MethodPost, "/api/v1/filestore/folder", false, `{"path":"` + folder + `/collected"}`},
		{"rename within", http.MethodPut, "/api/v1/filestore/rename?path=" + folder + "/notes.md&new_path=" + folder + "/notes-old.md", false, ""},
		{"delete", http.MethodDelete, "/api/v1/filestore/delete?path=" + folder + "/notes.md", false, ""},
	}
}

func fsServe(t *testing.T, srv *HelixAPIServer, p fsProbe, token string) (int, string) {
	t.Helper()
	var body io.Reader = http.NoBody
	contentType := ""
	if p.upload {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("files", "receipt.json")
		require.NoError(t, err)
		_, _ = part.Write([]byte(`{"ok":true}`))
		require.NoError(t, w.Close())
		body, contentType = &buf, w.FormDataContentType()
	} else if p.body != "" {
		body = strings.NewReader(p.body)
	}
	req := httptest.NewRequest(p.method, p.target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func newFilestoreFolderServer(t *testing.T) (*HelixAPIServer, *fsWrites, *isoStore) {
	t.Helper()
	st, rec := newIsolationStore(t)
	srv := newRouteTestServer(t, st)
	writes := &fsWrites{}
	srv.Controller.Options.Filestore = recordingFilestore(t, writes)
	srv.fileServerHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("served")) })
	return srv, writes, rec
}

// sessionAndSandboxKeys are M's keys that reach the filestore at all; the
// #3607 project key is refused it by its route allowlist.
var sessionAndSandboxKeys = []string{"task-confined session key", "unconfined session key", "sandbox key"}

func TestFilestore_MainKeysRefusedOnVerifyFolders(t *testing.T) {
	srv, writes, rec := newFilestoreFolderServer(t)
	var probes []fsProbe
	for _, folder := range []string{"engagements/" + isoVerifyID, "projects/" + isoVerifyID} {
		probes = append(probes, fsProbes(folder)...)
	}
	// Rename across projects, either side, and across prefixes into V.
	probes = append(probes,
		fsProbe{"rename out of V", http.MethodPut, "/api/v1/filestore/rename?path=engagements/" + isoVerifyID + "/findings.json&new_path=engagements/" + isoMainID + "/stolen.json", false, ""},
		fsProbe{"rename into V", http.MethodPut, "/api/v1/filestore/rename?path=engagements/" + isoMainID + "/fake.json&new_path=engagements/" + isoVerifyID + "/findings.json", false, ""},
		fsProbe{"rename M project into V engagement", http.MethodPut, "/api/v1/filestore/rename?path=projects/" + isoMainID + "/fake.json&new_path=engagements/" + isoVerifyID + "/findings.json", false, ""},
		// Prefix confusion and traversal, plain and URL-encoded.
		fsProbe{"sibling prefix", http.MethodGet, "/api/v1/filestore/list?path=engagements/" + isoMainID + "-x", false, ""},
		fsProbe{"sibling prefix projects", http.MethodGet, "/api/v1/filestore/get?path=projects/" + isoMainID + "x/a", false, ""},
		fsProbe{"traversal", http.MethodGet, "/api/v1/filestore/get?path=engagements/" + isoMainID + "/../" + isoVerifyID + "/findings.json", false, ""},
		fsProbe{"encoded traversal", http.MethodGet, "/api/v1/filestore/get?path=engagements/" + isoMainID + "/%2e%2e/" + isoVerifyID + "/findings.json", false, ""},
		fsProbe{"encoded slash traversal", http.MethodGet, "/api/v1/filestore/get?path=engagements%2F" + isoMainID + "%2F..%2F" + isoVerifyID + "%2Ffindings.json", false, ""},
		fsProbe{"cross-prefix traversal", http.MethodDelete, "/api/v1/filestore/delete?path=projects/" + isoMainID + "/../../engagements/" + isoVerifyID + "/findings.json", false, ""},
		fsProbe{"backslash traversal", http.MethodGet, "/api/v1/filestore/get?path=engagements\\" + isoMainID + "\\..\\" + isoVerifyID, false, ""},
		fsProbe{"double slash", http.MethodGet, "/api/v1/filestore/list?path=//engagements/" + isoVerifyID, false, ""},
		fsProbe{"engagements root", http.MethodGet, "/api/v1/filestore/list?path=engagements", false, ""},
		fsProbe{"viewer sibling prefix", http.MethodGet, "/api/v1/filestore/viewer/users/" + isoOwnerID + "/engagements/" + isoMainID + "-x/a", false, ""},
	)
	for kind, key := range isoKeys {
		for _, p := range probes {
			t.Run(kind+"/"+p.name+" "+p.target, func(t *testing.T) {
				writes.take()
				rec.takeWrites()
				code, body := fsServe(t, srv, p, key.Key)
				assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound, http.StatusBadRequest}, code, "must be refused, got %d: %s", code, body)
				assert.Empty(t, writes.take(), "a refused request must not change the filestore")
				assert.Empty(t, rec.takeWrites(), "a refused request must not write")
			})
		}
	}
}

func TestFilestore_MainKeysServedOnTheirOwnFolders(t *testing.T) {
	srv, writes, _ := newFilestoreFolderServer(t)
	var probes []fsProbe
	for _, folder := range []string{"engagements/" + isoMainID, "projects/" + isoMainID} {
		probes = append(probes, fsProbes(folder)...)
	}
	probes = append(probes, fsProbe{"rename across own folders", http.MethodPut,
		"/api/v1/filestore/rename?path=engagements/" + isoMainID + "/notes.md&new_path=projects/" + isoMainID + "/notes.md", false, ""})
	for _, kind := range sessionAndSandboxKeys {
		for _, p := range probes {
			t.Run(kind+"/"+p.name+" "+p.target, func(t *testing.T) {
				writes.take()
				code, body := fsServe(t, srv, p, isoKeys[kind].Key)
				assert.Equal(t, http.StatusOK, code, body)
				if p.method != http.MethodGet {
					assert.NotEmpty(t, writes.take(), "an allowed write reaches the filestore")
				}
			})
		}
	}
}

// The portal's org key keeps the whole namespace: it reads and writes V's
// engagement folder.
func TestFilestore_OrgKeyUnchanged(t *testing.T) {
	srv, writes, _ := newFilestoreFolderServer(t)
	for _, p := range append(fsProbes("engagements/"+isoVerifyID), fsProbes("projects/"+isoVerifyID)...) {
		writes.take()
		code, body := fsServe(t, srv, p, isoOrgKey.Key)
		assert.Equal(t, http.StatusOK, code, "%s %s: %s", p.name, p.target, body)
	}
}

func TestEnforceKeyFilestorePath_Allowlist(t *testing.T) {
	user := &types.User{ID: isoOwnerID, ProjectID: isoMainID, SessionID: isoMainSes}
	for _, p := range []string{
		"projects/" + isoMainID, "projects/" + isoMainID + "/notes.md", "/projects/" + isoMainID + "/a/b",
		"engagements/" + isoMainID, "engagements/" + isoMainID + "/collected/x.txt", "engagements//" + isoMainID + "/findings.json",
		"engagements/" + isoMainID + "/run/../findings.json",
	} {
		assert.NoError(t, enforceKeyFilestorePath(user, p), p)
	}
	for _, p := range []string{
		"", ".", "projects", "engagements", "engagements/" + isoVerifyID, "projects/" + isoVerifyID + "/a",
		"engagements/" + isoMainID + "-x/a", "engagements/" + isoMainID + "x", "projects/" + isoMainID + ".bak",
		"engagements/" + isoMainID + "/../" + isoVerifyID, "projects/" + isoMainID + "/../../engagements/" + isoVerifyID,
		`engagements\` + isoMainID + `\..\` + isoVerifyID, "notes.md", "apps-like/engagements/" + isoMainID,
	} {
		assert.ErrorIs(t, enforceKeyFilestorePath(user, p), errFilestoreOutsideKeyProject, p)
	}
	// Traversal out of the namespace is a bad path, not a folder refusal.
	assert.Error(t, enforceKeyFilestorePath(user, "../"+isoMainID))
	assert.NoError(t, enforceKeyFilestorePath(&types.User{ID: isoOwnerID, OrganizationID: isoOrgID}, "engagements/"+isoVerifyID))
}
