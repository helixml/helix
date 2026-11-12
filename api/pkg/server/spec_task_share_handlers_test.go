package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Public design docs are rendered on the API origin from agent-written
// markdown, which may contain raw HTML. Scripts and event handlers must not
// survive, and the page must forbid script execution outright.
func TestViewDesignDocsPublic_SanitizesMarkdownHTML(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{
		ID:                 "spt_1",
		Name:               "task",
		Status:             types.TaskStatusSpecReview,
		PublicDesignDocs:   true,
		RequirementsSpec:   "# Requirements\n\n<script>alert('req')</script>\n\n**kept**",
		TechnicalDesign:    `<img src="x" onerror="alert('design')">`,
		ImplementationPlan: `[click](javascript:alert('plan'))`,
	}, nil)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	req := httptest.NewRequest(http.MethodGet, "/spec-tasks/spt_1/view", nil)
	req = mux.SetURLVars(req.WithContext(context.Background()), map[string]string{"id": "spt_1"})
	rr := httptest.NewRecorder()
	server.viewDesignDocsPublic(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.NotContains(t, body, "<script>alert")
	require.NotContains(t, body, "onerror")
	require.NotContains(t, body, "javascript:alert")
	require.Contains(t, body, "<strong>kept</strong>")
	require.Contains(t, rr.Header().Get("Content-Security-Policy"), "default-src 'none'")
}
