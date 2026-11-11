package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// A password signup on an OIDC deployment bypasses the SSO domain allow list
// and waitlist, so the register endpoint must refuse it before touching the
// store (the server here has no store: reaching it would panic).
func TestRegister_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderOIDC
	cfg.Auth.RegistrationEnabled = true
	s := &HelixAPIServer{Cfg: cfg}

	body := `{"email":"probe@example.com","password":"pw","password_confirm":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.register(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "Registration is disabled")
}
