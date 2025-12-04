package shared

import (
	"fmt"
	"regexp"
	"strings"
)

// CitationInfo holds information about a citation extracted from the response
type CitationInfo struct {
	Number  int
	DocID   string
	Snippet string
}

// ProcessCitationsForChat processes LLM responses to make citations readable in chat platforms.
// It converts [DOC_ID:xxx] markers to numbered citations, extracts source info from XML excerpts,
// and appends a references section at the end.
// This is used for chat integrations (Teams, Slack) that can't render internal document links.
func ProcessCitationsForChat(text string) string {
	// Extract citation info from excerpts before removing them
	citations := extractCitationsFromExcerpts(text)

	// Remove XML excerpt blocks
	text = removeExcerptBlocks(text)

	// Convert [DOC_ID:xxx] markers to numbered citations and get the mapping
	text, citationMap := convertDocIDsToNumberedCitationsWithMap(text)

	// Build references section if we have citations
	references := buildReferencesSection(citations, citationMap)
	if references != "" {
		text = strings.TrimSpace(text) + "\n\n" + references
	}

	return text
}

// extractCitationsFromExcerpts extracts document IDs and snippets from <excerpts> XML blocks
func extractCitationsFromExcerpts(text string) map[string]string {
	citations := make(map[string]string)

	// Match individual excerpt tags
	excerptPattern := regexp.MustCompile(`<excerpt>[\s\S]*?<document_id>([^<]+)</document_id>[\s\S]*?<snippet>([\s\S]*?)</snippet>[\s\S]*?</excerpt>`)
	matches := excerptPattern.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			docID := strings.TrimSpace(match[1])
			snippet := strings.TrimSpace(match[2])
			// Only store the first snippet for each doc ID (avoid duplicates)
			if _, exists := citations[docID]; !exists {
				citations[docID] = snippet
			}
		}
	}

	return citations
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

// convertDocIDsToNumberedCitationsWithMap converts [DOC_ID:xxx] markers to numbered citations
// and returns both the converted text and the mapping of doc IDs to citation numbers
func convertDocIDsToNumberedCitationsWithMap(text string) (string, map[string]int) {
	docIDPattern := regexp.MustCompile(`\[DOC_ID:([^\]]+)\]`)
	matches := docIDPattern.FindAllStringSubmatch(text, -1)

	citationMap := make(map[string]int)

	if len(matches) == 0 {
		return text, citationMap
	}

	// Create a map of document IDs to citation numbers (in order of appearance)
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

	return result, citationMap
}

// buildReferencesSection creates a references section from citation info
func buildReferencesSection(excerptSnippets map[string]string, citationMap map[string]int) string {
	if len(citationMap) == 0 {
		return ""
	}

	// Build ordered list of references
	type ref struct {
		num     int
		snippet string
	}
	refs := make([]ref, len(citationMap))

	for docID, num := range citationMap {
		snippet := excerptSnippets[docID]
		if snippet == "" {
			snippet = "Source document"
		}
		// Truncate long snippets
		if len(snippet) > 100 {
			snippet = snippet[:97] + "..."
		}
		// Clean up snippet - remove newlines and extra spaces
		snippet = strings.Join(strings.Fields(snippet), " ")
		refs[num-1] = ref{num: num, snippet: snippet}
	}

	// Build the references section
	var sb strings.Builder
	sb.WriteString("---\n**Sources:**\n")
	for _, r := range refs {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", r.num, r.snippet))
	}

	return sb.String()
}

// convertDocIDsToNumberedCitations converts [DOC_ID:xxx] markers to numbered citations [1], [2], etc.
// This is a simpler version that doesn't return the mapping.
func convertDocIDsToNumberedCitations(text string) string {
	result, _ := convertDocIDsToNumberedCitationsWithMap(text)
	return result
}

// ConvertDocIDsToNumberedCitations is exported for backward compatibility
// Prefer using ProcessCitationsForChat which handles both DOC_ID markers and excerpt blocks
func ConvertDocIDsToNumberedCitations(text string) string {
	return convertDocIDsToNumberedCitations(text)
}
