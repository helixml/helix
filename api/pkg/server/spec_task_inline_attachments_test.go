package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateTaskFromPromptPersistsInlineAttachmentsBeforeQueuedTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockFilestore := filestore.NewMockFileStore(ctrl)
	ctx := context.Background()
	user := types.User{ID: "user-1", Email: "user@example.com"}
	project := &types.Project{
		ID:     "project-1",
		UserID: user.ID,
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeCodexCLI,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "provider-1",
			Model:          "model-1",
		},
	}

	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(42, nil)
	mockFilestore.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).Return(filestore.Item{Directory: true}, nil)

	var storedBody []byte
	mockFilestore.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string, reader io.Reader) (filestore.Item, error) {
			var err error
			storedBody, err = io.ReadAll(reader)
			require.NoError(t, err)
			return filestore.Item{Path: path, Name: "brief.md"}, nil
		})

	attachmentCreated := false
	var attachmentRow *types.SpecTaskAttachment
	mockStore.EXPECT().CreateSpecTaskAttachment(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, attachment *types.SpecTaskAttachment) error {
			attachmentCreated = true
			attachmentRow = attachment
			return nil
		})
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
			require.True(t, attachmentCreated, "attachment must exist before queued task is published")
			require.Equal(t, attachmentRow.SpecTaskID, task.ID)
			require.Equal(t, types.TaskStatusQueuedImplementation, task.Status)
			return nil
		})

	service := services.NewSpecDrivenTaskService(
		mockStore, nil, "test-agent", nil, nil, nil, nil, nil, services.NewDisabledKoditService(),
	)
	service.SetTestMode(true)
	cfg := &config.ServerConfig{}
	server := &HelixAPIServer{
		Cfg:   cfg,
		Store: mockStore,
		Controller: &controller.Controller{
			Ctx: ctx,
			Options: controller.Options{
				Config:    cfg,
				Store:     mockStore,
				Filestore: mockFilestore,
			},
		},
		specDrivenTaskService: service,
	}

	requestBody, err := json.Marshal(types.CreateTaskRequest{
		ProjectID:      project.ID,
		Prompt:         "Use the attached engagement brief",
		JustDoItMode:   true,
		AutoStart:      true,
		SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu,
		Attachments: []types.SpecTaskInlineAttachment{{
			Name:          "brief.md",
			ContentBase64: base64.StdEncoding.EncodeToString([]byte("# Rules of engagement\n")),
			Caption:       "engagement scope",
		}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewReader(requestBody))
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()

	server.createTaskFromPrompt(response, req)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Equal(t, []byte("# Rules of engagement\n"), storedBody)
	require.NotNil(t, attachmentRow)
	require.Equal(t, "brief.md", attachmentRow.Filename)
	require.Equal(t, "text/markdown", attachmentRow.MimeType)
	require.Equal(t, "engagement scope", attachmentRow.Caption)
	require.Equal(t, user.ID, attachmentRow.UserID)
	require.Equal(t, project.ID, attachmentRow.ProjectID)
	require.True(t, strings.HasPrefix(attachmentRow.SpecTaskID, "spt_"))
	require.Contains(t, attachmentRow.FilestorePath, attachmentRow.SpecTaskID)

	var created types.SpecTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	require.Equal(t, attachmentRow.SpecTaskID, created.ID)
	require.Equal(t, types.TaskStatusQueuedImplementation, created.Status)
}

func TestPrepareInlineSpecTaskAttachmentsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		inputs     []types.SpecTaskInlineAttachment
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid base64",
			inputs:     []types.SpecTaskInlineAttachment{{Name: "brief.md", ContentBase64: "%%%"}},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid base64 content for brief.md",
		},
		{
			name: "duplicate filename",
			inputs: []types.SpecTaskInlineAttachment{
				{Name: "brief.md", ContentBase64: base64.StdEncoding.EncodeToString([]byte("one"))},
				{Name: "folder/brief.md", ContentBase64: base64.StdEncoding.EncodeToString([]byte("two"))},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "duplicate attachment filename: brief.md",
		},
		{
			name:       "scripted svg",
			inputs:     []types.SpecTaskInlineAttachment{{Name: "bad.svg", ContentBase64: base64.StdEncoding.EncodeToString([]byte("<svg><script/></svg>"))}},
			wantStatus: http.StatusBadRequest,
			wantError:  "bad.svg contains a <script> tag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := prepareInlineSpecTaskAttachments(test.inputs)
			var inputErr *specTaskAttachmentInputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, test.wantStatus, inputErr.status)
			require.Contains(t, inputErr.message, test.wantError)
		})
	}
}
