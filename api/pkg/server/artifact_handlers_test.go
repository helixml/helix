package server

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestReadArtifactZIP(t *testing.T) {
	archive := artifactTestZIP(t, map[string]string{
		"index.html":    "<h1>Hello</h1>",
		"assets/app.js": "document.body.dataset.ready = 'yes'",
	})
	files, err := readArtifactZIP(archive)
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "assets/app.js", files[0].Metadata.Path)
	require.Equal(t, "text/html; charset=utf-8", files[1].Metadata.ContentType)
}

func TestReadArtifactZIPRejectsTraversal(t *testing.T) {
	archive := artifactTestZIP(t, map[string]string{"../index.html": "nope"})
	_, err := readArtifactZIP(archive)
	require.ErrorContains(t, err, "invalid artifact path")
}

func TestResolveArtifactFile(t *testing.T) {
	artifact := &types.Artifact{
		ID: "art_test", Kind: types.ArtifactKindSPA, Entrypoint: "index.html",
		ActiveVersion: &types.ArtifactVersion{ID: "artv_test", Files: []types.ArtifactFile{
			{Path: "index.html", SHA256: "one"},
			{Path: "assets/app.js", SHA256: "two"},
		}},
	}

	filename, _, ok := resolveArtifactFile(artifact, "")
	require.True(t, ok)
	require.Equal(t, "index.html", filename)
	filename, _, ok = resolveArtifactFile(artifact, "assets/app.js")
	require.True(t, ok)
	require.Equal(t, "assets/app.js", filename)
	filename, _, ok = resolveArtifactFile(artifact, "settings/profile")
	require.True(t, ok)
	require.Equal(t, "index.html", filename)
	filename, _, ok = resolveArtifactFile(artifact, "settings/")
	require.True(t, ok)
	require.Equal(t, "index.html", filename)
	_, _, ok = resolveArtifactFile(artifact, "../secret")
	require.False(t, ok)
}

func TestArtifactSecurityHeaders(t *testing.T) {
	header := http.Header{}
	setArtifactSecurityHeaders(header, "https://helix.example/path")
	require.Contains(t, header.Get("Content-Security-Policy"), "connect-src 'self' https: wss:")
	require.Contains(t, header.Get("Content-Security-Policy"), "frame-ancestors https://helix.example")
	require.Equal(t, "no-referrer", header.Get("Referrer-Policy"))
	require.Contains(t, header.Get("Permissions-Policy"), "camera=()")

	invalid := http.Header{}
	setArtifactSecurityHeaders(invalid, "javascript:alert(1)")
	require.Contains(t, invalid.Get("Content-Security-Policy"), "frame-ancestors 'none'")

	document := http.Header{}
	setArtifactDocumentSecurityHeaders(document, "https://helix.example/path")
	require.Equal(t, "default-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors https://helix.example", document.Get("Content-Security-Policy"))
	require.NotContains(t, document.Get("Content-Security-Policy"), "unsafe-inline")
}

func TestArtifactRequestOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://api:8080/artifacts/art_123", nil)
	require.Equal(t, "https://helix.example", artifactRequestOrigin(r, "https://helix.example/"))
	require.Equal(t, "http://api:8080", artifactRequestOrigin(r, ""))

	r.Header.Set("X-Forwarded-Proto", "https")
	require.Equal(t, "https://api:8080", artifactRequestOrigin(r, ""))
}

func TestDefaultAndValidateArtifactEntrypoint(t *testing.T) {
	files := []types.ArtifactFile{{Path: "replacement.html", ContentType: "text/html; charset=utf-8"}}

	entrypoint, err := defaultAndValidateArtifactEntrypoint(files, "index.html", true)
	require.NoError(t, err)
	require.Equal(t, "replacement.html", entrypoint)

	_, err = defaultAndValidateArtifactEntrypoint(files, "index.html", false)
	require.ErrorContains(t, err, "entrypoint does not exist")
}

