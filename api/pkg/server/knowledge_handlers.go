package server

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listKnowledge godoc
// @Summary List knowledge
// @Description List knowledge
// @Tags    knowledge

// @Success 200 {array} types.Knowledge
// @Router /api/v1/knowledge [get]
// @Security BearerAuth
func (s *HelixAPIServer) listKnowledge(_ http.ResponseWriter, r *http.Request) ([]*types.Knowledge, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	appID := r.URL.Query().Get("app_id")

	knowledges, err := s.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
		AppID:     appID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	for idx, knowledge := range knowledges {
		knowledge.Progress = s.knowledgeManager.GetStatus(knowledge.ID)

		if knowledge.RefreshEnabled && knowledge.RefreshSchedule != "" {
			nextRun, err := s.knowledgeManager.NextRun(ctx, knowledge.ID)
			if err != nil {
				log.Error().Err(err).Msg("error getting next run")
			}
			knowledges[idx].NextRun = nextRun
		}
	}

	return knowledges, nil
}

// getKnowledge godoc
// @Summary Get knowledge
// @Description Get knowledge
// @Tags    knowledge

// @Success 200 {object} types.Knowledge
// @Router /api/v1/knowledge/{id} [get]
func (s *HelixAPIServer) getKnowledge(_ http.ResponseWriter, r *http.Request) (*types.Knowledge, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	existing, err := s.Store.GetKnowledge(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != user.ID {
		return nil, system.NewHTTPError403("you do not have permission to view this knowledge")
	}

	// Ephemeral progress from the knowledge manager
	existing.Progress = s.knowledgeManager.GetStatus(id)

	return existing, nil
}

// listKnowledgeVersions godoc
// @Summary List knowledge versions
// @Description List knowledge versions
// @Tags    knowledge
// @Success 200 {array} types.KnowledgeVersion
// @Router /api/v1/knowledge/{id}/versions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listKnowledgeVersions(_ http.ResponseWriter, r *http.Request) ([]*types.KnowledgeVersion, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	existing, err := s.Store.GetKnowledge(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != user.ID {
		return nil, system.NewHTTPError403("you do not have permission to delete this knowledge")
	}

	versions, err := s.Store.ListKnowledgeVersions(r.Context(), &store.ListKnowledgeVersionQuery{
		KnowledgeID: id,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return versions, nil
}

// deleteKnowledge godoc
// @Summary Delete knowledge
// @Description Delete knowledge
// @Tags    knowledge
// @Success 200 {object} types.Knowledge
// @Router /api/v1/knowledge/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteKnowledge(_ http.ResponseWriter, r *http.Request) (*types.Knowledge, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	existing, err := s.Store.GetKnowledge(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != user.ID {
		return nil, system.NewHTTPError403("you do not have permission to delete this knowledge")
	}

	err = s.deleteKnowledgeAndVersions(existing)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return existing, nil
}

