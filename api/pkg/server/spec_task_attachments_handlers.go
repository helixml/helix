package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Uploads remain available while an agent is working. StageUploadedAttachments
// commits late files and queues a note to the active session, so blocking them
// during implementation contradicts the delivery mechanism and makes
// just-do-it tasks impossible to correct. Once delivery has reached a PR the
// task input is terminal and uploads are locked again.
var specTaskAttachmentUploadReadOnlyStatuses = map[types.SpecTaskStatus]bool{
	types.TaskStatusPreparing:   true,
	types.TaskStatusPullRequest: true,
	types.TaskStatusDone:        true,
}

// Deletion remains stricter than upload because removing a committed attachment
// also requires removing it from helix-specs. A corrected file can still be
// uploaded and the running agent is notified.
var specTaskAttachmentDeleteReadOnlyStatuses = map[types.SpecTaskStatus]bool{
	types.TaskStatusPreparing:            true,
	types.TaskStatusSpecApproved:         true,
	types.TaskStatusImplementationQueued: true,
	types.TaskStatusImplementation:       true,
	types.TaskStatusImplementationReview: true,
	types.TaskStatusPullRequest:          true,
	types.TaskStatusDone:                 true,
	types.TaskStatusImplementationFailed: true,
}

func specTaskFromPromptMaxRequestBytes() int64 {
	const jsonOverheadBytes = 4 * 1024 * 1024
	return int64(base64.StdEncoding.EncodedLen(types.SpecTaskInlineAttachmentsMaxBytes) + jsonOverheadBytes)
}

func specTaskAttachmentUploadsLocked(status types.SpecTaskStatus) bool {
	return specTaskAttachmentUploadReadOnlyStatuses[status]
}

func specTaskAttachmentDeletesLocked(status types.SpecTaskStatus) bool {
	return specTaskAttachmentDeleteReadOnlyStatuses[status]
}

type preparedSpecTaskAttachment struct {
	filename string
	mimeType string
	caption  string
	body     []byte
}

type specTaskAttachmentInputError struct {
	status  int
	message string
}

func (e *specTaskAttachmentInputError) Error() string {
	return e.message
}

func asSpecTaskAttachmentInputError(err error) *specTaskAttachmentInputError {
	var inputErr *specTaskAttachmentInputError
	if errors.As(err, &inputErr) {
		return inputErr
	}
	return &specTaskAttachmentInputError{status: http.StatusBadRequest, message: err.Error()}
}

// joinSpecTaskAttachmentRejections reports every rejected file in a batch in one
// error, so the caller can fix them all in a single round-trip. Nothing in the
// batch has been written when this is returned. The body stays plain text because
// clients render it verbatim.
func joinSpecTaskAttachmentRejections(total int, rejected []*specTaskAttachmentInputError) error {
	if len(rejected) == 0 {
		return nil
	}
	status := http.StatusRequestEntityTooLarge
	lines := make([]string, 0, len(rejected))
	for _, r := range rejected {
		if r.status != http.StatusRequestEntityTooLarge {
			status = http.StatusBadRequest
		}
		lines = append(lines, "- "+r.message)
	}
	return &specTaskAttachmentInputError{
		status: status,
		message: fmt.Sprintf(
			"%d of %d attachment(s) rejected; nothing was saved:\n%s",
			len(rejected), total, strings.Join(lines, "\n"),
		),
	}
}

func prepareSpecTaskAttachment(name string, body []byte, caption string) (*preparedSpecTaskAttachment, error) {
	if int64(len(body)) > types.SpecTaskAttachmentMaxBytes {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusRequestEntityTooLarge,
			message: fmt.Sprintf("%s exceeds max size", name),
		}
	}
	filename := sanitiseAttachmentFilename(name)
	if filename == "" {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("invalid filename: %s", name),
		}
	}
	if len(filename) > types.SpecTaskAttachmentFilenameMaxBytes {
		return nil, &specTaskAttachmentInputError{
			status: http.StatusBadRequest,
			message: fmt.Sprintf(
				"filename %s exceeds %d bytes",
				filename,
				types.SpecTaskAttachmentFilenameMaxBytes,
			),
		}
	}
	mimeType := detectAttachmentMime(filename, body)
	if !types.SpecTaskAttachmentAllowedMimeTypes[mimeType] {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("unsupported mime type for %s: %s", name, mimeType),
		}
	}
	if mimeType == "image/svg+xml" && svgContainsScript(body) {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("%s contains a <script> tag — SVG with scripts is not allowed", name),
		}
	}
	if strings.ContainsRune(caption, '\x00') {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("caption for %s contains a NUL byte", name),
		}
	}
	if utf8.RuneCountInString(caption) > types.SpecTaskAttachmentCaptionMaxRunes {
		return nil, &specTaskAttachmentInputError{
			status: http.StatusBadRequest,
			message: fmt.Sprintf(
				"caption for %s exceeds %d characters",
				name,
				types.SpecTaskAttachmentCaptionMaxRunes,
			),
		}
	}
	return &preparedSpecTaskAttachment{
		filename: filename,
		mimeType: mimeType,
		caption:  caption,
		body:     body,
	}, nil
}

