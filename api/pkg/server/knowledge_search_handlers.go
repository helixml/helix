package server

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

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
		return nil, system.NewHTTPError500(err.Error())
	}

	var (
		results   []*types.KnowledgeSearchResult
		resultsMu sync.Mutex
	)

	if len(knowledges) == 0 {
		return results, nil
	}

	pool := pool.New().
		WithMaxGoroutines(20).
		WithErrors()

	for _, knowledge := range knowledges {
		knowledge := knowledge

		client, err := s.Controller.GetRagClient(ctx, knowledge)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		pool.Go(func() error {
			resp, err := client.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
			})
			if err != nil {
				return fmt.Errorf("error querying RAG for knowledge %s: %w", knowledge.ID, err)
			}

			resultsMu.Lock()
			if len(resp) == 0 {
				resp = []*types.SessionRAGResult{}
			}

			results = append(results, &types.KnowledgeSearchResult{
				KnowledgeID: knowledge.ID,
				Results:     resp,
			})
			resultsMu.Unlock()

			return nil
		})
	}

	err = pool.Wait()
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return results, nil
}
