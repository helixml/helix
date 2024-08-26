package text

import (
	"github.com/tmc/langchaingo/textsplitter"
)

type MarkdownSplitter struct {
	chunkSize     int
	chunkOverflow int

	splitter *textsplitter.MarkdownTextSplitter
}

func NewMarkdownSplitter(chunkSize, chunkOverflow int) *MarkdownSplitter {
	splitter := textsplitter.NewMarkdownTextSplitter(
		textsplitter.WithChunkSize(chunkSize),
		textsplitter.WithChunkOverlap(chunkOverflow),
	)

	return &MarkdownSplitter{
		chunkSize:     chunkSize,
		chunkOverflow: chunkOverflow,
		splitter:      splitter,
	}
}

func (m *MarkdownSplitter) SplitText(text string) ([]string, error) {
	return m.splitter.SplitText(text)
}
