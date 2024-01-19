package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"gopkg.in/yaml.v3"
)

//go:embed qapair_prompts.yaml
var qapairPrompts string

type QAPairPrompts struct {
	System string   `yaml:"system"`
	User   []string `yaml:"user"`
}

func main() {
	var prompts QAPairPrompts
	err := yaml.Unmarshal([]byte(qapairPrompts), &prompts)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	spew.Dump(prompts)

	Query()
}

func loadFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func chatWithModel(apiHost, fileContent string) ([]string, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	messages := []map[string]string{
		{"role": "system", "content": "You are an intelligent professor. You create question and answer pairs from given context for your students. Respond with an array of strict JSON 'question' & 'answer' pairs."},
		{"role": "user", "content": fmt.Sprintf("Here is your context:\n%s", fileContent)},
	}
	data := map[string]interface{}{
		"model":    "nous-hermes2-mixtral",
		"messages": messages,
		"stream":   false,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	response, err := http.Post(apiHost, headers["Content-Type"], strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result []string
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("> %s", line)
		var message map[string]interface{}
		err := json.Unmarshal([]byte(line), &message)
		if err != nil {
			return nil, err
		}
		result = append(result, message["message"].(map[string]interface{})["content"].(string))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func Query() {
	targets := []string{
		"http://mind.local:11434/api/chat",
		"https://api.openai.com/",
	}

	fileContent, err := loadFile("text.txt")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	response, err := chatWithModel(targets[0], fileContent)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for _, message := range response {
		fmt.Println(message)
	}
}
