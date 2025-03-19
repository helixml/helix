package rag

import (
	"fmt"
	"regexp"
)

func BuildDocumentID(documentID string) string {
	return fmt.Sprintf("[DOC_ID:%s]", documentID)
}

func BuildFilterAction(documentID string) string {
	return fmt.Sprintf("@filter(%s)", BuildDocumentID(documentID))
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
