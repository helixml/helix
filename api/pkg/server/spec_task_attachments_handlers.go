package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Statuses past which attachments are read-only — the agent has already started using
// them and changing them mid-flight would be confusing.
var specTaskAttachmentReadOnlyStatuses = map[types.SpecTaskStatus]bool{
	types.TaskStatusSpecApproved:           true,
	types.TaskStatusImplementationQueued:   true,
	types.TaskStatusImplementation:         true,
	types.TaskStatusImplementationReview:   true,
	types.TaskStatusPullRequest:            true,
	types.TaskStatusDone:                   true,
	types.TaskStatusImplementationFailed:   true,
}

func specTaskAttachmentsLocked(status types.SpecTaskStatus) bool {
	return specTaskAttachmentReadOnlyStatuses[status]
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
	if specTaskAttachmentsLocked(task.Status) {
		http.Error(w, "task is past spec_review — attachments are read-only", http.StatusConflict)
		return
	}

	// Enforce a request-body cap matching per-task budget (10 files * 10 MB + overhead).
	if err := r.ParseMultipartForm(int64(types.SpecTaskAttachmentMaxPerTask) * types.SpecTaskAttachmentMaxBytes); err != nil {
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
		filename := sanitiseAttachmentFilename(fh.Filename)
		if filename == "" {
			http.Error(w, fmt.Sprintf("invalid filename: %s", fh.Filename), http.StatusBadRequest)
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

		mimeType := detectAttachmentMime(filename, body)
		if !types.SpecTaskAttachmentAllowedMimeTypes[mimeType] {
			http.Error(w, fmt.Sprintf("unsupported mime type for %s: %s", fh.Filename, mimeType), http.StatusBadRequest)
			return
		}
		if mimeType == "image/svg+xml" && svgContainsScript(body) {
			http.Error(w, fmt.Sprintf("%s contains a <script> tag — SVG with scripts is not allowed", fh.Filename), http.StatusBadRequest)
			return
		}

		attID := system.GenerateSpecTaskAttachmentID()
		storageName := fmt.Sprintf("%s__%s", attID, filename)
		item, err := s.Controller.FilestoreSpecTaskAttachmentUpload(taskID, storageName, bytes.NewReader(body))
		if err != nil {
			log.Error().Err(err).Str("task_id", taskID).Str("filename", filename).Msg("Failed to write attachment to filestore")
			http.Error(w, "failed to save file", http.StatusInternalServerError)
			return
		}

		row := &types.SpecTaskAttachment{
			ID:            attID,
			SpecTaskID:    taskID,
			ProjectID:     task.ProjectID,
			UserID:        user.ID,
			Filename:      filename,
			MimeType:      mimeType,
			SizeBytes:     int64(len(body)),
			Caption:       caption,
			FilestorePath: item.Path,
		}
		if err := s.Store.CreateSpecTaskAttachment(ctx, row); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to create attachment row — orphan blob will remain")
			http.Error(w, "failed to record attachment", http.StatusInternalServerError)
			return
		}
		created = append(created, row)
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
	if specTaskAttachmentsLocked(task.Status) {
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

	if err := s.Controller.FilestoreSpecTaskAttachmentDelete(att.FilestorePath); err != nil {
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