func validateInlineSpecTaskAttachments(inputs []types.SpecTaskInlineAttachment) error {
	return validateInlineSpecTaskAttachmentsWithLimits(
		inputs,
		types.SpecTaskAttachmentMaxPerTask,
		types.SpecTaskAttachmentMaxBytes,
		types.SpecTaskInlineAttachmentsMaxBytes,
	)
}

func validateInlineSpecTaskAttachmentsWithLimits(
	inputs []types.SpecTaskInlineAttachment,
	maxCount int,
	maxFileBytes int,
	maxTotalBytes int,
) error {
	if len(inputs) > maxCount {
		return &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("too many attachments — limit is %d per task", maxCount),
		}
	}

	seen := make(map[string]struct{}, len(inputs))
	totalBytes := 0
	var rejected []*specTaskAttachmentInputError
	reject := func(status int, format string, args ...any) {
		rejected = append(rejected, &specTaskAttachmentInputError{status: status, message: fmt.Sprintf(format, args...)})
	}
	for _, input := range inputs {
		if strings.TrimSpace(input.Name) == "" {
			reject(http.StatusBadRequest, "attachment name is required")
			continue
		}
		if input.ContentBase64 == "" {
			reject(http.StatusBadRequest, "content_base64 is required for %s", input.Name)
			continue
		}
		if len(input.ContentBase64) > base64.StdEncoding.EncodedLen(maxFileBytes) {
			reject(http.StatusRequestEntityTooLarge, "%s exceeds max size", input.Name)
			continue
		}
		body, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			reject(http.StatusBadRequest, "invalid base64 content for %s", input.Name)
			continue
		}
		if len(body) > maxFileBytes {
			reject(http.StatusRequestEntityTooLarge, "%s exceeds max size", input.Name)
			continue
		}
		totalBytes += len(body)
		attachment, err := prepareSpecTaskAttachment(input.Name, body, input.Caption)
		if err != nil {
			rejected = append(rejected, asSpecTaskAttachmentInputError(err))
			continue
		}
		if _, exists := seen[attachment.filename]; exists {
			reject(http.StatusBadRequest, "duplicate attachment filename: %s", attachment.filename)
			continue
		}
		seen[attachment.filename] = struct{}{}
		attachment.body = nil
	}
	if totalBytes > maxTotalBytes {
		reject(http.StatusRequestEntityTooLarge, "inline attachments exceed total size limit of %d bytes", maxTotalBytes)
	}
	return joinSpecTaskAttachmentRejections(len(inputs), rejected)
}

func prepareInlineSpecTaskAttachment(input types.SpecTaskInlineAttachment) (*preparedSpecTaskAttachment, error) {
	body, err := base64.StdEncoding.DecodeString(input.ContentBase64)
	if err != nil {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("invalid base64 content for %s", input.Name),
		}
	}
	return prepareSpecTaskAttachment(input.Name, body, input.Caption)
}

