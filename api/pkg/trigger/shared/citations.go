package shared

import (
	"fmt"
	"regexp"
	"strings"
)

// ProcessCitationsForChat processes LLM responses to make citations readable in chat platforms.
// It converts [DOC_ID:xxx] markers to numbered citations and removes XML excerpt blocks.
// This is used for chat integrations (Teams, Slack) that can't render internal document links.
func ProcessCitationsForChat(text string) string {
	// First, remove XML excerpt blocks (these contain source snippets that the web UI renders nicely)
	text = removeExcerptBlocks(text)

	// Then convert [DOC_ID:xxx] markers to numbered citations
	text = convertDocIDsToNumberedCitations(text)

	return text
}

// removeExcerptBlocks removes <excerpts>...</excerpts> XML blocks from the text.
// These blocks contain source snippets that the web UI renders in a sidebar,
// but would appear as raw text in chat platforms.
func removeExcerptBlocks(text string) string {
	// Remove complete <excerpts>...</excerpts> blocks
	excerptsPattern := regexp.MustCompile(`<excerpts>[\s\S]*?</excerpts>`)
	text = excerptsPattern.ReplaceAllString(text, "")

	// Also handle partial/unclosed excerpts (e.g., streaming or malformed)
	// Remove anything from <excerpts> to end of string if no closing tag
	if strings.Contains(text, "<excerpts>") && !strings.Contains(text, "</excerpts>") {
		idx := strings.Index(text, "<excerpts>")
		text = text[:idx]
	}

	// Clean up any resulting multiple newlines
	multipleNewlines := regexp.MustCompile(`\n{3,}`)
	text = multipleNewlines.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

// convertDocIDsToNumberedCitations converts [DOC_ID:xxx] markers to numbered citations [1], [2], etc.
func convertDocIDsToNumberedCitations(text string) string {
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

// ConvertDocIDsToNumberedCitations is exported for backward compatibility
// Prefer using ProcessCitationsForChat which handles both DOC_ID markers and excerpt blocks
func ConvertDocIDsToNumberedCitations(text string) string {
	return convertDocIDsToNumberedCitations(text)
}
