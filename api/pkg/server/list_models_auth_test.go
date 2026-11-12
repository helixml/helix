package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// GET /v1/models sits behind authMiddleware.auth, which lets anonymous
// requests through. Without a caller the handler must refuse before it asks
// any provider (the provider manager is nil here, so reaching it panics).
func TestListModels_RefusesAnonymousCaller(t *testing.T) {
	st, _ := newVictimStore(t)
	s := newRouteTestServer(t, st)

	for _, target := range []string{"/v1/models", "/v1/models?provider=openai"} {
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost:8080"+target, nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code, target)
	}
}
