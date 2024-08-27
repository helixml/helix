package prompts

import (
	"bytes"
	_ "embed"
	"strings"
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

type Prompt struct {
	Name     string `yaml:"name"`
	Template string `yaml:"template"`
}

type PromptConfig struct {
	Prompts []Prompt `yaml:"prompts"`
}

// this prompt is applied as the system prompt for a session that has been fine-tuned on some documents
func TextFinetuneSystemPrompt(documentIDs []string, documentGroupID string) (string, error) {
	tmplData := struct {
		DocumentIDs   string
		DocumentGroup string
		DocumentCount int
	}{
		DocumentIDs:   strings.Join(documentIDs, ","),
		DocumentGroup: documentGroupID,
		DocumentCount: len(documentIDs),
	}
	tmpl := template.Must(template.New("TextFinetuneSystemPrompt").Parse(templates.FinetuningTemplate))

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
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
	return string(buf.Bytes()), nil
}

type KnowledgePromptRequest struct {
	UserPrompt       string
	RAGResults       []*RagContent
	KnowledgeResults []*BackgroundKnowledge
	PromptTemplate   string // Override the default prompt template
}

// KnowledgePrompt generates a prompt for knowledge-based questions, optionally including RAG results
func KnowledgePrompt(req *KnowledgePromptRequest) (string, error) {

	tmplData := struct {
		RagResults       []*RagContent
		KnowledgeResults []*BackgroundKnowledge
		Question         string
	}{
		RagResults:       req.RAGResults,
		KnowledgeResults: req.KnowledgeResults,
		Question:         req.UserPrompt,
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
	return string(buf.Bytes()), nil
}
