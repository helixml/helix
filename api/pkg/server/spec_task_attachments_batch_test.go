package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type attachmentBatchFixture struct {
	server *HelixAPIServer
	store  *store.MockStore
	root   string
	user   types.User
	task   *types.SpecTask

	mu   sync.Mutex
	rows map[string]*types.SpecTaskAttachment
}

// newAttachmentBatchFixture wires the upload handler to a real on-disk filestore
// and a mock store that keeps attachment rows in memory, so tests can assert on
// what actually remains after a request.
func newAttachmentBatchFixture(t *testing.T) *attachmentBatchFixture {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	root := t.TempDir()
	cfg := &config.ServerConfig{}
	f := &attachmentBatchFixture{
		store: mockStore,
		root:  root,
		user:  types.User{ID: "user-1"},
		task:  &types.SpecTask{ID: "spt_batch", ProjectID: "project-1", Status: types.TaskStatusBacklog},
		rows:  map[string]*types.SpecTaskAttachment{},
	}
	f.server = &HelixAPIServer{
		Cfg:   cfg,
		Store: mockStore,
		Controller: &controller.Controller{
			Ctx: context.Background(),
			Options: controller.Options{
				Config:    cfg,
				Store:     mockStore,
				Filestore: filestore.NewFileSystemStorage(root, "", ""),
			},
		},
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), f.task.ID).Return(f.task, nil).AnyTimes()
	mockStore.EXPECT().GetProject(gomock.Any(), f.task.ProjectID).Return(&types.Project{
		ID: f.task.ProjectID, UserID: f.user.ID,
	}, nil).AnyTimes()
	mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), f.task.ID).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSpecTaskAttachment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			delete(f.rows, id)
			return nil
		},
	).AnyTimes()
	return f
}

func (f *attachmentBatchFixture) upload(t *testing.T, files map[string][]byte, order []string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, name := range order {
		part, err := w.CreateFormFile("files", name)
		require.NoError(t, err)
		_, err = part.Write(files[name])
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/"+f.task.ID+"/attachments", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req = req.WithContext(setRequestUser(req.Context(), f.user))
	req = mux.SetURLVars(req, map[string]string{"taskId": f.task.ID})
	response := httptest.NewRecorder()
	f.server.uploadSpecTaskAttachments(response, req)
	return response
}

func (f *attachmentBatchFixture) blobs(t *testing.T) []string {
	var found []string
	require.NoError(t, filepath.WalkDir(f.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = append(found, path)
		}
		return nil
	}))
	return found
}

func TestUploadSpecTaskAttachmentsRejectsWholeBatchAndListsEveryBadFile(t *testing.T) {
	f := newAttachmentBatchFixture(t)
	// No CreateSpecTaskAttachment expectation: gomock fails the test if any row is written.
	files := map[string][]byte{
		"ROOT-CAUSE.md":    []byte("# root cause\n"),
		"leaker.bin":       append([]byte("\x7fELF\x02\x01\x01"), make([]byte, 64)...),
		"digest.log":       []byte("plain log line\n"),
		"diagram.svg":      []byte(`<svg><script>alert(1)</script></svg>`),
		"after-the-bad.md": []byte("# never written\n"),
	}
	response := f.upload(t, files, []string{"ROOT-CAUSE.md", "leaker.bin", "digest.log", "diagram.svg", "after-the-bad.md"})

	require.Equal(t, http.StatusBadRequest, response.Code)
	body := response.Body.String()
	require.Contains(t, body, "2 of 5 attachment(s) rejected; nothing was saved")
	require.Contains(t, body, "unsupported mime type for leaker.bin")
	require.Contains(t, body, "diagram.svg contains a <script> tag")
	require.NotContains(t, body, "ROOT-CAUSE.md")
	require.Empty(t, f.rows)
	require.Empty(t, f.blobs(t))
}

func TestUploadSpecTaskAttachmentsRollsBackWhenAWriteFailsMidBatch(t *testing.T) {
	f := newAttachmentBatchFixture(t)
	calls := 0
	f.store.EXPECT().CreateSpecTaskAttachment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, row *types.SpecTaskAttachment) error {
			calls++
			if calls == 3 {
				return errors.New("database unavailable")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.rows[row.ID] = row
			return nil
		},
	).Times(3)
	files := map[string][]byte{
		"one.md":   []byte("one"),
		"two.md":   []byte("two"),
		"three.md": []byte("three"),
		"four.md":  []byte("four"),
	}
	response := f.upload(t, files, []string{"one.md", "two.md", "three.md", "four.md"})

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, response.Body.String(), "failed to save three.md; the batch was rolled back and nothing was saved")
	require.Empty(t, f.rows)
	require.Empty(t, f.blobs(t))
}

