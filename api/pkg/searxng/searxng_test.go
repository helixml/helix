package searxng

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Search(t *testing.T) {
	// TODO: setup searxng in drone
	t.Skip("Skipping searxng test")

	s := NewSearXNG(&Config{
		BaseURL: "http://localhost:8112",
	})

	results, err := s.Search(context.Background(), &SearchRequest{
		MaxResults: 10,
		Queries:    []SearchQuery{{Query: "golang", Category: GeneralCategory}},
	})
	require.NoError(t, err)

	// Will need to find something that makes sense:
	// - https://go.dev
	// - https://github.com/golang/go

	var (
		goDevDomain    = "https://go.dev"
		githubGoDomain = "https://github.com/golang/go"

		goDevFound    = false
		githubGoFound = false
	)

	for _, result := range results {
		if strings.Contains(result.URL, goDevDomain) {
			goDevFound = true
		}
		if strings.Contains(result.URL, githubGoDomain) {
			githubGoFound = true
		}
	}

	require.True(t, goDevFound)
	require.True(t, githubGoFound)
}