func (s *HelixAPIServer) deleteKnowledgeAndVersions(k *types.Knowledge) error {
	ctx := context.Background()

	versions, err := s.Store.ListKnowledgeVersions(ctx, &store.ListKnowledgeVersionQuery{
		KnowledgeID: k.ID,
	})
	if err != nil {
		return err
	}

	// Get rag client
	ragClient, err := s.Controller.GetRagClient(ctx, k)
	if err != nil {
		log.Error().Err(err).Msg("error getting rag client")
	} else {
		err = ragClient.Delete(ctx, &types.DeleteIndexRequest{
			DataEntityID: k.GetDataEntityID(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("data_entity_id", k.GetDataEntityID()).
				Msg("error deleting knowledge")
		}
	}

	// Delete all versions from the store
	for _, version := range versions {
		err = ragClient.Delete(ctx, &types.DeleteIndexRequest{
			DataEntityID: version.GetDataEntityID(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("data_entity_id", k.GetDataEntityID()).
				Msg("error deleting knowledge version")
		}
	}

	err = s.Store.DeleteKnowledge(ctx, k.ID)
	if err != nil {
		return err
	}

	return nil
}

// refreshKnowledge godoc
// @Summary Refresh knowledge
// @Description Refresh knowledge
// @Tags    knowledge
// @Success 200 {object} types.Knowledge
// @Router /api/v1/knowledge/{id}/refresh [post]
// @Security BearerAuth
func (s *HelixAPIServer) refreshKnowledge(_ http.ResponseWriter, r *http.Request) (*types.Knowledge, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	existing, err := s.Store.GetKnowledge(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != user.ID {
		return nil, system.NewHTTPError403("you do not have permission to refresh this knowledge")
	}

	switch existing.State {
	case types.KnowledgeStateIndexing:
		return nil, system.NewHTTPError400("knowledge is already being indexed")
	case types.KnowledgeStatePending:
		return nil, system.NewHTTPError400("knowledge is queued for indexing, please wait")
	}

	// Push back to pending
	existing.State = types.KnowledgeStatePending
	existing.Message = ""

	updated, err := s.Store.UpdateKnowledge(r.Context(), existing)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return updated, nil
}

// completeKnowledgePreparation godoc
// @Summary Complete knowledge preparation
// @Description Complete knowledge preparation and move to pending state for indexing
// @Tags    knowledge
// @Success 200 {object} types.Knowledge
// @Router /api/v1/knowledge/{id}/complete [post]
// @Security BearerAuth
func (s *HelixAPIServer) completeKnowledgePreparation(_ http.ResponseWriter, r *http.Request) (*types.Knowledge, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	existing, err := s.Store.GetKnowledge(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != user.ID {
		return nil, system.NewHTTPError403("you do not have permission to complete preparation for this knowledge")
	}

	if existing.State != types.KnowledgeStatePreparing {
		return nil, system.NewHTTPError400("knowledge is not in preparing state")
	}

	// Move from preparing to pending
	existing.State = types.KnowledgeStatePending
	existing.Message = ""

	updated, err := s.Store.UpdateKnowledge(r.Context(), existing)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return updated, nil
}

// downloadKnowledgeFiles godoc
// @Summary Download knowledge files as zip
// @Description Download all files from a filestore-backed knowledge as a zip file
// @Tags    knowledge
// @Produce application/zip
// @Success 200 {file} application/zip
// @Router /api/v1/knowledge/{id}/download [get]
// @Security BearerAuth
func (s *HelixAPIServer) downloadKnowledgeFiles(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	existing, err := s.Store.GetKnowledge(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "knowledge not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if existing.Owner != user.ID {
		http.Error(w, "you do not have permission to download this knowledge", http.StatusForbidden)
		return
	}

	// Only allow download for filestore-backed knowledge
	if existing.Source.Filestore == nil {
		http.Error(w, "knowledge is not filestore-backed", http.StatusBadRequest)
		return
	}

	if existing.Source.Filestore.Path == "" {
		http.Error(w, "knowledge has no filestore path", http.StatusBadRequest)
		return
	}

	// Get the full filestore path for the knowledge
	fullPath, err := s.Controller.GetFilestoreAppKnowledgePath(types.OwnerContext{
		Owner:     existing.Owner,
		OwnerType: existing.OwnerType,
	}, existing.AppID, existing.Source.Filestore.Path)
	if err != nil {
		log.Error().Err(err).Str("knowledge_id", id).Msg("failed to get filestore path")
		http.Error(w, "failed to get filestore path", http.StatusInternalServerError)
		return
	}

	// Set appropriate headers for zip file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+existing.Name+"-files.zip\"")
	w.Header().Set("Cache-Control", "no-cache")

	// Create a zip writer that streams directly to the response
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Recursively add all files to the zip
	err = s.addFilesToZip(r.Context(), zipWriter, fullPath, "", id)
	if err != nil {
		log.Error().Err(err).Str("knowledge_id", id).Str("path", fullPath).Msg("failed to add files to zip")
		// Don't return error here as headers are already sent
		return
	}

	log.Info().Str("knowledge_id", id).Str("knowledge_name", existing.Name).Msg("successfully created zip download")
}

// addFilesToZip recursively adds files from a directory to a zip writer
func (s *HelixAPIServer) addFilesToZip(ctx context.Context, zipWriter *zip.Writer, basePath, relativePath, knowledgeID string) error {
	// Construct the full path to list
	fullPath := basePath
	if relativePath != "" {
		fullPath = filepath.Join(basePath, relativePath)
	}

	// List all files in the current directory
	files, err := s.Controller.Options.Filestore.List(ctx, fullPath)
	if err != nil {
		return err
	}

	// Process each file/directory
	for _, file := range files {
		if file.Directory {
			// Recursively process subdirectory
			subPath := relativePath
			if subPath != "" {
				subPath = filepath.Join(subPath, file.Name)
			} else {
				subPath = file.Name
			}

			err := s.addFilesToZip(ctx, zipWriter, basePath, subPath, knowledgeID)
			if err != nil {
				log.Error().Err(err).Str("knowledge_id", knowledgeID).Str("directory", subPath).Msg("failed to process directory")
				continue // Continue with other files
			}
		} else {
			// Add file to zip
			err := s.addFileToZip(ctx, zipWriter, file, relativePath, knowledgeID)
			if err != nil {
				log.Error().Err(err).Str("knowledge_id", knowledgeID).Str("file_path", file.Path).Msg("failed to add file to zip")
				continue // Continue with other files
			}
		}
	}

	return nil
}

// addFileToZip adds a single file to the zip writer
func (s *HelixAPIServer) addFileToZip(ctx context.Context, zipWriter *zip.Writer, file filestore.Item, relativePath, knowledgeID string) error {
	// Open the file from filestore
	fileReader, err := s.Controller.Options.Filestore.OpenFile(ctx, file.Path)
	if err != nil {
		return err
	}
	defer fileReader.Close()

	// Determine the path within the zip
	zipPath := filepath.Base(file.Path)
	if relativePath != "" {
		zipPath = filepath.Join(relativePath, filepath.Base(file.Path))
	}

	// Create a file in the zip
	zipFile, err := zipWriter.Create(zipPath)
	if err != nil {
		return err
	}

	// Copy file content to zip
	_, err = io.Copy(zipFile, fileReader)
	if err != nil {
		return err
	}

	log.Debug().Str("knowledge_id", knowledgeID).Str("file_name", zipPath).Msg("added file to zip")
	return nil
}
