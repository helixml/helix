package searxng

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func Test_Search(t *testing.T) {
	s := NewSearxNG(&Config{
		BaseURL:    "http://localhost:8112",
		MaxResults: 10,
	})

	results, err := s.Search(context.Background(), &SearchRequest{
		Queries: []SearchQuery{{Query: "golang", Category: GeneralCategory}},
	})
	require.NoError(t, err)

	spew.Dump(results)
	t.Errorf("results: %v", results)
}
