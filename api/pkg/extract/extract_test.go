package extract

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractURL(t *testing.T) {
	u := os.Getenv("TEXT_EXTRACTION_URL")
	if u == "" {
		t.Skip("TEXT_EXTRACTION_URL not set, skipping unit tests")
	}

	extractor := NewDefaultExtractor(u)

	t.Run("Extract URL", func(t *testing.T) {
		ctx := context.Background()
		text, err := extractor.Extract(ctx, &ExtractRequest{
			URL: "https://www.theguardian.com/environment/article/2024/jun/06/tiger-shark-regurgitates-eats-echidna-australia",
		})
		require.NoError(t, err)

		assert.Contains(t,
			text,
			"# Tiger shark regurgitates whole echidna, leaving Australian scientists ‘stunned’")

		assert.Contains(t,
			text,
			"The last thing a group of scientists busy tagging marine animals along the coast of north Queensland expected to see was a shark regurgitate a fully intact echidna – but that is exactly what happened")

		assert.Contains(t,
			text,
			"The echidna incident showed a connection between terrestrial and marine food webs, he said – and “we don’t really understand what the overlap” between the two is yet.")
	})
}
