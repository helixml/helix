package qapairs

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"text/template"
	"time"

	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	ext_openai "github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

//go:embed qapair_config.yaml
var qapairConfig string

type Prompt struct {
	Name       string                 `yaml:"name"`
	System     string                 `yaml:"system"`
	User       string                 `yaml:"user"`
	JsonSchema map[string]interface{} `yaml:"json_schema"`
}

type Text struct {
	Name string `yaml:"name"`
	// either File or Contents should be non-empty
	File     string `yaml:"file"`
	Contents string `yaml:"contents"`
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
	Prompts      []Prompt `yaml:"prompts"`
	Texts        []Text   `yaml:"texts"`
	Concurrency  int      `yaml:"concurrency"`
	ChunkSize    int      `yaml:"chunk_size"`
	NumQuestions int      `yaml:"num_questions"`
}

// TODO: maybe optimize (or at least factor!) to not read the yaml on every call

func AllPrompts() ([]string, error) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, prompt := range config.Prompts {
		result = append(result, prompt.Name)
	}
	return result, nil
}

func GetNumQuestions() (int, error) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		return 0, err
	}
	return config.NumQuestions, nil
}

func GetConcurrency() (int, error) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		return 0, err
	}
	return config.Concurrency, nil
}

func GetChunkSize() (int, error) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		return 0, err
	}
	return config.ChunkSize, nil
}

func FindPrompt(name string) (Prompt, error) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		return Prompt{}, fmt.Errorf("failed to unmarshal qapair config: %v", err)
	}

	for _, prompt := range config.Prompts {
		if prompt.Name == name {
			return prompt, nil
		}
	}

	return Prompt{}, fmt.Errorf("could not find prompt with name %s", name)
}

func Run(client openai.Client, ownerID, sessionID, model string, promptFilter, textFilter []string) error {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal qapair config: %v", err)
	}

	prompts := config.Prompts
	texts := config.Texts

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
			for _, t := range texts {
				if t.Name == name {
					filteredTexts = append(filteredTexts, t)
				}
			}
		}
	} else {
		filteredTexts = texts
	}

	// for _, target := range filteredTargets {
	for _, prompt := range filteredPrompts {
		for _, text := range filteredTexts {
			fmt.Printf("Running helix qapairs --target=\"%s\" --prompt=\"%s\" --text=\"%s\"\n", model, prompt.Name, text.Name)
			resp, err := Query(client, ownerID, sessionID, model, prompt, text, "", "", 0)
			if err != nil {
				return fmt.Errorf("error querying model: %v", err)
			}
			bs, err := yaml.Marshal(resp)
			if err != nil {
				return fmt.Errorf("error marshalling response to yaml (%v): %w ", resp, err)
			}
			fmt.Println(string(bs))
		}
	}

	return nil
}

type TemplateData struct {
	NumQuestions    int
	DocumentID      string
	DocumentGroupID string
	DocumentChunk   string
}

func Query(client openai.Client, ownerID, sessionID, model string, prompt Prompt, text Text, documentID, documentGroupID string, numQuestions int) ([]types.DataPrepTextQuestionRaw, error) {
	// Perform the query for the given target and prompt
	var (
		contents string
		err      error
	)

	if text.Contents != "" {
		contents = text.Contents
	} else {
		contents, err = loadFile(text.File)
		if err != nil {
			return nil, fmt.Errorf("failed to load file %s: %w", text.File, err)
		}
	}

	if documentID == "" {
		documentID = "doc123"
	}
	if documentGroupID == "" {
		documentGroupID = "group123"
	}
	if numQuestions == 0 {
		numQuestions, err = GetNumQuestions()
		if err != nil {
			return nil, err
		}
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
	debug := fmt.Sprintf("prompt %s", prompt.Name)
	// try not enforcing json schema initially, only retry if we fail to parse
	resp, err := chatWithModel(client, ownerID, sessionID, model, systemPrompt, userPrompt, debug, nil)
	if err != nil {
		log.Warn().Msgf("ChatCompletion error non-JSON mode, trying again (%s): %v\n", debug, err)
		resp, err = chatWithModel(client, ownerID, sessionID, model, systemPrompt, userPrompt, debug, prompt.JsonSchema)
		if err != nil {
			log.Warn().Msgf("ChatCompletion error JSON mode, giving up, but not propagating the error further for now. (%s): %v\n", debug, err)
			latency := time.Since(startTime).Milliseconds()
			log.Warn().Msgf("Took: %.2f seconds. FAILED", float32(latency)/1000)
			return []types.DataPrepTextQuestionRaw{}, nil
		}
	}
	latency := time.Since(startTime).Milliseconds()

	log.Info().Msgf("Took: %.2f seconds", float32(latency)/1000)

	err = os.MkdirAll("runs", os.ModePerm)
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("runs/%d_%s_%s.yaml", timestamp, prompt.Name, text.Name)

	respBytes, err := yaml.Marshal(resp)
	if err != nil {
		log.Error().Err(err).Msgf("failed to marshal response")
		return nil, fmt.Errorf("failed to marshal response, error: %w", err)
	}

	logData := Log{
		Date:      time.Now().String(),
		Model:     model,
		System:    systemPrompt,
		User:      userPrompt,
		Text:      contents,
		Result:    string(respBytes),
		LatencyMs: latency,
	}

	logDataBytes, err := yaml.Marshal(logData)
	if err != nil {
		log.Error().Err(err).Msgf("failed to marshal logData")
		return nil, fmt.Errorf("failed to marshal log data, error: %w", err)
	}

	err = os.WriteFile(filename, logDataBytes, 0644)
	if err != nil {
		log.Error().Err(err).Msgf("failed to write file %s", filename)
		return nil, fmt.Errorf("failed to write log file, error: %w", err)
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

func chatWithModel(client openai.Client, ownerID, sessionID, model, system, user, debug string, jsonSchema map[string]interface{}) ([]types.DataPrepTextQuestionRaw, error) {
	req := ext_openai.ChatCompletionRequest{
		Model: model,
		Messages: []ext_openai.ChatCompletionMessage{
			{
				Role:    ext_openai.ChatMessageRoleSystem,
				Content: system,
			},
			{
				Role:    ext_openai.ChatMessageRoleUser,
				Content: user,
			},
		},
	}

	if jsonSchema != nil {
		req.ResponseFormat = &ext_openai.ChatCompletionResponseFormat{
			Type: ext_openai.ChatCompletionResponseFormatTypeJSONObject,
			// TODO:
			// JSONSchema: jsonSchema,
		}
	}

	ctx := openai.SetContextValues(context.Background(), &openai.ContextValues{
		OwnerID:       ownerID,
		SessionID:     sessionID,
		InteractionID: "n/a",
	})

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ChatCompletion error (%s): %v", debug, err)
	}

	answer := resp.Choices[0].Message.Content

	log.Printf("XXX Raw response (%s) to %s json=%t: %s\n", resp.ID, debug, jsonSchema != nil, answer)

	if jsonSchema == nil {
		answer = tools.AttemptFixJSON(answer)
	}

	return TryVariousJSONFormats(answer, fmt.Sprintf("%s respID=%s", debug, resp.ID))
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

func TryVariousJSONFormats(jsonString, debug string) ([]types.DataPrepTextQuestionRaw, error) {
	var res []types.DataPrepTextQuestionRaw
	var err error

	// Try a single qapair
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

	return nil, fmt.Errorf("error parsing JSON (%s):\n\n%s", debug, jsonString)
}
