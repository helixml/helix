package crawler

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/types"

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

	fmt.Println(docs[0].Content)
}
