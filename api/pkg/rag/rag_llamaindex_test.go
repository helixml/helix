package rag

import (
	"context"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRAG(t *testing.T) {
	indexURL := os.Getenv("RAG_INDEX_URL")
	queryURL := os.Getenv("RAG_QUERY_URL")
	if indexURL == "" || queryURL == "" {
		t.Skip("RAG_INDEX_URL or RAG_QUERY_URL not set, skipping unit tests")
	}

	indexer := NewLlamaindex(indexURL, queryURL)

	ctx := context.Background()

	dataSharksID := system.GenerateDataEntityID()

	contentSharks := `The researcher, who works for the Biopixel Oceans Foundation, said tiger sharks were an “opportunistic predator” that would “eat whatever they think they can overpower or whatever thing they think is nutritious”.

	The sharks will visit temperate waters seasonally and are known to frequent waters in south western Western Australia, around the tropical north and down the east coast until the southern coast of New South Wales.`

	dataTaylorID := system.GenerateDataEntityID()
	contentTayloer := `My first thought is, why on earth would you want to attempt to spend a month Swift-free? I’ve barely had a chance to properly listen to The Tortured Poets Department yet and was all set to stream the Eras film with my daughters (we couldn’t get concert tickets). But I gamely accept the challenge. First things first: I resolve to stay off Spotify, swerve the Guardian’s music desk whenever possible and unsubscribe from our Swift Notes newsletter.`

	t.Run("Index", func(t *testing.T) {
		// Index some text
		err := indexer.Index(ctx, &types.SessionRAGIndexChunk{
			DataEntityID:    dataSharksID,
			Filename:        "test_file_1.txt",
			DocumentID:      "test_doc_1",
			DocumentGroupID: "test_group_1",
			ContentOffset:   0,
			Content:         contentSharks,
		})
		require.NoError(t, err)

		err = indexer.Index(ctx, &types.SessionRAGIndexChunk{
			DataEntityID:    dataTaylorID,
			Filename:        "test_file_2.txt",
			DocumentID:      "test_doc_2",
			DocumentGroupID: "test_group_2",
			ContentOffset:   0,
			Content:         contentTayloer,
		})
		require.NoError(t, err)
	})

	t.Run("Query_SameDataEntityID", func(t *testing.T) {
		// Query the same data entity ID
		results, err := indexer.Query(ctx, &types.SessionRAGQuery{
			DataEntityID: dataSharksID,
			Prompt:       "shark",
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "test_file_1.txt", results[0].Filename)
		require.Equal(t, "test_doc_1", results[0].DocumentID)
		require.Equal(t, "test_group_1", results[0].DocumentGroupID)

		assert.Contains(t, results[0].Content, "tiger sharks were an “opportunistic predator” that would")

	})

	t.Run("Query_DifferentDataEntityID", func(t *testing.T) {
		// Query the same data entity ID
		results, err := indexer.Query(ctx, &types.SessionRAGQuery{
			DataEntityID: "something-else",
			Prompt:       "shark",
		})
		require.NoError(t, err)
		require.Len(t, results, 0)
	})
}
