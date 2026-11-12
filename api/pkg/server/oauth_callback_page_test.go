package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOAuthPopupNotifyScript_TargetsOwnOriginAndEncodesPayload(t *testing.T) {
	script := oauthPopupNotifyScript(map[string]string{
		"type":         "oauth-success",
		"connectionId": "</script><script>alert(1)</script>",
		"providerId":   "it's",
	})

	require.Contains(t, script, "window.location.origin")
	require.NotContains(t, script, "'*'")
	require.NotContains(t, script, "</script>")
	require.Contains(t, script, `\u003c/script\u003e`)
	require.Contains(t, script, `"providerId":"it's"`)
}

func TestHandleOAuthCallback_ErrorPagePostsOnlyToOwnOrigin(t *testing.T) {
	s := &HelixAPIServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/flow/callback?error=access_denied", nil)
	rec := httptest.NewRecorder()

	s.handleOAuthCallback(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `"type":"oauth-failure"`)
	require.Contains(t, body, "postMessage(")
	require.Contains(t, body, ", window.location.origin);")
	require.NotContains(t, body, "'*'")
}
