package text

import (
	"fmt"
	"strings"
)

type DataPrepTextSplitterChunk struct {
	Filename string
	Index    int
	Text     string
}

type DataPrepTextSplitterOptions struct {
	ChunkSize int
	Overflow  int
}

type DataPrepTextSplitter struct {
	Options DataPrepTextSplitterOptions
	Chunks  []*DataPrepTextSplitterChunk
}

func NewDataPrepSplitter(options DataPrepTextSplitterOptions) (*DataPrepTextSplitter, error) {
	return &DataPrepTextSplitter{
		Options: options,
		Chunks:  []*DataPrepTextSplitterChunk{},
	}, nil
}

func (splitter *DataPrepTextSplitter) AddDocument(filename string, content string) error {
	parts, err := chunkWithOverflow(content, splitter.Options.ChunkSize, splitter.Options.Overflow)
	if err != nil {
		return err
	}
	for i, part := range parts {
		splitter.Chunks = append(splitter.Chunks, &DataPrepTextSplitterChunk{
			Filename: filename,
			Index:    i,
			Text:     part,
		})
	}
	return nil
}

func chunkWithOverflow(str string, maxChunkSize, overflowSize int) ([]string, error) {
	if maxChunkSize <= 0 {
		return nil, fmt.Errorf("maxChunkSize must be positive")
	}
	if overflowSize < 0 {
		return nil, fmt.Errorf("overflowSize cannot be negative")
	}

	var result []string
	var previousEnd string

	for len(str) > 0 {
		chunkEnd := maxChunkSize
		if chunkEnd > len(str) {
			chunkEnd = len(str)
		}

		chunk := str[:chunkEnd]

		// Find the last space character within the chunk
		lastSpace := strings.LastIndex(chunk, " ")
		if lastSpace != -1 {
			chunkEnd = lastSpace + 1
			chunk = str[:chunkEnd]
		}

		// Add overflow from the previous end if available
		if len(previousEnd) > 0 {
			overflowStart := len(previousEnd) - overflowSize
			if overflowStart < 0 {
				overflowStart = 0
			}
			chunk = previousEnd[overflowStart:] + chunk
		}

		// Save the end of the current chunk for the next iteration
		previousEnd = chunk

		// Add chunk to the result
		result = append(result, chunk)

		// Move to the next chunk
		str = str[chunkEnd:]
	}

	return result, nil
}
