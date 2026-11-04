package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TST-01-194: out-of-enum priority and free-form type must be rejected, not persisted.
func TestCreateTaskFromPromptRejectsOutOfEnumTypeAndPriority(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "pentest PoC",
			body:    `{"project_id":"project1","name":"t","prompt":"x","priority":"asap","type":"<script>alert(1)</script>","auto_start":false}`,
			wantErr: "invalid type",
		},
		{
			name:    "unknown priority",
			body:    `{"project_id":"project1","name":"t","prompt":"x","priority":"asap","type":"bug","auto_start":false}`,
			wantErr: "invalid priority",
		},
		{
			name:    "script marker type",
			body:    `{"project_id":"project1","name":"t","prompt":"x","priority":"high","type":"<script>alert(1)</script>","auto_start":false}`,
			wantErr: "invalid type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			// Strict mock: any CreateSpecTask/write call fails the test.
			mockStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Store: mockStore}
			user := types.User{ID: "user1"}
			mockStore.EXPECT().GetProject(gomock.Any(), "project1").
				Return(&types.Project{ID: "project1", UserID: user.ID}, nil).AnyTimes()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewBufferString(tt.body))
			req = req.WithContext(setRequestUser(req.Context(), user))
			rec := httptest.NewRecorder()

			server.createTaskFromPrompt(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), tt.wantErr)
			require.NotContains(t, rec.Body.String(), "<script>")
		})
	}
}

func TestUpdateSpecTaskRejectsOutOfEnumPriority(t *testing.T) {
	ctrl := gomock.NewController(t)
	server := &HelixAPIServer{Store: store.NewMockStore(ctrl)}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/task1", bytes.NewBufferString(`{"priority":"asap"}`))
	req = mux.SetURLVars(req, map[string]string{"taskId": "task1"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user1"}))
	rec := httptest.NewRecorder()

	server.updateSpecTask(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid priority")
}
