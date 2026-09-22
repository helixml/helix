package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func prepareInlineSpecTaskAttachments(inputs []types.SpecTaskInlineAttachment) ([]*preparedSpecTaskAttachment, error) {
	return prepareInlineSpecTaskAttachmentsWithLimits(
		inputs,
		types.SpecTaskAttachmentMaxPerTask,
		types.SpecTaskAttachmentMaxBytes,
		types.SpecTaskInlineAttachmentsMaxBytes,
	)
}

func prepareInlineSpecTaskAttachmentsWithLimits(
	inputs []types.SpecTaskInlineAttachment,
	maxCount int,
	maxFileBytes int,
	maxTotalBytes int,
) ([]*preparedSpecTaskAttachment, error) {
	if len(inputs) > maxCount {
		return nil, &specTaskAttachmentInputError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("too many attachments — limit is %d per task", maxCount),
		}
	}

	prepared := make([]*preparedSpecTaskAttachment, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	totalBytes := 0
	for _, input := range inputs {
		if strings.TrimSpace(input.Name) == "" {
			return nil, &specTaskAttachmentInputError{
				status:  http.StatusBadRequest,
				message: "attachment name is required",
			}
		}
		if input.ContentBase64 == "" {
			return nil, &specTaskAttachmentInputError{
				status:  http.StatusBadRequest,
				message: fmt.Sprintf("content_base64 is required for %s", input.Name),
			}
		}
		if len(input.ContentBase64) > base64.StdEncoding.EncodedLen(maxFileBytes) {
			return nil, &specTaskAttachmentInputError{
				status:  http.StatusRequestEntityTooLarge,
				message: fmt.Sprintf("%s exceeds max size", input.Name),
			}
		}
		body, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return nil, &specTaskAttachmentInputError{
				status:  http.StatusBadRequest,
				message: fmt.Sprintf("invalid base64 content for %s", input.Name),
			}
		}
		if len(body) > maxFileBytes {
			return nil, &specTaskAttachmentInputError{
				status:  http.StatusRequestEntityTooLarge,
				message: fmt.Sprintf("%s exceeds max size", input.Name),
			}
		}
		attachment, err := prepareSpecTaskAttachment(input.Name, body, input.Caption)
		if err != nil {
			return nil, err
		}
		totalBytes += len(body)
		if totalBytes > maxTotalBytes {
			return nil, &specTaskAttachmentInputError{
				status: http.StatusRequestEntityTooLarge,
				message: fmt.Sprintf(
					"inline attachments exceed total size limit of %d bytes",
					maxTotalBytes,
				),
			}
		}
		if _, exists := seen[attachment.filename]; exists {
			return nil, &specTaskAttachmentInputError{
				status:  http.StatusBadRequest,
				message: fmt.Sprintf("duplicate attachment filename: %s", attachment.filename),
			}
		}
		seen[attachment.filename] = struct{}{}
		prepared = append(prepared, attachment)
	}
	return prepared, nil
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
		if deleteErr := s.Controller.FilestoreSpecTaskAttachmentDelete(ctx, item.Path); deleteErr != nil {
			log.Warn().Err(deleteErr).Str("path", item.Path).Msg("Failed to delete attachment blob after row creation failed")
		}
		return nil, fmt.Errorf("create attachment row: %w", err)
	}
	return row, nil
}

func (s *HelixAPIServer) persistInlineSpecTaskAttachments(
	ctx context.Context,
	taskID string,
	projectID string,
	userID string,
	attachments []*preparedSpecTaskAttachment,
) error {
	for _, attachment := range attachments {
		if _, err := s.persistSpecTaskAttachment(ctx, taskID, projectID, userID, attachment); err != nil {
			return err
		}
	}
	return nil
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

	created := make([]*types.SpecTaskAttachment, 0, len(files))
	for _, fh := range files {
		if fh.Size > types.SpecTaskAttachmentMaxBytes {
			http.Error(w, fmt.Sprintf("%s is too large (%d > %d bytes)", fh.Filename, fh.Size, types.SpecTaskAttachmentMaxBytes), http.StatusRequestEntityTooLarge)
			return
		}
		src, err := fh.Open()
		if err != nil {
			http.Error(w, "failed to open uploaded file", http.StatusInternalServerError)
			return
		}
		// Read the whole body once: needed for content-sniff and SVG script check, then
		// the same bytes are written to filestore. Capped at the per-file limit by
		// ParseMultipartForm + the Size check above.
		body, err := io.ReadAll(io.LimitReader(src, types.SpecTaskAttachmentMaxBytes+1))
		_ = src.Close()
		if err != nil {
			http.Error(w, "failed to read uploaded file", http.StatusInternalServerError)
			return
		}
		if int64(len(body)) > types.SpecTaskAttachmentMaxBytes {
			http.Error(w, fmt.Sprintf("%s exceeds max size", fh.Filename), http.StatusRequestEntityTooLarge)
			return
		}

		attachment, err := prepareSpecTaskAttachment(fh.Filename, body, caption)
		if err != nil {
			writeSpecTaskAttachmentInputError(w, err)
			return
		}
		row, err := s.persistSpecTaskAttachment(ctx, taskID, task.ProjectID, user.ID, attachment)
		if err != nil {
			log.Error().Err(err).Str("task_id", taskID).Str("filename", attachment.filename).Msg("Failed to persist attachment")
			http.Error(w, "failed to record attachment", http.StatusInternalServerError)
			return
		}
		created = append(created, row)
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
	// Force download for SVGs (defence-in-depth against any browser that ignores script-strip).
	if att.MimeType == "image/svg+xml" {
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
	return ct
}

// svgContainsScript runs a cheap case-insensitive substring check for "<script".
// Not a full XML parser — that's overkill given we also serve SVGs as
// Content-Disposition: attachment (defence in depth).
func svgContainsScript(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "<script")
}
