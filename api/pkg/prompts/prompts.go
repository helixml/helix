package prompts

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v3"
)

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
func RAGInferencePrompt(userPrompt string, ragResults []types.SessionRagResult) (string, error) {
	promptTemplate, err := getPromptTemplate("rag-inference-prompt")
	if err != nil {
		return "", err
	}

	// convert the RAG results to a list of strings
	var ragResultStrings []string
	for _, result := range ragResults {
		ragResultStrings = append(ragResultStrings, result.Content)
	}
	tmplData := struct {
		RagResults []string
		Question   string
	}{
		RagResults: ragResultStrings,
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
