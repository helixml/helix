package text

import (
	"fmt"
	"strings"
)

type dataPrepDocuments struct {
	rawData []string
}

func newDataPrepDocuments() *dataPrepDocuments {
	return &dataPrepDocuments{
		rawData: []string{},
	}
}

func (docs *dataPrepDocuments) AddDocument(content string) error {
	docs.rawData = append(docs.rawData, content)
	return nil
}

func chunkWithOverflow(strs []string, maxChunkSize, overflowSize int) ([]string, error) {
	if maxChunkSize <= 0 {
		return nil, fmt.Errorf("maxChunkSize must be positive")
	}
	if overflowSize < 0 {
		return nil, fmt.Errorf("overflowSize cannot be negative")
	}

	var result []string
	var previousEnd string

	for i, str := range strs {
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

			// Add overflow from the start of the next string if it's the last chunk of the current string
			if len(str) == 0 && i < len(strs)-1 {
				nextStart := maxChunkSize - overflowSize
				if nextStart < 0 {
					nextStart = 0
				}
				if nextStart < len(strs[i+1]) {
					previousEnd += strs[i+1][:nextStart]
				}
			}
		}
	}

	return result, nil
}

func (docs *dataPrepDocuments) GetChunks(maxChunkSize int, overflowSize int) ([]string, error) {
	return chunkWithOverflow(docs.rawData, maxChunkSize, overflowSize)
}

func ConvertConversation(data DataPrepTextConversation) ShareGPTConversations {
	res := ShareGPTConversations{
		Conversations: []ShareGPTConversation{
			{
				From:  "human",
				Value: data.Question,
			},
			{
				From:  "gpt",
				Value: data.Answer,
			},
		},
	}
	return res
}