func TestUploadSpecTaskAttachmentsReportsRollbackFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockFilestore := filestore.NewMockFileStore(ctrl)
	cfg := &config.ServerConfig{}
	server := &HelixAPIServer{
		Store: mockStore,
		Controller: &controller.Controller{
			Ctx:     context.Background(),
			Options: controller.Options{Config: cfg, Store: mockStore, Filestore: mockFilestore},
		},
	}
	mockStore.EXPECT().DeleteSpecTaskAttachment(gomock.Any(), "att-1").Return(nil)
	mockFilestore.EXPECT().Delete(gomock.Any(), "/a").Return(nil)
	mockStore.EXPECT().DeleteSpecTaskAttachment(gomock.Any(), "att-2").Return(errors.New("database unavailable"))
	mockFilestore.EXPECT().Delete(gomock.Any(), "/b").Return(nil)

	leaked := server.rollbackSpecTaskAttachments(context.Background(), []*types.SpecTaskAttachment{
		{ID: "att-1", Filename: "a.md", FilestorePath: "/a"},
		{ID: "att-2", Filename: "b.md", FilestorePath: "/b"},
	})
	require.Equal(t, []string{"b.md"}, leaked)
}

func TestValidateInlineSpecTaskAttachmentsListsEveryBadFile(t *testing.T) {
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	err := validateInlineSpecTaskAttachments([]types.SpecTaskInlineAttachment{
		{Name: "good.md", ContentBase64: encode("# ok")},
		{Name: "bad.svg", ContentBase64: encode("<svg><script/></svg>")},
		{Name: "broken.md", ContentBase64: "%%%"},
		{Name: "binary", ContentBase64: encode("\x7fELF\x02\x01\x01" + strings.Repeat("\x00", 64))},
	})
	var inputErr *specTaskAttachmentInputError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, http.StatusBadRequest, inputErr.status)
	require.Contains(t, inputErr.message, "3 of 4 attachment(s) rejected")
	require.Contains(t, inputErr.message, "bad.svg contains a <script> tag")
	require.Contains(t, inputErr.message, "invalid base64 content for broken.md")
	require.Contains(t, inputErr.message, "unsupported mime type for binary")
	require.NotContains(t, inputErr.message, "good.md")
}

func TestJoinSpecTaskAttachmentRejectionsStatus(t *testing.T) {
	tooLarge := &specTaskAttachmentInputError{status: http.StatusRequestEntityTooLarge, message: "big"}
	badType := &specTaskAttachmentInputError{status: http.StatusBadRequest, message: "bad"}
	require.NoError(t, joinSpecTaskAttachmentRejections(1, nil))

	var inputErr *specTaskAttachmentInputError
	require.ErrorAs(t, joinSpecTaskAttachmentRejections(2, []*specTaskAttachmentInputError{tooLarge, tooLarge}), &inputErr)
	require.Equal(t, http.StatusRequestEntityTooLarge, inputErr.status)
	require.ErrorAs(t, joinSpecTaskAttachmentRejections(2, []*specTaskAttachmentInputError{tooLarge, badType}), &inputErr)
	require.Equal(t, http.StatusBadRequest, inputErr.status)
}

func TestDetectAttachmentMimeAcceptsCompressedArchives(t *testing.T) {
	gzipBytes := []byte{0x1f, 0x8b, 0x08, 0x00, 0, 0, 0, 0, 0, 0x03}
	zipBytes := append([]byte("PK\x03\x04"), make([]byte, 26)...)
	for name, tc := range map[string]struct {
		filename string
		body     []byte
		want     string
	}{
		"gz":                  {"leaker-full.log.gz", gzipBytes, "application/gzip"},
		"tgz":                 {"logs.tgz", gzipBytes, "application/gzip"},
		"zip":                 {"bundle.zip", zipBytes, "application/zip"},
		"renamed text is not": {"fake.gz", []byte("<html><script>x</script></html>"), "text/html"},
	} {
		t.Run(name, func(t *testing.T) {
			got := detectAttachmentMime(tc.filename, tc.body)
			require.Equal(t, tc.want, got)
			_, err := prepareSpecTaskAttachment(tc.filename, tc.body, "")
			if strings.HasPrefix(tc.want, "application/") {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
