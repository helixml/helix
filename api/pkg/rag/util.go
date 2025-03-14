package rag

import (
	"fmt"
	"regexp"
)

// Extract document IDs from the prompt
func ParseDocumentIDs(prompt string) []string {
	re := regexp.MustCompile(`\[DOC_ID:(\d+)\]`)
	matches := re.FindAllStringSubmatch(prompt, -1)

	// Convert matches to slice of strings
	documentIDs := make([]string, len(matches))
	for i, match := range matches {
		documentIDs[i] = match[1]
	}
	return documentIDs
}

func BuildDocumentID(documentID string) string {
	return fmt.Sprintf("[DOC_ID:%s]", documentID)
}
