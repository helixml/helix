package knowledge

import (
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitData_Markdown(t *testing.T) {
	contents, err := os.ReadFile("./testdata/example_code.md")
	require.NoError(t, err)

	k := &types.Knowledge{}
	k.RAGSettings.ChunkSize = 2000

	chunks, err := splitData(k, []*indexerData{{
		Source: "example_code.md",
		Data:   contents,
	}})
	require.NoError(t, err)

	assert.Equal(t, 1, len(chunks))

	spew.Dump(chunks)
}
