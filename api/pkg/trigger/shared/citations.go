package shared

import (
	"fmt"
	"regexp"
)

// ConvertDocIDsToNumberedCitations converts [DOC_ID:xxx] markers to numbered citations
// This is used for chat integrations (Teams, Slack) that can't render internal document links
func ConvertDocIDsToNumberedCitations(text string) string {
	docIDPattern := regexp.MustCompile(`\[DOC_ID:([^\]]+)\]`)
	matches := docIDPattern.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return text
	}

	// Create a map of document IDs to citation numbers
	citationMap := make(map[string]int)
	citationCounter := 1

	for _, match := range matches {
		docID := match[1]
		if _, exists := citationMap[docID]; !exists {
			citationMap[docID] = citationCounter
			citationCounter++
		}
	}

	// Replace all [DOC_ID:xxx] with [N]
	result := docIDPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatch := docIDPattern.FindStringSubmatch(match)
		if len(submatch) > 1 {
			docID := submatch[1]
			if num, exists := citationMap[docID]; exists {
				return fmt.Sprintf("[%d]", num)
			}
		}
		return match
	})

	return result
}
