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
			},
		},
	}

	d, err := NewDefault(k)
	require.NoError(t, err)

	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)

	const (
		apiToolsText          = `By connecting to external APIs, your AI can access up-to-date information that may not be present in its training data`
		privateDeploymentText = `The stack might take a minute to boot up`
	)

	var (
		apiToolsTextFound          bool
		privateDeploymentTextFound bool
	)

	for _, doc := range docs {
		if strings.Contains(doc.Content, apiToolsText) {
			apiToolsTextFound = true

			assert.Equal(t, doc.SourceURL, "https://docs.helix.ml/helix/develop/helix-tools/")
		}
		if strings.Contains(doc.Content, privateDeploymentText) {
			privateDeploymentTextFound = true

			assert.Equal(t, doc.SourceURL, "https://docs.helix.ml/helix/private-deployment/controlplane/")
		}
	}

	require.True(t, apiToolsTextFound, "api tools text not found")
	require.True(t, privateDeploymentTextFound, "private deployment text not found")
}
