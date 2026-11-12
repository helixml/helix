package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser/browsertest"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

const browserSkillSecret = "INTERNAL-ONLY-RESPONSE"

func internalServer(t *testing.T) (*httptest.Server, *bool) {
	hit := new(bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*hit = true
		_, _ = w.Write([]byte("<html><body>" + browserSkillSecret + "</body></html>"))
	}))
	t.Cleanup(srv.Close)
	return srv, hit
}

// The browser tool's URL comes from the model; with the default policy it
// must not fetch loopback or other internal addresses.
func TestBrowserSkill_NoBrowserFetchFollowsOutboundPolicy(t *testing.T) {
	srv, hit := internalServer(t)
	s := NewBrowserSkill(&types.ToolBrowserConfig{NoBrowser: true}, nil, nil, nil, false, netguard.Policy{})
	_, err := s.Tools[0].Execute(context.Background(), agent.Meta{}, map[string]any{"url": srv.URL, "prompt": "x"})
	require.ErrorIs(t, err, netguard.ErrBlockedDestination)
	require.False(t, *hit, "internal server was reached")
}

func TestBrowserSkill_BrowserFetchFollowsOutboundPolicy(t *testing.T) {
	b := browsertest.New(t)
	srv, hit := internalServer(t)
	s := NewBrowserSkill(&types.ToolBrowserConfig{}, b, nil, nil, false, netguard.Policy{})
	out, _ := s.Tools[0].Execute(context.Background(), agent.Meta{}, map[string]any{"url": srv.URL, "prompt": "x"})
	require.False(t, strings.Contains(out, browserSkillSecret), "browser returned internal content: %q", out)
	require.False(t, *hit, "internal server was reached")

	// The same browser still renders pages the policy allows.
	allowing := NewBrowserSkill(&types.ToolBrowserConfig{}, b, nil, nil, false, netguard.Policy{AllowPrivate: true})
	out, err := allowing.Tools[0].Execute(context.Background(), agent.Meta{}, map[string]any{"url": srv.URL, "prompt": "x"})
	require.NoError(t, err)
	require.Contains(t, out, browserSkillSecret)
}
