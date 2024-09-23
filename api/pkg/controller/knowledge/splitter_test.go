package knowledge

import (
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitData_Markdown(t *testing.T) {
	contents, err := os.ReadFile("./testdata/example_code.md")
	require.NoError(t, err)

	k := &types.Knowledge{}
	k.RAGSettings.ChunkSize = 2000
	k.RAGSettings.ChunkOverflow = 20
	k.RAGSettings.TextSplitter = types.TextSplitterTypeMarkdown

	chunks, err := splitData(k, []*indexerData{{
		Source: "example_code.md",
		Data:   contents,
	}})
	require.NoError(t, err)

	assert.Equal(t, 1, len(chunks))

	assert.Contains(t, chunks[0].Text, "For example if the payload fragment looks like this:")
	assert.Contains(t, chunks[0].Text, "local encoded_payload, err = json.encode(json_payload)")
}
