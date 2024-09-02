package crawler

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault_Crawl(t *testing.T) {
	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://docs.helix.ml/helix"},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
				Excludes: []string{"searchbot/*"},
			},
		},
	}

	d, err := NewDefault(k)
	require.NoError(t, err)

	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)

	const (
		appsText              = `When I submit a request that uses an App, it hangs`
		privateDeploymentText = `The stack might take a minute to boot up`
	)

	var (
		appsTextFound              bool
		privateDeploymentTextFound bool
	)

	for _, doc := range docs {
		// Uncomment to save the chunks to a file for debugging
		// os.WriteFile(fmt.Sprintf("doc-%s.html", doc.Title), []byte(doc.Content), 0644)

		if strings.Contains(doc.Content, appsText) {
			appsTextFound = true

			assert.Equal(t, doc.SourceURL, "https://docs.helix.ml/helix/develop/apps/")
		}
		if strings.Contains(doc.Content, privateDeploymentText) {
			privateDeploymentTextFound = true

			assert.Equal(t, doc.SourceURL, "https://docs.helix.ml/helix/private-deployment/controlplane/")
		}
	}

	require.True(t, appsTextFound, "apps text not found")
	require.True(t, privateDeploymentTextFound, "private deployment text not found")

	t.Logf("docs: %d", len(docs))
}
