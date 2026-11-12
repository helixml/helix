package teams

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/infracloudio/msbotbuilder-go/core"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

// With no App ID the Bot Framework SDK treats authentication as disabled and
// accepts an Authorization header that is not a Bot Framework JWT. The
// activity handler must refuse to serve in that mode rather than process an
// unsigned activity.
func TestHandleActivity_RefusesWithoutBotCredentials(t *testing.T) {
	adapter, err := core.NewBotAdapter(core.AdapterSetting{})
	require.NoError(t, err)

	for name, trigger := range map[string]*types.TeamsTrigger{
		"nil trigger":    nil,
		"empty app id":   {AppPassword: "pw"},
		"empty password": {AppID: "app"},
		"blank app id":   {AppID: " ", AppPassword: "pw"},
	} {
		t.Run(name, func(t *testing.T) {
			bot := &TeamsBot{app: &types.App{ID: "app-1"}, trigger: trigger, adapter: adapter, tokenCache: &tokenCache{}}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/webhook/app-1",
				strings.NewReader(`{"type":"message","text":"hi","serviceUrl":"https://smba.trafficmanager.net/"}`))
			req.Header.Set("Authorization", "not-a-bot-framework-token")
			rr := httptest.NewRecorder()

			bot.HandleActivity(rr, req)

			require.Equal(t, http.StatusServiceUnavailable, rr.Code, rr.Body.String())
		})
	}
}
