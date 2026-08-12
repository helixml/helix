package prompts

import (
	"bytes"
	"text/template"

	"github.com/helixml/helix/api/pkg/prompts/templates"
)

type RagContent struct {
	DocumentID string
	Content    string
}

type BackgroundKnowledge struct {
	Description string
	Content     string
	DocumentID  string
	Source      string // source of the document (URL)
}

// RecurringTaskSystemPrompt defines the explicit lifecycle contract for an
// unattended recurring task. A normal text response does not complete the run.
func RecurringTaskSystemPrompt() string {
	return `You are executing an unattended recurring task. Complete the requested work, then call the task_completed tool exactly once when the work is fully finished. Use the summary argument for a concise completion report suitable for a notification. A written response alone does not complete the task. Do not call task_completed if the work failed or remains blocked.`
}

// this prompt is applied before the user prompt is forwarded to the LLM
// we inject the list of RAG results we loaded from the vector store
func RAGInferencePrompt(userPrompt string, rag []*RagContent) (string, error) {
	tmplData := struct {
		RagResults []*RagContent
		Question   string
	}{
		RagResults: rag,
		Question:   userPrompt,
	}
	tmpl := template.Must(template.New("RAGInferencePrompt").Parse(templates.RagTemplate))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

type KnowledgePromptRequest struct {
	UserPrompt       string
	RAGResults       []*RagContent
	KnowledgeResults []*BackgroundKnowledge
	IsVision         bool
	PromptTemplate   string // Override the default prompt template
}

// KnowledgePrompt generates a prompt for knowledge-based questions, optionally including RAG results
func KnowledgePrompt(req *KnowledgePromptRequest) (string, error) {

	tmplData := struct {
		RagResults       []*RagContent
		KnowledgeResults []*BackgroundKnowledge
		Question         string
		IsVision         bool
	}{
		RagResults:       req.RAGResults,
		KnowledgeResults: req.KnowledgeResults,
		Question:         req.UserPrompt,
		IsVision:         req.IsVision,
	}

	promptTemplate := req.PromptTemplate
	if promptTemplate == "" {
		promptTemplate = templates.KnowledgeTemplate
	}

	tmpl := template.Must(template.New("KnowledgePrompt").Parse(promptTemplate))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
