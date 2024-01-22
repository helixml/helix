package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"text/template"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
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

var target []string
var prompt []string
var text []string

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qapair",
		Short: "A CLI tool for running QA pair commands",
		Run: func(cmd *cobra.Command, args []string) {
			Run(target, prompt, text)
		},
	}

	rootCmd.Flags().StringSliceVar(&target, "target", []string{},
		"Target(s) to use, defaults to all",
	)
	rootCmd.Flags().StringSliceVar(&prompt, "prompt", []string{},
		"Prompt(s) to use, defaults to all",
	)
	rootCmd.Flags().StringSliceVar(&text, "text", []string{},
		"Text(s) to use, defaults to all",
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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
				err := Query(target, prompt, text, "", "", 0)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
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

func Query(target Target, prompt Prompt, text Text, documentID, documentGroupID string, numQuestions int) error {
	// Perform the query for the given target and prompt
	// ...

	contents, err := loadFile(text.File)
	if err != nil {
		return err
	}

	if documentID == "" {
		documentID = "doc123"
	}
	if documentGroupID == "" {
		documentGroupID = "group123"
	}
	if numQuestions == 0 {
		numQuestions = 10
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
		return err
	}

	systemPrompt := buf.String()

	tmpl = template.Must(template.New("userPrompt").Parse(prompt.User))
	var buf2 bytes.Buffer
	err = tmpl.Execute(&buf2, tmplData)
	if err != nil {
		return err
	}

	userPrompt := buf2.String()

	startTime := time.Now()
	resp, err := chatWithModel(target.ApiUrl, os.Getenv(target.TokenFromEnv), target.Model, systemPrompt, userPrompt)
	if err != nil {
		return err
	}
	latency := time.Since(startTime).Milliseconds()

	log.Printf("Took: %.2f seconds", float32(latency)/1000)

	err = os.MkdirAll("runs", os.ModePerm)
	if err != nil {
		return err
	}

	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("runs/%d_%s_%s_%s.yaml", timestamp, target.Name, prompt.Name, text.Name)

	logData := Log{
		Date:      time.Now().String(),
		ApiUrl:    target.ApiUrl,
		Model:     target.Model,
		System:    systemPrompt,
		User:      userPrompt,
		Text:      contents,
		Result:    resp,
		LatencyMs: latency,
	}

	logDataBytes, err := yaml.Marshal(logData)
	if err != nil {
		return err
	}

	err = os.WriteFile(filename, logDataBytes, 0644)
	if err != nil {
		return err
	}

	return nil
}

func loadFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func chatWithModel(apiUrl, token, model, system, user string) (string, error) {
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
		return "", err
	}

	fmt.Println(resp.Choices[0].Message.Content)
	return resp.Choices[0].Message.Content, nil
}
