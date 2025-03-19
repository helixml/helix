package prompts

import (
	"strings"
	"testing"
)

func TestRAGInferencePrompt(t *testing.T) {
	// Test with rag results
	ragResults := []*RagContent{
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

func TestKnowledgePrompt(t *testing.T) {
	// Test case 1: When RagResults are supplied
	t.Run("With RagResults", func(t *testing.T) {
		req := &KnowledgePromptRequest{
			UserPrompt: "What is the capital of France?",
			RAGResults: []*RagContent{
				{
					DocumentID: "doc1",
					Content:    "Paris is the capital of France.",
				},
			},
		}

		prompt, err := KnowledgePrompt(req)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !strings.Contains(prompt, "What is the capital of France?") {
			t.Errorf("prompt does not contain user question")
		}
		if !strings.Contains(prompt, "doc1") {
			t.Errorf("prompt does not contain RAG document ID")
		}
		if !strings.Contains(prompt, "Paris is the capital of France.") {
			t.Errorf("prompt does not contain RAG content")
		}
	})

	// Test case 2: When KnowledgeResults are supplied
	t.Run("With KnowledgeResults", func(t *testing.T) {
		req := &KnowledgePromptRequest{
			UserPrompt: "Tell me about the Eiffel Tower.",
			KnowledgeResults: []*BackgroundKnowledge{
				{
					Description: "Eiffel Tower facts",
					Content:     "The Eiffel Tower is 324 meters tall.",
					Source:      "https://example.com/eiffel-tower",
				},
			},
		}

		prompt, err := KnowledgePrompt(req)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !strings.Contains(prompt, "Tell me about the Eiffel Tower.") {
			t.Errorf("prompt does not contain user question")
		}
		if !strings.Contains(prompt, "Eiffel Tower facts") {
			t.Errorf("prompt does not contain knowledge description")
		}
		if !strings.Contains(prompt, "The Eiffel Tower is 324 meters tall.") {
			t.Errorf("prompt does not contain knowledge content")
		}
	})
}

func TestTextFinetuneSystemPrompt(t *testing.T) {
	// Test case 1: Multiple document IDs
	t.Run("Multiple Documents", func(t *testing.T) {
		documentIDs := []string{"doc1", "doc2", "doc3"}
		documentGroupID := "group1"

		prompt, err := TextFinetuneSystemPrompt(documentIDs, documentGroupID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Check that all document IDs are included in the prompt
		for _, id := range documentIDs {
			if !strings.Contains(prompt, id) {
				t.Errorf("prompt does not contain document ID: %s", id)
			}
		}

		// Check that the document group ID is included in the prompt
		if !strings.Contains(prompt, documentGroupID) {
			t.Errorf("prompt does not contain document group ID")
		}

		// Check that the document count is correct
		expectedCountStr := "3 document(s)"
		if !strings.Contains(prompt, expectedCountStr) {
			t.Errorf("prompt does not contain correct document count")
		}
	})

	// Test case 2: Single document ID
	t.Run("Single Document", func(t *testing.T) {
		documentIDs := []string{"doc1"}
		documentGroupID := "group2"

		prompt, err := TextFinetuneSystemPrompt(documentIDs, documentGroupID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !strings.Contains(prompt, "doc1") {
			t.Errorf("prompt does not contain document ID")
		}

		if !strings.Contains(prompt, documentGroupID) {
			t.Errorf("prompt does not contain document group ID")
		}

		expectedCountStr := "1 document(s)"
		if !strings.Contains(prompt, expectedCountStr) {
			t.Errorf("prompt does not contain correct document count")
		}
	})

	// Test case 3: No document IDs
	t.Run("No Documents", func(t *testing.T) {
		documentIDs := []string{}
		documentGroupID := "group3"

		prompt, err := TextFinetuneSystemPrompt(documentIDs, documentGroupID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !strings.Contains(prompt, documentGroupID) {
			t.Errorf("prompt does not contain document group ID")
		}

		expectedCountStr := "0 document(s)"
		if !strings.Contains(prompt, expectedCountStr) {
			t.Errorf("prompt does not contain correct document count")
		}
	})
}
