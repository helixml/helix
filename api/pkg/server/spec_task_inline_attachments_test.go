package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSpecTaskInlineAttachmentOpenAPIRequiresPayloadFields(t *testing.T) {
	document, err := os.ReadFile("swagger.json")
	require.NoError(t, err)
	var schema struct {
		Definitions map[string]struct {
			Required []string `json:"required"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(document, &schema))
	require.ElementsMatch(t, []string{"name", "content_base64"}, schema.Definitions["types.SpecTaskInlineAttachment"].Required)
}

func requireInlineAttachmentIngestionLease(t *testing.T, operationCtx context.Context) {
	t.Helper()
	deadline, ok := operationCtx.Deadline()
	require.True(t, ok)
	require.Greater(t, time.Until(deadline), 9*time.Minute)
}

func TestCreateTaskFromPromptCleansUpFailedInlineAttachmentIngestion(t *testing.T) {
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
	var taskID string
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		taskID = task.ID
		require.Equal(t, types.TaskStatusPreparing, task.Status)
		return nil
	})
	mockFilestore.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).DoAndReturn(
		func(operationCtx context.Context, _ string) (filestore.Item, error) {
			requireInlineAttachmentIngestionLease(t, operationCtx)
			return filestore.Item{Directory: true}, nil
		},
	)
	mockFilestore.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(filestore.Item{Path: "/attachments/brief.md"}, nil)
	mockStore.EXPECT().CreateSpecTaskAttachment(gomock.Any(), gomock.Any()).Return(errors.New("database unavailable"))
	// The first delete removes the just-written blob; the second removes the
	// task-owned attachment prefix so an interrupted partial batch cannot linger.
	mockFilestore.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockStore.EXPECT().DeleteSpecTaskAttachmentsByTaskID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, gotTaskID string) error {
		require.Equal(t, taskID, gotTaskID)
		return nil
	})
	mockStore.EXPECT().DeleteSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, gotTaskID string) error {
		require.Equal(t, taskID, gotTaskID)
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
		}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewReader(requestBody))
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()

	server.createTaskFromPrompt(response, req)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotEmpty(t, taskID)
}

func TestCreateTaskFromPromptPublishesOnlyAfterInlineAttachmentsPersist(t *testing.T) {
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
		DoAndReturn(func(operationCtx context.Context, attachment *types.SpecTaskAttachment) error {
			requireInlineAttachmentIngestionLease(t, operationCtx)
			attachmentCreated = true
			attachmentRow = attachment
			return nil
		})
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
			require.False(t, attachmentCreated)
			require.Equal(t, types.TaskStatusPreparing, task.Status)
			return nil
		})
	mockStore.EXPECT().TransitionSpecTaskStatus(
		gomock.Any(),
		gomock.Any(),
		[]types.SpecTaskStatus{types.TaskStatusPreparing},
		types.TaskStatusQueuedImplementation,
		nil,
	).DoAndReturn(func(_ context.Context, taskID string, _ []types.SpecTaskStatus, _ types.SpecTaskStatus, _ map[string]any) (bool, error) {
		require.True(t, attachmentCreated, "attachment must exist before queued task is published")
		require.Equal(t, attachmentRow.SpecTaskID, taskID)
		return true, nil
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

func TestCreateTaskFromPromptCleansUpWhenAttachmentPublicationFails(t *testing.T) {
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
	var taskID string
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		taskID = task.ID
		return nil
	})
	mockFilestore.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).Return(filestore.Item{Directory: true}, nil)
	mockFilestore.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(filestore.Item{Path: "/attachments/brief.md"}, nil)
	mockStore.EXPECT().CreateSpecTaskAttachment(gomock.Any(), gomock.Any()).Return(nil)
	mockStore.EXPECT().TransitionSpecTaskStatus(
		gomock.Any(),
		gomock.Any(),
		[]types.SpecTaskStatus{types.TaskStatusPreparing},
		types.TaskStatusQueuedImplementation,
		nil,
	).Return(false, errors.New("database unavailable"))
	mockStore.EXPECT().DeleteSpecTaskAttachmentsByTaskID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, gotTaskID string) error {
		require.Equal(t, taskID, gotTaskID)
		return nil
	})
	mockFilestore.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)
	mockStore.EXPECT().DeleteSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, gotTaskID string) error {
		require.Equal(t, taskID, gotTaskID)
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
		}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewReader(requestBody))
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()

	server.createTaskFromPrompt(response, req)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotEmpty(t, taskID)
}

func TestPrepareInlineSpecTaskAttachmentsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		inputs     []types.SpecTaskInlineAttachment
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing name",
			inputs:     []types.SpecTaskInlineAttachment{{ContentBase64: base64.StdEncoding.EncodeToString([]byte("brief"))}},
			wantStatus: http.StatusBadRequest,
			wantError:  "attachment name is required",
		},
		{
			name:       "missing content",
			inputs:     []types.SpecTaskInlineAttachment{{Name: "brief.md"}},
			wantStatus: http.StatusBadRequest,
			wantError:  "content_base64 is required for brief.md",
		},
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
		{
			name:       "caption with NUL",
			inputs:     []types.SpecTaskInlineAttachment{{Name: "brief.md", ContentBase64: base64.StdEncoding.EncodeToString([]byte("brief")), Caption: "bad\x00caption"}},
			wantStatus: http.StatusBadRequest,
			wantError:  "caption for brief.md contains a NUL byte",
		},
		{
			name:       "caption too long",
			inputs:     []types.SpecTaskInlineAttachment{{Name: "brief.md", ContentBase64: base64.StdEncoding.EncodeToString([]byte("brief")), Caption: strings.Repeat("x", types.SpecTaskAttachmentCaptionMaxRunes+1)}},
			wantStatus: http.StatusBadRequest,
			wantError:  "caption for brief.md exceeds 1024 characters",
		},
		{
			name: "filename too long",
			inputs: []types.SpecTaskInlineAttachment{{
				Name:          strings.Repeat("a", types.SpecTaskAttachmentFilenameMaxBytes-2) + ".md",
				ContentBase64: base64.StdEncoding.EncodeToString([]byte("brief")),
			}},
			wantStatus: http.StatusBadRequest,
			wantError:  "exceeds 255 bytes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateInlineSpecTaskAttachments(test.inputs)
			var inputErr *specTaskAttachmentInputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, test.wantStatus, inputErr.status)
			require.Contains(t, inputErr.message, test.wantError)
		})
	}
}

func TestPrepareInlineSpecTaskAttachmentsEnforcesAllSizeLimits(t *testing.T) {
	encode := func(value string) string {
		return base64.StdEncoding.EncodeToString([]byte(value))
	}
	tests := []struct {
		name          string
		inputs        []types.SpecTaskInlineAttachment
		maxCount      int
		maxFileBytes  int
		maxTotalBytes int
		wantStatus    int
		wantError     string
	}{
		{
			name: "too many attachments",
			inputs: []types.SpecTaskInlineAttachment{
				{Name: "one.md", ContentBase64: encode("1")},
				{Name: "two.md", ContentBase64: encode("2")},
				{Name: "three.md", ContentBase64: encode("3")},
			},
			maxCount: 2, maxFileBytes: 10, maxTotalBytes: 10,
			wantStatus: http.StatusBadRequest, wantError: "too many attachments — limit is 2 per task",
		},
		{
			name:          "file too large",
			inputs:        []types.SpecTaskInlineAttachment{{Name: "large.md", ContentBase64: encode("12345")}},
			maxCount:      2,
			maxFileBytes:  4,
			maxTotalBytes: 10,
			wantStatus:    http.StatusRequestEntityTooLarge,
			wantError:     "large.md exceeds max size",
		},
		{
			name: "total too large",
			inputs: []types.SpecTaskInlineAttachment{
				{Name: "one.md", ContentBase64: encode("123")},
				{Name: "two.md", ContentBase64: encode("456")},
			},
			maxCount: 2, maxFileBytes: 4, maxTotalBytes: 5,
			wantStatus: http.StatusRequestEntityTooLarge, wantError: "inline attachments exceed total size limit of 5 bytes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateInlineSpecTaskAttachmentsWithLimits(
				test.inputs,
				test.maxCount,
				test.maxFileBytes,
				test.maxTotalBytes,
			)
			var inputErr *specTaskAttachmentInputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, test.wantStatus, inputErr.status)
			require.Contains(t, inputErr.message, test.wantError)
		})
	}
}
