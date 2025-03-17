package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// uiAt godoc
// @Summary uiAt
// @Description uiAt
// @Tags    ui
// @Success 200 {object} types.UIAtResponse
// @Param q    query string true "Query string"
// @Param app_id    query string true "App ID"
// @Router /api/v1/ui/at [get]
func (s *HelixAPIServer) uiAt(_ http.ResponseWriter, r *http.Request) (*types.UIAtResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	var data []types.UIAtData
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
					data = append(data, types.UIAtData{
						Label: label,
						Value: rag.BuildDocumentID(doc.DocumentID),
					})
				}
			}
		}
	}

	// Now filter down all results to only include the ones that match the query
	filteredData := []types.UIAtData{}
	for _, d := range data {
		if strings.Contains(strings.ToLower(d.Label), strings.ToLower(q)) {
			filteredData = append(filteredData, d)
		}
	}

	return &types.UIAtResponse{
		Data: filteredData,
	}, nil
}
