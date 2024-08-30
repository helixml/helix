package text

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownSplitter(t *testing.T) {
	f, err := os.Open("sample_web.md")
	require.NoError(t, err)

	content, err := io.ReadAll(f)
	require.NoError(t, err)

	splitter := NewMarkdownSplitter(2024, 256)
	docs, err := splitter.SplitText(string(content))
	require.NoError(t, err)

	// Validate some chunks
	assert.Contains(t, docs[2], "## [Introduction]")
	assert.Contains(t, docs[2], "Most of the AI services out there consists of pretty similar parts. You have a frontend, a backend, a database and a machine learning model. In this guide we will show you how to setup a ChatGPT style service with")

	assert.Contains(t, docs[9], "## [Connecting it all together]")
	assert.Contains(t, docs[9], "To test Stripe top-ups locally, use ")
}
