package extract

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTika_Extract(t *testing.T) {
	u := os.Getenv("TIKA_URL")
	if u == "" {
		u = "http://localhost:9998"
	}

	extractor := NewTikaExtractor(u)

	t.Run("ExtractContent", func(t *testing.T) {
		ctx := context.Background()

		bts, err := os.ReadFile("./testdata/cb750.pdf")
		require.NoError(t, err)

		text, err := extractor.Extract(ctx, &ExtractRequest{
			Content: bts,
		})
		require.NoError(t, err)

		t.Log(text)

		// t.Error("xx")

		// fmt.Println(text)

		// os.WriteFile("out_2.md", []byte(text), 0o644)

		assert.Contains(t,
			text,
			"Front  Inspect the brake pads from in front")

		assert.Contains(t,
			text,
			"Check that the side stand operates")
	})
}
