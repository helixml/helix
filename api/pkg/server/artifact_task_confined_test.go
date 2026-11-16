package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func artifactProvenanceServer(t *testing.T) (*HelixAPIServer, *store.MockStore, *filestore.MockFileStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	fs := filestore.NewMockFileStore(ctrl)
	srv := &HelixAPIServer{
		Cfg:   &config.ServerConfig{WebServer: config.WebServer{URL: "https://helix.example", DevSubdomain: "dev"}},
		Store: st,
		Controller: &controller.Controller{Options: controller.Options{
			Store: st, Filestore: fs,
		}},
	}
	return srv, st, fs
}

func artifactUploadRequest(t *testing.T, user *types.User, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	part, err := w.CreateFormFile("artifact", "index.html")
	require.NoError(t, err)
	_, err = part.Write([]byte("<html><body>finding 7</body></html>"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+confinedProjectID+"/artifacts", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req = req.WithContext(setRequestUser(req.Context(), *user))
	return mux.SetURLVars(req, map[string]string{"id": confinedProjectID})
}

// A task-confined key's artifact records its task, server-side: the
// request's source_spec_task_id naming the verifier is ignored.
func TestCreateProjectArtifact_RecordsConfinedKeyTask(t *testing.T) {
	for _, tc := range []struct {
		name     string
		user     *types.User
		wantTask string
	}{
		{"confined key records its task", finderKeyUser(), finderTaskID},
		{"unconfined key records none", unconfinedFinderKeyUser(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, st, fs := artifactProvenanceServer(t)
			st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil).AnyTimes()
			// The form names the verifier's task; it is the same project, so the
			// form check passes, and the server still records the key's task.
			st.EXPECT().GetSpecTask(gomock.Any(), verifierTaskID).Return(&types.SpecTask{ID: verifierTaskID, ProjectID: confinedProjectID}, nil)
			fs.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(filestore.Item{}, nil)
			var created *types.Artifact
			var version *types.ArtifactVersion
			st.EXPECT().CreateArtifact(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, a *types.Artifact, v *types.ArtifactVersion) error {
					created, version = a, v
					return nil
				})
			st.EXPECT().ListVHostRoutesByTarget(gomock.Any(), types.VHostTargetArtifact, gomock.Any()).
				Return([]*types.VHostRoute{{Hostname: "x.dev.helix.example"}}, nil).Times(2)

			rec := httptest.NewRecorder()
			srv.createProjectArtifact(rec, artifactUploadRequest(t, tc.user, map[string]string{
				"name": "report", "source_spec_task_id": verifierTaskID,
			}))
			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantTask, created.SpecTaskID)
			if tc.wantTask != "" {
				assert.Contains(t, rec.Body.String(), `"spec_task_id":"`+tc.wantTask+`"`, "the response exposes the provenance")
			} else {
				assert.NotContains(t, rec.Body.String(), `"spec_task_id"`)
			}
			if tc.user.TaskConfined {
				assert.Equal(t, finderTaskID, version.SourceSpecTaskID)
				assert.Equal(t, finderSessionID, version.SourceSessionID)
			}
		})
	}
}

// A confined key's version names its own task even when the form names
// another lane's.
func TestArtifactVersionFromUpload_ConfinedKeyCannotClaimAnotherTask(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Form = map[string][]string{"source_spec_task_id": {verifierTaskID}, "source_session_id": {verifierSessionID}}
	req = req.WithContext(setRequestUser(req.Context(), *finderKeyUser()))
	v := artifactVersionFromUpload(req, "art_1", "artv_1", "p", 1, confinedOwnerID, &artifactUpload{})
	assert.Equal(t, finderTaskID, v.SourceSpecTaskID)
	assert.Equal(t, finderSessionID, v.SourceSessionID)
}