func TestValidateArtifactEntrypointSupportsDocuments(t *testing.T) {
	tests := []struct {
		name         string
		file         types.ArtifactFile
		expectedKind types.ArtifactKind
	}{
		{name: "PDF", file: types.ArtifactFile{Path: "report.pdf", ContentType: "application/pdf"}, expectedKind: types.ArtifactKindPDF},
		{name: "image", file: types.ArtifactFile{Path: "diagram.png", ContentType: "image/png"}, expectedKind: types.ArtifactKindImage},
		{name: "HTML", file: types.ArtifactFile{Path: "page.html", ContentType: "text/html; charset=utf-8"}, expectedKind: types.ArtifactKindSingleFile},
		{name: "markdown extension", file: types.ArtifactFile{Path: "notes.md", ContentType: "text/plain; charset=utf-8"}, expectedKind: types.ArtifactKindMarkdown},
		{name: "markdown content type", file: types.ArtifactFile{Path: "notes.txt", ContentType: "text/markdown; charset=utf-8"}, expectedKind: types.ArtifactKindMarkdown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entrypoint, err := validateArtifactEntrypoint([]types.ArtifactFile{test.file}, test.file.Path)
			require.NoError(t, err)
			require.Equal(t, test.file.Path, entrypoint)
			require.Equal(t, test.expectedKind, artifactKindFromFiles([]types.ArtifactFile{test.file}))
		})
	}
}

func TestValidateArtifactEntrypointRejectsUnsupportedSingleFile(t *testing.T) {
	_, err := validateArtifactEntrypoint([]types.ArtifactFile{{Path: "script.js", ContentType: "text/javascript"}}, "script.js")
	require.ErrorContains(t, err, "must be HTML, Markdown, PDF, or an image")
}

func TestValidateArtifactEntrypointRequiresHTMLForBundle(t *testing.T) {
	files := []types.ArtifactFile{
		{Path: "report.pdf", ContentType: "application/pdf"},
		{Path: "cover.png", ContentType: "image/png"},
	}
	_, err := validateArtifactEntrypoint(files, "report.pdf")
	require.ErrorContains(t, err, "bundle entrypoint must be an HTML file")
}

func TestArtifactAccessRedirectPath(t *testing.T) {
	redirectPath, err := artifactAccessRedirectPath("settings/", "tab=billing")
	require.NoError(t, err)
	require.Equal(t, "/settings/?tab=billing", redirectPath)
	redirectPath, err = artifactAccessRedirectPath("", "preview=true")
	require.NoError(t, err)
	require.Equal(t, "/?preview=true", redirectPath)
	_, err = artifactAccessRedirectPath("../secrets", "")
	require.Error(t, err)
}

func TestActivePrivateArtifactRouteUser(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	userID, ok := activePrivateArtifactRouteUser(&types.VHostRoute{
		TargetKind:   types.VHostTargetArtifactPrivate,
		AccessUserID: "usr_test",
		ExpiresAt:    &future,
	}, now)
	require.True(t, ok)
	require.Equal(t, "usr_test", userID)

	past := now.Add(-time.Second)
	_, ok = activePrivateArtifactRouteUser(&types.VHostRoute{
		TargetKind:   types.VHostTargetArtifactPrivate,
		AccessUserID: "usr_test",
		ExpiresAt:    &past,
	}, now)
	require.False(t, ok)
}

func artifactTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for filename, body := range files {
		part, err := writer.Create(filename)
		require.NoError(t, err)
		_, err = part.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func TestArtifactDownloadFilename(t *testing.T) {
	require.Equal(t, "Q3 report.pdf", artifactDownloadFilename(&types.Artifact{Name: "Q3 report", Entrypoint: "document.pdf"}, ".pdf"))
	require.Equal(t, "notes.md", artifactDownloadFilename(&types.Artifact{Name: "notes.md", Entrypoint: "notes.md"}, ".md"))
	require.Equal(t, "dashboard.zip", artifactDownloadFilename(&types.Artifact{Name: "dashboard", Entrypoint: "index.html"}, ".zip"))
	require.Equal(t, "etcpasswd.html", artifactDownloadFilename(&types.Artifact{Name: "../../etc/passwd", Entrypoint: "index.html"}, ".html"))
	require.Equal(t, "index.html", artifactDownloadFilename(&types.Artifact{Name: "  ", Entrypoint: "index.html"}, ".html"))
	require.Equal(t, "artifact.zip", artifactDownloadFilename(&types.Artifact{Name: "...", Entrypoint: "index.html"}, ".zip"))
}

func TestServeArtifactDownloadSingleFile(t *testing.T) {
	mockStore, mockFilestore, server := newArtifactDownloadTestServer(t)
	body := "# Engagement notes\n"
	artifact := artifactDownloadTestArtifact("Engagement notes", "notes.md", []types.ArtifactFile{{
		Path: "notes.md", Size: int64(len(body)), ContentType: "text/markdown; charset=utf-8", SHA256: "notes-sha",
	}})
	mockStore.EXPECT().GetArtifact(gomock.Any(), artifact.ID).Return(artifact, nil)
	mockFilestore.EXPECT().OpenFile(gomock.Any(), "artifacts/artv_test/notes.md").Return(io.NopCloser(strings.NewReader(body)), nil)

	response := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/artifacts/art_test/download", nil), map[string]string{"artifact_id": artifact.ID})
	server.serveArtifactDownload(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "attachment; filename=\"Engagement notes.md\"", response.Header().Get("Content-Disposition"))
	require.Equal(t, "text/markdown; charset=utf-8", response.Header().Get("Content-Type"))
	require.Equal(t, body, response.Body.String())
}

func TestServeArtifactDownloadBundle(t *testing.T) {
	mockStore, mockFilestore, server := newArtifactDownloadTestServer(t)
	files := []types.ArtifactFile{
		{Path: "index.html", Size: 14, ContentType: "text/html; charset=utf-8", SHA256: "index-sha"},
		{Path: "assets/app.js", Size: 15, ContentType: "text/javascript; charset=utf-8", SHA256: "app-sha"},
	}
	artifact := artifactDownloadTestArtifact("Assessment dashboard", "index.html", files)
	mockStore.EXPECT().GetArtifact(gomock.Any(), artifact.ID).Return(artifact, nil)
	mockFilestore.EXPECT().OpenFile(gomock.Any(), "artifacts/artv_test/index.html").Return(io.NopCloser(strings.NewReader("<h1>Hello</h1>")), nil)
	mockFilestore.EXPECT().OpenFile(gomock.Any(), "artifacts/artv_test/assets/app.js").Return(io.NopCloser(strings.NewReader("console.log('x')")), nil)

	response := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/artifacts/art_test/download", nil), map[string]string{"artifact_id": artifact.ID})
	server.serveArtifactDownload(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/zip", response.Header().Get("Content-Type"))
	require.Equal(t, "attachment; filename=\"Assessment dashboard.zip\"", response.Header().Get("Content-Disposition"))

	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	require.NoError(t, err)
	require.Len(t, archive.File, 2)
	require.Equal(t, "index.html", archive.File[0].Name)
	require.Equal(t, "assets/app.js", archive.File[1].Name)
	entry, err := archive.File[1].Open()
	require.NoError(t, err)
	defer entry.Close()
	content, err := io.ReadAll(entry)
	require.NoError(t, err)
	require.Equal(t, "console.log('x')", string(content))
}

func TestServeArtifactDownloadRejectsAnonymousPrivateAccess(t *testing.T) {
	mockStore, _, server := newArtifactDownloadTestServer(t)
	artifact := artifactDownloadTestArtifact("Private report", "report.pdf", []types.ArtifactFile{{Path: "report.pdf"}})
	artifact.Visibility = types.ArtifactVisibilityProject
	mockStore.EXPECT().GetArtifact(gomock.Any(), artifact.ID).Return(artifact, nil)

	response := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/artifacts/art_test/download", nil), map[string]string{"artifact_id": artifact.ID})
	server.serveArtifactDownload(response, request)

	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func newArtifactDownloadTestServer(t *testing.T) (*store.MockStore, *filestore.MockFileStore, *HelixAPIServer) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockFilestore := filestore.NewMockFileStore(ctrl)
	server := &HelixAPIServer{
		Cfg:   &config.ServerConfig{WebServer: config.WebServer{URL: "https://helix.example"}},
		Store: mockStore,
		Controller: &controller.Controller{Options: controller.Options{
			Filestore: mockFilestore,
		}},
	}
	return mockStore, mockFilestore, server
}

func artifactDownloadTestArtifact(name, entrypoint string, files []types.ArtifactFile) *types.Artifact {
	return &types.Artifact{
		ID: "art_test", ProjectID: "prj_test", Name: name, Entrypoint: entrypoint,
		Visibility: types.ArtifactVisibilityPublic,
		ActiveVersion: &types.ArtifactVersion{
			ID: "artv_test", StoragePrefix: "artifacts/artv_test", Files: files, CreatedAt: time.Unix(1, 0),
		},
	}
}
