package qapairs

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

//go:embed qapair_config.yaml
var qapairConfig string

type Target struct {
	Name         string `yaml:"name"`
	ApiUrl       string `yaml:"api_url"`
	Model        string `yaml:"model"`
	TokenFromEnv string `yaml:"token_from_env"`
}

type Prompt struct {
	Name   string `yaml:"name"`
	System string `yaml:"system"`
	User   string `yaml:"user"`
}

type Text struct {
	Name string `yaml:"name"`
	File string `yaml:"file"`
}

type Log struct {
	Date      string `yaml:"date"`
	ApiUrl    string `yaml:"api_url"`
	Model     string `yaml:"model"`
	System    string `yaml:"system"`
	User      string `yaml:"user"`
	Text      string `yaml:"text"`
	Result    string `yaml:"result"`
	LatencyMs int64  `yaml:"latency"`
}

type Config struct {
	Prompts []Prompt `yaml:"prompts"`
	Targets []Target `yaml:"targets"`
	Texts   []Text   `yaml:"texts"`
}

func Run(targetFilter, promptFilter, textFilter []string) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	targets := config.Targets
	prompts := config.Prompts
	texts := config.Texts

	filteredTargets := []Target{}
	if len(targetFilter) > 0 {
		for _, name := range targetFilter {
			for _, t := range targets {
				if t.Name == name {
					filteredTargets = append(filteredTargets, t)
				}
			}
		}
	} else {
		filteredTargets = targets
	}

	filteredPrompts := []Prompt{}
	if len(promptFilter) > 0 {
		for _, name := range promptFilter {
			for _, p := range prompts {
				if p.Name == name {
					filteredPrompts = append(filteredPrompts, p)
				}
			}
		}
	} else {
		filteredPrompts = prompts
	}

	filteredTexts := []Text{}
	if len(textFilter) > 0 {
		for _, name := range textFilter {
			for _, p := range texts {
				if p.Name == name {
					filteredTexts = append(filteredTexts, p)
				}
			}
		}
	} else {
		filteredTexts = texts
	}

	for _, target := range filteredTargets {
		for _, prompt := range filteredPrompts {
			for _, text := range filteredTexts {
				fmt.Printf("Running %s(prompt=\"%s\", text=\"%s\")\n", target.Name, prompt.Name, text.Name)
				resp, err := Query(target, prompt, text, "", "", 0)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				bs, err := yaml.Marshal(resp)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				fmt.Println(string(bs))
			}
		}
	}
}

type TemplateData struct {
	NumQuestions    int
	DocumentID      string
	DocumentGroupID string
	DocumentChunk   string
}

func Query(target Target, prompt Prompt, text Text, documentID, documentGroupID string, numQuestions int) ([]types.DataPrepTextQuestionRaw, error) {
	// Perform the query for the given target and prompt

	contents, err := loadFile(text.File)
	if err != nil {
		return nil, err
	}

	if documentID == "" {
		documentID = "doc123"
	}
	if documentGroupID == "" {
		documentGroupID = "group123"
	}
	if numQuestions == 0 {
		numQuestions = 50
	}

	tmplData := TemplateData{
		NumQuestions:    numQuestions,
		DocumentID:      documentID,
		DocumentGroupID: documentGroupID,
		DocumentChunk:   contents,
	}

	tmpl := template.Must(template.New("systemPrompt").Parse(prompt.System))
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, tmplData)
	if err != nil {
		return nil, err
	}

	systemPrompt := buf.String()

	tmpl = template.Must(template.New("userPrompt").Parse(prompt.User))
	var buf2 bytes.Buffer
	err = tmpl.Execute(&buf2, tmplData)
	if err != nil {
		return nil, err
	}

	userPrompt := buf2.String()

	startTime := time.Now()
	resp, err := chatWithModel(target.ApiUrl, os.Getenv(target.TokenFromEnv), target.Model, systemPrompt, userPrompt)
	if err != nil {
		log.Printf("Error, continuing: %v", err)
	}
	latency := time.Since(startTime).Milliseconds()

	log.Printf("Took: %.2f seconds", float32(latency)/1000)

	err = os.MkdirAll("runs", os.ModePerm)
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("runs/%d_%s_%s_%s.yaml", timestamp, target.Name, prompt.Name, text.Name)

	respBytes, err := yaml.Marshal(resp)
	if err != nil {
		return nil, err
	}

	logData := Log{
		Date:      time.Now().String(),
		ApiUrl:    target.ApiUrl,
		Model:     target.Model,
		System:    systemPrompt,
		User:      userPrompt,
		Text:      contents,
		Result:    string(respBytes),
		LatencyMs: latency,
	}

	logDataBytes, err := yaml.Marshal(logData)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(filename, logDataBytes, 0644)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func loadFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func chatWithModel(apiUrl, token, model, system, user string) ([]types.DataPrepTextQuestionRaw, error) {
	cfg := openai.DefaultConfig(token)
	cfg.BaseURL = apiUrl
	client := openai.NewClientWithConfig(cfg)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: system,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: user,
				},
			},
		},
	)
	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return nil, err
	}

	answer := resp.Choices[0].Message.Content
	answer = strings.TrimPrefix(answer, "```json")
	// sometimes GPT4 in it's wisdom puts a message after the enclosing ```json``` block
	parts := strings.Split(answer, "```")
	answer = parts[0]

	return TryVariousJSONFormats(answer)

}

// for prompt engineering purposes, the LLMs output various formats. Try all of them:
type TopLevelQAPairs struct {
	Questions []types.DataPrepTextQuestionRaw `json:"questions"`
}

type WrappedQAPairs struct {
	Questions []QuestionSet `json:"questions"`
}

type QuestionSet struct {
	Questions []types.DataPrepTextQuestionRaw `json:"questions"`
}

func TryVariousJSONFormats(jsonString string) ([]types.DataPrepTextQuestionRaw, error) {
	var res []types.DataPrepTextQuestionRaw
	var err error

	// Try the top-level format
	err = json.Unmarshal([]byte(jsonString), &res)
	if err == nil {
		return res, nil
	}

	// Try the wrapped format
	var wrapped WrappedQAPairs
	err = json.Unmarshal([]byte(jsonString), &wrapped)
	if err == nil {
		var questions []types.DataPrepTextQuestionRaw
		for _, questionSet := range wrapped.Questions {
			questions = append(questions, questionSet.Questions...)
		}
		return questions, nil
	}

	// Try the top-level format
	var topLevel TopLevelQAPairs
	err = json.Unmarshal([]byte(jsonString), &topLevel)
	if err == nil {
		return topLevel.Questions, nil
	}

	return nil, fmt.Errorf("error parsing JSON:\n\n%s", jsonString)
}
