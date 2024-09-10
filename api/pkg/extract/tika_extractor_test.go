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

		assert.Contains(t,
			text,
			"Start the engine, pull the clutch lever in, and shift the transmission into gear.")

		assert.Contains(t,
			text,
			"Check the condition of the brake pad wear indicators.")
	})
}
