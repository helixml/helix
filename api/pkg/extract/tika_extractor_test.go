package extract

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTika_Extract(t *testing.T) {
	u := os.Getenv("TEXT_EXTRACTION_TIKA_URL")
	if u == "" {
		u = "http://localhost:9998"
	}

	extractor := NewTikaExtractor(u)

	t.Run("ExtractURL", func(t *testing.T) {
		ctx := context.Background()
		text, err := extractor.Extract(ctx, &ExtractRequest{
			URL: "https://www.theguardian.com/environment/article/2024/jun/06/tiger-shark-regurgitates-eats-echidna-australia",
		})
		require.NoError(t, err)

		assert.Contains(t,
			text,
			"# Tiger shark regurgitates whole echidna, leaving Australian scientists")

		assert.Contains(t,
			text,
			"The last thing a group of scientists busy tagging marine animals along the coast")

		assert.Contains(t,
			text,
			"The echidna incident showed a connection between terrestrial and marine food webs")
	})

	t.Run("ExtractContent", func(t *testing.T) {
		ctx := context.Background()

		bts, err := os.ReadFile("./testdata/cb750.pdf")
		require.NoError(t, err)

		text, err := extractor.Extract(ctx, &ExtractRequest{
			Content: bts,
		})
		require.NoError(t, err)

		assert.Contains(t,
			text,
			"Front  Inspect the brake pads from in front")

		assert.Contains(t,
			text,
			"Check that the side stand operates")
	})

	t.Run("ExtractContent_HR_Guide", func(t *testing.T) {
		ctx := context.Background()

		bts, err := os.ReadFile("./testdata/hr_guide.pdf")
		require.NoError(t, err)

		text, err := extractor.Extract(ctx, &ExtractRequest{
			Content: bts,
		})
		require.NoError(t, err)

		err = os.WriteFile("out.md", []byte(text), 0o644)
		require.NoError(t, err)

		assert.Contains(t,
			text,
			"to thriving communities. And in doing so, you help build a stronger, more resilient")

		assert.Contains(t,
			text,
			"This policy applies to bona fide non-occupational illnesses and injuries")
	})
}
