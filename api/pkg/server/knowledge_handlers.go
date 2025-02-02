package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

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
