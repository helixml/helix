package readability

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseArticle(t *testing.T) {
	content, err := os.ReadFile("./testdata/article.html")
	require.NoError(t, err)

	parser := NewParser()
	article, err := parser.Parse(context.Background(), string(content), "https://www.theguardian.com/uk-news/2024/sep/13/plans-unveiled-for-cheaper-high-speed-alternative-to-scrapped-hs2-northern-leg")
	require.NoError(t, err)

	os.WriteFile("parsed.html", []byte(article.Content), 0o644)
}

func TestParseArticleWithCodeBlock(t *testing.T) {
	content, err := os.ReadFile("./testdata/example_code_block.html")
	require.NoError(t, err)

	parser := NewParser()
	article, err := parser.Parse(context.Background(), string(content), "https://webhookrelay.com/v1/examples/transform/multipart-form-data/")
	require.NoError(t, err)

	os.WriteFile("parsed_code_block.html", []byte(article.Content), 0o644)
}
