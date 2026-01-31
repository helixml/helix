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

// LinkFormat specifies the format for generating links in chat platforms
type LinkFormat string

const (
	// LinkFormatMarkdown generates [text](url) style links (for Teams)
	LinkFormatMarkdown LinkFormat = "markdown"
	// LinkFormatSlack generates <url|text> style links (for Slack)
	LinkFormatSlack LinkFormat = "slack"
)

// ProcessCitationsForChat processes LLM responses to make citations readable in chat platforms.
// It converts [DOC_ID:xxx] markers to numbered citations, extracts source info from XML excerpts,
// and appends a references section at the end.
// This is used for chat integrations (Teams, Slack) that can't render internal document links.
func ProcessCitationsForChat(text string) string {
	return ProcessCitationsForChatWithLinks(text, nil, LinkFormatMarkdown)
}

// ProcessCitationsForChatWithLinks processes citations with clickable links when document URLs are available.
// documentIDs maps filenames/URLs to document IDs (reversed from session.Metadata.DocumentIDs)
// linkFormat specifies the chat platform's link format
func ProcessCitationsForChatWithLinks(text string, documentIDs map[string]string, linkFormat LinkFormat) string {
	// Extract citation info from excerpts before removing them
	excerptSnippets := extractCitationsFromExcerpts(text)

	// Remove XML excerpt blocks
	text = removeExcerptBlocks(text)

	// Build reverse map: docID -> URL/filename
	docIDToURL := make(map[string]string)
	if documentIDs != nil {
		for urlOrFilename, docID := range documentIDs {
			docIDToURL[docID] = urlOrFilename
		}
	}

	// Convert [DOC_ID:xxx] markers to numbered citations and get the mapping
	text, citationMap := convertDocIDsToNumberedCitationsWithMap(text)

	// Build references section with links if we have citations
	references := buildReferencesSectionWithLinks(excerptSnippets, citationMap, docIDToURL, linkFormat)
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

	// Only trim space if there's actual content (not just whitespace)
	// This preserves the original behavior for whitespace-only input
	if strings.TrimSpace(text) != "" {
		text = strings.TrimSpace(text)
	}

	return text
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

// buildReferencesSectionWithLinks creates a references section with clickable links when URLs are available
func buildReferencesSectionWithLinks(excerptSnippets map[string]string, citationMap map[string]int, docIDToURL map[string]string, linkFormat LinkFormat) string {
	if len(citationMap) == 0 {
		return ""
	}

	// Build ordered list of references
	type ref struct {
		num     int
		snippet string
		url     string
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

		// Get URL if available
		url := docIDToURL[docID]

		refs[num-1] = ref{num: num, snippet: snippet, url: url}
	}

	// Build the references section
	var sb strings.Builder
	sb.WriteString("---\n**Sources:**\n")
	for _, r := range refs {
		if r.url != "" && isClickableURL(r.url) {
			// Format link based on platform
			link := formatLink(r.url, fmt.Sprintf("[%d]", r.num), linkFormat)
			sb.WriteString(fmt.Sprintf("%s %s\n", link, r.snippet))
		} else {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", r.num, r.snippet))
		}
	}

	return sb.String()
}

// isClickableURL checks if a string is an external URL that can be linked
func isClickableURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// formatLink formats a URL as a clickable link for the specified platform
func formatLink(url, text string, format LinkFormat) string {
	switch format {
	case LinkFormatSlack:
		return fmt.Sprintf("<%s|%s>", url, text)
	case LinkFormatMarkdown:
		fallthrough
	default:
		return fmt.Sprintf("[%s](%s)", text, url)
	}
}

// buildReferencesSection creates a references section from citation info (legacy, no links)
func buildReferencesSection(excerptSnippets map[string]string, citationMap map[string]int) string {
	return buildReferencesSectionWithLinks(excerptSnippets, citationMap, nil, LinkFormatMarkdown)
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
