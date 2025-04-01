package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"sort"

	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// knowledgeSearch godoc
// @Summary Search knowledges
// @Description Search knowledges for a given app and prompt
// @Tags knowledge
// @Param app_id query string true "App ID"
// @Param knowledge_id query string false "Knowledge ID"
// @Param prompt query string true "Search prompt"
// @Success 200 {array} types.KnowledgeSearchResult
// @Router /api/v1/search [get]
// @Security BearerAuth
func (s *HelixAPIServer) knowledgeSearch(_ http.ResponseWriter, r *http.Request) ([]*types.KnowledgeSearchResult, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	appID := r.URL.Query().Get("app_id")             // Required (for now, we can relax this later)
	knowledgeID := r.URL.Query().Get("knowledge_id") // Optional knowledge ID to search within
	prompt := r.URL.Query().Get("prompt")            // Search query

	knowledges, err := s.Controller.Options.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		AppID: appID,
		Owner: user.ID,
		ID:    knowledgeID,
	})
	if err != nil {
		log.Error().Err(err).Msgf("error listing knowledges for app %s", appID)
		return nil, system.NewHTTPError500(err.Error())
	}

	var (
		results   []*types.KnowledgeSearchResult
		resultsMu sync.Mutex
	)

	if len(knowledges) == 0 {
		// Make an empty results list
		log.Warn().Msg("no knowledges found for app")
		return []*types.KnowledgeSearchResult{}, nil
	}

	log.Info().
		Str("app_id", appID).
		Str("knowledge_id", knowledgeID).
		Str("prompt", prompt).
		Msg("searching knowledges")

	pool := pool.New().
		WithMaxGoroutines(20).
		WithErrors()

	for _, knowledge := range knowledges {
		knowledge := knowledge

		client, err := s.Controller.GetRagClient(ctx, knowledge)
		if err != nil {
			log.Error().Err(err).Msgf("error getting RAG client for knowledge %s", knowledge.ID)
			return nil, system.NewHTTPError500(err.Error())
		}

		pool.Go(func() error {
			start := time.Now()
			filterActions := rag.ParseFilterActions(prompt)
			filterDocumentIDs := make([]string, 0)
			for _, filterAction := range filterActions {
				filterDocumentIDs = append(filterDocumentIDs, rag.ParseDocID(filterAction))
			}
			log.Trace().Interface("filterDocumentIDs", filterDocumentIDs).Msg("filterDocumentIDs")
			pipeline := types.TextPipeline
			if knowledge.RAGSettings.EnableVision {
				pipeline = types.VisionPipeline
			}
			resp, err := client.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
				DocumentIDList:    filterDocumentIDs,
				Pipeline:          pipeline,
			})
			if err != nil {
				log.Error().Err(err).Msgf("error querying RAG for knowledge %s", knowledge.ID)
				return fmt.Errorf("error querying RAG for knowledge %s: %w", knowledge.ID, err)
			}

			resultsMu.Lock()
			if len(resp) == 0 {
				resp = []*types.SessionRAGResult{}
			}

			results = append(results, &types.KnowledgeSearchResult{
				Knowledge:  knowledge,
				Results:    resp,
				DurationMs: time.Since(start).Milliseconds(),
			})
			resultsMu.Unlock()

			return nil
		})
	}

	err = pool.Wait()
	if err != nil {
		log.Error().Err(err).Msg("error waiting for RAG queries")
		return nil, system.NewHTTPError500(err.Error())
	}

	// Sort the results
	sort.Slice(results, func(i, j int) bool {
		// First, sort by number of entries (descending order)
		if len(results[i].Results) != len(results[j].Results) {
			return len(results[i].Results) > len(results[j].Results)
		}
		// If number of entries is the same, sort by knowledge ID alphabetically
		return results[i].Knowledge.ID < results[j].Knowledge.ID
	})

	return results, nil
}