// A confined key may change or delete only its own task's artifacts; it can
// still read the others. The refusal stops before any write (strict store).
func TestArtifact_ConfinedKeyChangesOnlyItsOwnTasks(t *testing.T) {
	verifierArtifact := &types.Artifact{ID: "art_verifier", ProjectID: confinedProjectID, SpecTaskID: verifierTaskID}
	peopleArtifact := &types.Artifact{ID: "art_people", ProjectID: confinedProjectID}
	for _, artifact := range []*types.Artifact{verifierArtifact, peopleArtifact} {
		for name, call := range map[string]func(*HelixAPIServer, http.ResponseWriter, *http.Request){
			"update": (*HelixAPIServer).updateArtifact,
			"delete": (*HelixAPIServer).deleteArtifact,
		} {
			t.Run(name+" "+artifact.ID, func(t *testing.T) {
				srv, st, _ := artifactProvenanceServer(t)
				st.EXPECT().GetArtifact(gomock.Any(), artifact.ID).Return(artifact, nil)
				st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
				rec := httptest.NewRecorder()
				call(srv, rec, confinedRequest(http.MethodPut, "/api/v1/artifacts/"+artifact.ID, "", map[string]string{"artifact_id": artifact.ID}))
				assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			})
		}
	}

	t.Run("read another task's artifact", func(t *testing.T) {
		srv, st, _ := artifactProvenanceServer(t)
		st.EXPECT().GetArtifact(gomock.Any(), verifierArtifact.ID).Return(verifierArtifact, nil)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		st.EXPECT().ListVHostRoutesByTarget(gomock.Any(), types.VHostTargetArtifact, verifierArtifact.ID).Return(nil, nil)
		rec := httptest.NewRecorder()
		srv.getArtifact(rec, confinedRequest(http.MethodGet, "/api/v1/artifacts/"+verifierArtifact.ID, "", map[string]string{"artifact_id": verifierArtifact.ID}))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"spec_task_id":"`+verifierTaskID+`"`)
	})

	t.Run("delete its own task's artifact", func(t *testing.T) {
		own := &types.Artifact{ID: "art_finder", ProjectID: confinedProjectID, SpecTaskID: finderTaskID}
		srv, st, fs := artifactProvenanceServer(t)
		st.EXPECT().GetArtifact(gomock.Any(), own.ID).Return(own, nil)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		st.EXPECT().DeleteVHostRoutesByTarget(gomock.Any(), gomock.Any(), own.ID).Return(nil).Times(2)
		fs.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)
		st.EXPECT().DeleteArtifact(gomock.Any(), own.ID).Return(nil)
		rec := httptest.NewRecorder()
		srv.deleteArtifact(rec, confinedRequest(http.MethodDelete, "/api/v1/artifacts/"+own.ID, "", map[string]string{"artifact_id": own.ID}))
		assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})

	t.Run("unconfined key keeps changing any project artifact", func(t *testing.T) {
		srv, st, fs := artifactProvenanceServer(t)
		st.EXPECT().GetArtifact(gomock.Any(), verifierArtifact.ID).Return(verifierArtifact, nil)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		st.EXPECT().DeleteVHostRoutesByTarget(gomock.Any(), gomock.Any(), verifierArtifact.ID).Return(nil).Times(2)
		fs.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)
		st.EXPECT().DeleteArtifact(gomock.Any(), verifierArtifact.ID).Return(nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+verifierArtifact.ID, http.NoBody)
		req = mux.SetURLVars(req.WithContext(setRequestUser(req.Context(), *unconfinedFinderKeyUser())), map[string]string{"artifact_id": verifierArtifact.ID})
		rec := httptest.NewRecorder()
		srv.deleteArtifact(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})
}

// ?spec_task_id= filters the project's artifact list in the store query.
func TestListProjectArtifacts_FiltersBySpecTask(t *testing.T) {
	srv, st, _ := artifactProvenanceServer(t)
	st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
	st.EXPECT().ListArtifacts(gomock.Any(), &store.ListArtifactsQuery{ProjectID: confinedProjectID, SpecTaskID: verifierTaskID}).Return(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+confinedProjectID+"/artifacts?spec_task_id="+verifierTaskID, http.NoBody)
	req = mux.SetURLVars(req.WithContext(setRequestUser(req.Context(), types.User{ID: confinedOwnerID})), map[string]string{"id": confinedProjectID})
	rec := httptest.NewRecorder()
	srv.listProjectArtifacts(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
