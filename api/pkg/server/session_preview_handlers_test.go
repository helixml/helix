package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestSetVHostRouteURL(t *testing.T) {
	for _, tc := range []struct {
		name        string
		https       bool
		serverURL   string
		hostname    string
		expectedURL string
	}{
		{
			name:        "https by default",
			https:       true,
			serverURL:   "https://helix.example.com",
			hostname:    "share-test.preview.example.com",
			expectedURL: "https://share-test.preview.example.com",
		},
		{
			name:        "project web service with development port",
			https:       false,
			serverURL:   "http://100.108.100.25:8080/",
			hostname:    "manual-test-standalone-html.ns.helix.ml",
			expectedURL: "http://manual-test-standalone-html.ns.helix.ml:8080",
		},
		{
			name:        "https with nonstandard port",
			https:       true,
			serverURL:   "https://helix.example.com:8443",
			hostname:    "share-test.preview.example.com",
			expectedURL: "https://share-test.preview.example.com:8443",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &HelixAPIServer{Cfg: &config.ServerConfig{}}
			s.Cfg.WebServer.PreviewURLHTTPS = tc.https
			s.Cfg.WebServer.URL = tc.serverURL
			route := &types.VHostRoute{Hostname: tc.hostname}

			s.setVHostRouteURL(route)

			require.Equal(t, tc.expectedURL, route.URL)
		})
	}
}
