package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

// Local provider detection probes the API host's ports and returns the model
// lists it finds, so it is limited to admins.
func TestDetectLocalProviders_RequiresAdmin(t *testing.T) {
	s := &HelixAPIServer{}
	for name, ctx := range map[string]context.Context{
		"anonymous": context.Background(),
		"non-admin": setRequestUser(context.Background(), types.User{ID: "u1", Type: types.OwnerTypeUser}),
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/detect-local", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		s.detectLocalProviders(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code, name)
	}
}
