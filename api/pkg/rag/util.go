package rag

import (
	"fmt"
	"regexp"
)

func BuildDocumentID(documentID string) string {
	return fmt.Sprintf("[DOC_ID:%s]", documentID)
}

func BuildFilterAction(documentName, documentID string) string {
	return fmt.Sprintf("@filter(%s%s)", BuildDocumentName(documentName), BuildDocumentID(documentID))
}

func ParseFilterActions(prompt string) []string {
	re := regexp.MustCompile(`@filter\(([^()]+)\)`)
	matches := re.FindAllStringSubmatch(prompt, -1)
	filterActions := make([]string, len(matches))
	for i, match := range matches {
		filterActions[i] = match[1]
	}
	return filterActions
}

func ParseDocID(text string) string {
	re := regexp.MustCompile(`\[DOC_ID:([a-zA-Z0-9_-]+)\]`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// BuildDocumentName builds a DOC_NAME bracket token, e.g. [DOC_NAME:oracle-10k]
func BuildDocumentName(documentName string) string {
	return fmt.Sprintf("[DOC_NAME:%s]", documentName)
}

// BuildFilterActionWithName builds an @filter action including DOC_NAME and DOC_ID
// Order will be [DOC_NAME:...][DOC_ID:...]
func BuildFilterActionWithName(documentName string, documentID string) string {
	return fmt.Sprintf("@filter(%s%s)", BuildDocumentName(documentName), BuildDocumentID(documentID))
}

// ParseDocName extracts the document name from a [DOC_NAME:...] token
func ParseDocName(text string) string {
	re := regexp.MustCompile(`\[DOC_NAME:([a-zA-Z0-9_.-]+)\]`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
