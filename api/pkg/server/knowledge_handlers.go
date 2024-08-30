package server

import (
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

	knowledge, err := s.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
		AppID:     appID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return knowledge, nil
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
		return nil, system.NewHTTPError403("you do not have permission to delete this knowledge")
	}

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

	// Get rag client
	ragClient, err := s.Controller.GetRagClient(r.Context(), existing)
	if err != nil {
		log.Error().Err(err).Msg("error getting rag client")
	} else {
		err = ragClient.Delete(r.Context(), &types.DeleteIndexRequest{
			DataEntityID: existing.GetDataEntityID(),
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", existing.ID).
				Str("data_entity_id", existing.GetDataEntityID()).
				Msg("error deleting knowledge")
		}
	}

	err = s.Store.DeleteKnowledge(r.Context(), id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return existing, nil
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
