package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/vhost"
	"github.com/rs/zerolog/log"
)

const (
	artifactMaxArchiveBytes = 100 << 20
	artifactMaxFileBytes    = 25 << 20
	artifactMaxTotalBytes   = 100 << 20
	artifactMaxFiles        = 5000
	artifactMaxPathBytes    = 1024
	artifactBootstrapTTL    = time.Minute
	artifactSessionTTL      = 8 * time.Hour
	artifactAccessCookie    = "helix_artifact_access"
	artifactBootstrapAud    = "helix-artifact-bootstrap"
	artifactSessionAud      = "helix-artifact-session"
)

type artifactUploadFile struct {
	Metadata types.ArtifactFile
	Data     []byte
}

type artifactUpload struct {
	Files         []artifactUploadFile
	Entrypoint    string
	ContentSHA256 string
}

type artifactAccessClaims struct {
	ArtifactID   string `json:"artifact_id"`
	ArtifactPath string `json:"artifact_path,omitempty"`
	jwt.RegisteredClaims
}

var artifactAccessBootstrapTemplate = template.Must(template.New("artifact-access").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Opening artifact</title></head><body>
<form id="artifact-access" method="post" action="{{.Action}}"><input type="hidden" name="token" value="{{.Token}}"><noscript><button type="submit">Open artifact</button></noscript></form>
<script>document.getElementById('artifact-access').submit()</script>
</body></html>`))

// listProjectArtifacts godoc
// @Summary List project artifacts
// @Description List static artifacts in a project. Access is inherited from the project.
// @Tags Artifacts
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} types.ArtifactsListResponse
// @Router /api/v1/projects/{id}/artifacts [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProjectArtifacts(w http.ResponseWriter, r *http.Request) {
	project, herr := s.requireProjectAccess(r, types.ActionList)
	if herr != nil {
		writeArtifactHTTPError(w, herr.StatusCode, herr.Message)
		return
	}
	artifacts, err := s.Store.ListArtifacts(r.Context(), &store.ListArtifactsQuery{ProjectID: project.ID})
	if err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, artifact := range artifacts {
		if err := s.populateArtifactURLs(r, artifact); err != nil {
			writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeArtifactJSON(w, http.StatusOK, &types.ArtifactsListResponse{Artifacts: artifacts})
}

// createProjectArtifact godoc
// @Summary Create a project artifact
// @Description Upload one HTML file or a ZIP containing a compiled static SPA.
// @Tags Artifacts
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Project ID"
// @Param name formData string true "Artifact name"
// @Param description formData string false "Description"
// @Param entrypoint formData string false "HTML entrypoint (default index.html)"
// @Param visibility formData string false "project or public"
// @Param with_subdomain formData bool false "Allocate a public default subdomain"
// @Param artifact formData file true "HTML file or ZIP bundle"
// @Success 201 {object} types.Artifact
// @Router /api/v1/projects/{id}/artifacts [post]
// @Security BearerAuth
func (s *HelixAPIServer) createProjectArtifact(w http.ResponseWriter, r *http.Request) {
	project, herr := s.requireProjectAccess(r, types.ActionCreate)
	if herr != nil {
		writeArtifactHTTPError(w, herr.StatusCode, herr.Message)
		return
	}
	if err := parseArtifactMultipart(w, r); err != nil {
		writeArtifactHTTPError(w, artifactUploadStatus(err), err.Error())
		return
	}
	defer removeArtifactMultipartFiles(r)

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		writeArtifactHTTPError(w, http.StatusBadRequest, "name is required")
		return
	}
	visibility, err := parseArtifactVisibility(r.FormValue("visibility"), types.ArtifactVisibilityProject)
	if err != nil {
		writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	withSubdomain, err := artifactFormBool(r, "with_subdomain")
	if err != nil {
		writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	if withSubdomain && visibility != types.ArtifactVisibilityPublic {
		writeArtifactHTTPError(w, http.StatusBadRequest, "with_subdomain requires visibility=public")
		return
	}
	if (withSubdomain || visibility == types.ArtifactVisibilityProject) && s.vhostBaseDomain() == "" {
		writeArtifactHTTPError(w, http.StatusBadRequest, "artifact subdomains are not configured on this Helix instance")
		return
	}

	upload, err := readArtifactUpload(r, r.FormValue("entrypoint"), true, false)
	if err != nil {
		writeArtifactHTTPError(w, artifactUploadStatus(err), err.Error())
		return
	}
	if err := s.validateArtifactProvenance(r, project.ID); err != nil {
		writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}

	user := getRequestUser(r)
	artifactID := system.GenerateArtifactID()
	versionID := system.GenerateArtifactVersionID()
	storagePrefix := filestore.GetArtifactVersionPrefix(s.Cfg.Controller.FilePrefixGlobal, project.ID, artifactID, versionID)
	if err := s.writeArtifactUpload(r, storagePrefix, upload); err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	artifact := &types.Artifact{
		ID: artifactID, ProjectID: project.ID, OrganizationID: project.OrganizationID,
		Name: name, Description: r.FormValue("description"), Kind: artifactKind(upload.Files),
		Entrypoint: upload.Entrypoint, Visibility: visibility, ActiveVersionID: versionID,
		CreatedBy: user.ID, UpdatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
	}
	version := artifactVersionFromUpload(r, artifactID, versionID, storagePrefix, 1, user.ID, upload)
	if err := s.Store.CreateArtifact(r.Context(), artifact, version); err != nil {
		s.cleanupArtifactStorage(r, storagePrefix)
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	artifact.ActiveVersion = version
	if withSubdomain || visibility == types.ArtifactVisibilityProject {
		if _, err := s.ensureArtifactSubdomain(r, artifact); err != nil {
			s.rollbackCreatedArtifact(r, artifact, storagePrefix)
			writeArtifactHTTPError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := s.populateArtifactURLs(r, artifact); err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeArtifactJSON(w, http.StatusCreated, artifact)
}

// getArtifact godoc
// @Summary Get an artifact
// @Tags Artifacts
// @Produce json
// @Param artifact_id path string true "Artifact ID"
// @Success 200 {object} types.Artifact
// @Router /api/v1/artifacts/{artifact_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.requireArtifactAccess(w, r, types.ActionGet)
	if !ok {
		return
	}
	if err := s.populateArtifactURLs(r, artifact); err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeArtifactJSON(w, http.StatusOK, artifact)
}

// updateArtifact godoc
// @Summary Update an artifact
// @Description Patch metadata and optionally upload a replacement HTML file or ZIP bundle as a new version.
// @Tags Artifacts
// @Accept multipart/form-data
// @Produce json
// @Param artifact_id path string true "Artifact ID"
// @Param name formData string false "Artifact name"
// @Param description formData string false "Description"
// @Param entrypoint formData string false "HTML entrypoint"
// @Param visibility formData string false "project or public"
// @Param with_subdomain formData bool false "Allocate or retain a public default subdomain"
// @Param artifact formData file false "Replacement HTML file or ZIP bundle"
// @Success 200 {object} types.Artifact
// @Router /api/v1/artifacts/{artifact_id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.requireArtifactAccess(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	previousVisibility := artifact.Visibility
	if err := parseArtifactMultipart(w, r); err != nil {
		writeArtifactHTTPError(w, artifactUploadStatus(err), err.Error())
		return
	}
	defer removeArtifactMultipartFiles(r)

	if value, exists := artifactFormField(r, "name"); exists {
		if strings.TrimSpace(value) == "" {
			writeArtifactHTTPError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		artifact.Name = strings.TrimSpace(value)
	}
	if value, exists := artifactFormField(r, "description"); exists {
		artifact.Description = value
	}
	if value, exists := artifactFormField(r, "visibility"); exists {
		visibility, err := parseArtifactVisibility(value, artifact.Visibility)
		if err != nil {
			writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}
		artifact.Visibility = visibility
	}
	withSubdomain, err := artifactFormBool(r, "with_subdomain")
	if err != nil {
		writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	withSubdomainSet := artifactFormHasField(r, "with_subdomain")
	if withSubdomain && artifact.Visibility != types.ArtifactVisibilityPublic {
		writeArtifactHTTPError(w, http.StatusBadRequest, "with_subdomain requires visibility=public")
		return
	}
	if (withSubdomain || artifact.Visibility == types.ArtifactVisibilityProject) && s.vhostBaseDomain() == "" {
		writeArtifactHTTPError(w, http.StatusBadRequest, "artifact subdomains are not configured on this Helix instance")
		return
	}
	if err := s.validateArtifactProvenance(r, artifact.ProjectID); err != nil {
		writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	existingRoutes, err := s.Store.ListVHostRoutesByTarget(r.Context(), types.VHostTargetArtifact, artifact.ID)
	if err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, "inspect artifact subdomain: "+err.Error())
		return
	}
	wantsRoute := artifact.Visibility == types.ArtifactVisibilityProject || withSubdomain
	if artifact.Visibility == types.ArtifactVisibilityPublic && !withSubdomainSet && previousVisibility == types.ArtifactVisibilityPublic {
		wantsRoute = len(existingRoutes) > 0
	}

	requestedEntrypoint := artifact.Entrypoint
	entrypointSet := false
	if value, exists := artifactFormField(r, "entrypoint"); exists {
		requestedEntrypoint = value
		entrypointSet = true
	}
	hasUpload := len(r.MultipartForm.File["artifact"]) > 0
	var version *types.ArtifactVersion
	var storagePrefix string
	if hasUpload {
		upload, err := readArtifactUpload(r, requestedEntrypoint, true, !entrypointSet)
		if err != nil {
			writeArtifactHTTPError(w, artifactUploadStatus(err), err.Error())
			return
		}
		artifact.Entrypoint = upload.Entrypoint
		versionID := system.GenerateArtifactVersionID()
		storagePrefix = filestore.GetArtifactVersionPrefix(s.Cfg.Controller.FilePrefixGlobal, artifact.ProjectID, artifact.ID, versionID)
		if err := s.writeArtifactUpload(r, storagePrefix, upload); err != nil {
			writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
			return
		}
		version = artifactVersionFromUpload(r, artifact.ID, versionID, storagePrefix, 0, getRequestUser(r).ID, upload)
	} else {
		entrypoint, err := validateArtifactEntrypoint(artifact.ActiveVersion.Files, requestedEntrypoint)
		if err != nil {
			writeArtifactHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}
		artifact.Entrypoint = entrypoint
	}
	artifact.UpdatedBy = getRequestUser(r).ID
	createdPrivateRoute := false
	if artifact.Visibility == types.ArtifactVisibilityProject {
		if _, err := s.ensureArtifactSubdomain(r, artifact); err != nil {
			if storagePrefix != "" {
				s.cleanupArtifactStorage(r, storagePrefix)
			}
			writeArtifactHTTPError(w, http.StatusConflict, err.Error())
			return
		}
		createdPrivateRoute = len(existingRoutes) == 0
	}
	if err := s.Store.UpdateArtifact(r.Context(), artifact, version); err != nil {
		if storagePrefix != "" {
			s.cleanupArtifactStorage(r, storagePrefix)
		}
		if createdPrivateRoute {
			if cleanupErr := s.Store.DeleteVHostRoutesByTarget(r.Context(), types.VHostTargetArtifact, artifact.ID); cleanupErr != nil {
				log.Error().Err(cleanupErr).Str("artifact_id", artifact.ID).Msg("failed to roll back artifact private route")
			}
		}
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !wantsRoute {
		if err := s.Store.DeleteVHostRoutesByTarget(r.Context(), types.VHostTargetArtifact, artifact.ID); err != nil {
			writeArtifactHTTPError(w, http.StatusInternalServerError, "remove artifact subdomain: "+err.Error())
			return
		}
	} else if artifact.Visibility == types.ArtifactVisibilityPublic {
		if _, err := s.ensureArtifactSubdomain(r, artifact); err != nil {
			writeArtifactHTTPError(w, http.StatusConflict, err.Error())
			return
		}
	}
	updated, err := s.Store.GetArtifact(r.Context(), artifact.ID)
	if err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.populateArtifactURLs(r, updated); err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeArtifactJSON(w, http.StatusOK, updated)
}

// deleteArtifact godoc
// @Summary Delete an artifact
// @Tags Artifacts
// @Param artifact_id path string true "Artifact ID"
// @Success 204
// @Router /api/v1/artifacts/{artifact_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.requireArtifactAccess(w, r, types.ActionDelete)
	if !ok {
		return
	}
	if err := s.deleteArtifactResources(r.Context(), artifact); err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HelixAPIServer) deleteArtifactResources(ctx context.Context, artifact *types.Artifact) error {
	if err := s.Store.DeleteVHostRoutesByTarget(ctx, types.VHostTargetArtifact, artifact.ID); err != nil {
		return fmt.Errorf("delete artifact subdomains: %w", err)
	}
	storagePrefix := filestore.GetArtifactPrefix(s.Cfg.Controller.FilePrefixGlobal, artifact.ProjectID, artifact.ID)
	if err := s.Controller.Options.Filestore.Delete(ctx, storagePrefix+"/"); err != nil {
		return fmt.Errorf("delete artifact files: %w", err)
	}
	if err := s.Store.DeleteArtifact(ctx, artifact.ID); err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}
	return nil
}

// listArtifactVersions godoc
// @Summary List artifact versions
// @Tags Artifacts
// @Produce json
// @Param artifact_id path string true "Artifact ID"
// @Success 200 {object} types.ArtifactVersionsListResponse
// @Router /api/v1/artifacts/{artifact_id}/versions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listArtifactVersions(w http.ResponseWriter, r *http.Request) {
	artifact, ok := s.requireArtifactAccess(w, r, types.ActionGet)
	if !ok {
		return
	}
	versions, err := s.Store.ListArtifactVersions(r.Context(), artifact.ID)
	if err != nil {
		writeArtifactHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeArtifactJSON(w, http.StatusOK, &types.ArtifactVersionsListResponse{Versions: versions})
}

func (s *HelixAPIServer) requireArtifactAccess(w http.ResponseWriter, r *http.Request, action types.Action) (*types.Artifact, bool) {
	artifactID := mux.Vars(r)["artifact_id"]
	artifact, err := s.Store.GetArtifact(r.Context(), artifactID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeArtifactHTTPError(w, status, "artifact not found")
		return nil, false
	}
	user := getRequestUser(r)
	if user == nil || user.ID == "" {
		writeArtifactHTTPError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	if err := s.authorizeUserToProjectByID(r.Context(), user, artifact.ProjectID, action); err != nil {
		writeArtifactHTTPError(w, http.StatusForbidden, err.Error())
		return nil, false
	}
	return artifact, true
}

func parseArtifactMultipart(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, artifactMaxArchiveBytes+(2<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			return fmt.Errorf("artifact upload exceeds %d bytes", artifactMaxArchiveBytes)
		}
		return fmt.Errorf("invalid multipart form: %w", err)
	}
	return nil
}

func removeArtifactMultipartFiles(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

func readArtifactUpload(r *http.Request, requestedEntrypoint string, required, allowEntrypointFallback bool) (*artifactUpload, error) {
	headers := r.MultipartForm.File["artifact"]
	if len(headers) == 0 {
		if required {
			return nil, errors.New("artifact file is required (use field name 'artifact')")
		}
		return nil, nil
	}
	if len(headers) != 1 {
		return nil, errors.New("exactly one artifact file or ZIP is required")
	}
	header := headers[0]
	if header.Size > artifactMaxArchiveBytes {
		return nil, fmt.Errorf("artifact upload exceeds %d bytes", artifactMaxArchiveBytes)
	}
	source, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("open artifact upload: %w", err)
	}
	body, err := io.ReadAll(io.LimitReader(source, artifactMaxArchiveBytes+1))
	closeErr := source.Close()
	if err != nil {
		return nil, fmt.Errorf("read artifact upload: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close artifact upload: %w", closeErr)
	}
	if len(body) > artifactMaxArchiveBytes {
		return nil, fmt.Errorf("artifact upload exceeds %d bytes", artifactMaxArchiveBytes)
	}

	var files []artifactUploadFile
	if strings.EqualFold(path.Ext(header.Filename), ".zip") || strings.Contains(strings.ToLower(header.Header.Get("Content-Type")), "zip") {
		files, err = readArtifactZIP(body)
	} else {
		filename, pathErr := normalizeArtifactPath(path.Base(header.Filename))
		if pathErr != nil {
			return nil, pathErr
		}
		if len(body) > artifactMaxFileBytes {
			return nil, fmt.Errorf("artifact file exceeds %d bytes", artifactMaxFileBytes)
		}
		files = []artifactUploadFile{newArtifactUploadFile(filename, body)}
	}
	if err != nil {
		return nil, err
	}
	metadata := make([]types.ArtifactFile, len(files))
	for i := range files {
		metadata[i] = files[i].Metadata
	}
	entrypoint, err := defaultAndValidateArtifactEntrypoint(metadata, requestedEntrypoint, allowEntrypointFallback)
	if err != nil {
		return nil, err
	}
	return &artifactUpload{Files: files, Entrypoint: entrypoint, ContentSHA256: artifactAggregateHash(files)}, nil
}

func readArtifactZIP(body []byte) ([]artifactUploadFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP artifact: %w", err)
	}
	if len(reader.File) > artifactMaxFiles {
		return nil, fmt.Errorf("artifact contains too many entries (%d > %d)", len(reader.File), artifactMaxFiles)
	}
	files := make([]artifactUploadFile, 0, len(reader.File))
	seen := make(map[string]struct{}, len(reader.File))
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.Flags&0x1 != 0 {
			return nil, fmt.Errorf("encrypted ZIP entry is not supported: %s", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
			return nil, fmt.Errorf("artifact entry must be a regular file: %s", entry.Name)
		}
		filename, err := normalizeArtifactPath(entry.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[filename]; exists {
			return nil, fmt.Errorf("duplicate artifact path: %s", filename)
		}
		seen[filename] = struct{}{}
		if entry.UncompressedSize64 > artifactMaxFileBytes {
			return nil, fmt.Errorf("artifact file %s exceeds %d bytes", filename, artifactMaxFileBytes)
		}
		source, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open ZIP entry %s: %w", filename, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(source, artifactMaxFileBytes+1))
		closeErr := source.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read ZIP entry %s: %w", filename, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close ZIP entry %s: %w", filename, closeErr)
		}
		if len(data) > artifactMaxFileBytes {
			return nil, fmt.Errorf("artifact file %s exceeds %d bytes", filename, artifactMaxFileBytes)
		}
		total += int64(len(data))
		if total > artifactMaxTotalBytes {
			return nil, fmt.Errorf("artifact uncompressed size exceeds %d bytes", artifactMaxTotalBytes)
		}
		files = append(files, newArtifactUploadFile(filename, data))
	}
	if len(files) == 0 {
		return nil, errors.New("artifact ZIP contains no files")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Metadata.Path < files[j].Metadata.Path })
	return files, nil
}

func normalizeArtifactPath(filename string) (string, error) {
	if filename == "" || len(filename) > artifactMaxPathBytes || strings.Contains(filename, "\\") || strings.HasPrefix(filename, "/") {
		return "", fmt.Errorf("invalid artifact path: %q", filename)
	}
	cleaned := path.Clean(filename)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != filename {
		return "", fmt.Errorf("invalid artifact path: %q", filename)
	}
	return cleaned, nil
}

func newArtifactUploadFile(filename string, data []byte) artifactUploadFile {
	digest := sha256.Sum256(data)
	return artifactUploadFile{
		Metadata: types.ArtifactFile{
			Path: filename, Size: int64(len(data)), ContentType: artifactContentType(filename, data),
			SHA256: hex.EncodeToString(digest[:]),
		},
		Data: data,
	}
}

func artifactContentType(filename string, data []byte) string {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(filename)))
	if contentType != "" {
		return contentType
	}
	return http.DetectContentType(data)
}

func defaultAndValidateArtifactEntrypoint(files []types.ArtifactFile, requested string, allowFallback bool) (string, error) {
	entrypoint := strings.TrimSpace(requested)
	if entrypoint != "" {
		validated, err := validateArtifactEntrypoint(files, entrypoint)
		if err == nil || !allowFallback {
			return validated, err
		}
		entrypoint = ""
	}
	if entrypoint == "" {
		entrypoint = "index.html"
		found := false
		for _, file := range files {
			if file.Path == entrypoint {
				found = true
				break
			}
		}
		if !found && len(files) == 1 {
			entrypoint = files[0].Path
		}
	}
	return validateArtifactEntrypoint(files, entrypoint)
}

func validateArtifactEntrypoint(files []types.ArtifactFile, requested string) (string, error) {
	entrypoint, err := normalizeArtifactPath(strings.TrimSpace(requested))
	if err != nil {
		return "", fmt.Errorf("invalid entrypoint: %w", err)
	}
	if !strings.EqualFold(path.Ext(entrypoint), ".html") && !strings.EqualFold(path.Ext(entrypoint), ".htm") {
		return "", errors.New("artifact entrypoint must be an HTML file")
	}
	for _, file := range files {
		if file.Path == entrypoint {
			return entrypoint, nil
		}
	}
	return "", fmt.Errorf("artifact entrypoint does not exist: %s", entrypoint)
}

func artifactAggregateHash(files []artifactUploadFile) string {
	hasher := sha256.New()
	for _, file := range files {
		_, _ = io.WriteString(hasher, file.Metadata.Path)
		_, _ = io.WriteString(hasher, "\x00")
		_, _ = io.WriteString(hasher, file.Metadata.SHA256)
		_, _ = io.WriteString(hasher, "\n")
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func artifactKind(files []artifactUploadFile) types.ArtifactKind {
	if len(files) == 1 {
		return types.ArtifactKindSingleFile
	}
	return types.ArtifactKindSPA
}

func artifactVersionFromUpload(r *http.Request, artifactID, versionID, storagePrefix string, versionNumber int, userID string, upload *artifactUpload) *types.ArtifactVersion {
	files := make([]types.ArtifactFile, len(upload.Files))
	var totalBytes int64
	for i := range upload.Files {
		files[i] = upload.Files[i].Metadata
		totalBytes += upload.Files[i].Metadata.Size
	}
	return &types.ArtifactVersion{
		ID: versionID, ArtifactID: artifactID, Version: versionNumber, StoragePrefix: storagePrefix,
		Files: files, FileCount: len(files), TotalBytes: totalBytes, ContentSHA256: upload.ContentSHA256,
		SourceSessionID: r.FormValue("source_session_id"), SourceSpecTaskID: r.FormValue("source_spec_task_id"),
		CreatedBy: userID, CreatedAt: time.Now(),
	}
}

func (s *HelixAPIServer) writeArtifactUpload(r *http.Request, storagePrefix string, upload *artifactUpload) error {
	for _, file := range upload.Files {
		filename := path.Join(storagePrefix, file.Metadata.Path)
		if _, err := s.Controller.Options.Filestore.WriteFile(r.Context(), filename, bytes.NewReader(file.Data)); err != nil {
			s.cleanupArtifactStorage(r, storagePrefix)
			return fmt.Errorf("write artifact file %s: %w", file.Metadata.Path, err)
		}
	}
	return nil
}

func (s *HelixAPIServer) cleanupArtifactStorage(r *http.Request, storagePrefix string) {
	if err := s.Controller.Options.Filestore.Delete(r.Context(), storagePrefix+"/"); err != nil {
		log.Warn().Err(err).Str("storage_prefix", storagePrefix).Msg("failed to clean up artifact storage")
	}
}

func (s *HelixAPIServer) rollbackCreatedArtifact(r *http.Request, artifact *types.Artifact, storagePrefix string) {
	if err := s.Store.DeleteArtifact(r.Context(), artifact.ID); err != nil {
		log.Error().Err(err).Str("artifact_id", artifact.ID).Msg("failed to roll back artifact metadata")
	}
	s.cleanupArtifactStorage(r, storagePrefix)
}

func (s *HelixAPIServer) validateArtifactProvenance(r *http.Request, projectID string) error {
	if sessionID := strings.TrimSpace(r.FormValue("source_session_id")); sessionID != "" {
		session, err := s.Store.GetSession(r.Context(), sessionID)
		if err != nil {
			return fmt.Errorf("source session not found: %w", err)
		}
		if session.ProjectID != projectID {
			return errors.New("source session belongs to another project")
		}
	}
	if taskID := strings.TrimSpace(r.FormValue("source_spec_task_id")); taskID != "" {
		task, err := s.Store.GetSpecTask(r.Context(), taskID)
		if err != nil {
			return fmt.Errorf("source spec task not found: %w", err)
		}
		if task.ProjectID != projectID {
			return errors.New("source spec task belongs to another project")
		}
	}
	return nil
}

func parseArtifactVisibility(value string, fallback types.ArtifactVisibility) (types.ArtifactVisibility, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	visibility := types.ArtifactVisibility(value)
	if visibility != types.ArtifactVisibilityProject && visibility != types.ArtifactVisibilityPublic {
		return "", errors.New("visibility must be project or public")
	}
	return visibility, nil
}

func artifactFormField(r *http.Request, name string) (string, bool) {
	values, ok := r.MultipartForm.Value[name]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

func artifactFormHasField(r *http.Request, name string) bool {
	_, ok := artifactFormField(r, name)
	return ok
}

func artifactFormBool(r *http.Request, name string) (bool, error) {
	value, ok := artifactFormField(r, name)
	if !ok || strings.TrimSpace(value) == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func artifactUploadStatus(err error) int {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "exceeds") || strings.Contains(message, "too large") {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func (s *HelixAPIServer) ensureArtifactSubdomain(r *http.Request, artifact *types.Artifact) (*types.VHostRoute, error) {
	routes, err := s.Store.ListVHostRoutesByTarget(r.Context(), types.VHostTargetArtifact, artifact.ID)
	if err != nil {
		return nil, fmt.Errorf("list artifact subdomains: %w", err)
	}
	if len(routes) > 0 {
		return routes[0], nil
	}
	base := s.vhostBaseDomain()
	hostname, err := vhost.AllocateDefaultSubdomain(r.Context(), artifact.Name, base, s.vhostReserveOpts(), 50)
	if err != nil {
		return nil, fmt.Errorf("allocate artifact subdomain: %w", err)
	}
	now := time.Now()
	route := &types.VHostRoute{
		Hostname: hostname, TargetKind: types.VHostTargetArtifact, TargetID: artifact.ID,
		IsDefault: true, VerifiedAt: &now,
	}
	if err := s.Store.CreateVHostRoute(r.Context(), route); err != nil {
		return nil, fmt.Errorf("create artifact subdomain: %w", err)
	}
	return route, nil
}

func (s *HelixAPIServer) populateArtifactURLs(r *http.Request, artifact *types.Artifact) error {
	origin := artifactRequestOrigin(r, s.Cfg.WebServer.URL)
	artifact.URL = strings.TrimRight(origin, "/") + "/artifacts/" + artifact.ID + "/"
	routes, err := s.Store.ListVHostRoutesByTarget(r.Context(), types.VHostTargetArtifact, artifact.ID)
	if err != nil {
		return fmt.Errorf("list artifact subdomains: %w", err)
	}
	if len(routes) > 0 && artifact.Visibility == types.ArtifactVisibilityPublic {
		artifact.SubdomainURL = artifactVHostOrigin(origin, routes[0].Hostname) + "/"
	}
	return nil
}

func artifactRequestOrigin(r *http.Request, configured string) string {
	if configured = strings.TrimRight(strings.TrimSpace(configured), "/"); configured != "" {
		return configured
	}
	if r.Host != "" {
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		return scheme + "://" + r.Host
	}
	return ""
}

// serveArtifactPath serves an artifact on the canonical Helix origin. Public
// artifacts need no login; project artifacts inherit project read access.
func (s *HelixAPIServer) serveArtifactPath(w http.ResponseWriter, r *http.Request) {
	artifactID := mux.Vars(r)["artifact_id"]
	artifact, err := s.Store.GetArtifact(r.Context(), artifactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if artifact.Visibility != types.ArtifactVisibilityPublic {
		user := getRequestUser(r)
		if user == nil || user.ID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if err := s.authorizeUserToProjectByID(r.Context(), user, artifact.ProjectID, types.ActionGet); err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		routes, err := s.Store.ListVHostRoutesByTarget(r.Context(), types.VHostTargetArtifact, artifact.ID)
		if err != nil || len(routes) == 0 {
			http.Error(w, "private artifact origin unavailable", http.StatusServiceUnavailable)
			return
		}
		redirectPath, err := artifactAccessRedirectPath(mux.Vars(r)["artifact_path"], r.URL.RawQuery)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		token, err := s.signArtifactAccessToken(user.ID, artifact.ID, redirectPath, artifactBootstrapAud, artifactBootstrapTTL)
		if err != nil {
			http.Error(w, "create artifact access", http.StatusInternalServerError)
			return
		}
		origin := artifactRequestOrigin(r, s.Cfg.WebServer.URL)
		action := artifactVHostOrigin(origin, routes[0].Hostname) + "/_helix/access"
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; form-action "+action+"; base-uri 'none'; frame-ancestors 'self'")
		if r.Method == http.MethodHead {
			return
		}
		if err := artifactAccessBootstrapTemplate.Execute(w, struct{ Action, Token string }{Action: action, Token: token}); err != nil {
			log.Warn().Err(err).Str("artifact_id", artifact.ID).Msg("render artifact access bootstrap")
		}
		return
	}
	s.serveArtifactFile(w, r, artifact, mux.Vars(r)["artifact_path"], true)
}

func artifactAccessRedirectPath(requested, rawQuery string) (string, error) {
	requested = strings.TrimPrefix(requested, "/")
	rawQuery = strings.TrimPrefix(rawQuery, "?")
	if requested == "" {
		redirectPath := "/"
		if rawQuery != "" {
			redirectPath += "?" + rawQuery
		}
		return redirectPath, nil
	}
	trailingSlash := strings.HasSuffix(requested, "/")
	if trailingSlash {
		requested = strings.TrimSuffix(requested, "/")
	}
	cleaned, err := normalizeArtifactPath(requested)
	if err != nil {
		return "", err
	}
	redirectPath := "/" + cleaned
	if trailingSlash {
		redirectPath += "/"
	}
	if rawQuery != "" {
		redirectPath += "?" + rawQuery
	}
	return redirectPath, nil
}

func artifactOriginScheme(origin string) string {
	if parsed, err := url.Parse(origin); err == nil && parsed.Scheme == "http" {
		return "http"
	}
	return "https"
}

func artifactVHostOrigin(origin, hostname string) string {
	host := hostname
	if parsed, err := url.Parse(origin); err == nil && parsed.Port() != "" {
		host += ":" + parsed.Port()
	}
	return artifactOriginScheme(origin) + "://" + host
}

func (s *HelixAPIServer) signArtifactAccessToken(userID, artifactID, artifactPath, audience string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := artifactAccessClaims{
		ArtifactID: artifactID, ArtifactPath: artifactPath,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "helix", Subject: userID, Audience: jwt.ClaimStrings{audience},
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.Cfg.Auth.Regular.JWTSecret))
}

func (s *HelixAPIServer) parseArtifactAccessToken(raw, artifactID, audience string) (*artifactAccessClaims, error) {
	claims := &artifactAccessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.Cfg.Auth.Regular.JWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("helix"), jwt.WithAudience(audience))
	if err != nil || !token.Valid {
		return nil, errors.New("invalid artifact access token")
	}
	if claims.ArtifactID != artifactID || claims.Subject == "" {
		return nil, errors.New("artifact access token does not match artifact")
	}
	return claims, nil
}

func (s *HelixAPIServer) servePrivateArtifactVHost(w http.ResponseWriter, r *http.Request, artifact *types.Artifact, requestedPath string) {
	if requestedPath == "_helix/access" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid artifact access", http.StatusBadRequest)
			return
		}
		raw := r.FormValue("token")
		claims, err := s.parseArtifactAccessToken(raw, artifact.ID, artifactBootstrapAud)
		if err != nil {
			http.Error(w, "invalid artifact access", http.StatusUnauthorized)
			return
		}
		session, err := s.signArtifactAccessToken(claims.Subject, artifact.ID, "", artifactSessionAud, artifactSessionTTL)
		if err != nil {
			http.Error(w, "create artifact session", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: artifactAccessCookie, Value: session, Path: "/", MaxAge: int(artifactSessionTTL.Seconds()),
			Secure: NewCookieManager(s.Cfg).SecureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, claims.ArtifactPath, http.StatusSeeOther)
		return
	}
	cookie, err := r.Cookie(artifactAccessCookie)
	if err != nil {
		http.Error(w, "artifact access required", http.StatusUnauthorized)
		return
	}
	claims, err := s.parseArtifactAccessToken(cookie.Value, artifact.ID, artifactSessionAud)
	if err != nil {
		http.Error(w, "artifact access required", http.StatusUnauthorized)
		return
	}
	if err := s.authorizeUserToProjectByID(r.Context(), &types.User{ID: claims.Subject}, artifact.ProjectID, types.ActionGet); err != nil {
		http.Error(w, "artifact access denied", http.StatusForbidden)
		return
	}
	s.serveArtifactFile(w, r, artifact, requestedPath, false)
}

func (s *HelixAPIServer) serveArtifactFile(w http.ResponseWriter, r *http.Request, artifact *types.Artifact, requestedPath string, sandboxed bool) {
	filename, metadata, ok := resolveArtifactFile(artifact, requestedPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	etag := `"` + artifact.ActiveVersion.ID + ":" + metadata.SHA256 + `"`
	setArtifactSecurityHeaders(w.Header(), sandboxed)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", metadata.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(metadata.Size, 10))
		return
	}
	reader, err := s.Controller.Options.Filestore.OpenFile(r.Context(), path.Join(artifact.ActiveVersion.StoragePrefix, filename))
	if err != nil {
		http.Error(w, "artifact file unavailable", http.StatusInternalServerError)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", metadata.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(metadata.Size, 10))
	if _, err := io.Copy(w, reader); err != nil {
		log.Warn().Err(err).Str("artifact_id", artifact.ID).Str("path", filename).Msg("serve artifact file")
	}
}

func resolveArtifactFile(artifact *types.Artifact, requested string) (string, types.ArtifactFile, bool) {
	requested = strings.TrimPrefix(requested, "/")
	directoryRequest := strings.HasSuffix(requested, "/")
	if requested == "" {
		requested = artifact.Entrypoint
	} else if directoryRequest {
		requested += "index.html"
	}
	cleaned, err := normalizeArtifactPath(requested)
	if err != nil {
		return "", types.ArtifactFile{}, false
	}
	for _, file := range artifact.ActiveVersion.Files {
		if file.Path == cleaned {
			return cleaned, file, true
		}
	}
	if artifact.Kind == types.ArtifactKindSPA && (path.Ext(cleaned) == "" || directoryRequest) {
		for _, file := range artifact.ActiveVersion.Files {
			if file.Path == artifact.Entrypoint {
				return artifact.Entrypoint, file, true
			}
		}
	}
	return "", types.ArtifactFile{}, false
}

func setArtifactSecurityHeaders(header http.Header, sandboxed bool) {
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	if sandboxed {
		// CSP sandbox gives the document an opaque origin. ES modules therefore
		// require an explicit CORS response even when loaded from the artifact's
		// canonical URL. These files are public; credentials remain unavailable to
		// the opaque origin and private artifacts are served on an isolated host.
		header.Set("Access-Control-Allow-Origin", "*")
		header.Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-modals allow-popups allow-downloads; default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'none'; frame-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	} else {
		header.Set("Content-Security-Policy", "default-src 'self' data: blob: https:; script-src 'self' 'unsafe-inline' 'unsafe-eval' https:; style-src 'self' 'unsafe-inline' https:; img-src 'self' data: blob: https:; font-src 'self' data: https:; connect-src 'self' https: wss:; object-src 'none'; base-uri 'self'")
	}
}

func writeArtifactHTTPError(w http.ResponseWriter, status int, message string) {
	writeArtifactJSON(w, status, &types.APIError{Message: message, Type: "artifact_error"})
}

func writeArtifactJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(value)
	}
}
