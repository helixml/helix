package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// contextMenuHandler godoc
// @Summary contextMenuHandler
// @Description contextMenuHandler
// @Tags    ui
// @Success 200 {object} types.ContextMenuResponse
// @Param app_id    query string true "App ID"
// @Param q    query string false "Query string"
// @Router /api/v1/context-menu [get]
func (s *HelixAPIServer) contextMenuHandler(_ http.ResponseWriter, r *http.Request) (*types.ContextMenuResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	filteredKnowlegeSources := make(map[string]string)
	q := r.URL.Query().Get("q")

	// In the future, there's going to be lots of different things a user can do with the @. This is
	// where we'd handle that logic.

	// If the user has specified an app_id, then search for all documents inside the knowledges
	// contained in that app.
	appID := r.URL.Query().Get("app_id")
	if appID != "" {
		// Verify that the user has access to the app
		app, err := s.Store.GetApp(ctx, appID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, system.NewHTTPError404(fmt.Sprintf("App %s not found", appID))
			}
			return nil, system.NewHTTPError500(err.Error())
		}

		err = s.authorizeUserToApp(ctx, user, app, types.ActionGet)
		// If there is no error, that means the user has access to the app and we can proceed
		if err != nil {
			log.Trace().Err(err).Str("app_id", appID).Str("user_id", user.ID).Msg("User does not have access to app")
		} else {
			knowledges, err := s.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
				AppID: appID,
				Owner: user.ID,
			})
			if err != nil {
				return nil, system.NewHTTPError500(err.Error())
			}

			// For each knowledge, pull out a list of all the document IDs and add them to the filters
			for _, knowledge := range knowledges {
				if knowledge.CrawledSources != nil {
					for _, doc := range knowledge.CrawledSources.URLs {
						splatted := strings.Split(doc.URL, "/")
						var label string
						if len(splatted) >= 2 {
							label = strings.Join(splatted[len(splatted)-2:], "/")
						} else if len(splatted) >= 1 {
							label = splatted[len(splatted)-1]
						} else {
							label = doc.URL
						}
						filteredKnowlegeSources[doc.DocumentID] = label
					}
				}
			}
		}
	}

	// Now filter down all results to only include the ones that match the query
	for k, v := range filteredKnowlegeSources {
		if !strings.Contains(strings.ToLower(v), strings.ToLower(q)) {
			delete(filteredKnowlegeSources, k)
		}
	}

	// Build the final menu structure
	data := []types.ContextMenuAction{}
	for k, label := range filteredKnowlegeSources {
		data = append(data, types.ContextMenuAction{
			ActionLabel: "filter",
			Label:       label,
			Value:       rag.BuildFilterAction(label, k),
		})
	}

	// Sort the results, which is a slice, by label alphabetically to ensure consistent responses
	sort.Slice(data, func(i, j int) bool {
		return data[i].Label < data[j].Label
	})

	return &types.ContextMenuResponse{
		Data: data,
	}, nil
}
