package text

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexSplitter(t *testing.T) {
	file, err := os.Open("sample.md")
	require.NoError(t, err)
	defer file.Close()

	splitter, err := NewRegexTextSplitter()
	require.NoError(t, err)

	bts, err := io.ReadAll(file)
	require.NoError(t, err)

	splitter.AddDocument("sample.md", string(bts), "sample")

	chunks := splitter.Chunks
	assert.Equal(t, 10, len(chunks))

	// fmt.Println(chunks[0].Text)
	// fmt.Println(chunks[1].Text)
}
