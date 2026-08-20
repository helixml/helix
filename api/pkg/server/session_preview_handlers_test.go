package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestSetSessionPreviewURL(t *testing.T) {
	for _, tc := range []struct {
		name        string
		https       bool
		serverURL   string
		expectedURL string
	}{
		{
			name:        "https by default",
			https:       true,
			serverURL:   "https://helix.example.com",
			expectedURL: "https://share-test.preview.example.com",
		},
		{
			name:        "http with development port",
			https:       false,
			serverURL:   "http://localhost:8080",
			expectedURL: "http://share-test.preview.example.com:8080",
		},
		{
			name:        "https with nonstandard port",
			https:       true,
			serverURL:   "https://helix.example.com:8443",
			expectedURL: "https://share-test.preview.example.com:8443",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &HelixAPIServer{Cfg: &config.ServerConfig{}}
			s.Cfg.WebServer.PreviewURLHTTPS = tc.https
			s.Cfg.WebServer.URL = tc.serverURL
			route := &types.VHostRoute{Hostname: "share-test.preview.example.com"}

			s.setSessionPreviewURL(route)

			require.Equal(t, tc.expectedURL, route.URL)
		})
	}
}
