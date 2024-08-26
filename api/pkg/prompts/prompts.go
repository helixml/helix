package prompts

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type RagContent struct {
	DocumentID string
	Content    string
}

type BackgroundKnowledge struct {
	Description string
	Content     string
	DocumentID  string
	Source      string
}

type Prompt struct {
	Name     string `yaml:"name"`
	Template string `yaml:"template"`
}

type PromptConfig struct {
	Prompts []Prompt `yaml:"prompts"`
}

//go:embed prompts.yaml
var promptConfigString string

var allPrompts []Prompt

func getPrompts() ([]Prompt, error) {
	if allPrompts != nil {
		return allPrompts, nil
	}
	var config PromptConfig
	err := yaml.Unmarshal([]byte(promptConfigString), &config)
	if err != nil {
		return nil, err
	}
	allPrompts = config.Prompts
	return allPrompts, nil
}

func getPromptTemplate(name string) (string, error) {
	prompts, err := getPrompts()
	if err != nil {
		return "", err
	}
	for _, prompt := range prompts {
		if prompt.Name == name {
			return prompt.Template, nil
		}
	}
	return "", fmt.Errorf("could not find prompt with name %s", name)
}

// this prompt is applied as the system prompt for a session that has been fine-tuned on some documents
func TextFinetuneSystemPrompt(documentIDs []string, documentGroupID string) (string, error) {
	promptTemplate, err := getPromptTemplate("finetune-system-prompt")
	if err != nil {
		return "", err
	}
	tmplData := struct {
		DocumentIDs   string
		DocumentGroup string
		DocumentCount int
	}{
		DocumentIDs:   strings.Join(documentIDs, ","),
		DocumentGroup: documentGroupID,
		DocumentCount: len(documentIDs),
	}
	tmpl := template.Must(template.New("TextFinetuneSystemPrompt").Parse(promptTemplate))
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

// this prompt is applied before the user prompt is forwarded to the LLM
// we inject the list of RAG results we loaded from the vector store
func RAGInferencePrompt(userPrompt string, rag []*RagContent) (string, error) {
	promptTemplate, err := getPromptTemplate("rag-inference-prompt")
	if err != nil {
		return "", err
	}

	tmplData := struct {
		RagResults []*RagContent
		Question   string
	}{
		RagResults: rag,
		Question:   userPrompt,
	}
	tmpl := template.Must(template.New("RAGInferencePrompt").Parse(promptTemplate))
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

const knowledgePromptTemplate = `
{{- if .RagResults }}
	We have found the following context you may refer to in your answer:
	{{- range .RagResults }}
		<article>
		DocumentID: {{ .DocumentID }}
		Content: {{ .Content }}
		</article>
	{{- end }}

	Please provided references in your answer in the format '[DOC_ID:DocumentID]'. For example, "According to [DOC_ID:f6962c8007], the answer is 42."            
{{- end }}

{{- if .Knowledge }}
Here is some background knowledge context that you may refer to in your answer:
{{- range .Knowledge }}
<article>
{{- if .Description }}
Description: {{ .Description }}
{{- end }}
Content: {{ .Content }}
{{- if .Source }}
Source: {{ .Source }}
{{- end }}        
</article>
{{- end }}

If you think you need to refer to the knowledge, then do so by providing a list of markdown bullet points with links ` + "`" + "[SOURCE](Source)" + "`" + ` to the knowledge source.	

For example:
"
You can find more information about this topic in:
- [this page](https://example.com/first/page).
- [this page](https://example.com/second/page).
"	
{{- end }}

Here is the question from the user:
{{.Question}}
`

// KnowledgePrompt generates a prompt for knowledge-based questions, optionally including RAG results
func KnowledgePrompt(userPrompt string, rag []*RagContent, knowledge []*BackgroundKnowledge) (string, error) {

	tmplData := struct {
		RagResults []*RagContent
		Knowledge  []*BackgroundKnowledge
		Question   string
	}{
		RagResults: rag,
		Knowledge:  knowledge,
		Question:   userPrompt,
	}
	tmpl := template.Must(template.New("KnowledgePrompt").Parse(knowledgePromptTemplate))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}