func writeSpecTaskAttachmentInputError(w http.ResponseWriter, err error) {
	var inputErr *specTaskAttachmentInputError
	if errors.As(err, &inputErr) {
		http.Error(w, inputErr.message, inputErr.status)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func (s *HelixAPIServer) persistSpecTaskAttachment(
	ctx context.Context,
	taskID string,
	projectID string,
	userID string,
	attachment *preparedSpecTaskAttachment,
) (*types.SpecTaskAttachment, error) {
	attID := system.GenerateSpecTaskAttachmentID()
	storageName := fmt.Sprintf("%s__%s", attID, attachment.filename)
	item, err := s.Controller.FilestoreSpecTaskAttachmentUpload(ctx, taskID, storageName, bytes.NewReader(attachment.body))
	if err != nil {
		s.cleanupFailedSpecTaskAttachmentBlob(ctx, item.Path)
		return nil, fmt.Errorf("write attachment to filestore: %w", err)
	}

	row := &types.SpecTaskAttachment{
		ID:            attID,
		SpecTaskID:    taskID,
		ProjectID:     projectID,
		UserID:        userID,
		Filename:      attachment.filename,
		MimeType:      attachment.mimeType,
		SizeBytes:     int64(len(attachment.body)),
		Caption:       attachment.caption,
		FilestorePath: item.Path,
	}
	if err := s.Store.CreateSpecTaskAttachment(ctx, row); err != nil {
		s.cleanupFailedSpecTaskAttachmentBlob(ctx, item.Path)
		return nil, fmt.Errorf("create attachment row: %w", err)
	}
	return row, nil
}

func (s *HelixAPIServer) cleanupFailedSpecTaskAttachmentBlob(ctx context.Context, path string) {
	if path == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.Controller.FilestoreSpecTaskAttachmentDelete(cleanupCtx, path); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("Failed to delete attachment blob after persistence failed")
	}
}

// readMultipartSpecTaskAttachment reads and validates one uploaded file. Validation
// failures are *specTaskAttachmentInputError; any other error is a server fault.
func readMultipartSpecTaskAttachment(fh *multipart.FileHeader, caption string) (*preparedSpecTaskAttachment, error) {
	if fh.Size > types.SpecTaskAttachmentMaxBytes {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusRequestEntityTooLarge,
			message: fmt.Sprintf("%s is too large (%d > %d bytes)", fh.Filename, fh.Size, types.SpecTaskAttachmentMaxBytes),
		}
	}
	src, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()
	body, err := io.ReadAll(io.LimitReader(src, types.SpecTaskAttachmentMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read uploaded file: %w", err)
	}
	return prepareSpecTaskAttachment(fh.Filename, body, caption)
}

// rollbackSpecTaskAttachments deletes rows and blobs created earlier in a failed
// batch. It returns the filenames it could not fully remove.
func (s *HelixAPIServer) rollbackSpecTaskAttachments(ctx context.Context, created []*types.SpecTaskAttachment) []string {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	var leaked []string
	for _, row := range created {
		rowErr := s.Store.DeleteSpecTaskAttachment(rollbackCtx, row.ID)
		blobErr := s.Controller.FilestoreSpecTaskAttachmentDelete(rollbackCtx, row.FilestorePath)
		if rowErr != nil || blobErr != nil {
			log.Error().
				AnErr("row_err", rowErr).
				AnErr("blob_err", blobErr).
				Str("attachment_id", row.ID).
				Str("path", row.FilestorePath).
				Msg("Failed to roll back attachment")
			leaked = append(leaked, row.Filename)
		}
	}
	return leaked
}

func (s *HelixAPIServer) cleanupInlineSpecTaskAttachments(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	if err := s.Store.DeleteSpecTaskAttachmentsByTaskID(ctx, taskID); err != nil {
		log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to delete inline attachment rows")
	}
	if err := s.Controller.FilestoreSpecTaskAttachmentsDeleteAll(ctx, taskID); err != nil {
		log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to delete inline attachment blobs")
	}
}

func (s *HelixAPIServer) cleanupFailedInlineSpecTask(ctx context.Context, taskID string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	s.cleanupInlineSpecTaskAttachments(cleanupCtx, taskID)
	if err := s.Store.DeleteSpecTask(cleanupCtx, taskID); err != nil {
		log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to delete task after inline attachment ingestion failed")
	}
}

// uploadSpecTaskAttachments godoc
// @Summary Upload attachments for a spec task
// @Description Upload one or more files (images, PDFs, text) to be made available to the agent.
// @Tags    spec-driven-tasks
// @Accept  multipart/form-data
// @Produce json
// @Param   taskId path string true "Spec task ID"
// @Param   files formData file true "Files to attach (multipart form data, field 'files')"
// @Param   caption formData string false "Optional caption for the attachment (single file uploads only)"
// @Success 201 {array} types.SpecTaskAttachment
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Failure 413 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/attachments [post]
// @Security BearerAuth
func (s *HelixAPIServer) uploadSpecTaskAttachments(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	taskID := mux.Vars(r)["taskId"]
	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if rejectPreparingSpecTaskMutation(w, task) {
		return
	}
	if specTaskAttachmentUploadsLocked(task.Status) {
		http.Error(w, "task has reached pull request delivery — attachments are read-only", http.StatusConflict)
		return
	}

	// Keep the in-memory buffer bounded to one file's worth; larger uploads spill to
	// temp files. This must NOT scale with SpecTaskAttachmentMaxPerTask, or a large cap
	// (e.g. 500) would let the parser buffer gigabytes in memory. Per-file and per-task
	// caps are still enforced below.
	if err := r.ParseMultipartForm(types.SpecTaskAttachmentMaxBytes); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "no files provided (use field name 'files')", http.StatusBadRequest)
		return
	}
	caption := r.FormValue("caption")

	// Per-task cap: existing + incoming must not exceed limit.
	existing, err := s.Store.ListSpecTaskAttachments(ctx, taskID)
	if err != nil {
		http.Error(w, "failed to load existing attachments", http.StatusInternalServerError)
		return
	}
	if len(existing)+len(files) > types.SpecTaskAttachmentMaxPerTask {
		http.Error(w, fmt.Sprintf("too many attachments — limit is %d per task", types.SpecTaskAttachmentMaxPerTask), http.StatusBadRequest)
		return
	}

	// Validate every file before writing any, so one bad file rejects the whole batch
	// and the caller hears about every bad file at once. Bodies are dropped after
	// validation and re-read from the multipart temp files when committing, keeping
	// memory bounded to one file.
	var rejected []*specTaskAttachmentInputError
	for _, fh := range files {
		if _, err := readMultipartSpecTaskAttachment(fh, caption); err != nil {
			var inputErr *specTaskAttachmentInputError
			if !errors.As(err, &inputErr) {
				log.Error().Err(err).Str("task_id", taskID).Str("filename", fh.Filename).Msg("Failed to read uploaded attachment")
				http.Error(w, "failed to read uploaded file", http.StatusInternalServerError)
				return
			}
			rejected = append(rejected, inputErr)
		}
	}
	if err := joinSpecTaskAttachmentRejections(len(files), rejected); err != nil {
		writeSpecTaskAttachmentInputError(w, err)
		return
	}

	created := make([]*types.SpecTaskAttachment, 0, len(files))
	for _, fh := range files {
		attachment, err := readMultipartSpecTaskAttachment(fh, caption)
		if err == nil {
			var row *types.SpecTaskAttachment
			row, err = s.persistSpecTaskAttachment(ctx, taskID, task.ProjectID, user.ID, attachment)
			attachment.body = nil
			if err == nil {
				created = append(created, row)
				continue
			}
		}
		log.Error().Err(err).Str("task_id", taskID).Str("filename", fh.Filename).Msg("Failed to persist attachment; rolling back batch")
		if leaked := s.rollbackSpecTaskAttachments(ctx, created); len(leaked) > 0 {
			http.Error(w, fmt.Sprintf(
				"failed to save %s, and rollback failed for %s — these attachments may still be on the task",
				fh.Filename, strings.Join(leaked, ", "),
			), http.StatusInternalServerError)
			return
		}
		http.Error(w, fmt.Sprintf("failed to save %s; the batch was rolled back and nothing was saved", fh.Filename), http.StatusInternalServerError)
		return
	}

	// Stage the uploaded attachments into the helix-specs branch immediately, so they
	// land in design/tasks/<taskDir>/attachments/ regardless of when this upload happens
	// relative to planning. This closes the race where a slow upload lost to start-planning
	// and the file was never committed nor surfaced to the agent. Staging is idempotent and
	// also notifies the agent if a planning session already exists. Failure here is
	// non-fatal: the row + blob exist, and planning-time staging remains a backstop.
	//
	// Detach from the request context so a client disconnect after the multipart body was
	// received doesn't abort the git commit.
	stageCtx, cancel := detachContext(ctx, 60*time.Second)
	defer cancel()
	if err := s.specDrivenTaskService.StageUploadedAttachments(stageCtx, taskID); err != nil {
		log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to stage uploaded attachments into helix-specs (planning-time staging will retry)")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// listSpecTaskAttachments godoc
// @Summary List attachments for a spec task
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "Spec task ID"
// @Success 200 {array} types.SpecTaskAttachment
// @Router /api/v1/spec-tasks/{taskId}/attachments [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSpecTaskAttachments(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	taskID := mux.Vars(r)["taskId"]
	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	attachments, err := s.Store.ListSpecTaskAttachments(ctx, taskID)
	if err != nil {
		http.Error(w, "failed to list attachments", http.StatusInternalServerError)
		return
	}
	if attachments == nil {
		attachments = []*types.SpecTaskAttachment{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(attachments)
}

// getSpecTaskAttachmentContent godoc
// @Summary Stream the bytes of a spec task attachment
// @Tags    spec-driven-tasks
// @Produce octet-stream
// @Param   taskId path string true "Spec task ID"
// @Param   attId path string true "Attachment ID"
// @Success 200 {file} binary
// @Router /api/v1/spec-tasks/{taskId}/attachments/{attId}/content [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecTaskAttachmentContent(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	attID := vars["attId"]

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	att, err := s.Store.GetSpecTaskAttachment(ctx, attID)
	if err != nil {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	if att.SpecTaskID != taskID {
		http.Error(w, "attachment does not belong to this task", http.StatusNotFound)
		return
	}

	// Public design docs: anonymous read allowed. Otherwise require ActionGet auth.
	if !task.PublicDesignDocs {
		user := getRequestUser(r)
		if user == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	rc, err := s.Controller.FilestoreSpecTaskAttachmentDownload(att.FilestorePath)
	if err != nil {
		log.Error().Err(err).Str("path", att.FilestorePath).Msg("Failed to open attachment from filestore")
		http.Error(w, "failed to load file", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", att.MimeType)
	// Force download for SVGs (defence-in-depth against any browser that ignores
	// script-strip) and for archives, which browsers cannot display inline.
	if att.MimeType == "image/svg+xml" || att.MimeType == "application/gzip" || att.MimeType == "application/zip" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", att.Filename))
	}
	if _, err := io.Copy(w, rc); err != nil {
		log.Warn().Err(err).Msg("Failed to stream attachment to client")
	}
}

// deleteSpecTaskAttachment godoc
// @Summary Delete a spec task attachment
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "Spec task ID"
// @Param   attId path string true "Attachment ID"
// @Success 204
// @Router /api/v1/spec-tasks/{taskId}/attachments/{attId} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteSpecTaskAttachment(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	attID := vars["attId"]

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if rejectPreparingSpecTaskMutation(w, task) {
		return
	}
	if specTaskAttachmentDeletesLocked(task.Status) {
		http.Error(w, "task is past spec_review — attachments are read-only", http.StatusConflict)
		return
	}
	att, err := s.Store.GetSpecTaskAttachment(ctx, attID)
	if err != nil {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	if att.SpecTaskID != taskID {
		http.Error(w, "attachment does not belong to this task", http.StatusNotFound)
		return
	}

	if err := s.Controller.FilestoreSpecTaskAttachmentDelete(ctx, att.FilestorePath); err != nil {
		log.Warn().Err(err).Str("path", att.FilestorePath).Msg("Failed to delete attachment blob — continuing to delete row")
	}
	if err := s.Store.DeleteSpecTaskAttachment(ctx, attID); err != nil {
		http.Error(w, "failed to delete attachment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readSpecTaskAttachmentBlob is the AttachmentBlobReader callback wired into
// SpecDrivenTaskService. It loads the bytes of an attachment from the filestore.
func (s *HelixAPIServer) readSpecTaskAttachmentBlob(_ context.Context, absolutePath string) ([]byte, error) {
	rc, err := s.Controller.FilestoreSpecTaskAttachmentDownload(absolutePath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// sanitiseAttachmentFilename trims path components and rejects hidden/empty names.
func sanitiseAttachmentFilename(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return ""
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return ""
	}
	return name
}

// detectAttachmentMime picks the more specific of: HTTP content-type sniff on the
// first 512 bytes, or the filename extension (which catches text/markdown and
// image/svg+xml — HTTP sniff returns text/plain or text/xml respectively).
func detectAttachmentMime(filename string, body []byte) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown"
	case ".svg":
		return "image/svg+xml"
	case ".csv":
		return "text/csv"
	}
	if len(body) == 0 {
		return ""
	}
	head := body
	if len(head) > 512 {
		head = head[:512]
	}
	ct := http.DetectContentType(head)
	// http.DetectContentType returns "image/jpeg" / "image/png" / "application/pdf" / ...
	// but for plain text it includes a charset; strip it.
	if i := strings.IndexByte(ct, ';'); i > 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	// The sniffer reports gzip magic bytes under the legacy x-gzip name.
	if ct == "application/x-gzip" {
		return "application/gzip"
	}
	return ct
}

// svgContainsScript runs a cheap case-insensitive substring check for "<script".
// Not a full XML parser — that's overkill given we also serve SVGs as
// Content-Disposition: attachment (defence in depth).
func svgContainsScript(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "<script")
}
