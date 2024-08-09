package prompts

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestRAGInferencePrompt(t *testing.T) {
	// Test with rag results
	ragResults := []*types.SessionRAGResult{
		{
			DocumentID: "doc1",
			Content:    "content1",
		},
	}
	prompt, err := RAGInferencePrompt("test", ragResults)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that the document ID is included the prompt
	if !strings.Contains(prompt, "doc1") {
		t.Errorf("prompt does not contain document ID")
	}
	// Check that the content is included the prompt
	if !strings.Contains(prompt, "content1") {
		t.Errorf("prompt does not contain content")
	}

	// Test with no rag results
	prompt, err = RAGInferencePrompt("test", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Make sure the query was not included in the prompt
	if strings.Contains(prompt, "test") {
		t.Errorf("prompt contains query")
	}
}
