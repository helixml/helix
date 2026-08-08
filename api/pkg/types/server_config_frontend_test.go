package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerConfigForFrontendIncludesPreviewConfiguration(t *testing.T) {
	for _, https := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[https], func(t *testing.T) {
			payload, err := json.Marshal(ServerConfigForFrontend{PreviewURLHTTPS: https})
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(payload, &got))
			require.Contains(t, got, "dev_subdomain")
			require.Equal(t, "", got["dev_subdomain"])
			require.Contains(t, got, "preview_url_https")
			require.Equal(t, https, got["preview_url_https"])
		})
	}
}
