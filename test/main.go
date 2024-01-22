package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
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
type Config struct {
	Prompts []Prompt `yaml:"prompts"`
	Targets []Target `yaml:"targets"`
	Text    []string `yaml:"text"`
}

var target []string
var prompt []string

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qapair",
		Short: "A CLI tool for running QA pair commands",
		Run: func(cmd *cobra.Command, args []string) {
			Run(target, prompt)
		},
	}

	rootCmd.Flags().StringSliceVar(&target, "target", []string{},
		"Target(s) to use, defaults to all",
	)
	rootCmd.Flags().StringSliceVar(&prompt, "prompt", []string{},
		"Prompt(s) to use, defaults to all",
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func Run(targetFilter, promptFilter []string) {
	var config Config
	err := yaml.Unmarshal([]byte(qapairConfig), &config)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	targets := config.Targets
	prompts := config.Prompts

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

	spew.Dump(filteredTargets)
	spew.Dump(filteredPrompts)

	// Query()
}

func loadFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func chatWithModel(apiUrl, token, model, fileContent string) ([]string, error) {
	cfg := openai.DefaultConfig(token)
	cfg.BaseURL = apiUrl
	client := openai.NewClientWithConfig(cfg)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Hello!",
				},
			},
		},
	)
	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return []string{}, err
	}

	fmt.Println(resp.Choices[0].Message.Content)

	return []string{}, nil
}

// func main() {
// 	var config Config
// 	err := yaml.Unmarshal([]byte(qapairConfig), &config)
// 	if err != nil {
// 		fmt.Println("Error:", err)
// 		return
// 	}

// 	spew.Dump(config)

// 	Query()
// }

// 		{"role": "system", "content": "You are an intelligent professor. You create question and answer pairs from given context for your students. Respond with an array of strict JSON 'question' & 'answer' pairs."},
// 		{"role": "user", "content": fmt.Sprintf("Here is your context:\n%s", fileContent)},
// 	}
// 	data := map[string]interface{}{
// 		"model":    "nous-hermes2-mixtral",
// 		"messages": messages,
// 		"stream":   false,
// 	}
// 	jsonData, err := json.Marshal(data)
// 	if err != nil {
// 		return nil, err
// 	}
// 	response, err := http.Post(apiHost, headers["Content-Type"], strings.NewReader(string(jsonData)))
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer response.Body.Close()
// 	var result []string
// 	scanner := bufio.NewScanner(response.Body)
// 	for scanner.Scan() {
// 		line := scanner.Text()
// 		log.Printf("> %s", line)
// 		var message map[string]interface{}
// 		err := json.Unmarshal([]byte(line), &message)
// 		if err != nil {
// 			return nil, err
// 		}
// 		result = append(result, message["message"].(map[string]interface{})["content"].(string))
// 	}
// 	if err := scanner.Err(); err != nil {
// 		return nil, err
// 	}
// 	return result, nil
// }

func Query() {
	// fileContent, err := loadFile("text.txt")
	// if err != nil {
	// 	fmt.Println("Error:", err)
	// 	return
	// }
	// response, err := chatWithModel(targets[0].ApiUrl, fileContent)
	// if err != nil {
	// 	fmt.Println("Error:", err)
	// 	return
	// }
	// for _, message := range response {
	// 	fmt.Println(message)
	// }
}
